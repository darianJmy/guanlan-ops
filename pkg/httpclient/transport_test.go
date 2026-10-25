package httpclient

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestTransportDecoratesWithoutMutatingCaller(t *testing.T) {
	for _, auth := range []Auth{{Type: "basic", Username: "user", Password: "pass"}, {Type: "bearer", Token: "token"}, {Type: "api_key", Token: "encoded-key"}} {
		req, _ := http.NewRequest("GET", "https://example.test", nil)
		req.Header.Set("X-Original", "keep")
		tr := Transport{Auth: auth, Headers: map[string]string{"X-Tenant": "tenant", "Authorization": "old"}, Base: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r == req || r.Header.Get("X-Tenant") != "tenant" || r.Header.Get("X-Original") != "keep" {
				t.Fatal("request was not correctly cloned and decorated")
			}
			if auth.Type == "basic" {
				u, p, ok := r.BasicAuth()
				if !ok || u != "user" || p != "pass" {
					t.Fatal("basic auth missing")
				}
			} else if auth.Type == "api_key" {
				if r.Header.Get("Authorization") != "ApiKey encoded-key" {
					t.Fatal("API key auth missing")
				}
			} else if r.Header.Get("Authorization") != "Bearer token" {
				t.Fatal("bearer auth missing")
			}
			r.Header.Set("X-Original", "changed")
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
		})}
		res, err := tr.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if req.Header.Get("Authorization") != "" || req.Header.Get("X-Tenant") != "" || req.Header.Get("X-Original") != "keep" {
			t.Fatal("caller headers mutated")
		}
	}
}
