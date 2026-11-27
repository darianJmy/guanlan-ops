package serviceportal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"guanlan/internal/serviceportal/config"
	"guanlan/internal/serviceportal/store"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	streamspdy "k8s.io/apimachinery/pkg/util/httpstream/spdy"
)

type forwardedLifecycleHarness struct {
	app                 *app
	durable             *store.Store
	connections         <-chan httpstream.Connection
	publicURL           string
	kubernetesRequests  atomic.Int32
	rejectNextService   atomic.Bool
	upgrades            atomic.Int32
	backgroundRequests  atomic.Int32
	backgroundUsedLogin atomic.Bool
}

func newForwardedLifecycleHarness(t *testing.T, onDemand bool, businessHandler http.Handler) *forwardedLifecycleHarness {
	t.Helper()
	return newForwardedLifecycleHarnessForServer(t, onDemand, httptest.NewServer(businessHandler))
}

func newForwardedLifecycleHarnessForServer(t *testing.T, onDemand bool, business *httptest.Server) *forwardedLifecycleHarness {
	t.Helper()
	h := &forwardedLifecycleHarness{durable: store.New()}
	t.Cleanup(business.Close)
	businessAddress := business.Listener.Addr().String()
	connections := make(chan httpstream.Connection, 8)
	h.connections = connections
	var connectionMu sync.Mutex
	var openConnections []httpstream.Connection
	kubernetes := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.kubernetesRequests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces/middleware/services/nacos":
			if h.rejectNextService.Swap(false) {
				http.Error(w, "temporary service lookup failure", http.StatusForbidden)
				return
			}
			io.WriteString(w, `{"apiVersion":"v1","kind":"Service","metadata":{"name":"nacos"},"spec":{"selector":{"app":"nacos"},"ports":[{"port":8848,"targetPort":8848}]}}`)
		case "/api/v1/namespaces/middleware/pods":
			io.WriteString(w, `{"apiVersion":"v1","kind":"PodList","items":[{"metadata":{"name":"nacos-0"},"status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}]}}]}`)
		case "/api/v1/namespaces/middleware/pods/nacos-0/portforward":
			if _, err := httpstream.Handshake(r, w, []string{"portforward.k8s.io"}); err != nil {
				t.Errorf("port-forward handshake: %v", err)
				return
			}
			var streamsMu sync.Mutex
			errorStreams := map[string]httpstream.Stream{}
			connection := streamspdy.NewResponseUpgrader().UpgradeResponse(w, r, func(stream httpstream.Stream, replySent <-chan struct{}) error {
				requestID := stream.Headers().Get(v1.PortForwardRequestIDHeader)
				switch stream.Headers().Get(v1.StreamType) {
				case v1.StreamTypeError:
					streamsMu.Lock()
					errorStreams[requestID] = stream
					streamsMu.Unlock()
				case v1.StreamTypeData:
					go func() {
						<-replySent
						defer stream.Close()
						streamsMu.Lock()
						errorStream := errorStreams[requestID]
						delete(errorStreams, requestID)
						streamsMu.Unlock()
						if errorStream != nil {
							defer errorStream.Close()
						}
						upstream, err := net.DialTimeout("tcp", businessAddress, time.Second)
						if err != nil {
							t.Errorf("mock Pod could not reach business server: %v", err)
							return
						}
						defer upstream.Close()
						go func() {
							_, _ = io.Copy(upstream, stream)
							_ = upstream.(*net.TCPConn).CloseWrite()
						}()
						_, _ = io.Copy(stream, upstream)
					}()
				default:
					return fmt.Errorf("unexpected stream type %q", stream.Headers().Get(v1.StreamType))
				}
				return nil
			})
			if connection == nil {
				t.Error("SPDY upgrade failed")
				return
			}
			h.upgrades.Add(1)
			connectionMu.Lock()
			openConnections = append(openConnections, connection)
			connectionMu.Unlock()
			connections <- connection
			defer connection.Close()
			<-connection.CloseChan()
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(kubernetes.Close)
	t.Cleanup(func() {
		connectionMu.Lock()
		defer connectionMu.Unlock()
		for _, connection := range openConnections {
			_ = connection.Close()
		}
	})

	reservation, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	publicPort := reservation.Addr().(*net.TCPAddr).Port
	reservation.Close()
	h.publicURL = fmt.Sprintf("http://127.0.0.1:%d", publicPort)
	h.durable.Load(store.State{
		Clusters:  []store.Cluster{{ID: "cluster", Context: "test", Kubeconfig: fmt.Sprintf(`{"apiVersion":"v1","kind":"Config","clusters":[{"name":"test","cluster":{"server":%q}}],"users":[{"name":"test","user":{"token":"test-kubernetes-token"}}],"contexts":[{"name":"test","context":{"cluster":"test","user":"test"}}],"current-context":"test"}`, kubernetes.URL)}},
		Resources: []store.Resource{{ID: "nacos", Name: "Nacos", ClusterID: "cluster", Namespace: "middleware", Service: "nacos", Port: "8848", PublicPort: publicPort, Path: "/nacos/", Desired: true, OnDemand: onDemand}},
	})
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("X-Service-Portal-Internal") != "test-internal-token" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Path == "/api/service-access/portal/runtime-state" {
			h.backgroundRequests.Add(1)
			if r.Header.Get("Authorization") != "" {
				h.backgroundUsedLogin.Store(true)
			}
			json.NewEncoder(w).Encode(h.durable.Snapshot())
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-user-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/service-access/identity":
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"user": store.User{ID: "guanlan:1", Name: "tester", Enabled: true, Admin: true}}})
		case "/api/service-access/portal/state":
			json.NewEncoder(w).Encode(h.durable.Snapshot())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(platform.Close)
	cfg := config.Config{GuanlanURL: platform.URL, InternalToken: "test-internal-token", MinPort: 1024, MaxPort: 65535, PublicURL: "http://127.0.0.1:8081"}
	h.app = lifecycleTestApp(t, cfg)
	return h
}

func (h *forwardedLifecycleHarness) nextConnection(t *testing.T) httpstream.Connection {
	t.Helper()
	select {
	case conn := <-h.connections:
		return conn
	case <-time.After(time.Second):
		t.Fatal("successful startup did not create an SPDY connection")
		return nil
	}
}

// Exercise the real client-go SPDY forwarder, public listener, access ticket,
// and business proxy together.
func TestLifecycleAutomaticallyForwardsRecoversAndRestoresEnabledService(t *testing.T) {
	var businessRequests atomic.Int32
	h := newForwardedLifecycleHarness(t, false, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessRequests.Add(1)
		if r.URL.Path != "/nacos/" {
			t.Errorf("unexpected business path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" || strings.Contains(r.Header.Get("Cookie"), "portal_access_") {
			t.Error("platform credentials reached the business application")
		}
		io.WriteString(w, "automatically forwarded business page")
	}))
	a := h.app
	ctx := context.Background()
	if err := a.reconcileAt(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	waitRunning := func(application *app) {
		t.Helper()
		awaitLifecycle(t, func() bool {
			resource, _ := application.db.Snapshot().Resource("nacos")
			return resource.Desired && resource.Status == "running"
		}, "enabled service did not establish its real SPDY port-forward")
	}
	waitRunning(a)
	if h.backgroundRequests.Load() != 1 || h.backgroundUsedLogin.Load() {
		t.Fatal("automatic startup used a browser login")
	}
	firstConnection := h.nextConnection(t)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: 3 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	publicURL := h.publicURL + "/nacos/"
	response, err := client.Get(publicURL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden || businessRequests.Load() != 0 {
		t.Fatal("automatically restored listener bypassed resource authentication")
	}
	openService := func(application *app) {
		t.Helper()
		issued := apiCall(application, http.MethodPost, "/api/resources/nacos/access", "test-user-token", "")
		if issued.Code != http.StatusOK {
			t.Fatalf("access ticket: %d %s", issued.Code, issued.Body)
		}
		var ticket struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(issued.Body.Bytes(), &ticket); err != nil {
			t.Fatal(err)
		}
		page, err := client.Get(ticket.URL)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(page.Body)
		page.Body.Close()
		if err != nil || page.StatusCode != http.StatusOK || string(body) != "automatically forwarded business page" {
			t.Fatalf("business proxy: status=%d body=%q error=%v", page.StatusCode, body, err)
		}
	}
	openService(a)
	if businessRequests.Load() != 1 {
		t.Fatal("authenticated ticket did not forward a request to the business application")
	}

	// An established connection dying must be retried without another start.
	firstConnection.Close()
	awaitLifecycle(t, func() bool {
		resource, _ := a.db.Snapshot().Resource("nacos")
		return resource.Desired && resource.Status == "retrying"
	}, "a disconnected forwarder did not remain enabled for recovery")
	if err := a.reconcileAt(ctx, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	waitRunning(a)
	openService(a)
	if h.upgrades.Load() != 2 {
		t.Fatalf("expected a replacement SPDY connection after disconnect, got %d", h.upgrades.Load())
	}

	// A fresh process restores the same durable enabled state; stopping the old
	// process clears no 观澜 configuration and retains no local database.
	client.CloseIdleConnections()
	a.operations.Lock()
	a.stopLocked("nacos")
	a.operations.Unlock()
	restarted := lifecycleTestApp(t, a.cfg)
	if err := restarted.reconcileAt(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	waitRunning(restarted)
	openService(restarted)
	if h.upgrades.Load() != 3 {
		t.Fatalf("fresh process did not restore its port-forward: %d connections", h.upgrades.Load())
	}
	if err := h.durable.Update(func(s *store.State) error { s.Resources[0].Desired = false; return nil }); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := restarted.reconcileAt(ctx, time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	resource, _ := restarted.db.Snapshot().Resource("nacos")
	if resource.Desired || resource.Status != "stopped" || h.upgrades.Load() != 3 {
		t.Fatalf("administrator-disabled service was restarted: %#v, upgrades=%d", resource, h.upgrades.Load())
	}
	if connection, err := net.DialTimeout("tcp", strings.TrimPrefix(h.publicURL, "http://"), time.Second); err == nil {
		connection.Close()
		t.Fatal("disabled service left its public listener open")
	}
}
