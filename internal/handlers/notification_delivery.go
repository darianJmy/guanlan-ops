package handlers

import (
	"guanlan/pkg/mailclient"

	"crypto/hmac"
	"crypto/sha256"

	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"net/url"
	"strconv"
	"strings"
	"time"

	"guanlan/internal/config"
	"guanlan/internal/models"
)

func prepareNotificationChannel(req saveNotificationChannelReq, existing *models.NotificationChannel) (models.NotificationChannel, error) {
	var result models.NotificationChannel
	if notificationType(req.Type) == "email" {
		recipients := append([]string(nil), req.EmailRecipients...)
		if err := mailclient.ValidateRecipients(recipients); err != nil {
			return result, err
		}
		data, err := json.Marshal(recipients)
		if err != nil {
			return result, err
		}
		result.EmailRecipientsJSON = string(data)
		return result, nil
	}

	hook := strings.TrimSpace(req.WebhookURL)
	if hook == "" && existing != nil {
		result.WebhookURLCipher = existing.WebhookURLCipher
	} else {
		parsed, err := url.Parse(hook)
		if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
			return result, errors.New("请填写有效的 HTTP/HTTPS Webhook 地址")
		}
		result.WebhookURLCipher, err = encryptWebhookURL(hook)
		if err != nil {
			return result, err
		}
	}
	if result.WebhookURLCipher == "" {
		return result, errors.New("请填写 Webhook 地址")
	}
	var err error
	result.SignSecretCipher, err = encryptSignSecret(req.Type, req.SignSecret, existing)
	return result, err
}

func emailRecipients(channel models.NotificationChannel) []string {
	recipients := []string{}
	_ = json.Unmarshal([]byte(channel.EmailRecipientsJSON), &recipients)
	return recipients
}

func sharedSMTPConfig(channel models.NotificationChannel) (mailclient.Config, error) {
	sender := config.AppConfig.SMTP
	c := mailclient.Config{Host: sender.Host, Port: sender.Port, Security: sender.Security,
		Username: sender.Username, Password: sender.Password, From: sender.From, To: emailRecipients(channel)}
	if strings.TrimSpace(sender.Host) == "" {
		return c, errors.New("尚未配置全局邮件服务，请在配置文件的 smtp 部分填写发信信息")
	}
	if err := mailclient.ValidateConfig(&c); err != nil {
		return c, fmt.Errorf("邮件配置不正确: %w", err)
	}
	return c, nil
}

func sendDingTalkWebhook(channel models.NotificationChannel, body []byte) error {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return err
	}
	title := notificationStringFromMap(data, "title")
	if title == "" {
		title = "观澜运维平台通知"
	}
	payload, err := json.Marshal(map[string]interface{}{"msgtype": "markdown", "markdown": map[string]string{"title": title, "text": "### " + title + "\n\n" + notificationStringFromMap(data, "content")}})
	if err != nil {
		return err
	}
	if channel.SignSecretCipher != "" {
		hook, err := decryptSecret(channel.WebhookURLCipher)
		if err != nil {
			return err
		}
		secret, err := decryptSecret(channel.SignSecretCipher)
		if err != nil {
			return err
		}
		parsed, err := url.Parse(hook)
		if err != nil {
			return err
		}
		timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(timestamp + "\n" + secret))
		query := parsed.Query()
		query.Set("timestamp", timestamp)
		query.Set("sign", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
		parsed.RawQuery = query.Encode()
		channel.WebhookURLCipher, err = encryptSecret(parsed.String())
		if err != nil {
			return err
		}
	}
	return postWebhook(channel, payload, "钉钉 Webhook")
}

func checkRobotResponse(kind string, body []byte) error {
	if kind != "feishu" && kind != "dingding" && kind != "qywechat" {
		return nil
	}
	var result struct {
		Code          *int   `json:"code"`
		ErrCode       *int   `json:"errcode"`
		StatusCode    *int   `json:"StatusCode"`
		Msg           string `json:"msg"`
		ErrMsg        string `json:"errmsg"`
		StatusMessage string `json:"StatusMessage"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return errors.New("机器人返回无效响应")
	}
	codes := []*int{result.ErrCode}
	message := result.ErrMsg
	if kind == "feishu" {
		codes = []*int{result.Code, result.StatusCode}
		message = result.Msg
		if message == "" {
			message = result.StatusMessage
		}
	}
	found := false
	for _, code := range codes {
		if code != nil {
			found = true
			if *code != 0 {
				return fmt.Errorf("机器人发送失败（%d）: %s", *code, message)
			}
		}
	}
	if !found {
		return errors.New("机器人响应缺少状态码")
	}
	return nil
}

func sendSMTPNotification(channel models.NotificationChannel, body []byte) error {
	cfg, err := sharedSMTPConfig(channel)
	if err != nil {
		return err
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return err
	}
	return mailclient.Send(cfg, notificationStringFromMap(data, "title"), notificationStringFromMap(data, "content"))
}
