package serviceportal

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"time"

	"guanlan/internal/serviceportal/store"
	"guanlan/internal/serviceportal/targetconfig"
)

const (
	reconcileInterval = 5 * time.Second
	retryJitterWindow = 10 * time.Second
	maxRetryDelay     = 30 * time.Second
)

type retryAttempt struct {
	delay time.Duration
	after time.Time
}

func (a *app) reconcileLoop(ctx context.Context) {
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()
	defer func() {
		a.operations.Lock()
		defer a.operations.Unlock()
		for _, resource := range a.db.Snapshot().Resources {
			a.stopLocked(resource.ID)
		}
	}()
	unavailable := false
	for {
		if err := a.reconcileAt(ctx, time.Now()); err != nil {
			if !unavailable && ctx.Err() == nil {
				log.Printf("service portal configuration sync failed: %v", err)
			}
			unavailable = true
		} else {
			unavailable = false
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// The internal service credential lets a fresh process restore enabled
// resources without retaining or waiting for a user's login token.
func (a *app) runtimeState(ctx context.Context) (store.State, error) {
	var state store.State
	if a.cfg.InternalToken == "" {
		return state, errors.New("未配置服务访问内部凭据")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.cfg.GuanlanURL+"/api/service-access/portal/runtime-state", nil)
	if err != nil {
		return state, errors.New("运行配置同步地址无效")
	}
	req.Header.Set("X-Service-Portal-Internal", a.cfg.InternalToken)
	resp, err := identityClient.Do(req)
	if err != nil {
		return state, errors.New("观澜 运行配置暂不可用")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return state, errors.New("观澜 拒绝运行配置同步，请检查版本与内部凭据")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil || len(body) > 8<<20 || json.Unmarshal(body, &state) != nil || state.Resources == nil || state.Clusters == nil {
		return state, errors.New("观澜 运行配置响应无效")
	}
	return state, nil
}

// All state application and lifecycle decisions share operations. Connection
// setup runs asynchronously, with completion checked against its owning run.
func (a *app) reconcileAt(ctx context.Context, now time.Time) error {
	a.expireOnDemand(now)
	sequence := a.stateFetch.Add(1)
	next, err := a.runtimeState(ctx)
	if err != nil {
		return err
	}
	a.operations.Lock()
	defer a.operations.Unlock()
	a.applyFetchedStateLocked(next, sequence)
	for _, resource := range a.db.Snapshot().Resources {
		if !resource.Desired || now.Before(a.retries[resource.ID].after) {
			continue
		}
		if err := a.startLocked(resource.ID, true); err != nil {
			a.scheduleRetryLocked(resource.ID, err.Error(), now)
		}
	}
	return nil
}

// A slow response must not replace configuration from a newer request. Fetches
// run outside operations so business requests and stop actions stay independent
// of another request's network latency.
func (a *app) applyFetchedStateLocked(next store.State, sequence uint64) {
	if sequence <= a.stateApplied {
		return
	}
	a.applyStateLocked(next)
	a.stateApplied = sequence
}

func sameTarget(a, b store.Resource) bool {
	aType, _ := targetconfig.Type(a.Type)
	bType, _ := targetconfig.Type(b.Type)
	if aType != bType || a.PublicPort != b.PublicPort || a.OnDemand != b.OnDemand {
		return false
	}
	if aType == targetconfig.HTTP {
		return a.TargetURL == b.TargetURL
	}
	return a.ClusterID == b.ClusterID && a.Namespace == b.Namespace && a.Service == b.Service && a.Port == b.Port && a.PublicPort == b.PublicPort
}

func sameCluster(a, b store.Cluster) bool {
	return a.ID == b.ID && a.Context == b.Context && a.Kubeconfig == b.Kubeconfig
}

func (a *app) applyStateLocked(next store.State) {
	current := a.db.Snapshot()
	// Cancel disabled, deleted, or reconfigured runs, including connections
	// still being established. Credential rotation also replaces the forwarder.
	a.mu.Lock()
	stop := []string{}
	for id, rr := range a.runs {
		resource, exists := next.Resource(id)
		cluster, clusterExists := next.Cluster(resource.ClusterID)
		clusterChanged := resource.Type != targetconfig.HTTP && (!clusterExists || !sameCluster(rr.cluster, cluster))
		if !exists || !resource.Desired || !sameTarget(rr.resource, resource) || clusterChanged {
			stop = append(stop, id)
		}
	}
	a.mu.Unlock()
	for _, id := range stop {
		a.stopLocked(id)
	}
	for id := range a.retries {
		resource, exists := next.Resource(id)
		old, _ := current.Resource(id)
		cluster, _ := next.Cluster(resource.ClusterID)
		oldCluster, _ := current.Cluster(old.ClusterID)
		if !exists || !resource.Desired || !sameTarget(resource, old) || (resource.Type != targetconfig.HTTP && !sameCluster(cluster, oldCluster)) {
			delete(a.retries, id)
		}
	}
	a.mu.Lock()
	for i := range next.Resources {
		resource := &next.Resources[i]
		switch {
		case !resource.Desired:
			resource.Status, resource.LastError = "stopped", ""
		case a.runs[resource.ID] != nil:
			rr := a.runs[resource.ID]
			resource.Status, resource.LastError = rr.status, rr.lastError
		default:
			resource.Status, resource.LastError = "connecting", ""
			if old, ok := current.Resource(resource.ID); ok && !a.retries[resource.ID].after.IsZero() {
				resource.Status, resource.LastError = "retrying", old.LastError
			}
		}
		if old, ok := current.Resource(resource.ID); ok {
			resource.Recovered = old.Recovered
		}
	}
	a.mu.Unlock()
	a.db.Load(next)
	a.kubernetes.prune(next)
}

func (a *app) scheduleRetryLocked(id, message string, now time.Time) {
	if a.retries == nil {
		a.retries = map[string]retryAttempt{}
	}
	a.retries[id] = nextRetry(a.retries[id], now)
	a.setResourceStatus(id, "retrying", message)
}

func nextRetry(previous retryAttempt, now time.Time) retryAttempt {
	delay := min(max(previous.delay*2, reconcileInterval), maxRetryDelay-retryJitterWindow)
	// Keep jitter after reaching the cap, with a window that spans multiple ticks.
	jitter := rand.N(retryJitterWindow)
	return retryAttempt{delay: delay, after: now.Add(delay + jitter)}
}

func (a *app) connectionClosed(id string, rr *run, err error) {
	a.operations.Lock()
	defer a.operations.Unlock()
	a.mu.Lock()
	if a.runs[id] != rr {
		a.mu.Unlock()
		return
	}
	delete(a.runs, id)
	rr.cancel()
	if rr.listener != nil {
		_ = rr.listener.Close()
	}
	a.mu.Unlock()
	resource, exists := a.db.Snapshot().Resource(id)
	if exists && resource.Desired {
		message := "服务访问连接已关闭"
		if err != nil {
			message = err.Error()
		}
		a.scheduleRetryLocked(id, message, time.Now())
	}
}
