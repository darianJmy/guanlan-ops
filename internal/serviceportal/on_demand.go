package serviceportal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httputil"
	"strconv"
	"time"

	"guanlan/internal/serviceportal/targetconfig"
)

const (
	onDemandIdleTimeout = 10 * time.Minute
	onDemandRequestWait = 45 * time.Second
)

type demandConnection struct {
	// proxy, address and err are published by closing ready. active and lastUsed are guarded by app.mu.
	ctx      context.Context
	cancel   context.CancelFunc
	ready    chan struct{}
	proxy    *httputil.ReverseProxy
	address  string
	err      error
	active   int
	lastUsed time.Time
}

func (a *app) onDemandHandler(ctx context.Context, rr *run) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		demand, retryAfter := a.takeDemand(ctx, rr, r.Context())
		if retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.FormatInt(int64((retryAfter+time.Second-1)/time.Second), 10))
			http.Error(w, "暂时无法连接服务，请稍后重试", http.StatusServiceUnavailable)
			return
		}
		if demand == nil {
			http.Error(w, "服务访问连接已关闭", http.StatusServiceUnavailable)
			return
		}
		defer a.releaseDemand(rr, demand)

		wait := time.NewTimer(onDemandRequestWait)
		defer wait.Stop()
		select {
		case <-r.Context().Done():
			return
		case <-ctx.Done():
			http.Error(w, "服务访问连接已关闭", http.StatusServiceUnavailable)
			return
		case <-wait.C:
			http.Error(w, "等待服务连接超时，请重试", http.StatusGatewayTimeout)
			return
		case <-demand.ready:
		}
		if demand.err != nil {
			http.Error(w, demand.err.Error(), http.StatusBadGateway)
			return
		}
		if demand.ctx.Err() != nil {
			http.Error(w, "服务访问连接已关闭，请重试", http.StatusServiceUnavailable)
			return
		}
		// Cancel streaming requests and upgraded connections if the service is disabled.
		requestCtx, cancel := context.WithCancel(r.Context())
		stop := context.AfterFunc(demand.ctx, cancel)
		defer stop()
		defer cancel()
		demand.proxy.ServeHTTP(w, r.WithContext(requestCtx))
	})
}

func (a *app) takeDemand(ctx context.Context, rr *run, requestCtx context.Context) (*demandConnection, time.Duration) {
	a.operations.Lock()
	defer a.operations.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	if ctx.Err() != nil || requestCtx.Err() != nil || a.runs[rr.resource.ID] != rr {
		return nil, 0
	}
	if rr.demand == nil {
		if retryAfter := time.Until(rr.demandRetry.after); retryAfter > 0 {
			return nil, retryAfter
		}
		upstreamCtx, cancel := context.WithCancel(ctx)
		demand := &demandConnection{ctx: upstreamCtx, cancel: cancel, ready: make(chan struct{})}
		rr.demand = demand
		a.setDemandStatus(rr, "connecting", "")
		go a.connectDemand(rr, demand)
	}
	rr.demand.active++
	return rr.demand, 0
}

func (a *app) releaseDemand(rr *run, demand *demandConnection) {
	a.operations.Lock()
	defer a.operations.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	demand.active--
	if demand.active != 0 || rr.demand != demand || a.runs[rr.resource.ID] != rr {
		return
	}
	demand.lastUsed = time.Now()
	if rr.status == "connecting" {
		a.detachDemand(rr, "idle", "")
	}
}

func (a *app) connectDemand(rr *run, demand *demandConnection) {
	defer demand.cancel()
	connection, err := a.openUpstream(demand.ctx, rr.resource, rr.cluster, demand.cancel)
	if err != nil {
		a.finishDemand(rr, demand, err)
		return
	}
	var proxy *httputil.ReverseProxy
	if rr.resource.Type != targetconfig.KubernetesTCP {
		var closeIdle func()
		proxy, closeIdle = newUpstreamProxy(connection, demand.cancel, true)
		defer closeIdle()
	}

	a.operations.Lock()
	a.mu.Lock()
	current := a.runs[rr.resource.ID] == rr && rr.demand == demand
	if current {
		a.setDemandStatus(rr, "running", "")
		rr.demandRetry = retryAttempt{}
		demand.proxy = proxy
		demand.address = connection.url.Host
		close(demand.ready)
	}
	a.mu.Unlock()
	a.operations.Unlock()
	if !current {
		return
	}

	select {
	case <-connection.done:
		if demand.ctx.Err() == nil {
			connection.invalidateTarget()
		}
	case <-demand.ctx.Done():
	}
	a.finishDemand(rr, demand, errors.New("服务访问连接已关闭，请重试"))
}

func (a *app) finishDemand(rr *run, demand *demandConnection, err error) {
	a.operations.Lock()
	defer a.operations.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	select {
	case <-demand.ready:
	default:
		demand.err = err
		close(demand.ready)
	}
	if a.runs[rr.resource.ID] == rr && rr.demand == demand {
		rr.demandRetry = nextRetry(rr.demandRetry, time.Now())
		a.detachDemand(rr, "failed", err.Error())
	}
}

func (a *app) expireOnDemand(now time.Time) {
	a.operations.Lock()
	defer a.operations.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, rr := range a.runs {
		if demand := rr.demand; demand != nil && demand.active == 0 && !now.Before(demand.lastUsed.Add(onDemandIdleTimeout)) {
			a.detachDemand(rr, "idle", "")
		}
	}
}

// The caller holds both operations and mu while replacing the active upstream.
func (a *app) detachDemand(rr *run, status, message string) {
	rr.demand.cancel()
	rr.demand = nil
	a.setDemandStatus(rr, status, message)
}

func (a *app) setDemandStatus(rr *run, status, message string) {
	rr.status, rr.lastError = status, message
	a.setResourceStatus(rr.resource.ID, status, message)
}
