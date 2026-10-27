package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"guanlan/internal/config"
	"guanlan/internal/database"
	"guanlan/internal/middleware"
	"guanlan/internal/models"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestExploreSQLValidation(t *testing.T) {
	for _, sql := range []string{"SELECT 1", "/* comment */ WITH 1 AS n SELECT n", "SELECT 'DROP; SETTINGS' AS text", "SELECT `format` FROM logs", "SELECT 1 -- FORMAT JSON"} {
		if err := validateExploreSQL(sql); err != nil {
			t.Fatalf("valid query %q: %v", sql, err)
		}
	}
	for _, sql := range []string{"", "DROP TABLE logs", "SELECT 1; DROP TABLE logs", "SELECT 1 SETTINGS readonly=0", "SELECT 1 FORMAT JSON", "SELECT 1 INTO OUTFILE '/tmp/x'", "WITH 1 AS n INSERT INTO logs SELECT n", "SELECT 'unterminated", "SELECT 1 /* unterminated", "SELECT 1 /*comment*/ SETTINGS readonly=0"} {
		if err := validateExploreSQL(sql); err == nil {
			t.Fatalf("accepted unsafe query %q", sql)
		}
	}
}
func TestExploreQueriesAndPermissions(t *testing.T) {
	previous := database.DB
	db, cleanup, err := database.New(config.Config{DBDSN: filepath.Join(t.TempDir(), "explore.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { cleanup(); database.DB = previous }()
	calls := 0
	result := `{"meta":[{"name":"value","type":"UInt8"}],"data":[{"value":1}],"rows":1}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("authentication missing")
		}
		if strings.Contains(r.URL.Path, "/api/traces/") {
			fmt.Fprint(w, `{"data":[{"traceID":"0123456789abcdef","spans":[{"traceID":"0123456789abcdef","spanID":"123","operationName":"GET","startTime":1700000000000000,"duration":2000,"processID":"p1"}],"processes":{"p1":{"serviceName":"api"}}}],"errors":null}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || !strings.HasPrefix(string(body), "SELECT * FROM (\n") || !strings.Contains(string(body), "LIMIT 500 FORMAT JSON") || r.URL.Query().Get("database") != "logs" || r.URL.Query().Get("readonly") != "1" || r.URL.Query().Get("param_from") != "1792800000" || r.URL.Query().Get("param_to") != "1792803600" {
			t.Errorf("incorrect SQL envelope: %s %s", r.URL, body)
		}
		fmt.Fprint(w, result)
	}))
	defer server.Close()
	ch := models.Datasource{Name: "sql", Type: "clickhouse", Config: `{"database":"logs"}`, URL: server.URL, Status: 1, AuthType: "bearer", AuthConfig: `{"token":"secret"}`}
	if err := validateDatasourceConnection(ch); err != nil {
		t.Fatal("SQL-only connection rejected:", err)
	}
	db.Create(&ch)
	trace := models.Datasource{Name: "trace", Type: "jaeger", URL: server.URL, Status: 1, AuthType: ch.AuthType, AuthConfig: ch.AuthConfig}
	db.Create(&trace)
	userID := uint(1)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", userID) })
	router.GET("/sources", ExploreDatasources)
	router.POST("/sql", middleware.RequirePermission("query:sql"), QuerySQL)
	router.POST("/traces", middleware.RequirePermission("observability:traces"), QueryTraces)
	call := func(method, path string, payload any) *httptest.ResponseRecorder {
		t.Helper()
		body, _ := json.Marshal(payload)
		r := httptest.NewRequest(method, path, strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	sql := map[string]any{"datasource_id": ch.ID, "query": "SELECT 1 AS value", "start": "2026-10-24T00:00:00Z", "end": "2026-10-24T01:00:00Z"}
	w := call("POST", "/sql", sql)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"value":1`) {
		t.Fatal(w.Body.String())
	}
	w = call("GET", "/sources", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"modes":["sql"]`) || !strings.Contains(w.Body.String(), `"modes":["traces"]`) || strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), server.URL) {
		t.Fatal("bad source projection:", w.Body.String())
	}
	w = call("POST", "/traces", map[string]any{"datasource_id": trace.ID, "trace_id": "0123456789abcdef"})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"duration":2000`) {
		t.Fatal(w.Body.String())
	}
	before := calls
	userID = 999
	if w := call("GET", "/sources", nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Fatal("unauthorized sources exposed")
	}
	if w := call("POST", "/sql", sql); w.Code != 403 {
		t.Fatal("SQL permission bypass")
	}
	if w := call("POST", "/traces", map[string]any{"datasource_id": trace.ID, "trace_id": "0123456789abcdef"}); w.Code != 403 {
		t.Fatal("trace permission bypass")
	}
	userID = 1
	sql["query"] = "SELECT 1 SETTINGS readonly=0"
	if w := call("POST", "/sql", sql); w.Code != 400 {
		t.Fatal("accepted SQL settings")
	}
	sql["query"] = "SELECT 1"
	if w := call("POST", "/traces", map[string]any{"datasource_id": trace.ID, "trace_id": "../../secrets"}); w.Code != 400 {
		t.Fatal("trace traversal accepted")
	}
	db.Model(&ch).Update("status", 0)
	if w := call("POST", "/sql", sql); w.Code != 400 {
		t.Fatal("disabled source accepted")
	}
	if calls != before {
		t.Fatal("invalid query reached upstream")
	}
	db.Model(&ch).Update("status", 1)
	for _, bad := range []string{"{}", result + "\nCode: 1. Failed", `{"meta":[],"data":[],"exception":"failed"}`} {
		result = bad
		if w := call("POST", "/sql", sql); w.Code != 502 {
			t.Fatal("invalid result accepted")
		}
	}
	// Log access must not imply arbitrary SQL access.
	db.Exec("DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE code='query:sql')")
	if w := call("POST", "/sql", sql); w.Code != 403 {
		t.Fatal("log permission grants SQL")
	}
	w = call("GET", "/sources", nil)
	if strings.Contains(w.Body.String(), `"type":"clickhouse"`) {
		t.Fatal("SQL-only source visible without SQL permission")
	}
}

func TestClickHouseDatabaseRequired(t *testing.T) {
	req := CreateDatasourceWithTunnelReq{Name: "ch", Type: "clickhouse", URL: "http://localhost:8123", AuthType: "none", Status: 1, Config: `{}`}
	if normalizeDatasourceReq(&req, nil) == nil {
		t.Fatal("missing database accepted")
	}
	req.Config = `{"database":"logs"}`
	if err := normalizeDatasourceReq(&req, nil); err != nil {
		t.Fatal(err)
	}
	name, err := clickHouseDatabase(req.Config)
	if err != nil || name != "logs" {
		t.Fatalf("database: %s %v", name, err)
	}
}
