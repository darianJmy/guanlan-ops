package serviceportal

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDirectBearerAccessChecksIdentityAndResourcePermissions(t *testing.T) {
	a, revoked := integrationApp(t)
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "" || strings.Contains(r.Header.Get("Cookie"), "portal_") {
			t.Error("platform credentials leaked to business service")
		}
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.RequestURI() != "/api/items?tenant=demo" || string(body) != `{"name":"test"}` {
			t.Errorf("business request changed: %s %s %s", r.Method, r.URL, body)
		}
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, "created")
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)
	request := func(id, authorization string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://portal.test/api/items?tenant=demo", strings.NewReader(`{"name":"test"}`))
		r.Header.Set("Authorization", authorization)
		r.AddCookie(&http.Cookie{Name: "portal_session", Value: "legacy"})
		w := httptest.NewRecorder()
		a.resourceHandler(id, httputil.NewSingleHostReverseProxy(u)).ServeHTTP(w, r)
		return w
	}
	if w := request("service-one", "Bearer user-token"); w.Code != 201 || w.Body.String() != "created" || len(w.Result().Cookies()) != 0 {
		t.Fatalf("direct API access: %d %s", w.Code, w.Body)
	}
	for _, tc := range []struct {
		id, auth string
		status   int
	}{
		{"service-two", "Bearer user-token", 403},
		{"service-one", "Bearer expired-token", 401},
		{"service-one", "Bearer ", 401},
		{"service-one", "Basic credentials", 401},
		{"service-one", "Bearer unavailable-token", 503},
		{"service-one", "", 403},
	} {
		if w := request(tc.id, tc.auth); w.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.id, tc.auth, w.Code, w.Body)
		}
	}
	revoked.Store(true)
	if w := request("service-one", "Bearer user-token"); w.Code != 401 {
		t.Fatalf("revoked token: %d", w.Code)
	}
	if calls != 1 {
		t.Fatalf("denied requests reached upstream: %d", calls)
	}
}

func TestExplicitPlatformHeaderPreservesApplicationAuthorization(t *testing.T) {
	a, _ := integrationApp(t)
	a.access = map[string]accessSession{"admin-session": {ResourceID: "service-two", Authorization: "Bearer admin-token", Expires: time.Now().Add(time.Hour)}}
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Guanlan-Authorization") != "" || strings.Contains(r.Header.Get("Cookie"), "portal_") {
			t.Error("platform credentials leaked to upstream")
		}
		if r.Header.Get("Authorization") != "Bearer application-token" && r.Header.Get("Authorization") != "Basic app-credentials" {
			t.Errorf("application authorization changed: %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(204)
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)
	request := func(id, platform, application string, cookie bool) int {
		r := httptest.NewRequest("GET", "http://portal.test/api/items", nil)
		r.Header.Set("X-Guanlan-Authorization", platform)
		r.Header.Set("Authorization", application)
		if cookie {
			r.AddCookie(&http.Cookie{Name: resourceCookie(id), Value: "admin-session"})
		}
		w := httptest.NewRecorder()
		a.resourceHandler(id, httputil.NewSingleHostReverseProxy(u)).ServeHTTP(w, r)
		return w.Code
	}
	for _, application := range []string{"Bearer application-token", "Basic app-credentials"} {
		if status := request("service-one", "Bearer user-token", application, false); status != 204 {
			t.Fatalf("separate credentials: %d", status)
		}
	}
	for _, tc := range []struct {
		id, platform string
		cookie       bool
		status       int
	}{
		{"service-one", "", false, 401},
		{"service-one", "Bearer expired-token", false, 401},
		{"service-one", "Bearer unavailable-token", false, 503},
		{"service-two", "Bearer user-token", true, 403},
		{"service-two", "Bearer expired-token", true, 401},
	} {
		if status := request(tc.id, tc.platform, "Bearer application-token", tc.cookie); status != tc.status {
			t.Fatalf("explicit header precedence: got %d want %d", status, tc.status)
		}
	}
	if calls != 2 {
		t.Fatalf("denied requests reached upstream: %d", calls)
	}
	r := httptest.NewRequest("GET", "http://portal.test/api/items", nil)
	r.Header.Add("X-Guanlan-Authorization", "Bearer user-token")
	r.Header.Add("X-Guanlan-Authorization", "Bearer admin-token")
	w := httptest.NewRecorder()
	a.resourceHandler("service-one", nil).ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("ambiguous platform credentials accepted: %d", w.Code)
	}
}

func TestDirectBearerAccessRechecksDirectoryACL(t *testing.T) {
	a, revoked := groupedApp(t)
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) }))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)
	request := func(id, token string) int {
		r := httptest.NewRequest("GET", "http://portal.test/api/items", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		a.resourceHandler(id, httputil.NewSingleHostReverseProxy(u)).ServeHTTP(w, r)
		return w.Code
	}
	if status := request("service-one", "user-token"); status != 204 {
		t.Fatalf("authorized directory: %d", status)
	}
	if status := request("service-two", "manager-token"); status != 403 {
		t.Fatalf("global management bypassed directory ACL: %d", status)
	}
	revoked.Store(true)
	if status := request("service-one", "user-token"); status != 403 {
		t.Fatalf("revoked directory: %d", status)
	}
	if calls != 1 {
		t.Fatalf("denied directory requests reached upstream: %d", calls)
	}
}
