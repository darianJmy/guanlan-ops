package mailclient

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Security string   `json:"security"`
	Username string   `json:"username"`
	Password string   `json:"password,omitempty"`
	From     string   `json:"from"`
	To       []string `json:"to"`
}

func ValidateConfig(c *Config) error {
	c.Host = strings.TrimSpace(c.Host)
	c.Username = strings.TrimSpace(c.Username)
	c.From = strings.TrimSpace(c.From)
	if c.Host == "" || strings.ContainsAny(c.Host, "/:@ \t\r\n") {
		return errors.New("请填写 SMTP 服务器主机名，不包含协议或端口")
	}
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("SMTP 端口必须为 1–65535")
	}
	switch c.Security {
	case "tls", "starttls", "none":
	default:
		return errors.New("请选择 SMTP 加密方式")
	}
	if strings.ContainsAny(c.Username, "\r\n") {
		return errors.New("SMTP 账号格式不正确")
	}
	if c.Username != "" && c.Password == "" {
		return errors.New("请填写 SMTP 密码或授权码")
	}
	if c.Username == "" {
		c.Password = ""
	}
	from, err := mail.ParseAddress(c.From)
	if err != nil || strings.ContainsAny(c.From, "\r\n") {
		return errors.New("发件人邮箱格式不正确")
	}
	c.From = from.String()
	return ValidateRecipients(c.To)
}
func ValidateRecipients(recipients []string) error {
	if len(recipients) == 0 {
		return errors.New("请至少填写一个收件人邮箱")
	}
	for i, value := range recipients {
		address, err := mail.ParseAddress(strings.TrimSpace(value))
		if err != nil || strings.ContainsAny(value, "\r\n") {
			return errors.New("收件人邮箱格式不正确")
		}
		recipients[i] = address.Address
	}
	return nil
}
func Message(config Config, title, content string) []byte {
	title = strings.ReplaceAll(strings.ReplaceAll(title, "\r", " "), "\n", " ")
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n", config.From, strings.Join(config.To, ", "), mime.BEncoding.Encode("UTF-8", title), time.Now().Format(time.RFC1123Z))
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	for len(encoded) > 76 {
		msg.WriteString(encoded[:76] + "\r\n")
		encoded = encoded[76:]
	}
	msg.WriteString(encoded + "\r\n")
	return msg.Bytes()
}

// AlternativeMessage keeps a plain-text fallback for clients without HTML support.
func AlternativeMessage(config Config, title, content, htmlContent string) []byte {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range []struct{ kind, content string }{{"text/plain", content}, {"text/html", htmlContent}} {
		header := textproto.MIMEHeader{}
		header.Set("Content-Type", part.kind+"; charset=UTF-8")
		header.Set("Content-Transfer-Encoding", "base64")
		dst, _ := writer.CreatePart(header)
		encoded := base64.StdEncoding.EncodeToString([]byte(part.content))
		for len(encoded) > 76 {
			fmt.Fprint(dst, encoded[:76]+"\r\n")
			encoded = encoded[76:]
		}
		fmt.Fprint(dst, encoded+"\r\n")
	}
	writer.Close()
	title = strings.NewReplacer("\r", " ", "\n", " ").Replace(title)
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%q\r\n\r\n", config.From, strings.Join(config.To, ", "), mime.BEncoding.Encode("UTF-8", title), time.Now().Format(time.RFC1123Z), writer.Boundary())
	msg.Write(body.Bytes())
	return msg.Bytes()
}

func Send(config Config, title, content string) error {
	return send(config, title, content, "")
}

func SendAlternative(config Config, title, content, htmlContent string) error {
	return send(config, title, content, htmlContent)
}

func send(config Config, title, content, htmlContent string) error {
	if err := ValidateConfig(&config); err != nil {
		return err
	}
	var err error
	addr := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	tlsConfig := &tls.Config{ServerName: config.Host, MinVersion: tls.VersionTLS12}
	var conn net.Conn
	if config.Security == "tls" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("SMTP 连接失败: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, config.Host)
	if err != nil {
		return fmt.Errorf("SMTP 握手失败: %w", err)
	}
	defer client.Close()
	if config.Security == "starttls" {
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("SMTP STARTTLS 失败: %w", err)
		}
	}
	if config.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", config.Username, config.Password, config.Host)); err != nil {
			return fmt.Errorf("SMTP 认证失败: %w", err)
		}
	}
	from, _ := mail.ParseAddress(config.From)
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("SMTP 发件人被拒绝: %w", err)
	}
	for _, to := range config.To {
		if err := client.Rcpt(to); err != nil {
			return fmt.Errorf("SMTP 收件人被拒绝: %w", err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	message := Message(config, title, content)
	if htmlContent != "" {
		message = AlternativeMessage(config, title, content, htmlContent)
	}
	if _, err = writer.Write(message); err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return fmt.Errorf("SMTP 邮件发送失败: %w", err)
	}
	// DATA was accepted; a later disconnect must not mark the delivered message as failed.
	_ = client.Quit()
	return nil
}
