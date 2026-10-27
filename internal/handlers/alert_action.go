package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"guanlan/pkg/shellutil"
	"guanlan/pkg/textutil"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	actionStatusQueued   = "queued"
	actionStatusRunning  = "running"
	actionStatusSuccess  = "success"
	actionStatusFailed   = "failed"
	actionStatusSkipped  = "skipped"
	actionStatusCanceled = "canceled"
)

var alertActionWorkerSlots = make(chan struct{}, 4)

type saveAlertActionRuleReq struct {
	Name                  string                     `json:"name"`
	Matchers              []models.AlertLabelMatcher `json:"matchers"`
	TriggerStatus         string                     `json:"trigger_status"`
	TargetID              uint                       `json:"target_id"`
	SkipSuppressed        bool                       `json:"skip_suppressed"`
	CooldownSeconds       int                        `json:"cooldown_seconds"`
	MaxRetries            int                        `json:"max_retries"`
	NotifyOnSuccess       bool                       `json:"notify_on_success"`
	NotifyOnFailure       bool                       `json:"notify_on_failure"`
	NotificationChannelID uint                       `json:"notification_channel_id"`
	Status                int                        `json:"status"`
}

func ListAlertActionRules(c *gin.Context) {
	centerID, ok := requireFaultCenter(c)
	if !ok {
		return
	}
	var rows []models.AlertActionRule
	database.DB.Where("fault_center_id = ?", centerID).Order("id ASC").Find(&rows)
	result := make([]gin.H, 0, len(rows))
	for i := range rows {
		result = append(result, alertActionRuleResponse(rows[i]))
	}
	resp.Success(c, http.StatusOK, "成功", result)
}

func CreateAlertActionRule(c *gin.Context) {
	centerID, ok := requireFaultCenter(c)
	if !ok {
		return
	}
	var req saveAlertActionRuleReq
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "自动化处置规则格式不正确")
		return
	}
	rule, err := buildAlertActionRule(centerID, req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if alertActionRuleNameExists(centerID, rule.Name, 0) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "规则名称已存在")
		return
	}
	if err := database.DB.Create(&rule).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "创建自动化处置规则失败")
		return
	}
	resp.Success(c, http.StatusOK, "自动化处置规则已创建", alertActionRuleResponse(rule))
}

func UpdateAlertActionRule(c *gin.Context) {
	centerID, ok := requireFaultCenter(c)
	if !ok {
		return
	}
	ruleID, _ := strconv.Atoi(c.Param("ruleId"))
	var existing models.AlertActionRule
	if ruleID < 1 || database.DB.Where("id = ? AND fault_center_id = ?", ruleID, centerID).First(&existing).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "自动化处置规则不存在")
		return
	}
	var req saveAlertActionRuleReq
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "自动化处置规则格式不正确")
		return
	}
	rule, err := buildAlertActionRule(centerID, req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if alertActionRuleNameExists(centerID, rule.Name, existing.ID) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "规则名称已存在")
		return
	}
	updates := map[string]interface{}{
		"name": rule.Name, "match_all": false, "matchers": rule.MatchersJSON,
		"trigger_status": rule.TriggerStatus, "action_type": rule.ActionType, "target_id": rule.TargetID,
		"node_type": "", "node_id": 0,
		"skip_suppressed": rule.SkipSuppressed, "cooldown_seconds": rule.CooldownSeconds,
		"max_retries": rule.MaxRetries, "timeout_seconds": rule.TimeoutSeconds, "status": rule.Status,
		"notify_on_success": rule.NotifyOnSuccess, "notify_on_failure": rule.NotifyOnFailure,
		"notification_channel_id": rule.NotificationChannelID,
		"webhook_url_cipher":      "", "webhook_secret_cipher": "",
	}
	if err := database.DB.Model(&existing).Updates(updates).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "更新自动化处置规则失败")
		return
	}
	database.DB.First(&existing, existing.ID)
	resp.Success(c, http.StatusOK, "自动化处置规则已更新", alertActionRuleResponse(existing))
}

func DeleteAlertActionRule(c *gin.Context) {
	centerID, ok := requireFaultCenter(c)
	if !ok {
		return
	}
	ruleID, _ := strconv.Atoi(c.Param("ruleId"))
	var rule models.AlertActionRule
	if ruleID < 1 || database.DB.Where("id = ? AND fault_center_id = ?", ruleID, centerID).First(&rule).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "自动化处置规则不存在")
		return
	}
	now := time.Now()
	database.DB.Model(&models.AlertActionExecution{}).Where("rule_id = ? AND status = ?", rule.ID, actionStatusQueued).Updates(map[string]interface{}{"status": actionStatusCanceled, "finished_at": now, "error": "处置规则已删除", "next_run_at": nil})
	database.DB.Delete(&rule)
	resp.Success(c, http.StatusOK, "自动化处置规则已删除")
}

func buildAlertActionRule(centerID uint, req saveAlertActionRuleReq) (models.AlertActionRule, error) {
	result := models.AlertActionRule{FaultCenterID: centerID}
	result.Name = strings.TrimSpace(req.Name)
	result.MatchAll = false
	result.TriggerStatus = strings.ToLower(strings.TrimSpace(req.TriggerStatus))
	result.ActionType = "automation_task"
	result.SkipSuppressed = req.SkipSuppressed
	result.CooldownSeconds, result.MaxRetries, result.TimeoutSeconds, result.Status = req.CooldownSeconds, 0, 300, req.Status
	result.NotifyOnSuccess, result.NotifyOnFailure, result.NotificationChannelID = req.NotifyOnSuccess, req.NotifyOnFailure, req.NotificationChannelID
	if result.Name == "" || len(result.Name) > 128 {
		return result, fmt.Errorf("请输入不超过 128 个字符的规则名称")
	}
	matchers, err := normalizeAlertLabelMatchers(req.Matchers)
	if err != nil || len(matchers) == 0 {
		return result, fmt.Errorf("请完整填写匹配标签")
	}
	result.Matchers, result.MatchersJSON = matchers, encodeJSON(matchers)
	if result.TriggerStatus != alertStatusFiring && result.TriggerStatus != alertStatusResolved {
		return result, fmt.Errorf("触发时机不正确")
	}
	if result.Status != 0 && result.Status != 1 {
		return result, fmt.Errorf("启用状态不正确")
	}
	if result.CooldownSeconds < 0 || result.CooldownSeconds > 2592000 {
		return result, fmt.Errorf("冷却时间超出允许范围")
	}
	result.TargetID = req.TargetID
	var task models.AutomationTask
	if database.DB.First(&task, result.TargetID).Error != nil || task.Status != 1 {
		return result, fmt.Errorf("自动化任务不存在或已停用")
	}
	result.TimeoutSeconds = task.TimeoutSeconds
	if result.NotifyOnSuccess || result.NotifyOnFailure {
		var channel models.NotificationChannel
		if database.DB.First(&channel, result.NotificationChannelID).Error != nil || channel.Status != 1 {
			return result, fmt.Errorf("请选择已启用的通知对象")
		}
	}
	return result, nil
}

func alertActionRuleResponse(rule models.AlertActionRule) gin.H {
	matchers := decodeAlertLabelMatchers(rule.MatchersJSON)
	webhookURL := ""
	if rule.ActionType == "webhook" {
		webhookURL = decryptedWebhookURL(rule.WebhookURLCipher)
	}
	return gin.H{
		"id": rule.ID, "fault_center_id": rule.FaultCenterID, "name": rule.Name,
		"match_all": rule.MatchAll, "matchers": matchers, "trigger_status": rule.TriggerStatus,
		"action_type": rule.ActionType, "target_id": rule.TargetID, "node_type": rule.NodeType, "node_id": rule.NodeID,
		"webhook_url": webhookURL, "has_webhook_secret": rule.WebhookSecretCipher != "",
		"skip_suppressed":  rule.SkipSuppressed,
		"cooldown_seconds": rule.CooldownSeconds, "max_retries": rule.MaxRetries,
		"timeout_seconds": rule.TimeoutSeconds, "status": rule.Status,
		"notify_on_success": rule.NotifyOnSuccess, "notify_on_failure": rule.NotifyOnFailure,
		"notification_channel_id": rule.NotificationChannelID,
		"created_at":              rule.CreatedAt, "updated_at": rule.UpdatedAt,
	}
}

func alertActionRuleNameExists(centerID uint, name string, excludeID uint) bool {
	query := database.DB.Model(&models.AlertActionRule{}).Where("fault_center_id = ? AND name = ?", centerID, name)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	query.Count(&count)
	return count > 0
}

func alertActionTransitionTriggered(eventType, toStatus string) bool {
	return (eventType == "created" || eventType == "state_changed") && (toStatus == alertStatusFiring || toStatus == alertStatusResolved)
}

func enqueueAlertActions(tx *gorm.DB, alert models.AlertInstance, eventType, toStatus string, now time.Time) error {
	if tx == nil || !alertActionTransitionTriggered(eventType, toStatus) {
		return nil
	}
	var rules []models.AlertActionRule
	if err := tx.Where("fault_center_id = ? AND trigger_status = ? AND status = 1", alert.FaultCenterID, toStatus).Order("id ASC").Find(&rules).Error; err != nil {
		return err
	}
	for i := range rules {
		rule := rules[i]
		matchers := decodeAlertLabelMatchers(rule.MatchersJSON)
		if !rule.MatchAll && !alertMatchesLabelMatchers(alert, matchers) {
			continue
		}
		nextRunAt := &now
		execution := models.AlertActionExecution{
			FaultCenterID: rule.FaultCenterID, RuleID: rule.ID, RuleName: rule.Name, AlertID: alert.ID, AlertTitle: alert.Title,
			TriggerStatus: toStatus, ActionType: rule.ActionType, ScopeKey: alertActionScopeKey(alert),
			Status: actionStatusQueued, MaxAttempts: rule.MaxRetries + 1, NextRunAt: nextRunAt,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&execution).Error; err != nil {
			return err
		}
	}
	return nil
}

func alertActionScopeKey(alert models.AlertInstance) string {
	namespaces := alertLabelNamespaces(alert)
	encoded, _ := json.Marshal(namespaces)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func dispatchDueAlertActions(now time.Time) {
	database.DB.Model(&models.AlertActionExecution{}).
		Where("status = ? AND lease_until IS NOT NULL AND lease_until < ?", actionStatusRunning, now).
		Updates(map[string]interface{}{"status": actionStatusQueued, "next_run_at": now, "lease_until": nil, "error": "执行进程中断，任务已重新排队"})

	var rows []models.AlertActionExecution
	database.DB.Where("status = ? AND next_run_at IS NOT NULL AND next_run_at <= ? AND (lease_until IS NULL OR lease_until < ?)", actionStatusQueued, now, now).Order("next_run_at ASC, id ASC").Limit(cap(alertActionWorkerSlots)).Find(&rows)
	for i := range rows {
		select {
		case alertActionWorkerSlots <- struct{}{}:
		default:
			return
		}
		execution := &rows[i]
		leaseUntil := now.Add(2 * time.Hour)
		claimed := database.DB.Model(&models.AlertActionExecution{}).Where("id = ? AND status = ? AND (lease_until IS NULL OR lease_until < ?)", execution.ID, actionStatusQueued, now).Updates(map[string]interface{}{"status": actionStatusRunning, "lease_until": leaseUntil, "started_at": now, "attempt": gorm.Expr("attempt + 1")})
		if claimed.RowsAffected != 1 {
			<-alertActionWorkerSlots
			continue
		}
		database.DB.First(execution, execution.ID)
		go func(row models.AlertActionExecution) {
			defer func() { <-alertActionWorkerSlots }()
			runAlertActionExecution(&row, now)
		}(*execution)
	}
}

func runAlertActionExecution(execution *models.AlertActionExecution, now time.Time) {
	var rule models.AlertActionRule
	var alert models.AlertInstance
	if database.DB.First(&rule, execution.RuleID).Error != nil || rule.Status != 1 || database.DB.First(&alert, execution.AlertID).Error != nil {
		finishAlertActionExecution(execution, actionStatusSkipped, "", "规则或告警不存在或已停用", now)
		return
	}
	if rule.SkipSuppressed {
		if _, reason := alertNotificationSuppression(&alert, now); reason != "" {
			finishAlertActionExecution(execution, actionStatusSkipped, "", reason, now)
			return
		}
	}
	if rule.CooldownSeconds > 0 {
		var count int64
		database.DB.Model(&models.AlertActionExecution{}).Where("rule_id = ? AND scope_key = ? AND status = ? AND id <> ? AND finished_at > ?", rule.ID, execution.ScopeKey, actionStatusSuccess, execution.ID, now.Add(-time.Duration(rule.CooldownSeconds)*time.Second)).Count(&count)
		if count > 0 {
			finishAlertActionExecution(execution, actionStatusSkipped, "", "相同标签范围仍在冷却时间内", now)
			return
		}
	}
	output, err := executeAlertAction(rule, alert, execution.TriggerStatus, now)
	if err == nil {
		finishAlertActionExecution(execution, actionStatusSuccess, output, "", time.Now())
		notifyAlertActionResult(rule, alert, execution, true, output, "")
		return
	}
	if execution.Attempt < execution.MaxAttempts {
		due := time.Now().Add(alertActionRetryDelay(execution.Attempt))
		database.DB.Model(execution).Updates(map[string]interface{}{"status": actionStatusQueued, "output": textutil.Ellipsis(output, 8000), "error": err.Error(), "next_run_at": due, "lease_until": nil})
		return
	}
	finishAlertActionExecution(execution, actionStatusFailed, output, err.Error(), time.Now())
	notifyAlertActionResult(rule, alert, execution, false, output, err.Error())
}

func finishAlertActionExecution(execution *models.AlertActionExecution, status, output, errorMessage string, now time.Time) {
	database.DB.Model(execution).Updates(map[string]interface{}{"status": status, "output": textutil.Ellipsis(output, 8000), "error": textutil.Ellipsis(errorMessage, 2000), "next_run_at": nil, "lease_until": nil, "finished_at": now})
}

func alertActionRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	seconds := 5 << min(attempt-1, 5)
	return time.Duration(seconds) * time.Second
}

func executeAlertAction(rule models.AlertActionRule, alert models.AlertInstance, triggerStatus string, now time.Time) (string, error) {
	switch rule.ActionType {
	case "automation_task":
		var task models.AutomationTask
		if database.DB.First(&task, rule.TargetID).Error != nil || task.Status != 1 {
			return "", fmt.Errorf("自动化任务不存在或已停用")
		}
		record := executeAutomationTask(task, "fault", alert.ID, buildAlertWorkflowInput(alert, triggerStatus, now))
		if record.Status != "success" {
			return record.Output, fmt.Errorf("%s", record.Error)
		}
		return record.Output, nil
	case "script":
		var script models.Script
		if database.DB.First(&script, rule.TargetID).Error != nil || script.Status != 1 {
			return "", fmt.Errorf("脚本不存在或已停用")
		}
		status, output, durationMS := executeAlertActionScript(script, rule.NodeType, rule.NodeID, time.Duration(rule.TimeoutSeconds)*time.Second)
		database.DB.Create(&models.ScriptExecution{ScriptID: script.ID, Status: status, Output: output, DurationMs: durationMS})
		if status != "success" {
			return output, fmt.Errorf("脚本执行失败")
		}
		return output, nil
	case "workflow":
		var workflow models.Workflow
		if database.DB.First(&workflow, rule.TargetID).Error != nil || workflow.Status != 1 {
			return "", fmt.Errorf("工作流不存在或已停用")
		}
		record := runWorkflow(&workflow, buildAlertWorkflowInput(alert, triggerStatus, now))
		if record.Status != "success" {
			return record.Result, fmt.Errorf("工作流执行失败")
		}
		return record.Result, nil
	case "webhook":
		return executeAlertActionWebhook(rule, alert, triggerStatus, now)
	default:
		return "", fmt.Errorf("不支持的动作类型")
	}
}

func buildAlertWorkflowInput(alert models.AlertInstance, triggerStatus string, now time.Time) map[string]interface{} {
	namespaces := alertLabelNamespaces(alert)
	alertJSON, _ := json.Marshal(alert)
	alertObject := map[string]interface{}{}
	_ = json.Unmarshal(alertJSON, &alertObject)
	var lastResult interface{}
	if json.Unmarshal([]byte(alert.LastResult), &lastResult) != nil {
		lastResult = alert.LastResult
	}
	lastResult = normalizeAutomationJSONValue(lastResult)
	return map[string]interface{}{
		"event":             "alert." + triggerStatus,
		"trigger_status":    triggerStatus,
		"triggered_at":      now,
		"alert":             alertObject,
		"system_labels":     namespaces["system_labels"],
		"labels":            namespaces["labels"],
		"datasource_labels": namespaces["datasource_labels"],
		"external_labels":   namespaces["external_labels"],
		"last_result":       lastResult,
	}
}

func normalizeAutomationJSONValue(value interface{}) interface{} {
	switch item := value.(type) {
	case string:
		var decoded interface{}
		if json.Unmarshal([]byte(item), &decoded) == nil {
			return normalizeAutomationJSONValue(decoded)
		}
		return item
	case map[string]interface{}:
		for key, child := range item {
			item[key] = normalizeAutomationJSONValue(child)
		}
		return item
	case []interface{}:
		for index, child := range item {
			item[index] = normalizeAutomationJSONValue(child)
		}
		return item
	default:
		return value
	}
}

func notifyAlertActionResult(rule models.AlertActionRule, alert models.AlertInstance, execution *models.AlertActionExecution, success bool, output, errorMessage string) {
	if rule.NotificationChannelID == 0 || success && !rule.NotifyOnSuccess || !success && !rule.NotifyOnFailure {
		return
	}
	var channel models.NotificationChannel
	if database.DB.First(&channel, rule.NotificationChannelID).Error != nil || channel.Status != 1 {
		return
	}
	statusText := "失败"
	eventType := "alert.action.failure"
	if success {
		statusText, eventType = "成功", "alert.action.success"
	}
	title := fmt.Sprintf("自动化处置%s：%s", statusText, alert.Title)
	payload := map[string]interface{}{
		"title": title, "content": title, "alert_id": alert.ID, "fault_center_id": alert.FaultCenterID,
		"rule_id": rule.ID, "rule_name": rule.Name, "automation_task_id": rule.TargetID,
		"status": map[bool]string{true: "success", false: "failed"}[success],
		"output": textutil.Ellipsis(output, 4000), "error": textutil.Ellipsis(errorMessage, 1000),
	}
	_ = sendNotificationToChannel(channel, eventType, fmt.Sprintf("alert-action:%d:%s", execution.ID, payload["status"]), title, payload)
}

func executeAlertActionScript(script models.Script, nodeType string, nodeID uint, timeout time.Duration) (string, string, int64) {
	if nodeType == "" || nodeType == "direct" {
		return runScriptWithTimeout(script, timeout)
	}
	startedAt := time.Now()
	commandName := "sh"
	content := script.Content
	if script.Type == "python" {
		commandName = "python3"
	}
	if script.SourceType == "git" {
		commandName, content = "bash", buildGitScriptCommand(script)
	}
	output, _, _, err := executeRemoteProbeCommand(ProbeReq{NodeType: nodeType, NodeID: nodeID}, shellutil.Command(commandName, []string{"-c", content}), timeout)
	if err != nil {
		return "fail", strings.TrimSpace(output + "\n" + err.Error()), time.Since(startedAt).Milliseconds()
	}
	return "success", strings.TrimSpace(output), time.Since(startedAt).Milliseconds()
}

func executeAlertActionWebhook(rule models.AlertActionRule, alert models.AlertInstance, triggerStatus string, now time.Time) (string, error) {
	webhookURL, err := decryptSecret(rule.WebhookURLCipher)
	if err != nil || strings.TrimSpace(webhookURL) == "" {
		return "", fmt.Errorf("Webhook 地址解密失败")
	}
	namespaces := alertLabelNamespaces(alert)
	payload := map[string]interface{}{
		"event": "alert." + triggerStatus, "alert_id": alert.ID, "alertname": alert.Title,
		"severity": alert.Severity, "status": triggerStatus, "source": alert.Source,
		"source_id": alert.SourceID, "target": alert.Target, "system_labels": namespaces["system_labels"], "labels": namespaces["labels"],
		"datasource_labels": namespaces["datasource_labels"], "external_labels": namespaces["external_labels"],
		"started_at": alert.StartedAt, "fired_at": alert.FiredAt, "resolved_at": alert.ResolvedAt, "sent_at": now,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AIOps-Event", "alert."+triggerStatus)
	if strings.TrimSpace(rule.WebhookSecretCipher) != "" {
		secret, decryptErr := decryptSecret(rule.WebhookSecretCipher)
		if decryptErr != nil {
			return "", fmt.Errorf("Webhook 签名密钥解密失败")
		}
		req.Header.Set("X-AIOps-Signature", alertActionWebhookSignature([]byte(secret), body))
	}
	client := http.Client{Timeout: time.Duration(rule.TimeoutSeconds) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	output := strings.TrimSpace(string(responseBody))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return output, fmt.Errorf("Webhook 返回 %d", resp.StatusCode)
	}
	return output, nil
}

func alertActionWebhookSignature(secret, body []byte) string {
	digest := hmac.New(sha256.New, secret)
	_, _ = digest.Write(body)
	return "sha256=" + hex.EncodeToString(digest.Sum(nil))
}

func sortedAlertActionMatcherSummary(matchers []models.AlertLabelMatcher) string {
	parts := make([]string, 0, len(matchers))
	for _, matcher := range matchers {
		parts = append(parts, matcher.Namespace+"."+matcher.Key+"="+matcher.Value)
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}
