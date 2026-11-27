package serviceportal

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"guanlan/internal/serviceportal/config"
	"guanlan/internal/serviceportal/store"
)

func integrationApp(t *testing.T) (*app, *atomic.Bool) {
	t.Helper()
	revoked := &atomic.Bool{}
	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer unavailable-token" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		admin := r.Header.Get("Authorization") == "Bearer admin-token"
		if !admin && (r.Header.Get("Authorization") != "Bearer user-token" || revoked.Load()) {
			w.WriteHeader(401)
			io.WriteString(w, `{"code":401}`)
			return
		}
		if r.URL.Path == "/api/service-access/identity" {
			u := store.User{ID: "guanlan:2", Name: "reader", Enabled: true}
			if admin {
				u.ID, u.Name, u.Admin = "guanlan:1", "admin", true
			}
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"user": u}})
			return
		}
		if r.URL.Path == "/api/service-access/users" && admin {
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": []store.User{{ID: "guanlan:2", Name: "reader", Enabled: true}}})
			return
		}
		w.WriteHeader(403)
	}))
	t.Cleanup(identity.Close)
	db := store.New()
	if err := db.Update(func(s *store.State) error {
		s.Clusters = []store.Cluster{{ID: "cluster", Name: "Production", Context: "prod", Kubeconfig: "kubeconfig"}}
		s.Resources = []store.Resource{
			{ID: "service-one", Name: "Nacos", ClusterID: "cluster", Namespace: "middleware", Service: "nacos", Port: "8848", PublicPort: 18001, Path: "/nacos/", Desired: true},
			{ID: "service-two", Name: "Private", ClusterID: "cluster", Namespace: "middleware", Service: "private", Port: "8080", PublicPort: 18002, Path: "/", Desired: true},
		}
		s.Grants["guanlan:2"] = []string{"service-one"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a := &app{cfg: config.Config{GuanlanURL: identity.URL, PublicURL: "http://portal.example.test:8081", MinPort: 18000, MaxPort: 18999}, db: db, runs: map[string]*run{"service-one": {status: "running"}, "service-two": {status: "running"}}}
	return a, revoked
}

func apiCall(a *app, method, path, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	return w
}

func TestCentralIdentityIsRequiredAndPermissionsAreEnforced(t *testing.T) {
	a, _ := integrationApp(t)
	for _, endpoint := range []string{"/auth/local", "/auth/login", "/auth/oidc/callback", "/auth/logout", "/api/users/password", "/api/users/old-admin"} {
		w := apiCall(a, http.MethodPost, endpoint, "admin-token", `{}`)
		if w.Code != 404 {
			t.Fatalf("%s = %d", endpoint, w.Code)
		}
	}
	r := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	r.AddCookie(&http.Cookie{Name: "portal_session", Value: "old-session"})
	r.Header.Set("X-User-ID", "guanlan:1")
	r.Header.Set("X-User-Admin", "true")
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("legacy identity bypassed central auth: %d", w.Code)
	}
	if got := apiCall(a, "POST", "/api/users", "admin-token", `{}`); got.Code != http.StatusNotFound {
		t.Fatalf("local user creation still available: %d", got.Code)
	}
	// Durable service definitions, Kubernetes connections, and grants are
	// owned by 观澜. The runtime must not expose mutation endpoints for them.
	for _, endpoint := range []string{"/api/clusters", "/api/clusters/cluster", "/api/resources/service-one", "/api/permissions"} {
		if got := apiCall(a, http.MethodGet, endpoint, "admin-token", ""); got.Code != http.StatusNotFound {
			t.Fatalf("portal configuration endpoint %s = %d", endpoint, got.Code)
		}
	}
	if got := apiCall(a, http.MethodPost, "/api/resources", "user-token", `{}`); got.Code != http.StatusMethodNotAllowed {
		t.Fatalf("portal resource mutation = %d", got.Code)
	}
	resources := apiCall(a, "GET", "/api/resources", "user-token", "")
	if resources.Code != 200 || !strings.Contains(resources.Body.String(), "Production") || strings.Contains(resources.Body.String(), "Private") {
		t.Fatalf("resource filtering: %d %s", resources.Code, resources.Body)
	}
	for _, endpoint := range []string{"/api/resources/service-one/start", "/api/resources/service-one/stop"} {
		method := "GET"
		if got := apiCall(a, method, endpoint, "admin-token", ""); got.Code != 405 {
			t.Fatalf("unsupported method %s = %d", endpoint, got.Code)
		}
	}
}

func TestResourceListShowsRetryErrorWithoutActiveRun(t *testing.T) {
	a, _ := integrationApp(t)
	delete(a.runs, "service-one")
	a.setResourceStatus("service-one", "retrying", "TCP 转发需要配置 PORTAL_TCP_ALLOWED_CIDRS")

	response := apiCall(a, http.MethodGet, "/api/resources", "user-token", "")
	var resources []struct {
		Resource struct{ ID string }
		Status   string
		Error    string
	}
	if err := json.Unmarshal(response.Body.Bytes(), &resources); err != nil || response.Code != http.StatusOK {
		t.Fatalf("resource list = %d %s", response.Code, response.Body)
	}
	for _, resource := range resources {
		if resource.Resource.ID == "service-one" && resource.Status == "retrying" && strings.Contains(resource.Error, "PORTAL_TCP_ALLOWED_CIDRS") {
			return
		}
	}
	t.Fatal("resource list omitted the retry error")
}

func TestLaunchTicketIsSingleUseAndResourceScoped(t *testing.T) {
	a, revoked := integrationApp(t)
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		if r.Header.Get("Authorization") != "" || strings.Contains(r.Header.Get("Cookie"), "portal_") {
			t.Errorf("platform credentials leaked to upstream: %v", r.Header)
		}
		if cookie, err := r.Cookie("business"); err != nil || cookie.Value != "valid" {
			t.Error("business cookie lost")
		}
		io.WriteString(w, "business page")
	}))
	defer upstream.Close()
	u, _ := url.Parse(upstream.URL)
	handler := a.resourceHandler("service-one", httputil.NewSingleHostReverseProxy(u))
	launch := apiCall(a, "POST", "/api/resources/service-one/access", "user-token", "")
	if launch.Code != 200 || strings.Contains(launch.Body.String(), "user-token") {
		t.Fatalf("launch response = %d %s", launch.Code, launch.Body)
	}
	var opened struct {
		URL string `json:"url"`
	}
	json.Unmarshal(launch.Body.Bytes(), &opened)
	link, err := url.Parse(opened.URL)
	if err != nil || link.Host != "portal.example.test:18001" || link.Path != "/_portal/access" || link.Query().Get("ticket") == "" {
		t.Fatalf("launch URL: %s %v", opened.URL, err)
	}
	exchange := httptest.NewRequest("GET", opened.URL, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, exchange)
	if w.Code != 303 || w.Header().Get("Location") != "/nacos/" || len(w.Result().Cookies()) != 1 {
		t.Fatalf("exchange = %d %v", w.Code, w.Header())
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.Name != resourceCookie("service-one") || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie: %#v", cookie)
	}
	replayed := httptest.NewRecorder()
	handler.ServeHTTP(replayed, exchange)
	if replayed.Code != 401 {
		t.Fatalf("replayed ticket = %d", replayed.Code)
	}
	request := httptest.NewRequest("GET", "http://portal.example.test:18001/nacos/", nil)
	request.AddCookie(cookie)
	request.AddCookie(&http.Cookie{Name: "portal_session", Value: "legacy"})
	request.AddCookie(&http.Cookie{Name: "business", Value: "valid"})
	request.Header.Set("Authorization", "Bearer user-token")
	page := httptest.NewRecorder()
	handler.ServeHTTP(page, request)
	if page.Code != 200 || page.Body.String() != "business page" || upstreamCalls != 1 {
		t.Fatalf("business response = %d %s", page.Code, page.Body)
	}
	other := httptest.NewRecorder()
	a.resourceHandler("service-two", httputil.NewSingleHostReverseProxy(u)).ServeHTTP(other, request)
	if other.Code != 403 {
		t.Fatalf("cross-resource session = %d", other.Code)
	}
	withoutCookie := httptest.NewRequest("GET", "http://portal.example.test:18001/", nil)
	direct := httptest.NewRecorder()
	handler.ServeHTTP(direct, withoutCookie)
	if direct.Code != 403 {
		t.Fatalf("unauthenticated business access = %d", direct.Code)
	}
	revoked.Store(true)
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, request)
	if denied.Code != 403 || upstreamCalls != 1 {
		t.Fatalf("revoked identity = %d", denied.Code)
	}
	revoked.Store(false)
	a.db.Update(func(s *store.State) error { delete(s.Grants, "guanlan:2"); return nil })
	denied = httptest.NewRecorder()
	handler.ServeHTTP(denied, request)
	if denied.Code != 403 || upstreamCalls != 1 {
		t.Fatalf("revoked grant = %d", denied.Code)
	}
}

func TestPortalDoesNotPersistConfiguration(t *testing.T) {
	a, _ := integrationApp(t)
	before := a.db.Snapshot()
	for _, call := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodPost, "/api/resources", `{"id":"new-service"}`, http.StatusMethodNotAllowed},
		{http.MethodPut, "/api/clusters/cluster", `{}`, http.StatusNotFound},
		{http.MethodDelete, "/api/resources/service-one", ``, http.StatusNotFound},
		{http.MethodPost, "/api/permissions", `{"userId":"guanlan:2","resourceId":"service-one"}`, http.StatusNotFound},
	} {
		if got := apiCall(a, call.method, call.path, "admin-token", call.body); got.Code != call.status {
			t.Fatalf("configuration mutation %s %s = %d %s", call.method, call.path, got.Code, got.Body)
		}
	}
	after := a.db.Snapshot()
	if len(after.Resources) != len(before.Resources) || len(after.Clusters) != len(before.Clusters) {
		t.Fatalf("portal request mutated durable state: before=%#v after=%#v", before, after)
	}
}

func TestApplicationAuthorizationSurvivesPlatformAuthentication(t *testing.T) {
	a, _ := integrationApp(t)
	a.access = map[string]accessSession{"session": {ResourceID: "service-one", Authorization: "Bearer user-token", Expires: time.Now().Add(time.Hour)}}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer application-token" {
			t.Error("application authorization was removed")
		}
		w.WriteHeader(200)
	}))
	defer upstream.Close()
	u, _ := url.Parse(upstream.URL)
	r := httptest.NewRequest("GET", "http://portal.example.test:18001/nacos/", nil)
	r.AddCookie(&http.Cookie{Name: resourceCookie("service-one"), Value: "session"})
	r.Header.Set("Authorization", "Bearer application-token")
	w := httptest.NewRecorder()
	a.resourceHandler("service-one", httputil.NewSingleHostReverseProxy(u)).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("application request = %d", w.Code)
	}
}

func TestIdentityOutageKeepsAuthenticationDistinctFromAvailability(t *testing.T) {
	a, _ := integrationApp(t)
	if got := apiCall(a, "GET", "/api/me", "unavailable-token", ""); got.Code != 503 {
		t.Fatalf("identity outage = %d %s", got.Code, got.Body)
	}
	a.access = map[string]accessSession{"session": {ResourceID: "service-one", Authorization: "Bearer unavailable-token", Expires: time.Now().Add(time.Hour)}}
	r := httptest.NewRequest("GET", "http://portal.example.test:18001/nacos/", nil)
	r.AddCookie(&http.Cookie{Name: resourceCookie("service-one"), Value: "session"})
	w := httptest.NewRecorder()
	a.resourceHandler("service-one", nil).ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatalf("business identity outage = %d %s", w.Code, w.Body)
	}
}

func TestAutomaticStartCannotUndoCompletedStop(t *testing.T) {
	a, _ := integrationApp(t)
	delete(a.runs, "service-one")
	a.operations.Lock()
	done := make(chan error, 1)
	go func() { done <- a.startDesired("service-one") }()
	a.stopLocked("service-one")
	a.operations.Unlock()
	if err := <-done; err != nil {
		t.Fatalf("queued automatic start ignored stop: %v", err)
	}
	resource, _ := a.db.Snapshot().Resource("service-one")
	if resource.Desired || resource.Status != "stopped" || a.runs["service-one"] != nil {
		t.Fatalf("automatic start resurrected stopped resource: %#v", resource)
	}
}
