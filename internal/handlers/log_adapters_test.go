package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"guanlan/internal/config"
	"guanlan/internal/database"
	"guanlan/internal/middleware"
	"guanlan/internal/models"
)

func adapterConfig(kind string) string {
	cfg := logSourceConfig{Index: "logs-*,app-logs", Database: "observability", Table: "otel_logs", TimestampField: "Timestamp", MessageField: "Body", ServiceField: "ServiceName", LevelField: "SeverityText", TraceIDField: "TraceId"}
	if kind == "elasticsearch" {
		cfg.TimestampField = "@timestamp"
		cfg.MessageField = "event.message"
		cfg.ServiceField = "service.name"
		cfg.LevelField = "log.level"
		cfg.TraceIDField = "trace.id"
	}
	encoded, _ := json.Marshal(map[string]any{"database": cfg.Database, "log_query": cfg, "custom_headers": map[string]string{"X-Tenant": "prod"}})
	return string(encoded)
}

const esLogsResult = `{"timed_out":false,"_shards":{"failed":0},"hits":{"hits":[{"_source":{"@timestamp":"2026-10-24T01:00:00.123456789Z","event":{"message":"error: request failed"},"service":{"name":"api"},"log.level":"ERROR","trace":{"id":"trace-1"},"request_id":9007199254740993},"fields":{"@timestamp":["2026-10-24T01:00:00.123456789Z"]}}]}}`
const chLogsResult = `{"meta":[{"name":"__guanlan_timestamp","type":"String"},{"name":"Timestamp","type":"DateTime64(9, 'UTC')"},{"name":"Body","type":"String"}],"data":[{"__guanlan_timestamp":"1792803600123456789","Timestamp":"2026-10-24T01:00:00.123456789Z","Body":"error: request failed","ServiceName":"api","SeverityText":"ERROR","TraceId":"trace-1"}]}`

func TestLogAdapters(t *testing.T) {
	for _, kind := range []string{"elasticsearch", "clickhouse"} {
		t.Run(kind, func(t *testing.T) {
			previous := database.DB
			db, cleanup, err := database.New(config.Config{DBDSN: filepath.Join(t.TempDir(), "adapter.db")})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { cleanup(); database.DB = previous }()
			response := esLogsResult
			if kind == "clickhouse" {
				response = chLogsResult
			}
			status := http.StatusOK
			calls := 0
			keyword := "error' OR 1=1; DROP TABLE logs; --\n\\N\t中文"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				username, password, ok := r.BasicAuth()
				if !ok || username != "reader" || password != "secret" || r.Header.Get("X-Tenant") != "prod" {
					t.Error("lost auth or custom headers")
				}
				if r.Method != "POST" {
					t.Error("unexpected method")
				}
				body, _ := io.ReadAll(r.Body)
				if kind == "elasticsearch" {
					if r.URL.Path != "/proxy/logs-*,app-logs/_search" || r.URL.Query().Get("allow_partial_search_results") != "false" {
						t.Errorf("unexpected endpoint %s", r.URL)
					}
					var payload map[string]any
					if err := json.Unmarshal(body, &payload); err != nil {
						t.Fatal(err)
					}
					boolQuery := payload["query"].(map[string]any)["bool"].(map[string]any)
					if len(boolQuery["filter"].([]any)) != 1 || !strings.Contains(string(body), "strict_date_optional_time_nanos") || !strings.Contains(string(body), `"@timestamp"`) {
						t.Error("missing bounds or date mapping")
					}
				} else {
					if r.URL.Path != "/proxy/" || r.URL.Query().Get("readonly") != "1" || r.URL.Query().Get("max_execution_time") != "25" || r.URL.Query().Get("wait_end_of_query") != "1" {
						t.Errorf("missing ClickHouse bounds: %s", r.URL)
					}
					if !strings.Contains(string(body), "FROM `observability`.`otel_logs`") || !strings.Contains(string(body), "LIMIT {limit:UInt32}") || !strings.Contains(string(body), "{start:Int64}") || strings.Contains(string(body), "DROP TABLE") {
						t.Errorf("unsafe SQL: %s", body)
					}
					bound, err := base64.StdEncoding.DecodeString(r.URL.Query().Get("param_keyword"))
					if err != nil {
						t.Fatal(err)
					}
					if string(bound) != "*" && string(bound) != keyword {
						t.Errorf("keyword changed: %q", bound)
					}
				}
				w.WriteHeader(status)
				fmt.Fprint(w, response)
			}))
			defer server.Close()
			ds, err := prepareDatasourceForCreate(CreateDatasourceWithTunnelReq{Name: kind, Type: kind, URL: server.URL + "/proxy", Status: 1, AuthType: "basic", AuthConfig: `{"username":"reader","password":"secret"}`, Config: adapterConfig(kind)})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(ds.Capabilities, "logs") || isMetricDatasourceType(kind) {
				t.Fatal("incorrect capabilities")
			}
			if err := db.Create(&ds).Error; err != nil {
				t.Fatal(err)
			}
			if ok, message := testConnectionForDatasource(ds); !ok {
				t.Fatal(message)
			}
			userID := uint(1)
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set("user_id", userID) })
			router.POST("/logs", middleware.RequirePermission("observability:logs"), QueryLogs)
			router.GET("/sources", middleware.RequirePermission("observability:logs"), LogDatasources)
			request := LogQueryRequest{DatasourceID: ds.ID, Query: keyword, Start: time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 24, 2, 0, 0, 0, time.UTC), Limit: 100}
			call := func(method, path string) *httptest.ResponseRecorder {
				t.Helper()
				encoded, _ := json.Marshal(request)
				r := httptest.NewRequest(method, path, strings.NewReader(string(encoded)))
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				return w
			}
			w := call("POST", "/logs")
			if w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			var result struct {
				Data struct {
					Entries []logEntry `json:"entries"`
				} `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Data.Entries) != 1 {
				t.Fatal(w.Body.String())
			}
			entry := result.Data.Entries[0]
			if entry.Line != "error: request failed" || entry.Labels["service"] != "api" || entry.Labels["trace_id"] != "trace-1" || entry.Labels["level"] != "ERROR" || entry.Timestamp != "1792803600123456789" {
				t.Fatalf("incorrect mapped entry: %+v", entry)
			}
			if kind == "elasticsearch" && !strings.Contains(w.Body.String(), "9007199254740993") {
				t.Fatal("lost JSON integer precision")
			}
			w = call("GET", "/sources")
			if w.Code != 200 || !strings.Contains(w.Body.String(), kind) || strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), "log_query") || strings.Contains(w.Body.String(), server.URL) {
				t.Fatal("invalid source projection: " + w.Body.String())
			}
			before := calls
			userID = 999
			if w := call("POST", "/logs"); w.Code != 403 {
				t.Fatal("permission bypass")
			}
			userID = 1
			db.Model(&ds).Update("status", 0)
			if w := call("POST", "/logs"); w.Code != 400 {
				t.Fatal("disabled source accepted")
			}
			if calls != before {
				t.Fatal("unauthorized query reached server")
			}
			db.Model(&ds).Update("status", 1)
			for _, bad := range []string{"<html>not json</html>", "{}", response + "\nCode: 241. DB::Exception: failed"} {
				response = bad
				if w := call("POST", "/logs"); w.Code != 502 {
					t.Fatal("malformed response accepted: " + w.Body.String())
				}
			}
			if kind == "elasticsearch" {
				for _, bad := range []string{strings.Replace(esLogsResult, `"timed_out":false`, `"timed_out":true`, 1), strings.Replace(esLogsResult, `"failed":0`, `"failed":1`, 1), strings.Replace(esLogsResult, `"fields":{"@timestamp":["2026-10-24T01:00:00.123456789Z"]}`, `"fields":{}`, 1)} {
					response = bad
					if w := call("POST", "/logs"); w.Code != 502 {
						t.Fatal("partial/invalid ES result accepted")
					}
				}
				response = `{"hits":{"hits":[]},"_shards":{"failed":0},"timed_out":false}`
			} else {
				response = strings.Replace(chLogsResult, "DateTime64(9, 'UTC')", "UInt64", 1)
				if w := call("POST", "/logs"); w.Code != 502 {
					t.Fatal("numeric timestamp type accepted")
				}
				response = `{"meta":[{"name":"Timestamp","type":"DateTime"}],"data":[]}`
			}
			if w := call("POST", "/logs"); w.Code != 200 || !strings.Contains(w.Body.String(), `"entries":[]`) {
				t.Fatal("empty response mishandled: " + w.Body.String())
			}
			status = 403
			response = "credentials=secret"
			w = call("POST", "/logs")
			if w.Code != 502 || strings.Contains(w.Body.String(), "secret") {
				t.Fatal("upstream error leaked")
			}
		})
	}
}

func TestLogMappingValidation(t *testing.T) {
	for _, kind := range []string{"elasticsearch", "clickhouse"} {
		ds := models.Datasource{Type: kind, URL: "http://logs.test", Config: adapterConfig(kind)}
		if err := validateDatasourceConnection(ds); err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{"", `{"log_query":{}}`, `{"log_query":"invalid"}`, strings.Replace(ds.Config, `"timestamp_field":"`, `"timestamp_field":"bad;`, 1)} {
			copy := ds
			copy.Config = bad
			if kind == "clickhouse" && bad == "" {
				continue
			}
			if err := validateDatasourceConnection(copy); err == nil {
				t.Fatalf("accepted invalid config: %s", bad)
			}
		}
	}
	ds := models.Datasource{Type: "elasticsearch", Config: strings.Replace(adapterConfig("elasticsearch"), "logs-*,app-logs", "../_security", 1)}
	if _, err := readLogSourceConfig(ds); err == nil {
		t.Fatal("index traversal accepted")
	}
	ds = models.Datasource{Type: "clickhouse", Config: strings.Replace(adapterConfig("clickhouse"), "otel_logs", "logs` WHERE 1=1 --", 1)}
	if _, err := readLogSourceConfig(ds); err == nil {
		t.Fatal("identifier injection accepted")
	}
}

func TestLogAdapterRedirectAndCancellation(t *testing.T) {
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = true }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer source.Close()
	ds := models.Datasource{Type: "elasticsearch", URL: source.URL, Config: adapterConfig("elasticsearch"), AuthType: "bearer", AuthConfig: `{"token":"secret"}`}
	if _, err := doLogRequest(context.Background(), ds, "POST", source.URL, nil, "application/json"); err == nil || redirected {
		t.Fatal("redirect followed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := doLogRequest(ctx, ds, "POST", source.URL, nil, "application/json"); err == nil {
		t.Fatal("cancellation ignored")
	}
}
