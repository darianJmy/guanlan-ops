package handlers

import (
	"encoding/json"
	"fmt"
	"guanlan/internal/config"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSupportedMonitoringAPI(t *testing.T) {
	for _, kind := range []string{"prometheus", "victoriametrics"} {
		t.Run(kind, func(t *testing.T) { testSupportedMonitoringAPI(t, kind) })
	}
}

func testSupportedMonitoringAPI(t *testing.T, kind string) {
	previous := database.DB
	db, cleanup, err := database.New(config.Config{DBDSN: filepath.Join(t.TempDir(), "monitoring.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { cleanup(); database.DB = previous }()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		expectedPath := "/api/v1/status/buildinfo"
		if kind == "victoriametrics" {
			expectedPath = "/select/42/prometheus/api/v1/query"
		}
		if r.URL.Path != expectedPath || r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("X-Scope-OrgID") != "prod" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if kind == "victoriametrics" {
			if r.URL.Query().Get("query") != "vector(1)" {
				t.Error("invalid test query")
			}
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"1"]}]}}`))
		} else {
			_, _ = w.Write([]byte(`{"status":"success","data":{"version":"3.0.0"}}`))
		}
	}))
	defer server.Close()
	router := gin.New()
	router.GET("/sources", ListDatasourcesWithTunnel)
	router.POST("/sources", CreateDatasourceWithTunnel)
	router.PUT("/sources/batch/status", BatchUpdateDatasourceStatus)
	router.DELETE("/sources/batch", BatchDeleteDatasources)
	router.PUT("/sources/:id", UpdateDatasourceWithTunnel)
	router.DELETE("/sources/:id", DeleteDatasource)
	router.GET("/sources/:id", GetDatasourceAuth)
	router.POST("/sources/test", TestDatasourceConnection)
	router.POST("/sources/:id/test", TestSavedDatasourceConnection)
	call := func(method, path string, payload interface{}) (int, map[string]interface{}) {
		encoded, _ := json.Marshal(payload)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, path, strings.NewReader(string(encoded)))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		var body map[string]interface{}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("response: %s", recorder.Body.String())
		}
		return recorder.Code, body
	}
	endpoint := server.URL
	if kind == "victoriametrics" {
		endpoint += "/select/42/prometheus"
	}
	valid := map[string]interface{}{"name": "prod", "type": kind, "url": endpoint, "auth_type": "bearer", "auth_config": `{"token":"secret"}`, "config": `{"custom_headers":{"X-Scope-OrgID":"prod"}}`, "labels": `{"env":"prod"}`, "status": 1}
	for _, kind := range []string{"mysql", "postgresql", "redis", "k8s", "aliyun_ecs", "sls", "cls", "nacos", "kafka", "http", "unknown"} {
		valid["type"] = kind
		_, body := call(http.MethodPost, "/sources", valid)
		if body["code"] == float64(0) {
			t.Fatalf("created retired type %s", kind)
		}
		_, body = call(http.MethodPost, "/sources/test", valid)
		if body["code"] == float64(0) {
			t.Fatalf("tested retired type %s", kind)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("retired type attempted a network request")
	}
	valid["type"] = kind
	_, body := call(http.MethodPost, "/sources", valid)
	if body["code"] != float64(0) {
		t.Fatalf("create Prometheus: %+v", body)
	}
	var prom models.Datasource
	db.Where("name = ?", "prod").First(&prom)
	if prom.Type != kind || prom.Category != "observability" || prom.Capabilities != "metrics" {
		t.Fatalf("unexpected stored instance: %+v", prom)
	}
	valid["name"] = "disabled"
	valid["status"] = 0
	_, body = call(http.MethodPost, "/sources", valid)
	if body["code"] != float64(0) {
		t.Fatalf("create disabled: %+v", body)
	}
	var disabled models.Datasource
	db.Where("name = ?", "disabled").First(&disabled)
	if disabled.Status != 0 {
		t.Fatal("disabled instance was enabled by default")
	}
	old := models.Datasource{Name: "legacy mysql", Type: "mysql", URL: server.URL, Status: 1}
	db.Create(&old)
	_, body = call(http.MethodGet, "/sources", nil)
	data := body["data"].(map[string]interface{})
	if data["total"] != float64(2) {
		t.Fatalf("retired datasource leaked into list: %+v", body)
	}
	for _, entry := range data["list"].([]interface{}) {
		if entry.(map[string]interface{})["type"] != kind {
			t.Fatal("retired datasource listed")
		}
	}
	valid["name"] = "prod updated"
	valid["status"] = 1
	valid["auth_config"] = ""
	valid["category"] = "cloud"
	valid["capabilities"] = "logs,k8s"
	_, body = call(http.MethodPut, fmt.Sprintf("/sources/%d", prom.ID), valid)
	if body["code"] != float64(0) {
		t.Fatalf("update with saved auth: %+v", body)
	}
	db.First(&prom, prom.ID)
	if prom.AuthConfig != `{"token":"secret"}` || prom.Capabilities != "metrics" || prom.Category != "observability" {
		t.Fatalf("auth or policy lost: %+v", prom)
	}
	_, body = call(http.MethodPost, fmt.Sprintf("/sources/%d/test", prom.ID), valid)
	if body["code"] != float64(0) || requests.Load() != 1 {
		t.Fatalf("saved authentication/headers not used: %+v", body)
	}
	valid["type"] = "mysql"
	_, body = call(http.MethodPut, fmt.Sprintf("/sources/%d", prom.ID), valid)
	if body["code"] == float64(0) {
		t.Fatal("changed Prometheus to retired type")
	}
	valid["type"] = kind
	_, body = call(http.MethodPut, fmt.Sprintf("/sources/%d", old.ID), valid)
	if body["code"] == float64(0) {
		t.Fatal("rewrote historical datasource")
	}
	_, body = call(http.MethodGet, fmt.Sprintf("/sources/%d", old.ID), nil)
	if body["code"] == float64(0) {
		t.Fatal("historical datasource auth exposed")
	}
	_, body = call(http.MethodPost, fmt.Sprintf("/sources/%d/test", old.ID), valid)
	if body["code"] == float64(0) || requests.Load() != 1 {
		t.Fatal("historical datasource executed")
	}
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		path := "/sources/batch"
		if method == http.MethodPut {
			path += "/status"
		}
		_, body = call(method, path, map[string]interface{}{"ids": []uint{prom.ID, old.ID}, "status": 0})
		if body["code"] == float64(0) {
			t.Fatalf("mixed retired batch accepted: %s", method)
		}
	}
	db.First(&prom, prom.ID)
	if prom.Status != 1 {
		t.Fatal("mixed batch partially modified Prometheus")
	}
	catalog := collectDatasourceCatalog(0)
	if strings.Contains(catalog, "legacy mysql") || strings.Contains(catalog, "disabled") || !strings.Contains(catalog, "prod updated") || strings.Contains(catalog, "secret") {
		t.Fatalf("invalid AI catalog: %s", catalog)
	}
	if loaded, err := loadDatasourceByToolParam(map[string]interface{}{"datasource_id": float64(prom.ID)}, "prometheus"); err != nil || loaded.ID != prom.ID {
		t.Fatalf("metrics tool cannot load %s: %v", kind, err)
	}
	if _, _, _, err := normalizeDatasourceGroupInput(saveDatasourceGroupReq{Name: "mixed", DatasourceIDs: []uint{prom.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := datasourceIDsForSelection(db, nil, []uint{prom.ID}); err != nil {
		t.Fatal(err)
	}
	registry := buildOpsToolRegistry(0)
	if len(registry.Specs()) != 1 || registry.Specs()[0].DatasourceType != "prometheus" {
		t.Fatalf("retired AI tools remain: %+v", registry.Specs())
	}
	if _, err := loadDatasourceByToolParam(map[string]interface{}{"datasource_id": float64(old.ID)}, "mysql"); err == nil {
		t.Fatal("legacy tool datasource loaded")
	}
	group := models.DatasourceGroup{Name: "production"}
	db.Create(&group)
	db.Create(&models.DatasourceGroupMember{GroupID: group.ID, DatasourceID: disabled.ID})
	_, body = call(http.MethodDelete, fmt.Sprintf("/sources/%d", disabled.ID), nil)
	if body["code"] != float64(0) {
		t.Fatalf("delete grouped instance: %+v", body)
	}
	var remaining int64
	db.Model(&models.DatasourceGroupMember{}).Where("datasource_id = ?", disabled.ID).Count(&remaining)
	if remaining != 0 {
		t.Fatal("left orphan group membership")
	}
	db.Where("group_id = ?", group.ID).Delete(&models.DatasourceGroupMember{})
	db.Create(&models.AlertRuleDatasource{RuleID: 77, DatasourceID: prom.ID})
	_, body = call(http.MethodDelete, fmt.Sprintf("/sources/%d", prom.ID), nil)
	if body["code"] == float64(0) {
		t.Fatal("deleted alert rule dependency")
	}
	// Switching to no authentication removes the previous credentials.
	valid["auth_type"] = "none"
	_, body = call(http.MethodPut, fmt.Sprintf("/sources/%d", prom.ID), valid)
	if body["code"] != float64(0) {
		t.Fatalf("clear auth: %+v", body)
	}
	db.First(&prom, prom.ID)
	if prom.AuthConfig != "" {
		t.Fatal("old token was not cleared")
	}
}

func TestPrometheusConnectionRejectsNonPrometheusResponses(t *testing.T) {
	for _, content := range []string{"<html>login</html>", `{"status":"success"}`, `{"status":"error","data":{"version":"1"}}`, `{"status":"success","data":{"version":123}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(content)) }))
		ok, _ := testConnectionForDatasource(models.Datasource{Type: "prometheus", URL: server.URL, AuthType: "none"})
		server.Close()
		if ok {
			t.Fatalf("non-Prometheus endpoint accepted: %s", content)
		}
	}
	for _, auth := range []string{"kubeconfig", "ak_sk", "password"} {
		if err := validateDatasourceConnection(models.Datasource{Type: "prometheus", URL: "http://example.test", AuthType: auth}); err == nil {
			t.Fatalf("retired auth %s accepted", auth)
		}
	}
}
