package handlers

import (
	"encoding/json"
	"guanlan/internal/models"
	"guanlan/pkg/httpclient"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestNormalizeDatasourceLabels(t *testing.T) {
	raw, err := normalizeDatasourceLabels(`{"env":" production ","service":"gateway"}`)
	if err != nil {
		t.Fatalf("normalize labels: %v", err)
	}
	var labels map[string]string
	if err := json.Unmarshal([]byte(raw), &labels); err != nil {
		t.Fatalf("decode labels: %v", err)
	}
	if labels["env"] != "production" || labels["service"] != "gateway" {
		t.Fatalf("unexpected labels: %#v", labels)
	}
}

func TestDatasourceLabelsOnlyUseExplicitLabels(t *testing.T) {
	labels := datasourceLabels(models.Datasource{Labels: `{"service":"gateway"}`, Region: "cn", Env: "prod"})
	if len(labels) != 1 || labels["service"] != "gateway" || labels["region"] != "" || labels["env"] != "" {
		t.Fatalf("unexpected labels: %#v", labels)
	}
}

func TestDatasourceLabelSearchPattern(t *testing.T) {
	tests := map[string]string{
		"env:prod":    `%"env":"prod"%`,
		"owner = ops": `%"owner":"ops"%`,
		"gateway":     `%gateway%`,
	}
	for input, want := range tests {
		if got := datasourceLabelSearchPattern(input); got != want {
			t.Fatalf("datasourceLabelSearchPattern(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDatasourceRequestTransportAddsAuthAndCustomHeaders(t *testing.T) {
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		username, password, ok := req.BasicAuth()
		if !ok || username != "ops" || password != "secret" {
			t.Fatalf("unexpected basic auth: %q %q %v", username, password, ok)
		}
		if got := req.Header.Get("X-API-Key"); got != "token-123" {
			t.Fatalf("unexpected custom header: %q", got)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	})
	ds := models.Datasource{
		AuthType: "basic", AuthConfig: `{"username":"ops","password":"secret"}`,
		Config: `{"custom_headers":{"X-API-Key":"token-123"}}`,
	}
	cfg := datasourceHTTPConfig(ds)
	client := &http.Client{Transport: httpclient.Transport{Base: base, Auth: cfg.Auth, Headers: cfg.Headers}}
	resp, err := client.Get("http://example.test")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()
}
