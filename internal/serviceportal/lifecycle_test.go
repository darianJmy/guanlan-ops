package serviceportal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"guanlan/internal/serviceportal/config"
	"guanlan/internal/serviceportal/store"
)

type lifecycleHarness struct {
	app       *app
	durable   *store.Store
	requests  atomic.Int32
	mode      atomic.Int32
	withLogin atomic.Bool
}

func newLifecycleHarness(t *testing.T, kubeHandler http.Handler) *lifecycleHarness {
	t.Helper()
	h := &lifecycleHarness{durable: store.New()}
	kubernetes := httptest.NewServer(kubeHandler)
	t.Cleanup(kubernetes.Close)
	h.durable.Load(store.State{
		Clusters:  []store.Cluster{{ID: "cluster", Name: "Test", Context: "test", Kubeconfig: fmt.Sprintf(`{"apiVersion":"v1","kind":"Config","clusters":[{"name":"test","cluster":{"server":%q}}],"users":[{"name":"test","user":{"token":"test-kubernetes-token"}}],"contexts":[{"name":"test","context":{"cluster":"test","user":"test"}}],"current-context":"test"}`, kubernetes.URL)}},
		Resources: []store.Resource{{ID: "nacos", Name: "Nacos", ClusterID: "cluster", Namespace: "middleware", Service: "nacos", Port: "8848", PublicPort: 18001, Path: "/nacos/", Desired: true}},
		Grants:    map[string][]string{},
	})
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		validPath := r.URL.Path == "/api/service-access/portal/runtime-state" || r.URL.Path == "/api/service-access/portal/state" && r.Header.Get("Authorization") == "Bearer test-user-token"
		if !validPath || r.Method != http.MethodGet || r.Header.Get("X-Service-Portal-Internal") != "test-internal-token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		h.requests.Add(1)
		if r.Header.Get("Authorization") != "" {
			h.withLogin.Store(true)
		}
		switch h.mode.Load() {
		case 1:
			w.WriteHeader(http.StatusServiceUnavailable)
		case 2:
			io.WriteString(w, `{"code":503,"message":"temporarily unavailable","data":null}`)
		default:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(h.durable.Snapshot())
		}
	}))
	t.Cleanup(platform.Close)
	h.app = lifecycleTestApp(t, config.Config{GuanlanURL: platform.URL, InternalToken: "test-internal-token", MinPort: 18000, MaxPort: 18999})
	return h
}

func lifecycleTestApp(t *testing.T, cfg config.Config) *app {
	t.Helper()
	a := &app{cfg: cfg, db: store.New(), runs: map[string]*run{}}
	t.Cleanup(func() {
		a.operations.Lock()
		defer a.operations.Unlock()
		a.mu.Lock()
		ids := make([]string, 0, len(a.runs))
		for id := range a.runs {
			ids = append(ids, id)
		}
		a.mu.Unlock()
		for _, id := range ids {
			a.stopLocked(id)
		}
	})
	return a
}

func awaitLifecycle(t *testing.T, condition func() bool, message string) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal(message)
		case <-ticker.C:
		}
	}
}

func TestLifecycleColdStartRetriesWithoutAnInteractiveLogin(t *testing.T) {
	var attempts atomic.Int32
	h := newLifecycleHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces/middleware/services/nacos":
			attempts.Add(1)
			io.WriteString(w, `{"apiVersion":"v1","kind":"Service","metadata":{"name":"nacos"},"spec":{"selector":{"app":"nacos"},"ports":[{"port":8848,"targetPort":8848}]}}`)
		case "/api/v1/namespaces/middleware/pods":
			io.WriteString(w, `{"apiVersion":"v1","kind":"PodList","items":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	ctx := context.Background()
	clock := time.Now()
	for attempt := int32(1); attempt <= 9; attempt++ {
		if err := h.app.reconcileAt(ctx, clock); err != nil {
			t.Fatal(err)
		}
		awaitLifecycle(t, func() bool {
			resource, _ := h.app.db.Snapshot().Resource("nacos")
			return attempts.Load() == attempt && resource.Desired && resource.Status == "retrying" && resource.LastError != ""
		}, fmt.Sprintf("automatic connection attempt %d did not finish and remain enabled", attempt))
		clock = clock.Add(time.Minute)
	}
	if h.withLogin.Load() || h.requests.Load() != 9 {
		t.Fatalf("background restore depended on an interactive login: login=%t syncs=%d", h.withLogin.Load(), h.requests.Load())
	}
	// An unchanged enabled service must retain its retry delay rather than
	// hammering Kubernetes on every configuration read.
	if err := h.app.reconcileAt(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 9 {
		t.Fatalf("configuration refresh bypassed retry backoff: %d attempts", attempts.Load())
	}
}

func TestRetryBackoffSpreadsCappedAttemptsAcrossReconcileTicks(t *testing.T) {
	a := lifecycleTestApp(t, config.Config{})
	now := time.Now()
	ticks := make(map[time.Duration]bool)
	for range 64 {
		a.scheduleRetryLocked("service", "unavailable", now)
		retry := a.retries["service"]
		delay := retry.after.Sub(now)
		if delay < reconcileInterval || delay > maxRetryDelay {
			t.Fatalf("retry delay %v outside expected bounds", delay)
		}
		if retry.delay == maxRetryDelay-retryJitterWindow {
			ticks[(delay+reconcileInterval-1)/reconcileInterval] = true
		}
	}
	if len(ticks) < 2 {
		t.Fatalf("capped retries were eligible in only %d reconciliation ticks", len(ticks))
	}
}

func TestLifecycleDisableCancelsAConnectionAndSurvivesRestart(t *testing.T) {
	started, canceled := make(chan struct{}, 1), make(chan struct{}, 1)
	var attempts atomic.Int32
	h := newLifecycleHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-r.Context().Done()
		select {
		case canceled <- struct{}{}:
		default:
		}
	}))
	ctx := context.Background()
	if err := h.app.reconcileAt(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("automatic startup did not begin its Kubernetes connection")
	}
	if err := h.durable.Update(func(s *store.State) error { s.Resources[0].Desired = false; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := h.app.reconcileAt(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("disabling a service did not cancel the in-flight connection")
	}
	for _, a := range []*app{h.app, lifecycleTestApp(t, h.app.cfg)} {
		if err := a.reconcileAt(ctx, time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		resource, exists := a.db.Snapshot().Resource("nacos")
		if !exists || resource.Desired || resource.Status != "stopped" {
			t.Fatalf("disabled service was restored as active after synchronization: %#v", resource)
		}
	}
	if attempts.Load() != 1 {
		t.Fatalf("disabled service was reconnected: %d attempts", attempts.Load())
	}
}

func attachLifecycleListener(t *testing.T, h *lifecycleHarness) (net.Listener, <-chan struct{}) {
	t.Helper()
	state := h.durable.Snapshot()
	h.app.db.Load(state)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	h.app.mu.Lock()
	h.app.runs["nacos"] = &run{status: "running", cancel: cancel, listener: listener, resource: state.Resources[0], cluster: state.Clusters[0]}
	h.app.mu.Unlock()
	return listener, ctx.Done()
}

func TestLifecycleDeletingConfigurationClosesItsListener(t *testing.T) {
	h := newLifecycleHarness(t, http.NotFoundHandler())
	listener, canceled := attachLifecycleListener(t, h)
	if err := h.durable.Update(func(s *store.State) error {
		s.Resources = []store.Resource{}
		s.Clusters = []store.Cluster{}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.app.reconcileAt(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, exists := h.app.db.Snapshot().Resource("nacos"); exists {
		t.Fatal("deleted service remained in the runtime state")
	}
	select {
	case <-canceled:
	default:
		t.Fatal("deleted service connection was not canceled")
	}
	if conn, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second); err == nil {
		conn.Close()
		t.Fatal("deleted service listener is still accepting connections")
	}
}

func TestLifecycleSyncFailuresKeepExistingListenersAndConfiguration(t *testing.T) {
	for _, mode := range []int32{1, 2} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			h := newLifecycleHarness(t, http.NotFoundHandler())
			listener, canceled := attachLifecycleListener(t, h)
			h.mode.Store(mode)
			if err := h.app.reconcileAt(context.Background(), time.Now()); err == nil {
				t.Fatal("unavailable or invalid 观澜 response was accepted as empty configuration")
			}
			resource, exists := h.app.db.Snapshot().Resource("nacos")
			if !exists || !resource.Desired {
				t.Fatal("failed state synchronization discarded enabled configuration")
			}
			select {
			case <-canceled:
				t.Fatal("failed state synchronization canceled an active listener")
			default:
			}
			conn, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
			if err != nil {
				t.Fatalf("failed state synchronization closed an active listener: %v", err)
			}
			conn.Close()
		})
	}
}

func TestLifecycleForegroundSyncRejectsAnErrorEnvelope(t *testing.T) {
	h := newLifecycleHarness(t, http.NotFoundHandler())
	listener, canceled := attachLifecycleListener(t, h)
	h.mode.Store(2)
	if err := h.app.refreshState(context.Background(), "Bearer test-user-token"); err == nil {
		t.Fatal("HTTP 200 error envelope was accepted as empty configuration during a user request")
	}
	resource, exists := h.app.db.Snapshot().Resource("nacos")
	if !exists || !resource.Desired {
		t.Fatal("foreground state synchronization discarded enabled configuration")
	}
	select {
	case <-canceled:
		t.Fatal("foreground state synchronization canceled an active listener")
	default:
	}
	conn, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("foreground state synchronization closed an active listener: %v", err)
	}
	conn.Close()
}

func TestLifecycleSlowRefreshCannotUndoNewerDisable(t *testing.T) {
	h := newLifecycleHarness(t, http.NotFoundHandler())
	_, canceled := attachLifecycleListener(t, h)
	enabled := h.durable.Snapshot()
	disabled := h.durable.Snapshot()
	disabled.Resources[0].Desired = false
	firstStarted := make(chan struct{}, 1)
	releaseFirst := make(chan struct{}, 1)
	defer close(releaseFirst)
	var requests atomic.Int32
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/service-access/portal/state" || r.Header.Get("Authorization") != "Bearer test-user-token" || r.Header.Get("X-Service-Portal-Internal") != "test-internal-token" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if requests.Add(1) == 1 {
			firstStarted <- struct{}{}
			select {
			case <-releaseFirst:
			case <-r.Context().Done():
				return
			}
			json.NewEncoder(w).Encode(enabled)
			return
		}
		json.NewEncoder(w).Encode(disabled)
	}))
	t.Cleanup(platform.Close)
	h.app.cfg.GuanlanURL = platform.URL
	firstDone := make(chan error, 1)
	go func() { firstDone <- h.app.refreshState(context.Background(), "Bearer test-user-token") }()
	select {
	case <-firstStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("first configuration request did not begin")
	}
	secondDone := make(chan error, 1)
	go func() { secondDone <- h.app.refreshState(context.Background(), "Bearer test-user-token") }()
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a slow configuration request blocked the newer disable response")
	}
	select {
	case <-canceled:
	default:
		t.Fatal("newer disable response did not close the active connection")
	}
	releaseFirst <- struct{}{}
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("old configuration request did not complete after release")
	}
	resource, exists := h.app.db.Snapshot().Resource("nacos")
	h.app.mu.Lock()
	running := h.app.runs["nacos"] != nil
	h.app.mu.Unlock()
	if !exists || resource.Desired || resource.Status != "stopped" || running {
		t.Fatalf("late enabled response undid a newer disable: resource=%#v running=%t", resource, running)
	}
}
