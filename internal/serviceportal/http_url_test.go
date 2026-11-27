package serviceportal

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"guanlan/internal/serviceportal/store"
)

func httpURLResource(t *testing.T, target, path string, publicPort int) store.Resource {
	t.Helper()
	// Use the runtime's actual state wire format so the fixture also exercises
	// preservation of the new fields across configuration synchronization.
	wire, err := json.Marshal(map[string]any{"id": "http-service", "name": "HTTP console", "type": "http", "targetURL": target, "path": path, "publicPort": publicPort, "desired": true})
	if err != nil {
		t.Fatal(err)
	}
	var resource store.Resource
	if err := json.Unmarshal(wire, &resource); err != nil {
		t.Fatal(err)
	}
	return resource
}

func httpURLPublicPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}

func httpURLApp(t *testing.T, target, path string) (*app, int) {
	t.Helper()
	a, _ := integrationApp(t)
	a.runs = map[string]*run{}
	a.cfg.MinPort, a.cfg.MaxPort = 1, 65535
	a.cfg.PublicURL = "http://127.0.0.1:8081"
	publicPort := httpURLPublicPort(t)
	a.db.Load(store.State{Resources: []store.Resource{httpURLResource(t, target, path, publicPort)}})
	t.Cleanup(func() {
		a.operations.Lock()
		defer a.operations.Unlock()
		a.stopLocked("http-service")
	})
	return a, publicPort
}

func awaitHTTPURLRun(t *testing.T, a *app) *run {
	t.Helper()
	var running *run
	awaitLifecycle(t, func() bool {
		resource, _ := a.db.Snapshot().Resource("http-service")
		if resource.Status == "failed" || resource.Status == "retrying" {
			t.Fatalf("HTTP URL could not connect without Kubernetes: %s", resource.LastError)
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		if rr := a.runs["http-service"]; rr != nil && rr.status == "running" {
			running = rr
		}
		return running != nil
	}, "HTTP URL did not become running without a cluster or kubeconfig")
	return running
}

func httpURLClient() *http.Client {
	return &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}

func httpURLAccess(t *testing.T, a *app, client *http.Client, token, wantPath string) *http.Cookie {
	t.Helper()
	launch := apiCall(a, http.MethodPost, "/api/resources/http-service/access", token, "")
	var access struct {
		URL string `json:"url"`
	}
	if launch.Code != http.StatusOK || json.Unmarshal(launch.Body.Bytes(), &access) != nil || strings.Contains(access.URL, token) {
		t.Fatalf("HTTP URL launch = %d %s", launch.Code, launch.Body)
	}
	response, err := client.Get(access.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther || response.Header.Get("Location") != wantPath || len(response.Cookies()) != 1 {
		t.Fatalf("HTTP URL ticket exchange = %d, location=%q, cookies=%d", response.StatusCode, response.Header.Get("Location"), len(response.Cookies()))
	}
	return response.Cookies()[0]
}

func TestHTTPURLConnectsWithoutKubernetesAndPreservesAccessControls(t *testing.T) {
	var hits atomic.Int32
	var wantHost atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Host != wantHost.Load().(string) || r.URL.RequestURI() != "/console/?view=all" && r.URL.RequestURI() != "/assets/app.js?cache=1" {
			t.Errorf("HTTP URL origin/path/query changed: host=%q URI=%q", r.Host, r.URL.RequestURI())
		}
		if r.Header.Get("Authorization") != "" || strings.Contains(r.Header.Get("Cookie"), "portal_") {
			t.Error("HTTP URL proxy leaked platform credentials")
		}
		if cookie, err := r.Cookie("business"); err != nil || cookie.Value != "valid" {
			t.Error("HTTP URL proxy lost the application's cookie")
		}
		io.WriteString(w, "HTTP application")
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)
	wantHost.Store(u.Host)
	path := "/console/?view=all#overview"
	a, publicPort := httpURLApp(t, upstream.URL+path, "/")
	if err := a.startDesired("http-service"); err != nil {
		t.Fatalf("HTTP URL resource requires Kubernetes configuration: %v", err)
	}
	awaitHTTPURLRun(t, a)
	client := httpURLClient()
	page := "http://127.0.0.1:" + strconv.Itoa(publicPort) + path
	response, err := client.Get(page)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("unauthenticated HTTP URL access = %d", response.StatusCode)
	}
	if denied := apiCall(a, http.MethodPost, "/api/resources/http-service/access", "user-token", ""); denied.Code != http.StatusForbidden {
		t.Fatalf("ungranted HTTP URL ticket = %d", denied.Code)
	}
	if err := a.db.Update(func(s *store.State) error { s.Grants["guanlan:2"] = []string{"http-service"}; return nil }); err != nil {
		t.Fatal(err)
	}
	cookie := httpURLAccess(t, a, client, "user-token", path)
	request, _ := http.NewRequest(http.MethodGet, page, nil)
	request.AddCookie(cookie)
	request.AddCookie(&http.Cookie{Name: "portal_session", Value: "old-platform-cookie"})
	request.AddCookie(&http.Cookie{Name: "business", Value: "valid"})
	request.Header.Set("Authorization", "Bearer user-token")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || string(body) != "HTTP application" || hits.Load() != 1 {
		t.Fatalf("HTTP URL page = %d %s, read=%v hits=%d", response.StatusCode, body, err, hits.Load())
	}
	asset := request.Clone(request.Context())
	asset.URL, _ = url.Parse("http://127.0.0.1:" + strconv.Itoa(publicPort) + "/assets/app.js?cache=1")
	response, err = client.Do(asset)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || hits.Load() != 2 {
		t.Fatalf("HTTP URL asset path = %d, read=%v hits=%d", response.StatusCode, err, hits.Load())
	}
	if err := a.db.Update(func(s *store.State) error { delete(s.Grants, "guanlan:2"); return nil }); err != nil {
		t.Fatal(err)
	}
	response, err = client.Do(request.Clone(request.Context()))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden || hits.Load() != 2 {
		t.Fatalf("revoked HTTP URL access reached upstream: status=%d hits=%d", response.StatusCode, hits.Load())
	}
}

func TestHTTPURLHTTPSUsesCertificateVerification(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "TLS application") }))
	t.Cleanup(upstream.Close)
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	for _, trusted := range []bool{true, false} {
		t.Run(strconv.FormatBool(trusted), func(t *testing.T) {
			transport := original.(*http.Transport).Clone()
			transport.TLSClientConfig = &tls.Config{}
			if trusted {
				roots := x509.NewCertPool()
				roots.AddCert(upstream.Certificate())
				transport.TLSClientConfig.RootCAs = roots
			}
			http.DefaultTransport = transport
			t.Cleanup(transport.CloseIdleConnections)
			a, publicPort := httpURLApp(t, upstream.URL+"/secure/", "/secure/")
			if err := a.startDesired("http-service"); err != nil {
				t.Fatal(err)
			}
			awaitHTTPURLRun(t, a)
			client := httpURLClient()
			cookie := httpURLAccess(t, a, client, "admin-token", "/secure/")
			request, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(publicPort)+"/secure/", nil)
			request.AddCookie(cookie)
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			wantStatus := http.StatusBadGateway
			if trusted {
				wantStatus = http.StatusOK
			}
			if err != nil || response.StatusCode != wantStatus || trusted && string(body) != "TLS application" {
				t.Fatalf("HTTPS trusted=%t response=%d body=%s read=%v", trusted, response.StatusCode, body, err)
			}
		})
	}
}

func TestHTTPURLLifecycleRestoresRetriesAndReplacesTargetsWithoutClusters(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "first") }))
	t.Cleanup(first.Close)
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "second") }))
	t.Cleanup(second.Close)
	h := newLifecycleHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("HTTP URL lifecycle called Kubernetes")
		http.NotFound(w, r)
	}))
	h.app.cfg.MinPort, h.app.cfg.MaxPort = 1, 65535
	port := httpURLPublicPort(t)
	h.durable.Load(store.State{Resources: []store.Resource{httpURLResource(t, first.URL+"/", "/", port)}})
	if err := h.app.reconcileAt(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	initial := awaitHTTPURLRun(t, h.app)
	if err := h.app.reconcileAt(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if current := awaitHTTPURLRun(t, h.app); current != initial {
		t.Fatal("unchanged HTTP URL was restarted because its Kubernetes cluster is absent")
	}
	if err := h.durable.Update(func(s *store.State) error { s.Resources[0] = httpURLResource(t, second.URL+"/", "/", port); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := h.app.reconcileAt(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	replaced := awaitHTTPURLRun(t, h.app)
	if replaced == initial {
		t.Fatal("changing HTTP target URL did not replace its running proxy")
	}
	page := httptest.NewRecorder()
	replaced.proxy.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK || page.Body.String() != "second" {
		t.Fatalf("replaced HTTP URL still used the old target: %d %s", page.Code, page.Body)
	}
	second.Close()
	failed := httptest.NewRecorder()
	replaced.proxy.ServeHTTP(failed, httptest.NewRequest(http.MethodGet, "/", nil))
	if failed.Code != http.StatusBadGateway {
		t.Fatalf("lost HTTP URL target = %d", failed.Code)
	}
	awaitLifecycle(t, func() bool {
		resource, _ := h.app.db.Snapshot().Resource("http-service")
		return resource.Desired && resource.Status == "retrying"
	}, "lost HTTP URL target did not schedule a retry")
	if err := h.durable.Update(func(s *store.State) error { s.Resources[0] = httpURLResource(t, first.URL+"/", "/", port); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := h.app.reconcileAt(context.Background(), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	awaitHTTPURLRun(t, h.app)
	if err := h.durable.Update(func(s *store.State) error { s.Resources[0].Desired = false; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := h.app.reconcileAt(context.Background(), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	resource, _ := h.app.db.Snapshot().Resource("http-service")
	if resource.Desired || resource.Status != "stopped" {
		t.Fatalf("disabled HTTP URL = %#v", resource)
	}
	if conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second); err == nil {
		conn.Close()
		t.Fatal("disabled HTTP URL kept its public port open")
	}
	restarted := lifecycleTestApp(t, h.app.cfg)
	if err := restarted.reconcileAt(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	resource, _ = restarted.db.Snapshot().Resource("http-service")
	if resource.Desired || resource.Status != "stopped" {
		t.Fatal("restarting restored a disabled HTTP URL")
	}
	if err := h.durable.Update(func(s *store.State) error { s.Resources[0].Desired = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := restarted.reconcileAt(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	awaitHTTPURLRun(t, restarted)
	if h.withLogin.Load() {
		t.Fatal("HTTP URL automatic restore depended on an interactive login")
	}
}
