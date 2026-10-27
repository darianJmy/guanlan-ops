package handlers

import (
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestBuildProbeURL(t *testing.T) {
	got, err := buildProbeURL(ProbeReq{
		Host: "example.com", Port: 8443, HTTPS: true, Path: "health",
		Params: map[string]string{"message": "hello world"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com:8443/health?message=hello+world" {
		t.Fatalf("unexpected url: %s", got)
	}
}

func TestBuildProbeURLMergesParamsIntoFullTarget(t *testing.T) {
	got, err := buildProbeURL(ProbeReq{
		Target: "https://example.com/health?region=old&trace=1",
		Params: map[string]string{"region": "cn east", "verbose": "true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com/health?region=cn+east&trace=1&verbose=true" {
		t.Fatalf("unexpected url: %s", got)
	}
}

func TestExecuteDirectTCPProbe(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	result, _, err := executeTCPProbe(ProbeReq{Host: "127.0.0.1", Port: port, TimeoutMS: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if result.NodeType != "direct" || result.Port != port || !result.Connected {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestExecuteDirectSSLProbe(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer target.Close()

	address := strings.TrimPrefix(target.URL, "https://")
	host, rawPort, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(rawPort)
	result, _, err := executeSSLProbe(ProbeReq{Host: host, Port: port, ServerName: "localhost", TimeoutMS: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if result.Subject == "" || result.Issuer == "" || result.NotAfter.IsZero() {
		t.Fatalf("certificate metadata missing: %+v", result)
	}
	if result.NodeType != "direct" {
		t.Fatalf("unexpected node type: %s", result.NodeType)
	}
}

func TestProbeHostValidationRejectsShellInput(t *testing.T) {
	for _, host := range []string{"example.com;id", "example.com $(id)", "example.com/path"} {
		if err := validateProbeHost(host); err == nil {
			t.Fatalf("unsafe host accepted: %q", host)
		}
	}
	for _, host := range []string{"example.com", "127.0.0.1", "2001:db8::1"} {
		if err := validateProbeHost(host); err != nil {
			t.Fatalf("valid host rejected: %q: %v", host, err)
		}
	}
}

func TestNormalizeICMPCount(t *testing.T) {
	if got := normalizeICMPCount(0); got != 4 {
		t.Fatalf("default count = %d", got)
	}
	if got := normalizeICMPCount(99); got != 10 {
		t.Fatalf("maximum count = %d", got)
	}
}

func TestTLSProbeUsesHandshakeCertificate(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()

	address := strings.TrimPrefix(server.URL, "https://")
	host, rawPort, _ := net.SplitHostPort(address)
	port, _ := strconv.Atoi(rawPort)
	result, _, err := executeSSLProbe(ProbeReq{Host: host, Port: port, TimeoutMS: int(time.Second / time.Millisecond)})
	if err != nil || result.FingerprintSHA256 == "" {
		t.Fatalf("TLS handshake probe failed: result=%+v err=%v", result, err)
	}
}

func TestStatusCodeMatches(t *testing.T) {
	if !statusCodeMatches(204, "200-299,304") {
		t.Fatal("204 should match range")
	}
	if statusCodeMatches(500, "200-299,304") {
		t.Fatal("500 should not match")
	}
	if statusCodeExpressionValid("200-") {
		t.Fatal("invalid range accepted")
	}
}

func TestInstantProbeDirectHTTP(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer target.Close()

	gin.SetMode(gin.TestMode)
	body := `{"type":"http","target":` + strconvQuote(target.URL) + `,"method":"GET","success_codes":"201"}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/instant-probe", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	InstantProbe(ctx)

	var response struct {
		Code int `json:"code"`
		Data struct {
			Status     string `json:"status"`
			StatusCode int    `json:"status_code"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != 0 || response.Data.Status != "success" || response.Data.StatusCode != 201 {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}

func TestInstantProbeHTTPResponseKeyword(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":401,"message":"unauthorized"}`))
	}))
	defer target.Close()

	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name, keyword, wantStatus string
	}{
		{name: "matching keyword", keyword: `"status":401`, wantStatus: "success"},
		{name: "missing keyword", keyword: `"status":200`, wantStatus: "fail"},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload, _ := json.Marshal(ProbeReq{Type: "http", Target: target.URL, Method: "GET", SuccessCodes: "200", ResponseKeyword: test.keyword})
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/instant-probe", strings.NewReader(string(payload)))
			ctx.Request.Header.Set("Content-Type", "application/json")
			InstantProbe(ctx)

			var response struct {
				Data struct{ Status, Message string } `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Data.Status != test.wantStatus {
				t.Fatalf("status = %s, message = %s", response.Data.Status, response.Data.Message)
			}
			if test.wantStatus == "fail" && !strings.Contains(response.Data.Message, "未包含关键字") {
				t.Fatalf("unexpected message: %s", response.Data.Message)
			}
		})
	}
}

func strconvQuote(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}
