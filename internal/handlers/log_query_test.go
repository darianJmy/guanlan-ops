package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"guanlan/internal/config"
	"guanlan/internal/database"
	"guanlan/internal/middleware"
	"guanlan/internal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestLokiLogQuery(t *testing.T) {
	previous := database.DB
	db, cleanup, err := database.New(config.Config{DBDSN: filepath.Join(t.TempDir(), "logs.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { cleanup(); database.DB = previous }()
	upstreamCalls := 0
	response := `{"status":"success","data":{"resultType":"streams","result":[{"stream":{"app":"a"},"values":[["1700000000000000001","older",{"trace_id":"abc"}]]},{"stream":{"app":"b"},"values":[["1700000000000000002","newer"]]}]}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		if r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("X-Scope-OrgID") != "prod" {
			t.Error("missing auth or tenant")
		}
		if r.URL.Path == "/tenant/loki/api/v1/labels" {
			fmt.Fprint(w, `{"status":"success","data":["app"]}`)
			return
		}
		if r.URL.Path != "/tenant/loki/api/v1/query_range" || r.URL.Query().Get("query") != `{app="a"}` || r.URL.Query().Get("direction") != "backward" || r.URL.Query().Get("limit") != "2" || r.URL.Query().Get("start") == "" {
			t.Error("incorrect query forwarding")
		}
		fmt.Fprint(w, response)
	}))
	defer server.Close()
	ds := models.Datasource{Name: "logs", Type: "loki", URL: server.URL + "/tenant", Status: 1, AuthType: "bearer", AuthConfig: `{"token":"secret"}`, Config: `{"custom_headers":{"X-Scope-OrgID":"prod"}}`}
	if err := db.Create(&ds).Error; err != nil {
		t.Fatal(err)
	}
	if ok, msg := testConnectionForDatasource(ds); !ok {
		t.Fatal(msg)
	}
	userID := uint(1)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", userID) })
	router.POST("/logs", middleware.RequirePermission("observability:logs"), QueryLogs)
	router.GET("/sources", middleware.RequirePermission("observability:logs"), LogDatasources)
	payload := fmt.Sprintf(`{"datasource_id":%d,"query":"{app=\"a\"}","start":"2023-11-14T22:00:00Z","end":"2023-11-14T23:00:00Z","limit":2}`, ds.ID)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		return w
	}
	w := call("POST", "/logs", payload)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var history models.PromQueryHistory
	if err := db.Where("user_id = ?", userID).First(&history).Error; err != nil {
		t.Fatal(err)
	}
	if history.Query != `{app="a"}` || history.Mode != "logs" || history.DatasourceID != ds.ID {
		t.Fatalf("incorrect Loki query history: %+v", history)
	}
	var result struct {
		Data struct {
			Entries      []logEntry `json:"entries"`
			LimitReached bool       `json:"limit_reached"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Data.Entries) != 2 || result.Data.Entries[0].Timestamp != "1700000000000000002" || result.Data.Entries[1].Metadata["trace_id"] != "abc" || !result.Data.LimitReached {
		t.Fatalf("bad result: %s", w.Body.String())
	}
	w = call("GET", "/sources", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), server.URL) {
		t.Fatalf("unsafe projection: %s", w.Body.String())
	}
	before := upstreamCalls
	userID = 999
	if w := call("POST", "/logs", payload); w.Code != 403 {
		t.Fatalf("permission bypass: %s", w.Body.String())
	}
	userID = 1
	for _, bad := range []string{strings.Replace(payload, `"limit":2`, `"limit":5001`, 1), strings.Replace(payload, "2023-11-14T23:00:00Z", "2023-11-14T21:00:00Z", 1)} {
		if w := call("POST", "/logs", bad); w.Code != 400 {
			t.Fatal(w.Body.String())
		}
	}
	if err := db.Model(&ds).Update("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	if w := call("POST", "/logs", payload); w.Code != 400 {
		t.Fatal(w.Body.String())
	}
	if upstreamCalls != before {
		t.Fatal("invalid query reached upstream")
	}
	if err := db.Model(&ds).Updates(map[string]any{"status": 1, "type": "prometheus"}).Error; err != nil {
		t.Fatal(err)
	}
	if w := call("POST", "/logs", payload); w.Code != 400 {
		t.Fatal("accepted non-log source")
	}
	if err := db.Model(&ds).Update("type", "loki").Error; err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`<html>bad gateway</html>`, `{"status":"success","data":{"resultType":"matrix","result":[]}}`, `{"status":"success","data":{"resultType":"streams","result":[{"values":[["bad","line"]]}]}}`} {
		response = bad
		if w := call("POST", "/logs", payload); w.Code != 502 {
			t.Fatalf("accepted malformed result: %s", w.Body.String())
		}
	}
}
