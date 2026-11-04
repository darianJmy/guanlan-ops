package tracing_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
	"guanlan/pkg/httpclient"
	"guanlan/pkg/logger"
	"guanlan/pkg/middleware"
	"guanlan/pkg/tracing"
)

func TestOTLPRequestPropagationAndFlush(t *testing.T) {
	var mu sync.Mutex
	var spans []*tracepb.Span
	collectorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" {
			t.Errorf("unexpected export path %s", r.URL.Path)
		}
		data, _ := io.ReadAll(r.Body)
		var request collector.ExportTraceServiceRequest
		if err := proto.Unmarshal(data, &request); err != nil {
			t.Error(err)
		}
		mu.Lock()
		for _, resource := range request.ResourceSpans {
			for _, scope := range resource.ScopeSpans {
				spans = append(spans, scope.Spans...)
			}
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer collectorServer.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collectorServer.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "")
	t.Setenv("OTEL_EXPORTER_OTLP_COMPRESSION", "none")
	t.Setenv("OTEL_SDK_DISABLED", "false")
	t.Setenv("OTEL_TRACES_SAMPLER", "parentbased_always_on")
	shutdown, err := tracing.Init(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown(context.Background())
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	const traceID = "0123456789abcdef0123456789abcdef"
	const parentID = "0123456789abcdef"
	var propagated string
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		propagated = r.Header.Get("traceparent")
		w.WriteHeader(503)
	}))
	defer downstream.Close()
	client, cleanup := httpclient.New(httpclient.Config{Timeout: time.Second})
	defer cleanup()
	router := gin.New()
	router.Use(middleware.Access(), middleware.Recovery())
	router.GET("/items/:id", func(c *gin.Context) {
		logger.LogContext(c.Request.Context(), slog.LevelInfo, "business")
		req, _ := http.NewRequestWithContext(c.Request.Context(), "GET", downstream.URL+"/?token=secret-value", nil)
		res, err := client.Do(req)
		if err != nil {
			t.Error(err)
			c.Status(500)
			return
		}
		res.Body.Close()
		c.Status(res.StatusCode)
	})
	request := httptest.NewRequest("GET", "/items/123?token=secret-value", nil)
	request.Header.Set("traceparent", "00-"+traceID+"-"+parentID+"-01")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Header().Get("X-Trace-ID") != traceID {
		t.Fatal("incoming trace not continued")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(spans) != 2 {
		t.Fatalf("expected server/client spans, got %d", len(spans))
	}
	var serverSpan, clientSpan *tracepb.Span
	for _, span := range spans {
		if hex.EncodeToString(span.TraceId) != traceID {
			t.Fatal("trace ID changed")
		}
		if span.Kind == tracepb.Span_SPAN_KIND_SERVER {
			serverSpan = span
		} else {
			clientSpan = span
		}
		if span.Status.Code != tracepb.Status_STATUS_CODE_ERROR {
			t.Fatal("503 not recorded as error")
		}
		if strings.Contains(span.String(), "secret-value") {
			t.Fatal("query secret exported")
		}
	}
	if serverSpan == nil || clientSpan == nil {
		t.Fatal("missing span kind")
	}
	if serverSpan.Name != "GET /items/:id" || hex.EncodeToString(serverSpan.ParentSpanId) != parentID {
		t.Fatal("server parent/route mismatch")
	}
	if !bytes.Equal(clientSpan.ParentSpanId, serverSpan.SpanId) {
		t.Fatal("client not parented to server")
	}
	if propagated != "00-"+traceID+"-"+hex.EncodeToString(clientSpan.SpanId)+"-01" {
		t.Fatalf("wrong propagated header %s", propagated)
	}
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record["trace_id"] != traceID || record["span_id"] != hex.EncodeToString(serverSpan.SpanId) {
			t.Fatalf("log not correlated: %v", record)
		}
	}
}

func TestDisabledAndInvalidConfiguration(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	before := otel.GetTracerProvider()
	shutdown, err := tracing.Init(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if otel.GetTracerProvider() != before {
		t.Fatal("disabled tracing changed provider")
	}
	t.Setenv("OTEL_SDK_DISABLED", "false")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "not-a-url")
	if _, err := tracing.Init(context.Background()); err == nil {
		t.Fatal("invalid endpoint accepted")
	}
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "grpc")
	if _, err := tracing.Init(context.Background()); err == nil {
		t.Fatal("unsupported protocol accepted")
	}
}
