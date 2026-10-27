package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"guanlan/internal/config"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestVictoriaMetricsQueries(t *testing.T) {
	previous := database.DB
	db, cleanup, err := database.New(config.Config{DBDSN: filepath.Join(t.TempDir(), "vm.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { cleanup(); database.DB = previous }()
	for _, prefix := range []string{"", "/select/42/prometheus"} {
		t.Run("endpoint"+prefix, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				username, password, ok := r.BasicAuth()
				if !ok || username != "reader" || password != "secret" || r.Header.Get("X-Tenant") != "42" {
					t.Error("authentication or custom header missing")
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case prefix + "/api/v1/query":
					if r.URL.Query().Get("query") != "vector(1)" {
						t.Error("query was not preserved")
					}
					fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"1"]}]}}`)
				case prefix + "/api/v1/query_range":
					if r.URL.Query().Get("step") != "60" {
						t.Error("range parameters missing")
					}
					fmt.Fprint(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[1700000000,"1"]]}]}}`)
				case prefix + "/api/v1/label/__name__/values":
					fmt.Fprint(w, `{"status":"success","data":["node_cpu_seconds_total"]}`)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			ds := models.Datasource{Name: "vm" + prefix, Type: "victoriametrics", URL: server.URL + prefix, Status: 1, AuthType: "basic", AuthConfig: `{"username":"reader","password":"secret"}`, Config: `{"custom_headers":{"X-Tenant":"42"}}`}
			if err := db.Create(&ds).Error; err != nil {
				t.Fatal(err)
			}
			if ok, message := testConnectionForDatasource(ds); !ok {
				t.Fatal(message)
			}
			router := newVMQueryTestRouter()
			call := func(method, path, payload string) map[string]interface{} {
				t.Helper()
				recorder := httptest.NewRecorder()
				request := httptest.NewRequest(method, path, strings.NewReader(payload))
				request.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(recorder, request)
				var body map[string]interface{}
				if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				return body
			}
			for _, query := range []struct{ method, path, payload string }{
				{http.MethodPost, "/query", fmt.Sprintf(`{"datasource_id":%d,"query":"vector(1)"}`, ds.ID)},
				{http.MethodPost, "/range", fmt.Sprintf(`{"datasource_id":%d,"query":"vector(1)","start":"1700000000","end":"1700000060","step":"60"}`, ds.ID)},
				{http.MethodGet, fmt.Sprintf("/metadata?datasource_id=%d&target=metrics", ds.ID), ""},
			} {
				if body := call(query.method, query.path, query.payload); body["code"] != float64(0) {
					t.Fatalf("query failed: %+v", body)
				}
			}
			series, err := queryPromRuleDatasource(ds, "vector(1)")
			if err != nil || len(series) != 1 {
				t.Fatalf("rule query: %v, %+v", err, series)
			}
			if requests != 5 {
				t.Fatalf("expected five requests, got %d", requests)
			}
			db.Model(&ds).Update("status", 0)
			body := call(http.MethodPost, "/query", fmt.Sprintf(`{"datasource_id":%d,"query":"vector(1)"}`, ds.ID))
			if body["code"] == float64(0) || requests != 5 {
				t.Fatal("disabled instance queried")
			}
		})
	}
}

func TestVictoriaMetricsRejectsInvalidResponses(t *testing.T) {
	for _, content := range []string{
		`<html>login</html>`, `{"status":"success"}`, `{"status":"success","data":{"version":"1"}}`,
		`{"status":"error","data":{"resultType":"vector","result":[{"value":[1,"1"]}]}}`,
		`{"status":"success","data":{"resultType":"vector","result":[]}}`,
		`{"status":"success","data":{"resultType":"vector","result":[{"value":[1,"2"]}]}}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, content) }))
		ok, _ := testConnectionForDatasource(models.Datasource{Type: "victoriametrics", URL: server.URL})
		server.Close()
		if ok {
			t.Fatalf("invalid endpoint accepted: %s", content)
		}
	}
}

func newVMQueryTestRouter() *gin.Engine {
	router := gin.New()
	router.POST("/query", PromInstantQuery)
	router.POST("/range", PromRangeQuery)
	router.GET("/metadata", PromMetadata)
	return router
}
