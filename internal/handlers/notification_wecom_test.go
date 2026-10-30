package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"guanlan/internal/models"
)

type weComTestTransport func(*http.Request) (*http.Response, error)

func (f weComTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestWeComWebhook(t *testing.T) {
	encryptedURL, err := encryptSecret("https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test")
	if err != nil {
		t.Fatal(err)
	}
	channel := models.NotificationChannel{Type: "qywechat", WebhookURLCipher: encryptedURL}
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	for _, tc := range []struct {
		name, response string
		status         int
		wantError      bool
	}{
		{"success", `{"errcode":0,"errmsg":"ok"}`, 200, false},
		{"API rejection", `{"errcode":93000,"errmsg":"invalid bot"}`, 200, true},
		{"missing code", `{}`, 200, true},
		{"invalid JSON", `not JSON`, 200, true},
		{"HTTP rejection", `unavailable`, 503, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			http.DefaultTransport = weComTestTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodPost || r.URL.Query().Get("key") != "test" || r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("unexpected request")
				}
				var payload struct {
					MsgType  string `json:"msgtype"`
					Markdown struct {
						Content string `json:"content"`
					} `json:"markdown"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if payload.MsgType != "markdown" || payload.Markdown.Content != "### 🔴 <font color=\"warning\">P0 · 告警</font>\n\n正文" {
					t.Errorf("unexpected payload: %+v", payload)
				}
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.response))}, nil
			})
			err := sendWebhook(channel, []byte(`{"title":"告警","content":"正文","status":"firing","severity":"P0"}`))
			if (err != nil) != tc.wantError {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
	http.DefaultTransport = weComTestTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid content must not be sent")
		return nil, nil
	})
	if err := sendWebhook(channel, []byte(`invalid`)); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	body, _ := json.Marshal(map[string]string{"content": strings.Repeat("中", 1366)})
	if err := sendWebhook(channel, body); err == nil || !strings.Contains(err.Error(), "4096") {
		t.Fatalf("oversized UTF-8 content accepted: %v", err)
	}
}
