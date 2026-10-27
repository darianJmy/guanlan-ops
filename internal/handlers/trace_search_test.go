package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"guanlan/internal/database"
	"guanlan/internal/models"
)

func TestTraceSearchAndMetadata(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	previous := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previous; sqlDB, _ := db.DB(); sqlDB.Close() })
	if err := db.AutoMigrate(&models.Datasource{}); err != nil {
		t.Fatal(err)
	}
	ds := models.Datasource{ID: 1, Name: "trace", Type: "jaeger", Status: 1, URL: "https://jaeger.example/prefix"}
	if err := db.Create(&ds).Error; err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body := `{"data":[],"errors":null}`
		switch r.URL.Path {
		case "/prefix/api/services":
			body = `{"data":["api"]}`
		case "/prefix/api/services/api/operations":
			body = `{"data":["GET /orders"]}`
		case "/prefix/api/traces":
			q := r.URL.Query()
			if q.Get("service") != "api" || q.Get("operation") != "GET /orders" || q.Get("limit") != "20" || q.Get("minDuration") != "100ms" || q.Get("maxDuration") != "2s" || q.Get("start") != "1792886400000000" || q.Get("end") != "1792890000000000" || q.Get("tags") != `{"error":"true"}` {
				t.Fatalf("wrong params: %v", q)
			}
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer server.Close()
	db.Model(&ds).Update("url", server.URL+"/prefix")
	router := gin.New()
	router.POST("/traces", QueryTraces)
	router.GET("/metadata", TraceMetadata)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/metadata?datasource_id=1", "/metadata?datasource_id=1&service=api"} {
		if w := call("GET", path, ""); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	payload := map[string]any{"datasource_id": 1, "service": "api", "operation": "GET /orders", "start": "2026-10-25T00:00:00Z", "end": "2026-10-25T01:00:00Z", "min_duration": "100ms", "max_duration": "2s", "tags": map[string]string{"error": "true"}}
	send := func() *httptest.ResponseRecorder {
		data, _ := json.Marshal(payload)
		return call("POST", "/traces", string(data))
	}
	if w := send(); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	for _, tc := range []struct {
		key   string
		value any
	}{{"service", ""}, {"limit", 101}, {"min_duration", "oops"}, {"min_duration", "3s"}, {"trace_id", "../../secret"}, {"end", "2026-10-24T01:00:00Z"}} {
		original, exists := payload[tc.key]
		payload[tc.key] = tc.value
		if w := send(); w.Code != 400 {
			t.Fatalf("accepted %s=%v: %s", tc.key, tc.value, w.Body.String())
		}
		if exists {
			payload[tc.key] = original
		} else {
			delete(payload, tc.key)
		}
	}
	if calls != 3 {
		t.Fatalf("invalid requests reached upstream: %d", calls)
	}
}

func TestTraceResourceAttributesPreserved(t *testing.T) {
	raw := []byte(`{"traceID":"abc","spans":[{"spanID":"s1","processID":"p1","tags":[{"key":"http.response.status_code","type":"int64","value":200}]}],"processes":{"p1":{"serviceName":"api","tags":[{"key":"host.name","type":"string","value":"kube02"},{"key":"telemetry.sdk.name","type":"string","value":"opentelemetry"}]}}}`)
	var trace traceData
	if err := json.Unmarshal(raw, &trace); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.Processes["p1"].Tags) != 2 || !strings.Contains(string(encoded), `"host.name"`) || !strings.Contains(string(encoded), `"value":200`) {
		t.Fatalf("attributes lost: %s", encoded)
	}
}
