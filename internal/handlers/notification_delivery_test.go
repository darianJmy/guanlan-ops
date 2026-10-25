package handlers

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"guanlan/pkg/mailclient"
	"io"
	"net"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"testing"
	"time"

	"guanlan/internal/conf"
	"guanlan/internal/config"
	"guanlan/internal/models"
)

func TestSMTPConfigAndCredentials(t *testing.T) {
	previous := config.AppConfig.SMTP
	t.Cleanup(func() { config.AppConfig.SMTP = previous })
	config.AppConfig.SMTP = conf.SMTPConfig{}
	req := saveNotificationChannelReq{Type: "email", EmailRecipients: []string{"ops@example.com"}}
	channel, err := prepareNotificationChannel(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sharedSMTPConfig(channel); err == nil {
		t.Fatal("missing global SMTP accepted")
	}
	config.AppConfig.SMTP = conf.SMTPConfig{Host: "smtp.example.com", Port: 465, Security: "tls", Username: "user", Password: "secret", From: "alerts@example.com"}
	stored, err := sharedSMTPConfig(channel)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Password != "secret" || stored.Host != "smtp.example.com" || stored.To[0] != "ops@example.com" {
		t.Fatal("global sender or channel recipient not applied")
	}
	if channel.WebhookURLCipher != "" || channel.SignSecretCipher != "" || channel.EmailRecipientsJSON != `["ops@example.com"]` {
		t.Fatal("channel must only store recipients")
	}
	public, _ := json.Marshal(notificationChannelResponse(channel))
	if strings.Contains(string(public), `"secret"`) || strings.Contains(string(public), "smtp.example.com") {
		t.Fatal("SMTP configuration exposed in channel response")
	}
	config.AppConfig.SMTP.From = "new@example.com"
	updated, err := sharedSMTPConfig(channel)
	if err != nil || updated.From != "<new@example.com>" {
		t.Fatalf("channel ignored global sender change: %+v %v", updated, err)
	}
	for _, recipients := range [][]string{nil, {"invalid"}, {"ops@example.com\r\nBcc: other@example.com"}} {
		if _, err := prepareNotificationChannel(saveNotificationChannelReq{Type: "email", EmailRecipients: recipients}, nil); err == nil {
			t.Fatal("invalid recipient accepted")
		}
	}
	for _, tc := range []struct {
		name   string
		change func(*mailclient.Config)
	}{
		{"port", func(c *mailclient.Config) { c.Port = 0 }},
		{"security", func(c *mailclient.Config) { c.Security = "unknown" }},
		{"from injection", func(c *mailclient.Config) { c.From = "a@example.com\r\nBcc: b@example.com" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := stored
			tc.change(&c)
			if err := mailclient.ValidateConfig(&c); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}

func TestSMTPDelivery(t *testing.T) {
	previous := config.AppConfig.SMTP
	t.Cleanup(func() { config.AppConfig.SMTP = previous })
	for _, reject := range []bool{false, true} {
		t.Run(fmt.Sprintf("reject=%v", reject), func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			done := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					done <- err
					return
				}
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(5 * time.Second))
				reader := bufio.NewReader(conn)
				fmt.Fprint(conn, "220 localhost ESMTP\r\n")
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						done <- err
						return
					}
					switch {
					case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
						fmt.Fprint(conn, "250 localhost\r\n")
					case strings.HasPrefix(line, "MAIL FROM:"):
						fmt.Fprint(conn, "250 OK\r\n")
					case strings.HasPrefix(line, "RCPT TO:"):
						if reject {
							fmt.Fprint(conn, "550 rejected\r\n")
							done <- nil
							return
						}
						fmt.Fprint(conn, "250 OK\r\n")
					case strings.HasPrefix(line, "DATA"):
						fmt.Fprint(conn, "354 send data\r\n")
						var raw strings.Builder
						for {
							line, err = reader.ReadString('\n')
							if err != nil {
								done <- err
								return
							}
							if line == ".\r\n" {
								break
							}
							raw.WriteString(line)
						}
						message, err := mail.ReadMessage(strings.NewReader(raw.String()))
						if err != nil {
							done <- err
							return
						}
						content, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, message.Body))
						if err != nil || string(content) != "中文邮件正文" {
							done <- fmt.Errorf("incorrect MIME content %q: %v", content, err)
							return
						}
						if message.Header.Get("To") != "ops@example.com" {
							done <- fmt.Errorf("incorrect recipient")
							return
						}
						fmt.Fprint(conn, "250 accepted\r\n")
					case strings.HasPrefix(line, "QUIT"):
						fmt.Fprint(conn, "221 bye\r\n")
						done <- nil
						return
					default:
						done <- fmt.Errorf("unexpected SMTP command %s", line)
						return
					}
				}
			}()
			port := listener.Addr().(*net.TCPAddr).Port
			config.AppConfig.SMTP = conf.SMTPConfig{Host: "127.0.0.1", Port: port, Security: "none", From: "alerts@example.com"}
			channel, err := prepareNotificationChannel(saveNotificationChannelReq{Type: "email", EmailRecipients: []string{"ops@example.com"}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			channel.Type = "email"
			err = sendWebhook(channel, []byte(`{"title":"中文标题","content":"中文邮件正文"}`))
			if (err != nil) != reject {
				t.Fatalf("unexpected send result: %v", err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDingTalkSigningAndPayload(t *testing.T) {
	hook, _ := encryptSecret("https://oapi.dingtalk.com/robot/send?access_token=test")
	secret, _ := encryptSecret("SEC-test")
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	http.DefaultTransport = weComTestTransport(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query()
		timestamp, err := strconv.ParseInt(q.Get("timestamp"), 10, 64)
		if err != nil || time.Since(time.UnixMilli(timestamp)) > time.Minute {
			t.Errorf("invalid timestamp %v", err)
		}
		mac := hmac.New(sha256.New, []byte("SEC-test"))
		mac.Write([]byte(q.Get("timestamp") + "\nSEC-test"))
		if q.Get("sign") != base64.StdEncoding.EncodeToString(mac.Sum(nil)) || q.Get("access_token") != "test" {
			t.Error("invalid signature or lost token")
		}
		var payload struct {
			MsgType  string `json:"msgtype"`
			Markdown struct {
				Title string `json:"title"`
				Text  string `json:"text"`
			} `json:"markdown"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.MsgType != "markdown" || payload.Markdown.Title != "告警" || !strings.Contains(payload.Markdown.Text, "正文") {
			t.Errorf("invalid payload: %+v", payload)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"errcode":0,"errmsg":"ok"}`)), Header: make(http.Header)}, nil
	})
	if err := sendWebhook(models.NotificationChannel{Type: "dingding", WebhookURLCipher: hook, SignSecretCipher: secret}, []byte(`{"title":"告警","content":"正文"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestRobotResponseErrors(t *testing.T) {
	for _, tc := range []struct {
		kind, body string
		fail       bool
	}{
		{"feishu", `{"code":0,"msg":"success"}`, false},
		{"feishu", `{"StatusCode":0,"StatusMessage":"success"}`, false},
		{"feishu", `{"code":19021,"msg":"sign error"}`, true},
		{"dingding", `{"errcode":310000,"errmsg":"sign error"}`, true},
		{"qywechat", `{"errcode":93000,"errmsg":"invalid bot"}`, true},
		{"feishu", `{}`, true},
		{"dingding", `not json`, true},
	} {
		if err := checkRobotResponse(tc.kind, []byte(tc.body)); (err != nil) != tc.fail {
			t.Errorf("%s %s: %v", tc.kind, tc.body, err)
		}
	}
}
