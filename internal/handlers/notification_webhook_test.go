package handlers

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCustomWebhookConfigurationAndDelivery(t *testing.T) {
	if notificationType("webhook") != "webhook" {
		t.Fatal("webhook type not preserved")
	}
	channel, err := prepareNotificationChannel(saveNotificationChannelReq{Type: "webhook", WebhookURL: "https://example.com/notify?token=test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	channel.Type = "webhook"
	if channel.WebhookURLCipher == "https://example.com/notify?token=test" {
		t.Fatal("URL stored in plaintext")
	}
	updated, err := prepareNotificationChannel(saveNotificationChannelReq{Type: "webhook"}, &channel)
	if err != nil || updated.WebhookURLCipher != channel.WebhookURLCipher {
		t.Fatal("blank edit must preserve URL", err)
	}
	if _, err := prepareNotificationChannel(saveNotificationChannelReq{Type: "webhook", WebhookURL: "ftp://example.com"}, nil); err == nil {
		t.Fatal("invalid URL accepted")
	}
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	body := `{"title":"告警","content":"模板正文","labels":{"app":"nginx"}}`
	for _, status := range []int{200, 204, 400, 500} {
		http.DefaultTransport = weComTestTransport(func(r *http.Request) (*http.Response, error) {
			received, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" || string(received) != body || r.URL.Query().Get("token") != "test" {
				t.Fatal("incorrect webhook request")
			}
			return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
		})
		err := sendWebhook(channel, []byte(body))
		if (err != nil) != (status >= 300) {
			t.Fatalf("status %d: %v", status, err)
		}
	}
}
