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

	"guanlan/internal/serviceportal/store"
)

func groupedApp(t *testing.T) (*app, *atomic.Bool) {
	t.Helper()
	a, _ := integrationApp(t)
	revoked := &atomic.Bool{}
	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := store.User{ID: "guanlan:2", Name: "reader", Enabled: true, FolderAccess: map[uint]string{10: "view", 11: "none", 12: "edit", 13: "none", 14: "none"}}
		switch r.Header.Get("Authorization") {
		case "Bearer admin-token":
			u.ID, u.Name, u.Admin = "guanlan:1", "admin", true
			u.FolderAccess = map[uint]string{10: "manage", 11: "edit", 12: "view", 13: "none", 14: "manage"}
		case "Bearer manager-token":
			u.ID, u.Name, u.Admin = "guanlan:3", "restricted-manager", true
		case "Bearer user-token":
		default:
			w.WriteHeader(401)
			return
		}
		if revoked.Load() {
			u.FolderAccess[10] = "none"
		}
		if r.URL.Path == "/api/service-access/identity" {
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"user": u}})
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(identity.Close)
	a.cfg.GuanlanURL = identity.URL
	if err := a.db.Update(func(s *store.State) error {
		s.Resources[0].FolderID = 10
		s.Resources[1].FolderID = 13
		s.Grants = map[string][]string{}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return a, revoked
}

func TestGroupedAccessUsesCentralFolderRolesWithoutIndividualGrants(t *testing.T) {
	a, _ := groupedApp(t)
	resources := apiCall(a, "GET", "/api/resources", "user-token", "")
	if resources.Code != 200 || !strings.Contains(resources.Body.String(), `"access_level":"view"`) || !strings.Contains(resources.Body.String(), `"folder_id":10`) || strings.Contains(resources.Body.String(), "Private") {
		t.Fatalf("reader resources: %d %s", resources.Code, resources.Body)
	}
	manager := apiCall(a, "GET", "/api/resources", "manager-token", "")
	if manager.Code != 200 || strings.Contains(manager.Body.String(), "Private") {
		t.Fatalf("global manager bypassed folder ACL: %d %s", manager.Code, manager.Body)
	}
	for _, endpoint := range []string{"/api/resources/service-one/start", "/api/resources/service-one/stop", "/api/resources/service-one/folder"} {
		expected := 403
		if strings.HasSuffix(endpoint, "/folder") {
			expected = 404
		}
		if got := apiCall(a, "POST", endpoint, "manager-token", `{"folder_id":0}`); got.Code != expected {
			t.Fatalf("restricted manager %s = %d %s", endpoint, got.Code, got.Body)
		}
	}
	if got := apiCall(a, "DELETE", "/api/resources/service-one", "manager-token", ""); got.Code != 404 {
		t.Fatalf("restricted manager delete = %d", got.Code)
	}
	resource, _ := a.db.Snapshot().Resource("service-one")
	resource.FolderID = 0
	body, _ := json.Marshal(resource)
	if got := apiCall(a, "POST", "/api/resources", "manager-token", string(body)); got.Code != 405 {
		t.Fatalf("direct folder_id bypass = %d %s", got.Code, got.Body)
	}
	if got := apiCall(a, "POST", "/api/resources/service-one/stop", "user-token", ""); got.Code != 403 {
		t.Fatalf("reader mutation = %d", got.Code)
	}
	for _, method := range []string{"POST", "DELETE"} {
		if got := apiCall(a, method, "/api/permissions", "admin-token", `{"userId":"guanlan:2","resourceId":"service-one"}`); got.Code != 404 {
			t.Fatalf("grouped grant %s = %d %s", method, got.Code, got.Body)
		}
	}
}

func TestPortalDoesNotMutateFolderAssignments(t *testing.T) {
	a, _ := groupedApp(t)
	before := a.db.Snapshot()
	for _, folder := range []string{"11", "12", "0"} {
		if got := apiCall(a, "POST", "/api/resources/service-one/folder", "admin-token", `{"folder_id":`+folder+`}`); got.Code != 404 {
			t.Fatalf("portal folder mutation %s = %d", folder, got.Code)
		}
	}
	after := a.db.Snapshot()
	resource, _ := after.Resource("service-one")
	beforeResource, _ := before.Resource("service-one")
	if resource.FolderID != beforeResource.FolderID {
		t.Fatalf("portal folder mutation changed state: before=%#v after=%#v", beforeResource, resource)
	}
}

func TestPortalDoesNotExposeFolderUsage(t *testing.T) {
	a, _ := groupedApp(t)
	for _, token := range []string{"admin-token", "user-token", "manager-token"} {
		if got := apiCall(a, "GET", "/api/folders/10/usage", token, ""); got.Code != 404 {
			t.Fatalf("usage by %s = %d %s", token, got.Code, got.Body)
		}
	}
}

func TestFolderACLRevocationBlocksLaunchExchangeAndExistingBusinessSession(t *testing.T) {
	a, revoked := groupedApp(t)
	launch := apiCall(a, "POST", "/api/resources/service-one/access", "user-token", "")
	if launch.Code != 200 {
		t.Fatalf("group reader launch = %d %s", launch.Code, launch.Body)
	}
	var link struct {
		URL string `json:"url"`
	}
	json.Unmarshal(launch.Body.Bytes(), &link)
	revoked.Store(true)
	w := httptest.NewRecorder()
	a.resourceHandler("service-one", nil).ServeHTTP(w, httptest.NewRequest("GET", link.URL, nil))
	if w.Code != 403 {
		t.Fatalf("revoked ticket exchange = %d %s", w.Code, w.Body)
	}
	revoked.Store(false)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "private app") }))
	defer upstream.Close()
	u, _ := url.Parse(upstream.URL)
	handler := a.resourceHandler("service-one", httputil.NewSingleHostReverseProxy(u))
	a.access = map[string]accessSession{"session": {ResourceID: "service-one", Authorization: "Bearer user-token", Expires: time.Now().Add(time.Hour)}}
	r := httptest.NewRequest("GET", "http://portal.example.test:18001/nacos/", nil)
	r.AddCookie(&http.Cookie{Name: resourceCookie("service-one"), Value: "session"})
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("group reader business = %d %s", w.Code, w.Body)
	}
	revoked.Store(true)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("revoked existing group session = %d %s", w.Code, w.Body)
	}
	if got := apiCall(a, "POST", "/api/resources/service-one/access", "user-token", ""); got.Code != 403 {
		t.Fatalf("revoked launch = %d", got.Code)
	}
}
