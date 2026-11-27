package serviceportal

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"guanlan/internal/serviceportal/store"
	"guanlan/internal/serviceportal/targetconfig"
)

func TestKubernetesTCPForwardsTLSAndStopsConnections(t *testing.T) {
	t.Run("always-on", func(t *testing.T) { testKubernetesTCP(t, false) })
	t.Run("on-demand", func(t *testing.T) { testKubernetesTCP(t, true) })
}

func testKubernetesTCP(t *testing.T, onDemand bool) {
	t.Helper()
	business := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || r.URL.Path != "/nacos/" {
			t.Error("expected original TLS request to reach the Pod")
		}
		io.WriteString(w, "TLS from the Pod")
	}))
	h := newForwardedLifecycleHarnessForServer(t, onDemand, business)
	h.app.cfg.TCPGrantTTL = 15 * time.Minute
	if err := h.durable.Update(func(state *store.State) error {
		state.Resources[0].Type = targetconfig.KubernetesTCP
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := h.app.reconcileAt(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	wantStatus := "running"
	if onDemand {
		wantStatus = "idle"
	}
	awaitLifecycle(t, func() bool {
		resource, _ := h.app.db.Snapshot().Resource("nacos")
		return resource.Status == wantStatus
	}, "TCP listener did not start")

	address := strings.TrimPrefix(h.publicURL, "http://")
	client := business.Client()
	client.Timeout = 3 * time.Second
	assertTCPDenied := func() {
		t.Helper()
		client.CloseIdleConnections()
		if response, err := client.Get("https://" + address + "/nacos/"); err == nil {
			response.Body.Close()
			t.Fatalf("TCP connection without an active grant reached the Pod: %d", response.StatusCode)
		}
	}
	assertTCPDenied()
	if denied := apiCall(h.app, "POST", "/api/resources/nacos/access", "test-user-token", ""); denied.Code != http.StatusForbidden {
		t.Fatalf("TCP launch outside gateway status = %d", denied.Code)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/resources/nacos/access", nil)
	request.Header.Set("Authorization", "Bearer test-user-token")
	request.Header.Set("X-Service-Portal-Internal", h.app.cfg.InternalToken)
	request.Header.Set("X-Service-Portal-Client-IP", "127.0.0.1")
	launch := httptest.NewRecorder()
	h.app.routes().ServeHTTP(launch, request)
	var opened struct {
		URL     string
		Address string
	}
	wantURL := "https://" + address + "/nacos/"
	if err := json.Unmarshal(launch.Body.Bytes(), &opened); err != nil || launch.Code != http.StatusOK || opened.URL != wantURL || opened.Address != address {
		t.Fatalf("unexpected TCP launch response: %d %s", launch.Code, launch.Body)
	}
	if denied := apiCall(h.app, "POST", "/api/resources/nacos/access", "wrong-token", ""); denied.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated TCP launch status = %d", denied.Code)
	}
	if onDemand && h.kubernetesRequests.Load() != 0 {
		t.Fatal("opening the listener or requesting its address connected to Kubernetes")
	}

	if onDemand {
		h.rejectNextService.Store(true)
		client.Timeout = 15 * time.Second
	}
	response, err := client.Get(opened.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || string(body) != "TLS from the Pod" {
		t.Fatalf("unexpected TCP response: %d %q %v", response.StatusCode, body, err)
	}

	dialer := &net.Dialer{Timeout: time.Second}
	if onDemand {
		client.CloseIdleConnections()
		first := h.nextConnection(t)
		awaitLifecycle(t, func() bool {
			h.app.mu.Lock()
			defer h.app.mu.Unlock()
			demand := h.app.runs["nacos"].demand
			return demand != nil && demand.active == 0
		}, "TCP session never released its demand connection")
		h.app.expireOnDemand(time.Now().Add(onDemandIdleTimeout))
		select {
		case <-first.CloseChan():
		case <-time.After(time.Second):
			t.Fatal("idle tunnel stayed open")
		}
		if resource, _ := h.app.db.Snapshot().Resource("nacos"); resource.Status != "idle" {
			t.Fatalf("TCP status after expiry = %s", resource.Status)
		}
	}
	conn, err := tls.DialWithDialer(dialer, "tcp", address, client.Transport.(*http.Transport).TLSClientConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	h.app.mu.Lock()
	h.app.runs["nacos"].tcpGrants[netip.MustParseAddr("127.0.0.1")] = time.Now().Add(-time.Second)
	h.app.mu.Unlock()
	assertTCPDenied()
	if _, err := io.WriteString(conn, "GET /nacos/ HTTP/1.1\r\nHost: localhost\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := conn.Read(make([]byte, 1)); err != nil {
		t.Fatalf("established TCP connection failed after grant expiry: %v", err)
	}
	if onDemand {
		_ = h.nextConnection(t)
		h.app.expireOnDemand(time.Now().Add(2 * onDemandIdleTimeout))
		if resource, _ := h.app.db.Snapshot().Resource("nacos"); resource.Status != "running" {
			t.Fatalf("active TCP session was expired: %s", resource.Status)
		}
	}
	h.app.operations.Lock()
	h.app.stopLocked("nacos")
	h.app.operations.Unlock()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := io.Copy(io.Discard, conn); err != nil {
		if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
			t.Fatalf("existing TCP socket remained open: %v", err)
		}
	}
	if response, err := client.Get(opened.URL); err == nil {
		response.Body.Close()
		t.Fatal("existing TCP socket survived stop")
	}
}

func TestTCPSourceAllowedUsesSocketPeer(t *testing.T) {
	now := time.Now()
	rr := &run{resource: store.Resource{ID: "one"}, tcpGrants: map[netip.Addr]time.Time{
		netip.MustParseAddr("192.0.2.10"): now.Add(time.Minute),
	}}
	other := &run{resource: store.Resource{ID: "two"}}
	a := &app{runs: map[string]*run{"one": rr, "two": other}}
	for _, peer := range []struct {
		ip   string
		want bool
	}{
		{"192.0.2.10", true},
		{"::ffff:192.0.2.10", true},
		{"198.51.100.10", false},
		{"::1", false},
	} {
		address := &net.TCPAddr{IP: net.ParseIP(peer.ip)}
		if got := a.tcpSourceAllowed(rr, address, now); got != peer.want {
			t.Errorf("%s allowed = %t, want %t", peer.ip, got, peer.want)
		}
		if a.tcpSourceAllowed(other, address, now) {
			t.Errorf("%s allowed on a different service", peer.ip)
		}
	}
}
