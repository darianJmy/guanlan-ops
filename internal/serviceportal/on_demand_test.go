package serviceportal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"guanlan/internal/serviceportal/store"

	"github.com/gorilla/websocket"
)

func TestOnDemandKubernetesConnectionFollowsAuthorizedTraffic(t *testing.T) {
	enteredHTTP := make(chan struct{}, 2)
	releaseHTTP := make(chan struct{})
	var releaseOnce sync.Once
	unblockHTTP := func() { releaseOnce.Do(func() { close(releaseHTTP) }) }
	defer unblockHTTP()

	upgrader := websocket.Upgrader{}
	h := newForwardedLifecycleHarness(t, true, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Guanlan-Authorization") != "" || r.Header.Get("Authorization") != "" ||
			strings.Contains(r.Header.Get("Cookie"), "portal_access_") {
			t.Error("platform credentials reached the business application")
		}
		switch r.URL.Path {
		case "/nacos/hold":
			enteredHTTP <- struct{}{}
			select {
			case <-releaseHTTP:
			case <-r.Context().Done():
				return
			}
		case "/nacos/ws":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Errorf("backend WebSocket upgrade: %v", err)
				return
			}
			defer conn.Close()
			for {
				kind, message, err := conn.ReadMessage()
				if err != nil {
					return
				}
				if err := conn.WriteMessage(kind, message); err != nil {
					return
				}
			}
		}
		_, _ = io.WriteString(w, "on-demand application")
	}))
	ctx := context.Background()
	reconcile := func(now time.Time) {
		t.Helper()
		if err := h.app.reconcileAt(ctx, now); err != nil {
			t.Fatal(err)
		}
	}
	reconcile(time.Now())
	resource := func() (desired bool, status string) {
		res, _ := h.app.db.Snapshot().Resource("nacos")
		return res.Desired, res.Status
	}
	checkStatus := func(want string) {
		t.Helper()
		desired, got := resource()
		if !desired || got != want {
			t.Fatalf("on-demand service status = %q, enabled = %t; want %q and enabled", got, desired, want)
		}
	}
	awaitLifecycle(t, func() bool {
		desired, status := resource()
		return desired && status == "idle"
	}, "enabled on-demand service did not become idle")

	client := httpURLClient()
	defer client.CloseIdleConnections()
	send := func(path string, cookie *http.Cookie, token bool) (int, string, error) {
		req, err := http.NewRequest(http.MethodGet, h.publicURL+path, nil)
		if err != nil {
			return 0, "", err
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		if token {
			req.Header.Set("X-Guanlan-Authorization", "Bearer test-user-token")
		}
		resp, err := client.Do(req)
		if err != nil {
			return 0, "", err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body), err
	}
	if code, _, err := send("/nacos/", nil, false); err != nil || code != http.StatusForbidden {
		t.Fatalf("unauthenticated request = %d, %v", code, err)
	}
	issued := apiCall(h.app, http.MethodPost, "/api/resources/nacos/access", "test-user-token", "")
	var ticket struct {
		URL string `json:"url"`
	}
	if issued.Code != http.StatusOK || json.Unmarshal(issued.Body.Bytes(), &ticket) != nil {
		t.Fatalf("access ticket: %d %s", issued.Code, issued.Body)
	}
	exchange, err := client.Get(ticket.URL)
	if err != nil {
		t.Fatal(err)
	}
	exchange.Body.Close()
	cookies := exchange.Cookies()
	if exchange.StatusCode != http.StatusSeeOther || exchange.Header.Get("Location") != "/nacos/" || len(cookies) != 1 {
		t.Fatalf("ticket exchange = %d, location = %q, cookies = %d", exchange.StatusCode, exchange.Header.Get("Location"), len(cookies))
	}
	checkStatus("idle")
	if requests := h.kubernetesRequests.Load(); requests != 0 {
		t.Fatalf("startup, rejected request or access ticket triggered %d Kubernetes requests", requests)
	}

	result := make(chan error, 2)
	for _, cookie := range []*http.Cookie{cookies[0], nil} {
		go func() {
			code, body, err := send("/nacos/hold", cookie, cookie == nil)
			if err == nil && (code != http.StatusOK || body != "on-demand application") {
				err = fmt.Errorf("business response = %d %q", code, body)
			}
			result <- err
		}()
	}
	for range 2 {
		select {
		case <-enteredHTTP:
		case err := <-result:
			t.Fatalf("business response completed before upstream handler was reached: %v", err)
		case <-time.After(3 * time.Second):
			t.Fatal("concurrent authenticated requests did not reach the upstream handler")
		}
	}
	first := h.nextConnection(t)
	reconcile(time.Now().Add(time.Hour))
	checkStatus("running")
	if count := h.upgrades.Load(); count != 1 {
		t.Fatalf("concurrent access created %d SPDY connections, want 1", count)
	}
	unblockHTTP()
	for range 2 {
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	}

	openWebSocket := func() *websocket.Conn {
		t.Helper()
		wsURL := "ws" + strings.TrimPrefix(h.publicURL, "http") + "/nacos/ws"
		headers := http.Header{"Cookie": {cookies[0].String()}}
		dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
		ws, _, err := dialer.Dial(wsURL, headers)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ws.Close() })
		return ws
	}
	ws := openWebSocket()
	reconcile(time.Now().Add(time.Hour))
	checkStatus("running")
	_ = ws.SetWriteDeadline(time.Now().Add(3 * time.Second))
	if err := ws.WriteMessage(websocket.TextMessage, []byte("still connected")); err != nil {
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, message, err := ws.ReadMessage(); err != nil || string(message) != "still connected" {
		t.Fatalf("active WebSocket after idle check = %q, %v", message, err)
	}
	_ = ws.Close()

	awaitLifecycle(t, func() bool {
		reconcile(time.Now().Add(time.Hour))
		desired, status := resource()
		return desired && status == "idle"
	}, "no traffic remained, but the on-demand service did not become idle")
	select {
	case <-first.CloseChan():
	case <-time.After(3 * time.Second):
		t.Fatal("idle expiration did not close the SPDY connection")
	}
	reconcile(time.Now().Add(2 * time.Hour))
	checkStatus("idle")
	if count := h.upgrades.Load(); count != 1 {
		t.Fatalf("idle service reconnected in the background, upgrades = %d", count)
	}
	if code, body, err := send("/nacos/", cookies[0], false); err != nil || code != http.StatusOK || body != "on-demand application" {
		t.Fatalf("reopened session = %d %q, %v", code, body, err)
	}
	checkStatus("running")
	if count := h.upgrades.Load(); count != 2 {
		t.Fatalf("authenticated access after idle expiration created %d total SPDY connections, want 2", count)
	}

	second := h.nextConnection(t)
	ws = openWebSocket()
	if err := h.durable.Update(func(state *store.State) error {
		state.Resources[0].Desired = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reconcile(time.Now())
	if desired, status := resource(); desired || status != "stopped" {
		t.Fatalf("disabled service status = %q, enabled = %t", status, desired)
	}
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := ws.ReadMessage(); err == nil {
		t.Fatal("disabled service left the WebSocket open")
	} else if networkError, ok := err.(net.Error); ok && networkError.Timeout() {
		t.Fatal("disabling did not close the active WebSocket")
	}
	select {
	case <-second.CloseChan():
	case <-time.After(3 * time.Second):
		t.Fatal("disabling did not close the current SPDY connection")
	}
	if conn, err := net.DialTimeout("tcp", strings.TrimPrefix(h.publicURL, "http://"), time.Second); err == nil {
		conn.Close()
		t.Fatal("disabled service left the public listener open")
	}
}
