package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"io"
	"net/http"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/gin-gonic/gin"
)

type saveNotificationChannelReq struct {
	Name            string   `json:"name" binding:"required"`
	Type            string   `json:"type"`
	TemplateID      uint     `json:"template_id"`
	WebhookURL      string   `json:"webhook_url"`
	SignSecret      string   `json:"sign_secret"`
	Status          int      `json:"status"`
	Description     string   `json:"description"`
	EmailRecipients []string `json:"email_recipients"`
}

type saveNotificationTemplateReq struct {
	Name         string `json:"name" binding:"required"`
	Type         string `json:"type"`
	Description  string `json:"description"`
	Content      string `json:"content" binding:"required"`
	AdvancedCard bool   `json:"advanced_card"`
	Status       int    `json:"status"`
}

type testNotificationReq struct {
	Domain         string `json:"domain"`
	CertExpireTime string `json:"cert_expire_time"`
}

func ListNotificationChannels(c *gin.Context) {
	var rows []models.NotificationChannel
	database.DB.Order("id DESC").Find(&rows)
	data := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		data = append(data, notificationChannelResponse(row))
	}
	resp.Success(c, http.StatusOK, "成功", data)
}

func ListNotificationTemplates(c *gin.Context) {
	var rows []models.NotificationTemplate
	database.DB.Order("id DESC").Find(&rows)
	resp.Success(c, http.StatusOK, "成功", rows)
}

func CreateNotificationTemplate(c *gin.Context) {
	var req saveNotificationTemplateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请填写名称和告警模板")
		return
	}
	name := strings.TrimSpace(req.Name)
	content := strings.TrimSpace(req.Content)
	if name == "" || content == "" {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请填写名称和告警模板")
		return
	}
	if err := validateNotificationTemplateContent(content); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "Markdown 模板语法不正确："+err.Error())
		return
	}
	if notificationTemplateNameExists(name, 0) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "模板名称已存在")
		return
	}
	item := models.NotificationTemplate{
		Name:         name,
		Type:         notificationType(req.Type),
		Description:  strings.TrimSpace(req.Description),
		Content:      content,
		AdvancedCard: req.AdvancedCard,
		Status:       normalizedStatus(req.Status),
	}
	if err := database.DB.Create(&item).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "创建通知模板失败")
		return
	}
	resp.Success(c, http.StatusOK, "创建成功", item)
}

func UpdateNotificationTemplate(c *gin.Context) {
	var existing models.NotificationTemplate
	if err := database.DB.First(&existing, c.Param("id")).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "通知模板不存在")
		return
	}
	var req saveNotificationTemplateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请填写名称和告警模板")
		return
	}
	name := strings.TrimSpace(req.Name)
	content := strings.TrimSpace(req.Content)
	if name == "" || content == "" {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请填写名称和告警模板")
		return
	}
	if err := validateNotificationTemplateContent(content); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "Markdown 模板语法不正确："+err.Error())
		return
	}
	id, _ := strconv.Atoi(c.Param("id"))
	if notificationTemplateNameExists(name, uint(id)) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "模板名称已存在")
		return
	}
	updates := map[string]interface{}{
		"name":          name,
		"type":          notificationType(req.Type),
		"description":   strings.TrimSpace(req.Description),
		"content":       content,
		"advanced_card": req.AdvancedCard,
		"status":        normalizedStatus(req.Status),
	}
	if err := database.DB.Model(&existing).Updates(updates).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "更新通知模板失败")
		return
	}
	resp.Success(c, http.StatusOK, "更新成功")
}

func DeleteNotificationTemplate(c *gin.Context) {
	database.DB.Delete(&models.NotificationTemplate{}, c.Param("id"))
	resp.Success(c, http.StatusOK, "删除成功")
}

func notificationTemplateNameExists(name string, excludeID uint) bool {
	var count int64
	tx := database.DB.Model(&models.NotificationTemplate{}).Where("name = ?", strings.TrimSpace(name))
	if excludeID > 0 {
		tx = tx.Where("id <> ?", excludeID)
	}
	tx.Count(&count)
	return count > 0
}

func CreateNotificationChannel(c *gin.Context) {
	var req saveNotificationChannelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请填写通知对象名称")
		return
	}
	if req.TemplateID == 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请选择高级模板")
		return
	}
	channelType := notificationType(req.Type)
	config, err := prepareNotificationChannel(req, nil)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	item := models.NotificationChannel{
		Name:                strings.TrimSpace(req.Name),
		Type:                channelType,
		TemplateID:          req.TemplateID,
		WebhookURLCipher:    config.WebhookURLCipher,
		SignSecretCipher:    config.SignSecretCipher,
		EmailRecipientsJSON: config.EmailRecipientsJSON,
		Status:              normalizedStatus(req.Status),
		Description:         strings.TrimSpace(req.Description),
	}
	if err := database.DB.Create(&item).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "创建通知对象失败")
		return
	}
	resp.Success(c, http.StatusOK, "创建成功", notificationChannelResponse(item))
}

func UpdateNotificationChannel(c *gin.Context) {
	var req saveNotificationChannelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请填写通知对象名称")
		return
	}
	var existing models.NotificationChannel
	if err := database.DB.First(&existing, c.Param("id")).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "通知对象不存在")
		return
	}
	if req.TemplateID == 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请选择高级模板")
		return
	}
	channelType := notificationType(req.Type)
	updates := map[string]interface{}{
		"name":        strings.TrimSpace(req.Name),
		"type":        channelType,
		"template_id": req.TemplateID,
		"status":      normalizedStatus(req.Status),
		"description": strings.TrimSpace(req.Description),
	}
	if channelType != existing.Type {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "通知类型不能修改，请新建通知对象")
		return
	}
	config, err := prepareNotificationChannel(req, &existing)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	updates["webhook_url_cipher"] = config.WebhookURLCipher
	updates["sign_secret_cipher"] = config.SignSecretCipher
	updates["email_recipients_json"] = config.EmailRecipientsJSON
	if err := database.DB.Model(&existing).Updates(updates).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "更新通知对象失败")
		return
	}
	resp.Success(c, http.StatusOK, "更新成功")
}

func notificationChannelResponse(item models.NotificationChannel) gin.H {
	return gin.H{
		"email_recipients":       emailRecipients(item),
		"id":                     item.ID,
		"name":                   item.Name,
		"type":                   item.Type,
		"template_id":            item.TemplateID,
		"webhook_url":            decryptedWebhookURL(item.WebhookURLCipher),
		"sign_secret":            decryptedOptionalSecret(item.SignSecretCipher),
		"sign_secret_configured": strings.TrimSpace(item.SignSecretCipher) != "",
		"status":                 item.Status,
		"description":            item.Description,
		"created_at":             item.CreatedAt,
		"updated_at":             item.UpdatedAt,
	}
}

func DeleteNotificationChannel(c *gin.Context) {
	id := c.Param("id")
	database.DB.Model(&models.CertificateDomain{}).Where("notification_channel_id = ?", id).Update("notification_channel_id", 0)
	database.DB.Delete(&models.NotificationChannel{}, id)
	resp.Success(c, http.StatusOK, "删除成功")
}

func TestNotificationChannel(c *gin.Context) {
	var channel models.NotificationChannel
	if err := database.DB.First(&channel, c.Param("id")).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "通知对象不存在")
		return
	}
	var req testNotificationReq
	_ = c.ShouldBindJSON(&req)
	if strings.TrimSpace(req.Domain) == "" {
		req.Domain = "mova-tech.com"
	}
	if strings.TrimSpace(req.CertExpireTime) == "" {
		req.CertExpireTime = "2026-09-30 23:59:59"
	}
	payload := map[string]interface{}{
		"title":            "观澜运维平台通知测试",
		"content":          "这是一条通知对象测试消息。",
		"level":            "info",
		"severity":         "P2",
		"status":           "firing",
		"source":           "aiops",
		"time":             time.Now().Format("2006-01-02 15:04:05"),
		"domain":           req.Domain,
		"cert_expire_time": req.CertExpireTime,
	}
	recordTitle := fmt.Sprint(payload["title"])
	if err := sendNotificationToChannel(channel, "notification.test", fmt.Sprintf("test:%d", channel.ID), recordTitle, payload); err != nil {
		resp.Error(c, http.StatusOK, resp.CodeFailure, err.Error())
		return
	}
	resp.Success(c, http.StatusOK, "发送成功")
}

func ListNotificationRecords(c *gin.Context) {
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "page_size", 20)
	if pageSize > 100 {
		pageSize = 100
	}
	tx := database.DB.Model(&models.NotificationRecord{})
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		like := "%" + keyword + "%"
		tx = tx.Where("channel_name LIKE ? OR title LIKE ? OR event_key LIKE ?", like, like, like)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		tx = tx.Where("status = ?", status)
	}
	if eventType := strings.TrimSpace(c.Query("event_type")); eventType != "" {
		tx = tx.Where("event_type = ?", eventType)
	}
	if startTime, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(c.Query("start_time")), time.Local); err == nil {
		tx = tx.Where("created_at >= ?", startTime)
	}
	if endTime, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(c.Query("end_time")), time.Local); err == nil {
		tx = tx.Where("created_at <= ?", endTime)
	}
	var total int64
	var rows []models.NotificationRecord
	tx.Count(&total)
	tx.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows)
	resp.Success(c, http.StatusOK, "成功", gin.H{"list": rows, "total": total, "page": page, "page_size": pageSize})
}

func SendCertificateDomainAlert(item models.CertificateDomain, reason string) {
	certExpireTime := ""
	if item.ObservedExpiresAt != nil {
		certExpireTime = item.ObservedExpiresAt.Format("2006-01-02 15:04:05")
	}
	payload := map[string]interface{}{
		"title":            certificateAlertTitle(item, reason),
		"content":          certificateAlertContent(item, reason),
		"level":            certificateAlertLevel(item),
		"source":           "certificate-domain",
		"domain":           item.Domain,
		"cert_expire_time": certExpireTime,
		"port":             item.Port,
		"status":           item.Status,
		"days_remaining":   item.DaysRemaining,
		"expires_at":       item.ObservedExpiresAt,
		"checked_at":       item.LastCheckedAt,
		"error":            item.LastCheckError,
	}
	eventKey := certificateAlertEventKey(item, reason)
	var channels []models.NotificationChannel
	if item.NotificationChannelID > 0 {
		database.DB.Where("id = ? AND status = ?", item.NotificationChannelID, 1).Find(&channels)
	} else {
		database.DB.Where("status = ?", 1).Order("id DESC").Find(&channels)
	}
	for _, channel := range channels {
		if notificationSentRecently(channel.ID, eventKey) {
			continue
		}
		_ = sendNotificationToChannel(channel, "certificate.domain.alert", eventKey, certificateAlertTitle(item, reason), payload)
	}
}

func ShouldNotifyCertificateDomain(item models.CertificateDomain) (string, bool) {
	if item.Status == "failed" {
		return "failed", true
	}
	if item.Status == "expired" {
		return "expired", true
	}
	if item.ObservedExpiresAt == nil {
		return "", false
	}
	for _, day := range parseAlertDays(item.AlertBeforeDays) {
		if item.DaysRemaining == day {
			return fmt.Sprintf("%d-days", day), true
		}
	}
	return "", false
}

func sendNotificationToChannel(channel models.NotificationChannel, eventType, eventKey, title string, payload map[string]interface{}) error {
	applyChannelTemplate(channel, payload)
	applySystemNotificationEnvelope(payload)
	if payloadTitle := notificationStringFromMap(payload, "title"); payloadTitle != "" {
		title = payloadTitle
	}
	body, _ := json.Marshal(payload)
	record := models.NotificationRecord{
		ChannelID:   channel.ID,
		ChannelName: channel.Name,
		EventType:   eventType,
		EventKey:    eventKey,
		Title:       title,
		RequestBody: string(body),
		Status:      "failed",
	}
	err := sendWebhook(channel, body)
	if err != nil {
		record.Error = err.Error()
		database.DB.Create(&record)
		return err
	}
	record.Status = "success"
	database.DB.Create(&record)
	return nil
}

func applyChannelTemplate(channel models.NotificationChannel, payload map[string]interface{}) {
	if channel.TemplateID == 0 {
		return
	}
	var tpl models.NotificationTemplate
	if err := database.DB.First(&tpl, channel.TemplateID).Error; err != nil {
		return
	}
	content, err := renderNotificationMarkdown(tpl.Content, payload)
	if err == nil && strings.TrimSpace(content) != "" {
		payload["content"] = content
	}
}

const systemNotificationFooter = "🧑‍💻 观澜运维平台 · 运维团队"

func applySystemNotificationEnvelope(payload map[string]interface{}) {
	status := strings.ToLower(strings.TrimSpace(fmt.Sprint(payload["status"])))
	recovered := status == "resolved" || status == "recovered" || status == "recovery"
	if recovered {
		payload["title"] = "【已恢复】- 观澜运维平台 ✅"
	} else if status == "firing" {
		payload["title"] = "【告警中】- 观澜运维平台 🚨"
	} else {
		payload["title"] = "观澜运维平台通知 🔔"
	}
	payload["title_color"] = alertNotificationTitleColor(strings.TrimSpace(fmt.Sprint(payload["severity"])), recovered)
	content := strings.TrimSpace(notificationStringFromMap(payload, "content"))
	content = strings.TrimSpace(strings.TrimSuffix(content, systemNotificationFooter))
	if content == "" {
		payload["content"] = systemNotificationFooter
		return
	}
	payload["content"] = content + "\n\n" + systemNotificationFooter
}

func validateNotificationTemplateContent(content string) error {
	tpl, err := template.New("notification-template").Option("missingkey=zero").Parse(content)
	if err != nil {
		return err
	}
	if len(tpl.Templates()) != 1 {
		return errors.New("通知模板只支持 Markdown 正文，不能使用 define 定义区块")
	}
	return nil
}

func renderNotificationMarkdown(content string, payload map[string]interface{}) (string, error) {
	rendered := content
	for key, value := range notificationVars(payload) {
		rendered = strings.ReplaceAll(rendered, "${"+key+"}", value)
	}
	tpl, err := template.New("notification-markdown").Option("missingkey=zero").Parse(rendered)
	if err != nil {
		return "", err
	}
	var output bytes.Buffer
	if err := tpl.Execute(&output, notificationTemplateData(payload)); err != nil {
		return "", err
	}
	return strings.TrimSpace(output.String()), nil
}

func notificationTemplateData(payload map[string]interface{}) map[string]interface{} {
	data := make(map[string]interface{}, len(payload)+8)
	for key, value := range payload {
		data[key] = value
	}
	for _, key := range []string{"system_labels", "labels", "datasource_labels", "external_labels", "annotations"} {
		if data[key] == nil {
			data[key] = map[string]string{}
		}
	}
	status := strings.ToLower(strings.TrimSpace(fmt.Sprint(data["status"])))
	recovered := status == "resolved" || status == "recovered" || status == "recovery"
	data["is_recovered"], data["is_firing"] = recovered, !recovered
	vars := notificationVars(payload)
	data["first_trigger_time_format"] = vars["first_trigger_time_format"]
	data["recover_time_format"] = vars["recover_time_format"]
	return data
}

func sendWebhook(channel models.NotificationChannel, body []byte) error {
	switch channel.Type {
	case "feishu":
		return sendFeishuWebhook(channel, body)
	case "qywechat":
		return sendWeComWebhook(channel, body)
	case "email":
		return sendSMTPNotification(channel, body)
	case "dingding":
		return sendDingTalkWebhook(channel, body)
	case "smart", "urgent_assistant", "slack", "customhook", "webhook":
	default:
		return fmt.Errorf("暂不支持的通知类型: %s", channel.Type)
	}
	return postWebhook(channel, body, "Webhook")
}

func sendWeComWebhook(channel models.NotificationChannel, body []byte) error {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return fmt.Errorf("企业微信通知内容无效: %w", err)
	}
	title := notificationStringFromMap(data, "title")
	content := notificationStringFromMap(data, "content")
	if title != "" {
		content = "### " + title + "\n\n" + content
	}
	if len(content) > 4096 {
		return errors.New("企业微信 Markdown 通知内容不能超过 4096 字节，请精简通知模板或减少分组告警数量")
	}
	payload, err := json.Marshal(map[string]interface{}{
		"msgtype":  "markdown",
		"markdown": map[string]string{"content": content},
	})
	if err != nil {
		return err
	}
	return postWebhook(channel, payload, "企业微信 Webhook")
}

func sendFeishuWebhook(channel models.NotificationChannel, body []byte) error {
	var data map[string]interface{}
	_ = json.Unmarshal(body, &data)
	title := notificationStringFromMap(data, "title")
	content := notificationStringFromMap(data, "content")
	if title == "" {
		title = "观澜运维平台通知"
	}
	titleColor := notificationStringFromMap(data, "title_color")
	if titleColor == "" {
		titleColor = "blue"
	}
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	payload := map[string]interface{}{
		"msg_type": "interactive",
		"card": map[string]interface{}{
			"config": map[string]bool{
				"wide_screen_mode": true,
			},
			"header": map[string]interface{}{
				"template": titleColor,
				"title": map[string]string{
					"tag":     "plain_text",
					"content": title,
				},
			},
			"elements": []map[string]string{
				{
					"tag":     "markdown",
					"content": content,
				},
			},
		},
	}
	if strings.TrimSpace(channel.SignSecretCipher) != "" {
		sign, err := feishuWebhookSign(timestamp, channel.SignSecretCipher)
		if err != nil {
			return err
		}
		payload["timestamp"] = timestamp
		payload["sign"] = sign
	}
	feishuBody, _ := json.Marshal(payload)
	return postWebhook(channel, feishuBody, "飞书 Webhook")
}

func alertNotificationTitleColor(severity string, recovered bool) string {
	if recovered {
		return "green"
	}
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "P0":
		return "red"
	case "P1":
		return "orange"
	default:
		return "blue"
	}
}

func postWebhook(channel models.NotificationChannel, body []byte, name string) error {
	webhookURL, err := decryptSecret(channel.WebhookURLCipher)
	if err != nil {
		return errors.New(name + " 地址解密失败")
	}
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s 返回 %d: %s", name, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if err := checkRobotResponse(channel.Type, respBody); err != nil {
		return err
	}
	return nil
}

func notificationSentRecently(channelID uint, eventKey string) bool {
	var count int64
	database.DB.Model(&models.NotificationRecord{}).
		Where("channel_id = ? AND event_key = ? AND status = ? AND created_at > ?", channelID, eventKey, "success", time.Now().Add(-23*time.Hour)).
		Count(&count)
	return count > 0
}

func encryptWebhookURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("请填写 Webhook 地址")
	}
	if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		return "", errors.New("Webhook 地址必须以 http:// 或 https:// 开头")
	}
	return encryptSecret(value)
}

func decryptedWebhookURL(cipherText string) string {
	value, err := decryptSecret(cipherText)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func decryptedOptionalSecret(cipherText string) string {
	if strings.TrimSpace(cipherText) == "" {
		return ""
	}
	value, err := decryptSecret(cipherText)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func encryptSignSecret(channelType, value string, existing *models.NotificationChannel) (string, error) {
	value = strings.TrimSpace(value)
	if channelType != "feishu" && channelType != "dingding" {
		return "", nil
	}
	if value == "" {
		if existing != nil && strings.TrimSpace(existing.SignSecretCipher) != "" {
			return "", nil
		}
		return "", nil
	}
	return encryptSecret(value)
}

func notificationType(value string) string {
	if strings.TrimSpace(value) == "" {
		return "feishu"
	}
	value = strings.TrimSpace(value)
	if value == "webhook" ||
		value == "feishu" ||
		value == "smart" ||
		value == "urgent_assistant" ||
		value == "email" ||
		value == "dingding" ||
		value == "qywechat" ||
		value == "slack" ||
		value == "customhook" {
		return value
	}
	return "feishu"
}

func normalizedStatus(value int) int {
	if value == 2 {
		return 2
	}
	return 1
}

func certificateAlertTitle(item models.CertificateDomain, reason string) string {
	switch reason {
	case "failed":
		return fmt.Sprintf("证书检测失败: %s", item.Domain)
	case "expired":
		return fmt.Sprintf("证书已过期: %s", item.Domain)
	default:
		return fmt.Sprintf("证书即将过期: %s", item.Domain)
	}
}

func certificateAlertContent(item models.CertificateDomain, reason string) string {
	if reason == "failed" {
		return fmt.Sprintf("%s:%d 证书检测失败: %s", item.Domain, item.Port, item.LastCheckError)
	}
	return fmt.Sprintf("%s:%d 线上证书剩余 %d 天过期", item.Domain, item.Port, item.DaysRemaining)
}

func certificateAlertLevel(item models.CertificateDomain) string {
	if item.Status == "failed" || item.Status == "expired" {
		return "critical"
	}
	return "warning"
}

func certificateAlertEventKey(item models.CertificateDomain, reason string) string {
	if reason == "failed" {
		return fmt.Sprintf("certificate-domain:%d:failed", item.ID)
	}
	if reason == "expired" {
		return fmt.Sprintf("certificate-domain:%d:expired", item.ID)
	}
	return fmt.Sprintf("certificate-domain:%d:%d-days", item.ID, item.DaysRemaining)
}

func parseAlertDays(value string) []int {
	parts := strings.Split(defaultAlertDays(value), ",")
	days := make([]int, 0, len(parts))
	for _, part := range parts {
		n := atoiDefault(strings.TrimSpace(part), -1)
		if n >= 0 {
			days = append(days, n)
		}
	}
	return days
}

func queryInt(c *gin.Context, key string, fallback int) int {
	value := atoiDefault(c.Query(key), fallback)
	if value < 1 {
		return fallback
	}
	return value
}

func notificationVars(payload map[string]interface{}) map[string]string {
	vars := make(map[string]string, len(payload)*2)
	for key, value := range payload {
		textValue := ""
		switch typed := value.(type) {
		case nil:
		case string:
			textValue = typed
		case time.Time:
			textValue = typed.Format("2006-01-02 15:04:05")
		case *time.Time:
			if typed != nil {
				textValue = typed.Format("2006-01-02 15:04:05")
			}
		case fmt.Stringer:
			textValue = typed.String()
		case map[string]string:
			textValue = inspectionLabelSummary(typed)
			for itemKey, itemValue := range typed {
				vars[key+"."+itemKey] = itemValue
			}
		default:
			textValue = fmt.Sprint(typed)
		}
		vars[key] = textValue
		if alias := lowerCamelNotificationVariable(key); alias != key {
			vars[alias] = textValue
		}
	}
	vars["first_trigger_time_format"] = vars["fired_at"]
	vars["recover_time_format"] = vars["recovered_at"]
	return vars
}

func lowerCamelNotificationVariable(value string) string {
	parts := strings.Split(value, "_")
	if len(parts) < 2 {
		return value
	}
	var result strings.Builder
	result.WriteString(parts[0])
	for _, part := range parts[1:] {
		if part == "" {
			continue
		}
		result.WriteString(strings.ToUpper(part[:1]))
		result.WriteString(part[1:])
	}
	return result.String()
}

func feishuWebhookSign(timestamp, cipherText string) (string, error) {
	secret, err := decryptSecret(cipherText)
	if err != nil {
		return "", errors.New("飞书签名密钥解密失败")
	}
	stringToSign := timestamp + "\n" + secret
	mac := hmac.New(sha256.New, []byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}

func notificationStringFromMap(data map[string]interface{}, key string) string {
	value, ok := data[key]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
