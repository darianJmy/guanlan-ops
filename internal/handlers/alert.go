package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"guanlan/pkg/textutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	alertStatusPending    = "pending"
	alertStatusFiring     = "firing"
	alertStatusRecovering = "recovering"
	alertStatusResolved   = "resolved"
	alertStatusTerminated = "terminated"
)

type saveFaultCenterReq struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Status      int    `json:"status"`
}

type saveGroupingReq struct {
	Name            string                      `json:"name"`
	Matchers        []models.AlertLabelMatcher  `json:"matchers"`
	GroupBy         []models.AlertLabelSelector `json:"group_by"`
	WaitSeconds     int                         `json:"wait_seconds"`
	IntervalSeconds int                         `json:"interval_seconds"`
	Priority        int                         `json:"priority"`
	Status          int                         `json:"status"`
}

type saveInhibitionRuleReq struct {
	Name           string                      `json:"name" binding:"required"`
	SourceMatchers []models.AlertLabelMatcher  `json:"source_matchers" binding:"required"`
	TargetMatchers []models.AlertLabelMatcher  `json:"target_matchers" binding:"required"`
	EqualLabels    []models.AlertLabelSelector `json:"equal_labels"`
	Status         int                         `json:"status"`
}

type silenceAlertReq struct {
	DurationMinutes int    `json:"duration_minutes" binding:"required"`
	Reason          string `json:"reason"`
}

type saveSilenceRuleReq struct {
	Name        string                     `json:"name"`
	Matchers    []models.AlertLabelMatcher `json:"matchers"`
	SilenceType string                     `json:"silence_type"`
	Weekdays    []int                      `json:"weekdays"`
	StartTime   string                     `json:"start_time"`
	EndTime     string                     `json:"end_time"`
	StartAt     *time.Time                 `json:"start_at"`
	EndAt       *time.Time                 `json:"end_at"`
	Status      *int                       `json:"status"`
}

type saveNotificationRouteReq struct {
	LabelNamespace string `json:"label_namespace"`
	LabelKey       string `json:"label_key"`
	LabelValue     string `json:"label_value"`
	ChannelIDs     []uint `json:"channel_ids"`
	Status         int    `json:"status"`
}

type saveNotificationConfigReq struct {
	DefaultChannelIDs     []uint                     `json:"default_channel_ids"`
	RepeatIntervalSeconds int                        `json:"repeat_interval_seconds"`
	RecoveryNotification  bool                       `json:"recovery_notification"`
	Routes                []saveNotificationRouteReq `json:"routes"`
}

func ListFaultCenters(c *gin.Context) {
	var rows []models.FaultCenter
	database.DB.Order("id ASC").Find(&rows)
	now := time.Now()
	for i := range rows {
		database.DB.Model(&models.InspectionItem{}).Where("fault_center_id = ?", rows[i].ID).Count(&rows[i].TaskCount)
		database.DB.Model(&models.AlertInstance{}).Where("fault_center_id = ? AND status = ?", rows[i].ID, alertStatusPending).Count(&rows[i].PendingCount)
		database.DB.Model(&models.AlertInstance{}).Where("fault_center_id = ? AND status = ?", rows[i].ID, alertStatusFiring).Count(&rows[i].FiringCount)
		database.DB.Model(&models.AlertInstance{}).Where("fault_center_id = ? AND status = ?", rows[i].ID, alertStatusRecovering).Count(&rows[i].RecoveringCount)
		database.DB.Model(&models.AlertInstance{}).Where("fault_center_id = ? AND status = ?", rows[i].ID, alertStatusResolved).Count(&rows[i].ResolvedCount)
		database.DB.Model(&models.AlertInstance{}).
			Where("fault_center_id = ? AND status IN ? AND silenced_until > ?", rows[i].ID, []string{alertStatusPending, alertStatusFiring, alertStatusRecovering}, now).
			Count(&rows[i].SilencedCount)
	}
	resp.Success(c, http.StatusOK, "成功", rows)
}

func CreateFaultCenter(c *gin.Context) {
	var req saveFaultCenterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请填写故障中心名称")
		return
	}
	if err := validateFaultCenterReq(req, 0); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	row := models.FaultCenter{Name: strings.TrimSpace(req.Name), Description: strings.TrimSpace(req.Description), Status: req.Status}
	if err := database.DB.Create(&row).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "创建故障中心失败")
		return
	}
	resp.Success(c, http.StatusOK, "创建成功", row)
}

func UpdateFaultCenter(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var row models.FaultCenter
	if id < 1 || database.DB.First(&row, id).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "故障中心不存在")
		return
	}
	var req saveFaultCenterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请填写故障中心名称")
		return
	}
	if err := validateFaultCenterReq(req, row.ID); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	database.DB.Model(&row).Updates(map[string]interface{}{"name": strings.TrimSpace(req.Name), "description": strings.TrimSpace(req.Description), "status": req.Status})
	resp.Success(c, http.StatusOK, "更新成功")
}

func DeleteFaultCenter(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var center models.FaultCenter
	if id < 1 {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "故障中心不存在")
		return
	}
	if err := database.DB.First(&center, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "故障中心不存在")
		} else {
			resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "查询故障中心失败")
		}
		return
	}
	if center.IsDefault {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "默认故障中心不能删除")
		return
	}
	var taskCount, alertCount, certificateDomainCount int64
	database.DB.Model(&models.InspectionItem{}).Where("fault_center_id = ?", id).Count(&taskCount)
	database.DB.Model(&models.AlertInstance{}).Where("fault_center_id = ?", id).Count(&alertCount)
	database.DB.Model(&models.CertificateDomain{}).Where("fault_center_id = ?", id).Count(&certificateDomainCount)
	if taskCount > 0 || alertCount > 0 || certificateDomainCount > 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "故障中心已有拨测任务、证书域名或告警，请先迁移后再删除")
		return
	}
	result := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("fault_center_id = ?", id).Delete(&models.AlertGroupState{}).Error; err != nil {
			return err
		}
		if err := tx.Where("fault_center_id = ?", id).Delete(&models.AlertGroupingRule{}).Error; err != nil {
			return err
		}
		if err := tx.Where("fault_center_id = ?", id).Delete(&models.AlertActionExecution{}).Error; err != nil {
			return err
		}
		if err := tx.Where("fault_center_id = ?", id).Delete(&models.AlertActionRule{}).Error; err != nil {
			return err
		}
		deleted := tx.Delete(&models.FaultCenter{}, id)
		if deleted.Error != nil {
			return deleted.Error
		}
		if deleted.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if result == gorm.ErrRecordNotFound {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "故障中心不存在")
		return
	}
	if result != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "删除故障中心失败")
		return
	}
	resp.Success(c, http.StatusOK, "删除成功")
}

func validateFaultCenterReq(req saveFaultCenterReq, excludeID uint) error {
	name := strings.TrimSpace(req.Name)
	if name == "" || len([]rune(name)) > 128 {
		return fmt.Errorf("故障中心名称不能为空且不能超过 128 个字符")
	}
	if req.Status != 0 && req.Status != 1 {
		return fmt.Errorf("故障中心状态不正确")
	}
	var count int64
	query := database.DB.Model(&models.FaultCenter{}).Where("name = ?", name)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	query.Count(&count)
	if count > 0 {
		return fmt.Errorf("故障中心名称已存在")
	}
	return nil
}

func ListAlerts(c *gin.Context) {
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "page_size", 20)
	if pageSize > 100 {
		pageSize = 100
	}

	tx := database.DB.Model(&models.AlertInstance{})
	centerID := queryInt(c, "fault_center_id", 0)
	if centerID > 0 {
		tx = tx.Where("fault_center_id = ?", centerID)
	}
	switch strings.ToLower(strings.TrimSpace(c.Query("status"))) {
	case "active":
		tx = tx.Where("status IN ?", []string{alertStatusPending, alertStatusFiring, alertStatusRecovering})
	case "history":
		tx = tx.Where("status IN ?", []string{alertStatusResolved, alertStatusTerminated})
	case "silenced":
		tx = tx.Where("status IN ? AND silenced_until > ?", []string{alertStatusPending, alertStatusFiring, alertStatusRecovering}, time.Now())
	case alertStatusPending, alertStatusFiring, alertStatusRecovering, alertStatusResolved, alertStatusTerminated:
		tx = tx.Where("status = ?", strings.ToLower(strings.TrimSpace(c.Query("status"))))
	}
	switch strings.ToLower(strings.TrimSpace(c.Query("notification_status"))) {
	case "grouping":
		tx = tx.Where("notification_status IN ?", []string{notificationStatusWaitingGroup, notificationStatusWaitingUpdate, notificationStatusWaitingRecovery})
	case notificationStatusSilenced:
		tx = tx.Where("notification_status = ? OR silenced_until > ?", notificationStatusSilenced, time.Now())
	case notificationStatusInhibited, notificationStatusSent, notificationStatusRecoverySent, notificationStatusNoReceiver, notificationStatusSendFailed, notificationStatusResolvedUnnotified, notificationStatusRecoveryDisabled:
		tx = tx.Where("notification_status = ?", strings.ToLower(strings.TrimSpace(c.Query("notification_status"))))
	}
	if source := strings.TrimSpace(c.Query("source")); source != "" {
		tx = tx.Where("source = ?", source)
	}
	if severity := strings.ToLower(strings.TrimSpace(c.Query("severity"))); severity != "" {
		tx = tx.Where("LOWER(severity) = ?", severity)
	}
	tx = applyAlertTriggerDateFilters(tx, c.Query("start_date"), c.Query("end_date"))
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		like := "%" + keyword + "%"
		tx = tx.Where("title LIKE ? OR target LIKE ? OR labels LIKE ?", like, like, like)
	}

	var total int64
	var rows []models.AlertInstance
	tx.Count(&total)
	tx.Order("last_event_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows)
	attachAlertRuleDetails(rows)

	countStatus := func(status string) int64 {
		var count int64
		query := database.DB.Model(&models.AlertInstance{}).Where("status = ?", status)
		if centerID > 0 {
			query = query.Where("fault_center_id = ?", centerID)
		}
		query.Count(&count)
		return count
	}
	pending, firing, recovering, resolved, terminated := countStatus(alertStatusPending), countStatus(alertStatusFiring), countStatus(alertStatusRecovering), countStatus(alertStatusResolved), countStatus(alertStatusTerminated)

	resp.Success(c, http.StatusOK, "成功", gin.H{
		"list": rows, "total": total, "page": page, "page_size": pageSize,
		"summary": gin.H{"pending": pending, "firing": firing, "recovering": recovering, "resolved": resolved, "terminated": terminated},
	})
}

func applyAlertTriggerDateFilters(tx *gorm.DB, startDateValue, endDateValue string) *gorm.DB {
	const triggerTimeExpression = "COALESCE(fired_at, started_at)"
	if startDate, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(startDateValue), time.Local); err == nil {
		tx = tx.Where(triggerTimeExpression+" >= ?", startDate)
	}
	if endDate, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(endDateValue), time.Local); err == nil {
		tx = tx.Where(triggerTimeExpression+" < ?", endDate.AddDate(0, 0, 1))
	}
	return tx
}

func attachAlertRuleDetails(alerts []models.AlertInstance) {
	ruleIDs := make([]uint, 0)
	seen := make(map[uint]struct{})
	for i := range alerts {
		enrichAlertResultWithTemplateTimes(&alerts[i])
		var snapshot struct {
			Description string `json:"description"`
		}
		if json.Unmarshal([]byte(alerts[i].LastResult), &snapshot) == nil {
			alerts[i].RuleDescription = snapshot.Description
		}
		if alerts[i].Source != alertRuleSource || alerts[i].SourceID == 0 {
			continue
		}
		if _, exists := seen[alerts[i].SourceID]; exists {
			continue
		}
		seen[alerts[i].SourceID] = struct{}{}
		ruleIDs = append(ruleIDs, alerts[i].SourceID)
	}
	if len(ruleIDs) == 0 {
		return
	}

	var rules []models.AlertRule
	if err := database.DB.Select("id", "group_id", "description").Where("id IN ?", ruleIDs).Find(&rules).Error; err != nil {
		return
	}
	groupIDs := make([]uint, 0, len(rules))
	seenGroups := make(map[uint]struct{}, len(rules))
	for i := range rules {
		if rules[i].GroupID == 0 {
			continue
		}
		if _, exists := seenGroups[rules[i].GroupID]; exists {
			continue
		}
		seenGroups[rules[i].GroupID] = struct{}{}
		groupIDs = append(groupIDs, rules[i].GroupID)
	}
	var groups []models.AlertRuleGroup
	if len(groupIDs) > 0 {
		database.DB.Select("id", "name").Where("id IN ?", groupIDs).Find(&groups)
	}
	groupNames := make(map[uint]string, len(groups))
	for i := range groups {
		groupNames[groups[i].ID] = groups[i].Name
	}
	ruleDetails := make(map[uint]models.AlertRule, len(rules))
	for i := range rules {
		ruleDetails[rules[i].ID] = rules[i]
	}
	for i := range alerts {
		rule, exists := ruleDetails[alerts[i].SourceID]
		if !exists {
			continue
		}
		alerts[i].RuleDescription = rule.Description
		alerts[i].RuleGroupID = rule.GroupID
		alerts[i].RuleGroupName = groupNames[rule.GroupID]
	}
}

// enrichAlertResultWithTemplateTimes keeps the persisted evaluation snapshot
// untouched while exposing the same derived time fields available to notification
// templates in the alert detail response.
func enrichAlertResultWithTemplateTimes(alert *models.AlertInstance) {
	if alert == nil || strings.TrimSpace(alert.LastResult) == "" {
		return
	}
	var result map[string]interface{}
	if json.Unmarshal([]byte(alert.LastResult), &result) != nil || result == nil {
		return
	}
	result["first_trigger_time_format"] = formatNotificationTime(alert.FiredAt)
	result["recover_time_format"] = formatNotificationTime(alert.ResolvedAt)
	if encoded, err := json.Marshal(result); err == nil {
		alert.LastResult = string(encoded)
	}
}

func ListAlertEvents(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var alert models.AlertInstance
	if id < 1 || database.DB.First(&alert, id).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "告警不存在")
		return
	}
	var rows []models.AlertEvent
	if err := database.DB.Where("alert_id = ?", id).Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "查询告警时间线失败")
		return
	}
	resp.Success(c, http.StatusOK, "成功", rows)
}

func recordAlertEvent(tx *gorm.DB, alert models.AlertInstance, eventType, fromStatus, toStatus, fromSeverity, toSeverity, reason, value, evaluationStatus string, now time.Time) error {
	if tx == nil || alert.ID == 0 {
		return nil
	}
	if err := tx.Create(&models.AlertEvent{
		AlertID: alert.ID, EventType: eventType, FromStatus: fromStatus, ToStatus: toStatus,
		FromSeverity: fromSeverity, ToSeverity: toSeverity, Reason: reason, Value: textutil.Ellipsis(value, 255),
		Labels: alert.Labels, EvaluationStatus: evaluationStatus, CreatedAt: now,
	}).Error; err != nil {
		return err
	}
	return enqueueAlertActions(tx, alert, eventType, toStatus, now)
}

func syncCertificateDomainAlert(item models.CertificateDomain, now time.Time) error {
	active := item.Status == "failed" || item.Status == "expired" || item.Status == "expiring" || item.Status == "mismatch"
	var alert models.AlertInstance
	query := database.DB.Where("source = ? AND source_id = ? AND status IN ?", "certificate-domain", item.ID, []string{alertStatusPending, alertStatusFiring, alertStatusRecovering}).Order("id DESC").Limit(1).Find(&alert)
	if query.Error != nil {
		return query.Error
	}
	if query.RowsAffected == 0 && !active {
		return nil
	}

	var center models.FaultCenter
	centerQuery := database.DB.Where("status = ?", 1)
	if item.FaultCenterID > 0 {
		centerQuery = centerQuery.Where("id = ?", item.FaultCenterID)
	} else {
		centerQuery = centerQuery.Order("id ASC")
	}
	if err := centerQuery.First(&center).Error; err != nil {
		return errors.New("没有可用的故障中心")
	}
	severity := normalizeCertificateDomainSeverity(item.Severity)
	labels := map[string]string{
		"alertname": "域名证书异常", "domain": item.Domain, "port": strconv.Itoa(defaultPort(item.Port)),
		"certificate_status": item.Status,
	}
	encodedLabels, _ := json.Marshal(labels)
	snapshot, _ := json.Marshal(gin.H{
		"alertname": "域名证书异常", "source": "certificate-domain", "source_id": item.ID,
		"status": item.Status, "domain": item.Domain, "port": defaultPort(item.Port),
		"days_remaining": item.DaysRemaining, "expires_at": item.ObservedExpiresAt, "expiry_alert_days": item.ExpiryAlertDays,
		"severity": severity,
		"error":    item.LastCheckError, "labels": labels,
	})
	title := fmt.Sprintf("%s:%d · 证书%s", item.Domain, defaultPort(item.Port), statusTextForCertificateAlert(item.Status))
	fingerprint := fmt.Sprintf("certificate-domain:%d", item.ID)

	if query.RowsAffected == 0 {
		alert = models.AlertInstance{
			FaultCenterID: center.ID, Source: "certificate-domain", SourceID: item.ID, Fingerprint: fingerprint,
			Title: title, Target: item.Domain, Severity: severity, Status: alertStatusFiring, Labels: string(encodedLabels),
			FailureCount: 1, FailureThreshold: 1, LastProbeStatus: item.LastCheckStatus, LastResult: string(snapshot),
			StartedAt: now, FiredAt: &now, StateChangedAt: now, LastEventAt: now,
		}
		if err := database.DB.Create(&alert).Error; err != nil {
			return err
		}
		if err := recordAlertEvent(database.DB, alert, "created", "", alertStatusFiring, "", severity, "certificate_domain_unhealthy", item.Status, item.LastCheckStatus, now); err != nil {
			return err
		}
		applyActiveSilenceRule(&alert, now)
		sendCertificateDomainAlertNotification(item, &alert, false, now)
		return nil
	}

	previous, previousSeverity := alert.Status, alert.Severity
	next := alertStatusFiring
	if !active {
		next = alertStatusResolved
	}
	updates := map[string]interface{}{
		"fault_center_id": center.ID, "title": title, "target": item.Domain, "severity": severity,
		"status": next, "labels": string(encodedLabels), "last_probe_status": item.LastCheckStatus,
		"last_result": string(snapshot), "last_event_at": now,
	}
	applyAlertStateTransition(updates, previous, next, now)
	if err := database.DB.Model(&alert).Updates(updates).Error; err != nil {
		return err
	}
	alert.FaultCenterID, alert.Title, alert.Target, alert.Severity = center.ID, title, item.Domain, severity
	alert.Status, alert.Labels, alert.LastResult, alert.LastProbeStatus, alert.LastEventAt = next, string(encodedLabels), string(snapshot), item.LastCheckStatus, now
	eventType := alertTransitionEventType(previous, next, previousSeverity, severity)
	if eventType != "" {
		reason := "certificate_domain_changed"
		if next == alertStatusResolved {
			reason = "certificate_domain_recovered"
		}
		if err := recordAlertEvent(database.DB, alert, eventType, previous, next, previousSeverity, severity, reason, item.Status, item.LastCheckStatus, now); err != nil {
			return err
		}
	}
	applyActiveSilenceRule(&alert, now)
	sendCertificateDomainAlertNotification(item, &alert, next == alertStatusResolved, now)
	return nil
}

func statusTextForCertificateAlert(status string) string {
	switch status {
	case "failed":
		return "检测失败"
	case "expired":
		return "已过期"
	case "expiring":
		return "即将过期"
	case "mismatch":
		return "与资产不一致"
	default:
		return "状态异常"
	}
}

func alertTransitionEventType(previousStatus, nextStatus, previousSeverity, nextSeverity string) string {
	if previousStatus == "" {
		return "created"
	}
	if previousStatus != nextStatus {
		return "state_changed"
	}
	if previousSeverity != nextSeverity {
		return "severity_changed"
	}
	return ""
}

func ListAlertGroupingRules(c *gin.Context) {
	centerID, ok := requireFaultCenter(c)
	if !ok {
		return
	}
	var rows []models.AlertGroupingRule
	if err := database.DB.Where("fault_center_id = ?", centerID).Order("priority ASC, id ASC").Find(&rows).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "获取分组策略失败")
		return
	}
	for i := range rows {
		rows[i].Matchers = decodeAlertLabelMatchers(rows[i].MatchersJSON)
		rows[i].GroupBy = decodeAlertGroupingSelectors(rows[i].GroupByJSON)
	}
	resp.Success(c, http.StatusOK, "成功", rows)
}

func CreateAlertGroupingRule(c *gin.Context) {
	centerID, ok := requireFaultCenter(c)
	if !ok {
		return
	}
	rule, ok := bindAlertGroupingRule(c, centerID, 0)
	if !ok {
		return
	}
	if err := database.DB.Create(&rule).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "创建分组策略失败")
		return
	}
	rule.Matchers = decodeAlertLabelMatchers(rule.MatchersJSON)
	rule.GroupBy = decodeAlertGroupingSelectors(rule.GroupByJSON)
	resp.Success(c, http.StatusOK, "创建成功", rule)
}

func UpdateAlertGroupingRule(c *gin.Context) {
	centerID, ok := requireFaultCenter(c)
	if !ok {
		return
	}
	ruleID, _ := strconv.Atoi(c.Param("ruleId"))
	var existing models.AlertGroupingRule
	if ruleID < 1 || database.DB.Where("id = ? AND fault_center_id = ?", ruleID, centerID).First(&existing).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "分组策略不存在")
		return
	}
	rule, ok := bindAlertGroupingRule(c, centerID, existing.ID)
	if !ok {
		return
	}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&existing).Updates(map[string]interface{}{
			"name": rule.Name, "matchers": rule.MatchersJSON, "group_by": rule.GroupByJSON,
			"wait_seconds": rule.WaitSeconds, "interval_seconds": rule.IntervalSeconds,
			"priority": rule.Priority, "status": rule.Status,
		}).Error; err != nil {
			return err
		}
		return resetAlertGroupingRuleState(tx, centerID, existing.ID)
	})
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "更新分组策略失败")
		return
	}
	resp.Success(c, http.StatusOK, "更新成功")
}

func DeleteAlertGroupingRule(c *gin.Context) {
	centerID, ok := requireFaultCenter(c)
	if !ok {
		return
	}
	ruleID, _ := strconv.Atoi(c.Param("ruleId"))
	var rule models.AlertGroupingRule
	if ruleID < 1 || database.DB.Where("id = ? AND fault_center_id = ?", ruleID, centerID).First(&rule).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "分组策略不存在")
		return
	}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := resetAlertGroupingRuleState(tx, centerID, rule.ID); err != nil {
			return err
		}
		return tx.Delete(&rule).Error
	})
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "删除分组策略失败")
		return
	}
	resp.Success(c, http.StatusOK, "删除成功")
}

func bindAlertGroupingRule(c *gin.Context, centerID, excludeID uint) (models.AlertGroupingRule, bool) {
	var req saveGroupingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "分组策略格式不正确")
		return models.AlertGroupingRule{}, false
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len([]rune(name)) > 128 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "策略名称不能为空且不能超过 128 个字符")
		return models.AlertGroupingRule{}, false
	}
	if req.WaitSeconds < 0 || req.WaitSeconds > 86400 || req.IntervalSeconds < 60 || req.IntervalSeconds > 86400 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "等待时间需在 0 到 86400 秒之间，间隔时间需在 60 到 86400 秒之间")
		return models.AlertGroupingRule{}, false
	}
	if req.Priority < 1 || req.Priority > 9999 || (req.Status != 0 && req.Status != 1) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "优先级或启用状态不正确")
		return models.AlertGroupingRule{}, false
	}
	matchers, err := normalizeAlertLabelMatchers(req.Matchers)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return models.AlertGroupingRule{}, false
	}
	groupBy, err := normalizeAlertGroupingSelectors(req.GroupBy)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return models.AlertGroupingRule{}, false
	}
	query := database.DB.Model(&models.AlertGroupingRule{}).Where("fault_center_id = ? AND name = ?", centerID, name)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	if query.Count(&count); count > 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "分组策略名称已存在")
		return models.AlertGroupingRule{}, false
	}
	return models.AlertGroupingRule{
		FaultCenterID: centerID, Name: name, MatchersJSON: encodeJSON(matchers), GroupByJSON: encodeJSON(groupBy),
		WaitSeconds: req.WaitSeconds, IntervalSeconds: req.IntervalSeconds, Priority: req.Priority, Status: req.Status,
	}, true
}

func resetAlertGroupingRuleState(tx *gorm.DB, centerID, ruleID uint) error {
	prefix := fmt.Sprintf("rule:%d", ruleID)
	waiting := []string{notificationStatusWaitingGroup, notificationStatusWaitingUpdate, notificationStatusWaitingRecovery}
	if err := tx.Model(&models.AlertInstance{}).
		Where("fault_center_id = ? AND notification_group_key LIKE ? AND notification_status IN ?", centerID, prefix+"%", waiting).
		Updates(map[string]interface{}{"notification_status": "", "notification_reason": "", "notification_group_key": "", "next_notification_at": nil}).Error; err != nil {
		return err
	}
	if err := tx.Model(&models.AlertInstance{}).
		Where("fault_center_id = ? AND notification_group_key LIKE ?", centerID, prefix+"%").
		Updates(map[string]interface{}{"notification_group_key": "", "next_notification_at": nil}).Error; err != nil {
		return err
	}
	return tx.Where("fault_center_id = ? AND grouping_rule_id = ?", centerID, ruleID).Delete(&models.AlertGroupState{}).Error
}

func ListAlertInhibitionRules(c *gin.Context) {
	centerID, ok := requireFaultCenter(c)
	if !ok {
		return
	}
	var rows []models.AlertInhibitionRule
	database.DB.Where("fault_center_id = ?", centerID).Order("id DESC").Find(&rows)
	for i := range rows {
		rows[i].SourceMatchers = decodeAlertLabelMatchers(rows[i].SourceMatchersJSON)
		rows[i].TargetMatchers = decodeAlertLabelMatchers(rows[i].TargetMatchersJSON)
		rows[i].EqualLabels = decodeAlertLabelSelectors(rows[i].EqualLabelsJSON)
	}
	resp.Success(c, http.StatusOK, "成功", rows)
}

func CreateAlertInhibitionRule(c *gin.Context) {
	centerID, ok := requireFaultCenter(c)
	if !ok {
		return
	}
	var req saveInhibitionRuleReq
	if err := c.ShouldBindJSON(&req); err != nil || validateInhibitionRule(req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请完整填写规则名称、源标签和目标标签")
		return
	}
	if inhibitionRuleNameExists(centerID, strings.TrimSpace(req.Name), 0) {
		resp.Error(c, http.StatusConflict, resp.CodeConflict, "抑制规则名称已存在")
		return
	}
	sourceMatchers, _ := normalizeAlertLabelMatchers(req.SourceMatchers)
	targetMatchers, _ := normalizeAlertLabelMatchers(req.TargetMatchers)
	equalLabels, _ := normalizeAlertLabelSelectors(req.EqualLabels)
	row := models.AlertInhibitionRule{FaultCenterID: centerID, Name: strings.TrimSpace(req.Name), SourceMatchersJSON: encodeJSON(sourceMatchers), TargetMatchersJSON: encodeJSON(targetMatchers), EqualLabelsJSON: encodeJSON(equalLabels), SourceMatchers: sourceMatchers, TargetMatchers: targetMatchers, EqualLabels: equalLabels, Status: req.Status}
	database.DB.Create(&row)
	reconcileFaultCenterNotificationSuppressions(centerID, time.Now())
	resp.Success(c, http.StatusOK, "抑制规则已创建", row)
}

func UpdateAlertInhibitionRule(c *gin.Context) {
	centerID, ok := requireFaultCenter(c)
	if !ok {
		return
	}
	ruleID, _ := strconv.Atoi(c.Param("ruleId"))
	var row models.AlertInhibitionRule
	if ruleID < 1 || database.DB.Where("id = ? AND fault_center_id = ?", ruleID, centerID).First(&row).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "抑制规则不存在")
		return
	}
	var req saveInhibitionRuleReq
	if err := c.ShouldBindJSON(&req); err != nil || validateInhibitionRule(req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请完整填写规则名称、源标签和目标标签")
		return
	}
	if inhibitionRuleNameExists(centerID, strings.TrimSpace(req.Name), row.ID) {
		resp.Error(c, http.StatusConflict, resp.CodeConflict, "抑制规则名称已存在")
		return
	}
	sourceMatchers, _ := normalizeAlertLabelMatchers(req.SourceMatchers)
	targetMatchers, _ := normalizeAlertLabelMatchers(req.TargetMatchers)
	equalLabels, _ := normalizeAlertLabelSelectors(req.EqualLabels)
	database.DB.Model(&row).Updates(map[string]interface{}{"name": strings.TrimSpace(req.Name), "source_matchers": encodeJSON(sourceMatchers), "target_matchers": encodeJSON(targetMatchers), "equal_labels_v2": encodeJSON(equalLabels), "status": req.Status})
	reconcileFaultCenterNotificationSuppressions(centerID, time.Now())
	resp.Success(c, http.StatusOK, "抑制规则已更新")
}

func DeleteAlertInhibitionRule(c *gin.Context) {
	centerID, ok := requireFaultCenter(c)
	if !ok {
		return
	}
	ruleID, _ := strconv.Atoi(c.Param("ruleId"))
	result := database.DB.Where("id = ? AND fault_center_id = ?", ruleID, centerID).Delete(&models.AlertInhibitionRule{})
	if result.RowsAffected == 0 {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "抑制规则不存在")
		return
	}
	reconcileFaultCenterNotificationSuppressions(centerID, time.Now())
	resp.Success(c, http.StatusOK, "抑制规则已删除")
}

func GetAlertNotificationConfig(c *gin.Context) {
	centerID, ok := requireFaultCenter(c)
	if !ok {
		return
	}
	var config models.AlertNotificationConfig
	result := database.DB.Where("fault_center_id = ?", centerID).Limit(1).Find(&config)
	defaultIDs := []uint{}
	if result.RowsAffected > 0 {
		defaultIDs = parseChannelIDs(config.DefaultChannelIDs)
	} else {
		config.RepeatIntervalSeconds = 3600
		config.RecoveryNotification = true
	}
	var routes []models.AlertNotificationRoute
	database.DB.Where("fault_center_id = ?", centerID).Order("id ASC").Find(&routes)
	resp.Success(c, http.StatusOK, "成功", notificationConfigResponse(config, defaultIDs, routes))
}

func UpdateAlertNotificationConfig(c *gin.Context) {
	centerID, ok := requireFaultCenter(c)
	if !ok {
		return
	}
	var req saveNotificationConfigReq
	if c.ShouldBindJSON(&req) != nil || req.RepeatIntervalSeconds < 60 || req.RepeatIntervalSeconds > 604800 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "通知配置参数不正确")
		return
	}
	allChannelIDs := append([]uint{}, req.DefaultChannelIDs...)
	seenRoutes := make(map[string]bool)
	for i := range req.Routes {
		route := &req.Routes[i]
		route.LabelNamespace, route.LabelKey, route.LabelValue = strings.TrimSpace(route.LabelNamespace), strings.TrimSpace(route.LabelKey), strings.TrimSpace(route.LabelValue)
		key := route.LabelNamespace + "\x00" + route.LabelKey + "\x00" + route.LabelValue
		if !validAlertLabelKey(route.LabelNamespace, route.LabelKey) || route.LabelValue == "" || len(route.ChannelIDs) == 0 || seenRoutes[key] || (route.Status != 0 && route.Status != 1) {
			resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请完整填写告警路由，且标签条件不能重复")
			return
		}
		seenRoutes[key] = true
		allChannelIDs = append(allChannelIDs, route.ChannelIDs...)
	}
	if !activeNotificationChannelsExist(allChannelIDs) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "通知对象不存在或已停用")
		return
	}
	defaultChannelIDs := normalizeChannelIDs(req.DefaultChannelIDs)
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var config models.AlertNotificationConfig
		result := tx.Where("fault_center_id = ?", centerID).Limit(1).Find(&config)
		values := map[string]interface{}{
			"default_channel_ids": strings.Join(uintStrings(defaultChannelIDs), ","), "repeat_interval_seconds": req.RepeatIntervalSeconds,
			"recovery_notification": req.RecoveryNotification,
		}
		if result.RowsAffected > 0 {
			if err := tx.Model(&config).Updates(values).Error; err != nil {
				return err
			}
		} else if err := tx.Create(&models.AlertNotificationConfig{FaultCenterID: centerID, DefaultChannelIDs: strings.Join(uintStrings(defaultChannelIDs), ","), RepeatIntervalSeconds: req.RepeatIntervalSeconds, RecoveryNotification: req.RecoveryNotification}).Error; err != nil {
			return err
		}
		if err := tx.Where("fault_center_id = ?", centerID).Delete(&models.AlertNotificationRoute{}).Error; err != nil {
			return err
		}
		for _, input := range req.Routes {
			route := models.AlertNotificationRoute{FaultCenterID: centerID, LabelNamespace: input.LabelNamespace, LabelKey: input.LabelKey, LabelValue: input.LabelValue, ChannelIDs: strings.Join(uintStrings(normalizeChannelIDs(input.ChannelIDs)), ","), Status: input.Status}
			if err := tx.Create(&route).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "保存通知配置失败")
		return
	}
	resp.Success(c, http.StatusOK, "通知配置已保存")
}

func notificationConfigResponse(config models.AlertNotificationConfig, defaultIDs []uint, routes []models.AlertNotificationRoute) gin.H {
	routeData := make([]gin.H, 0, len(routes))
	for _, route := range routes {
		routeData = append(routeData, gin.H{"id": route.ID, "label_namespace": route.LabelNamespace, "label_key": route.LabelKey, "label_value": route.LabelValue, "channel_ids": parseChannelIDs(route.ChannelIDs), "status": route.Status})
	}
	return gin.H{"default_channel_ids": defaultIDs, "repeat_interval_seconds": config.RepeatIntervalSeconds, "recovery_notification": config.RecoveryNotification, "routes": routeData, "updated_at": config.UpdatedAt}
}

func activeNotificationChannelsExist(ids []uint) bool {
	ids = normalizeChannelIDs(ids)
	if len(ids) == 0 {
		return true
	}
	var count int64
	database.DB.Model(&models.NotificationChannel{}).Where("id IN ? AND status = 1", ids).Count(&count)
	return count == int64(len(ids))
}

func normalizeChannelIDs(ids []uint) []uint {
	seen := make(map[uint]bool)
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

func uintStrings(ids []uint) []string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, strconv.FormatUint(uint64(id), 10))
	}
	return values
}

func parseChannelIDs(value string) []uint {
	ids := make([]uint, 0)
	for _, part := range strings.Split(value, ",") {
		parsed, err := strconv.ParseUint(strings.TrimSpace(part), 10, 64)
		if err == nil && parsed > 0 {
			ids = append(ids, uint(parsed))
		}
	}
	return normalizeChannelIDs(ids)
}

func notificationRouteChannelIDs(alert models.AlertInstance, routes []models.AlertNotificationRoute, defaultIDs []uint) []uint {
	labels := alertLabelNamespaces(alert)
	for _, route := range routes {
		if route.Status == 1 && labels[route.LabelNamespace][route.LabelKey] == route.LabelValue {
			return parseChannelIDs(route.ChannelIDs)
		}
	}
	return normalizeChannelIDs(defaultIDs)
}

func ListAlertSilenceRules(c *gin.Context) {
	centerID, ok := requireFaultCenter(c)
	if !ok {
		return
	}
	var rows []models.AlertSilenceRule
	database.DB.Where("fault_center_id = ?", centerID).Order("id DESC").Find(&rows)
	for i := range rows {
		rows[i].Matchers = decodeAlertLabelMatchers(rows[i].MatchersJSON)
		_ = json.Unmarshal([]byte(rows[i].WeekdaysJSON), &rows[i].Weekdays)
	}
	resp.Success(c, http.StatusOK, "成功", rows)
}

func CreateAlertSilenceRule(c *gin.Context) {
	centerID, ok := requireFaultCenter(c)
	if !ok {
		return
	}
	var req saveSilenceRuleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "静默策略格式不正确")
		return
	}
	rule, err := buildSilenceRule(centerID, req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	desiredStatus := rule.Status
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&rule).Error; err != nil {
			return err
		}
		if desiredStatus == 0 {
			return tx.Model(&rule).Update("status", 0).Error
		}
		return nil
	}); err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "创建静默策略失败")
		return
	}
	rule.Status = desiredStatus
	reconcileFaultCenterSilences(centerID, time.Now())
	resp.Success(c, http.StatusOK, "静默策略已创建", rule)
}

func UpdateAlertSilenceRule(c *gin.Context) {
	centerID, ok := requireFaultCenter(c)
	if !ok {
		return
	}
	ruleID, _ := strconv.Atoi(c.Param("ruleId"))
	var existing models.AlertSilenceRule
	if ruleID < 1 || database.DB.Where("id = ? AND fault_center_id = ?", ruleID, centerID).First(&existing).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "静默策略不存在")
		return
	}
	var req saveSilenceRuleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "静默策略格式不正确")
		return
	}
	rule, err := buildSilenceRule(centerID, req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if err := database.DB.Model(&existing).Updates(map[string]interface{}{
		"name": rule.Name, "matchers": rule.MatchersJSON, "silence_type": rule.SilenceType,
		"weekdays": rule.WeekdaysJSON, "start_time": rule.StartTime, "end_time": rule.EndTime,
		"start_at": rule.StartAt, "end_at": rule.EndAt, "status": rule.Status,
	}).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "更新静默策略失败")
		return
	}
	reconcileFaultCenterSilences(centerID, time.Now())
	resp.Success(c, http.StatusOK, "静默策略已更新")
}

func DeleteAlertSilenceRule(c *gin.Context) {
	centerID, ok := requireFaultCenter(c)
	if !ok {
		return
	}
	ruleID, _ := strconv.Atoi(c.Param("ruleId"))
	result := database.DB.Where("id = ? AND fault_center_id = ?", ruleID, centerID).Delete(&models.AlertSilenceRule{})
	if result.RowsAffected == 0 {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "静默策略不存在")
		return
	}
	reconcileFaultCenterSilences(centerID, time.Now())
	resp.Success(c, http.StatusOK, "静默策略已删除")
}

func buildSilenceRule(centerID uint, req saveSilenceRuleReq) (models.AlertSilenceRule, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" || len([]rune(name)) > 128 {
		return models.AlertSilenceRule{}, fmt.Errorf("策略名称不能为空且不能超过 128 个字符")
	}
	if req.SilenceType != "once" && req.SilenceType != "weekly" {
		return models.AlertSilenceRule{}, fmt.Errorf("静默类型不正确")
	}
	status := 1
	if req.Status != nil {
		if *req.Status != 0 && *req.Status != 1 {
			return models.AlertSilenceRule{}, fmt.Errorf("启用状态不正确")
		}
		status = *req.Status
	}
	if req.SilenceType == "once" {
		if req.StartAt == nil || req.EndAt == nil || req.StartAt.IsZero() || req.EndAt.IsZero() || !req.EndAt.After(*req.StartAt) || !req.EndAt.After(time.Now()) {
			return models.AlertSilenceRule{}, fmt.Errorf("请选择正确的静默时段")
		}
	} else {
		req.StartAt, req.EndAt = nil, nil
	}
	matchers, err := normalizeAlertLabelMatchers(req.Matchers)
	if err != nil {
		return models.AlertSilenceRule{}, err
	}
	if len(matchers) > 10 {
		return models.AlertSilenceRule{}, fmt.Errorf("匹配标签最多配置 10 组")
	}
	if len(matchers) == 0 {
		return models.AlertSilenceRule{}, fmt.Errorf("至少配置一组匹配标签")
	}
	for _, matcher := range matchers {
		if matcher.Namespace == "system_labels" && matcher.Key == "severity" {
			return models.AlertSilenceRule{}, fmt.Errorf("告警静默不支持按告警等级匹配")
		}
	}
	weekdays, weekdaysJSON, startTime, endTime, err := normalizeSilenceSchedule(req.SilenceType, req.Weekdays, req.StartTime, req.EndTime)
	if err != nil {
		return models.AlertSilenceRule{}, err
	}
	return models.AlertSilenceRule{FaultCenterID: centerID, Name: name, MatchersJSON: encodeJSON(matchers), Matchers: matchers, SilenceType: req.SilenceType, WeekdaysJSON: weekdaysJSON, Weekdays: weekdays, StartTime: startTime, EndTime: endTime, StartAt: req.StartAt, EndAt: req.EndAt, Status: status}, nil
}

func normalizeSilenceSchedule(silenceType string, weekdays []int, startTime, endTime string) ([]int, string, string, string, error) {
	if silenceType != "weekly" {
		return []int{}, "[]", "", "", nil
	}
	startTime, endTime = strings.TrimSpace(startTime), strings.TrimSpace(endTime)
	if startTime == "" || endTime == "" {
		return nil, "", "", "", fmt.Errorf("请选择每日静默时间")
	}
	if _, err := time.Parse("15:04", startTime); err != nil {
		return nil, "", "", "", fmt.Errorf("每日静默时间格式不正确")
	}
	if _, err := time.Parse("15:04", endTime); err != nil {
		return nil, "", "", "", fmt.Errorf("每日静默时间格式不正确")
	}
	if startTime == endTime {
		return nil, "", "", "", fmt.Errorf("每日静默的开始和结束时间不能相同")
	}
	selected := make(map[int]bool, len(weekdays))
	for _, weekday := range weekdays {
		if weekday < 1 || weekday > 7 {
			return nil, "", "", "", fmt.Errorf("生效星期不正确")
		}
		selected[weekday] = true
	}
	normalized := make([]int, 0, len(selected))
	for weekday := 1; weekday <= 7; weekday++ {
		if selected[weekday] {
			normalized = append(normalized, weekday)
		}
	}
	if len(normalized) == 0 {
		return nil, "", "", "", fmt.Errorf("请至少选择一个生效星期")
	}
	encoded, _ := json.Marshal(normalized)
	return normalized, string(encoded), startTime, endTime, nil
}

func SilenceAlert(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var row models.AlertInstance
	if id < 1 || database.DB.First(&row, id).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "告警不存在")
		return
	}
	if row.Status == alertStatusResolved {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "已恢复告警不能静默")
		return
	}
	var req silenceAlertReq
	if err := c.ShouldBindJSON(&req); err != nil || req.DurationMinutes < 5 || req.DurationMinutes > 10080 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "静默时长需在 5 分钟到 7 天之间")
		return
	}
	until := time.Now().Add(time.Duration(req.DurationMinutes) * time.Minute)
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "人工静默"
	}
	database.DB.Model(&row).Updates(map[string]interface{}{"silenced_until": until, "silence_reason": reason, "silence_rule_id": nil, "notification_status": notificationStatusSilenced, "notification_reason": reason, "next_notification_at": nil})
	_ = recordAlertEvent(database.DB, row, "silenced", row.Status, row.Status, row.Severity, row.Severity, strings.TrimSpace(req.Reason), "", row.LastProbeStatus, time.Now())
	resp.Success(c, http.StatusOK, "告警已静默", gin.H{"silenced_until": until})
}

func UnsilenceAlert(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var alert models.AlertInstance
	if id < 1 || database.DB.First(&alert, id).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "告警不存在")
		return
	}
	result := database.DB.Model(&alert).Updates(map[string]interface{}{"silenced_until": nil, "silence_reason": "", "silence_rule_id": nil})
	if result.RowsAffected == 0 {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "告警不存在")
		return
	}
	alert.SilencedUntil, alert.SilenceRuleID, alert.SilenceReason = nil, nil, ""
	reconcileAlertNotificationSuppression(&alert, time.Now())
	_ = recordAlertEvent(database.DB, alert, "unsilenced", alert.Status, alert.Status, alert.Severity, alert.Severity, "manual_unsilence", "", alert.LastProbeStatus, time.Now())
	resp.Success(c, http.StatusOK, "静默已解除")
}

func requireFaultCenter(c *gin.Context) (uint, bool) {
	id, _ := strconv.Atoi(c.Param("id"))
	var count int64
	database.DB.Model(&models.FaultCenter{}).Where("id = ?", id).Count(&count)
	if id < 1 || count == 0 {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "故障中心不存在")
		return 0, false
	}
	return uint(id), true
}

func validateInhibitionRule(req saveInhibitionRuleReq) error {
	if strings.TrimSpace(req.Name) == "" || (req.Status != 0 && req.Status != 1) {
		return fmt.Errorf("invalid inhibition rule")
	}
	if source, err := normalizeAlertLabelMatchers(req.SourceMatchers); err != nil || len(source) == 0 {
		return fmt.Errorf("invalid source matchers")
	}
	if target, err := normalizeAlertLabelMatchers(req.TargetMatchers); err != nil || len(target) == 0 {
		return fmt.Errorf("invalid target matchers")
	}
	if _, err := normalizeAlertLabelSelectors(req.EqualLabels); err != nil {
		return err
	}
	return nil
}

func inhibitionRuleNameExists(centerID uint, name string, excludeID uint) bool {
	query := database.DB.Model(&models.AlertInhibitionRule{}).Where("fault_center_id = ? AND name = ?", centerID, name)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	query.Count(&count)
	return count > 0
}

func validAlertLabelNamespace(namespace string) bool {
	switch namespace {
	case "system_labels", "labels", "datasource_labels", "external_labels":
		return true
	default:
		return false
	}
}

func validAlertLabelKey(namespace, key string) bool {
	if !validAlertLabelNamespace(namespace) || key == "" || len(key) > 64 {
		return false
	}
	if namespace != "system_labels" {
		return true
	}
	switch key {
	case "alertname", "severity", "datasource":
		return true
	default:
		return false
	}
}

func normalizeAlertLabelMatchers(values []models.AlertLabelMatcher) ([]models.AlertLabelMatcher, error) {
	if len(values) > 10 {
		return nil, fmt.Errorf("匹配标签最多配置 10 组")
	}
	result := make([]models.AlertLabelMatcher, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value.Namespace = strings.TrimSpace(value.Namespace)
		value.Key = strings.TrimSpace(value.Key)
		value.Value = strings.TrimSpace(value.Value)
		identity := value.Namespace + "\x00" + value.Key
		if !validAlertLabelKey(value.Namespace, value.Key) || value.Value == "" || len(value.Value) > 255 || seen[identity] {
			return nil, fmt.Errorf("匹配标签的来源、键和值不正确或存在重复")
		}
		seen[identity] = true
		result = append(result, value)
	}
	return result, nil
}

func normalizeAlertLabelSelectors(values []models.AlertLabelSelector) ([]models.AlertLabelSelector, error) {
	if len(values) > 10 {
		return nil, fmt.Errorf("相等标签最多配置 10 组")
	}
	result := make([]models.AlertLabelSelector, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value.Namespace = strings.TrimSpace(value.Namespace)
		value.Key = strings.TrimSpace(value.Key)
		identity := value.Namespace + "\x00" + value.Key
		if !validAlertLabelKey(value.Namespace, value.Key) || seen[identity] {
			return nil, fmt.Errorf("相等标签的来源和键不正确或存在重复")
		}
		seen[identity] = true
		result = append(result, value)
	}
	return result, nil
}

func normalizeAlertGroupingSelectors(values []models.AlertLabelSelector) ([]models.AlertLabelSelector, error) {
	if len(values) > 10 {
		return nil, fmt.Errorf("分组标签最多配置 10 个")
	}
	result := make([]models.AlertLabelSelector, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value.Namespace = strings.TrimSpace(value.Namespace)
		value.Key = strings.TrimSpace(value.Key)
		identity := value.Namespace + "\x00" + value.Key
		if !validAlertLabelKey(value.Namespace, value.Key) || seen[identity] {
			return nil, fmt.Errorf("分组标签的来源和 Key 不正确或存在重复")
		}
		seen[identity] = true
		result = append(result, value)
	}
	return result, nil
}

func encodeJSON(value interface{}) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func decodeAlertLabelMatchers(value string) []models.AlertLabelMatcher {
	var result []models.AlertLabelMatcher
	if json.Unmarshal([]byte(value), &result) != nil {
		return []models.AlertLabelMatcher{}
	}
	return result
}

func decodeAlertLabelSelectors(value string) []models.AlertLabelSelector {
	var result []models.AlertLabelSelector
	if json.Unmarshal([]byte(value), &result) != nil {
		return []models.AlertLabelSelector{}
	}
	return result
}

func decodeAlertGroupingSelectors(value string) []models.AlertLabelSelector {
	value = strings.TrimSpace(value)
	if value == "" {
		return []models.AlertLabelSelector{}
	}
	var selectors []models.AlertLabelSelector
	if json.Unmarshal([]byte(value), &selectors) == nil {
		return selectors
	}
	return []models.AlertLabelSelector{}
}

func alertLabelNamespaces(alert models.AlertInstance) map[string]map[string]string {
	namespaces := map[string]map[string]string{
		"system_labels": {}, "labels": {}, "datasource_labels": {}, "external_labels": {},
	}
	var snapshot struct {
		AlertName        string            `json:"alertname"`
		Datasource       string            `json:"datasource"`
		Severity         string            `json:"severity"`
		Labels           map[string]string `json:"labels"`
		DatasourceLabels map[string]string `json:"datasource_labels"`
		ExternalLabels   map[string]string `json:"external_labels"`
	}
	if json.Unmarshal([]byte(alert.LastResult), &snapshot) == nil {
		if snapshot.Labels != nil {
			namespaces["labels"] = snapshot.Labels
		}
		if snapshot.DatasourceLabels != nil {
			namespaces["datasource_labels"] = snapshot.DatasourceLabels
		}
		if snapshot.ExternalLabels != nil {
			namespaces["external_labels"] = snapshot.ExternalLabels
		}
	}
	if alert.Source != alertRuleSource && len(namespaces["labels"]) == 0 {
		namespaces["labels"] = parseInspectionLabels(alert.Labels)
	}
	for _, key := range []string{"alertname", "severity", "datasource"} {
		delete(namespaces["labels"], key)
		delete(namespaces["datasource_labels"], key)
		delete(namespaces["external_labels"], key)
	}
	alertName := strings.TrimSpace(snapshot.AlertName)
	if alertName == "" {
		alertName = strings.TrimSpace(alert.Title)
	}
	severity := strings.TrimSpace(alert.Severity)
	if severity == "" {
		severity = strings.TrimSpace(snapshot.Severity)
	}
	for key, value := range map[string]string{"alertname": alertName, "severity": severity, "datasource": strings.TrimSpace(snapshot.Datasource)} {
		if value != "" {
			namespaces["system_labels"][key] = value
		}
	}
	return namespaces
}

func buildTaskAlertSnapshot(source, name, taskType, target, status, result, description, externalLabels string) (string, string) {
	labels := map[string]string{
		"alertname": name,
		"task_name": name,
		"task_type": taskType,
		"target":    target,
	}
	encodedLabels, _ := json.Marshal(labels)
	snapshot, _ := json.Marshal(gin.H{
		"alertname":         name,
		"description":       description,
		"source":            source,
		"status":            status,
		"result":            result,
		"labels":            labels,
		"datasource_labels": map[string]string{},
		"external_labels":   parseInspectionLabels(externalLabels),
	})
	return string(encodedLabels), string(snapshot)
}

func inhibitionEqualLabelsMatch(source, target models.AlertInstance, equalLabels []models.AlertLabelSelector) bool {
	if len(equalLabels) == 0 {
		return true
	}
	sourceLabels := alertLabelNamespaces(source)
	targetLabels := alertLabelNamespaces(target)
	for _, selector := range equalLabels {
		sourceValue, sourceOK := sourceLabels[selector.Namespace][selector.Key]
		targetValue, targetOK := targetLabels[selector.Namespace][selector.Key]
		if !sourceOK || !targetOK || sourceValue != targetValue {
			return false
		}
	}
	return true
}

func inspectionAlertNotificationAllowed(item models.InspectionItem, now time.Time) (bool, string) {
	if item.FaultCenterID == 0 {
		return true, ""
	}
	var center models.FaultCenter
	if database.DB.First(&center, item.FaultCenterID).Error != nil || center.Status != 1 {
		return false, ""
	}
	var alert models.AlertInstance
	if database.DB.Where("fault_center_id = ? AND source = ? AND source_id = ? AND status IN ?", item.FaultCenterID, "probe-task", item.ID, []string{alertStatusPending, alertStatusFiring, alertStatusRecovering}).Order("id DESC").First(&alert).Error != nil {
		return true, ""
	}
	return alertInstanceNotificationAllowed(&alert, item.AlertActive, now)
}

func alertInstanceNotificationAllowed(alert *models.AlertInstance, repeat bool, now time.Time) (bool, string) {
	_ = repeat
	if alert == nil || alert.FaultCenterID == 0 {
		return true, ""
	}
	var center models.FaultCenter
	if database.DB.First(&center, alert.FaultCenterID).Error != nil || center.Status != 1 {
		return false, ""
	}
	if status, reason := alertNotificationSuppression(alert, now); status != "" {
		updateAlertNotificationState(alert, status, reason, alert.NotificationGroupKey, nil)
		return false, ""
	}
	return true, ""
}

func groupNotificationDue(state models.AlertGroupState, waitSeconds, intervalSeconds int, now time.Time) bool {
	if state.LastNotifiedAt == nil {
		firstSeenAt := state.FirstSeenAt
		if firstSeenAt.IsZero() {
			firstSeenAt = state.CreatedAt
		}
		return !firstSeenAt.IsZero() && !now.Before(firstSeenAt.Add(time.Duration(waitSeconds)*time.Second))
	}
	return !now.Before(state.LastNotifiedAt.Add(time.Duration(intervalSeconds) * time.Second))
}

func markAlertGroupNotified(centerID uint, groupKey string, notifiedAt time.Time) {
	if centerID == 0 || groupKey == "" {
		return
	}
	var state models.AlertGroupState
	result := database.DB.Where("fault_center_id = ? AND group_key = ?", centerID, groupKey).First(&state)
	if result.Error == nil {
		database.DB.Model(&state).Update("last_notified_at", notifiedAt)
		return
	}
	database.DB.Create(&models.AlertGroupState{FaultCenterID: centerID, GroupKey: groupKey, FirstSeenAt: notifiedAt, LastNotifiedAt: &notifiedAt})
}

func alertMatchesLabelMatchers(alert models.AlertInstance, matchers []models.AlertLabelMatcher) bool {
	if len(matchers) == 0 {
		return false
	}
	labels := alertLabelNamespaces(alert)
	for _, matcher := range matchers {
		if labels[matcher.Namespace][matcher.Key] != matcher.Value {
			return false
		}
	}
	return true
}

func silenceRuleMatchesAlert(rule models.AlertSilenceRule, alert models.AlertInstance) bool {
	matchers := rule.Matchers
	if len(matchers) == 0 {
		matchers = decodeAlertLabelMatchers(rule.MatchersJSON)
	}
	return alertMatchesLabelMatchers(alert, matchers)
}

func silenceRuleActiveAt(rule models.AlertSilenceRule, now time.Time) bool {
	if rule.Status != 1 {
		return false
	}
	if rule.SilenceType != "weekly" {
		return rule.StartAt != nil && rule.EndAt != nil && !now.Before(*rule.StartAt) && now.Before(*rule.EndAt)
	}
	startClock, startErr := time.Parse("15:04", rule.StartTime)
	endClock, endErr := time.Parse("15:04", rule.EndTime)
	if startErr != nil || endErr != nil {
		return false
	}
	nowSecond := now.Hour()*3600 + now.Minute()*60 + now.Second()
	startSecond := startClock.Hour()*3600 + startClock.Minute()*60
	endSecond := endClock.Hour()*3600 + endClock.Minute()*60
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	if endSecond < startSecond && nowSecond < endSecond {
		weekday--
		if weekday == 0 {
			weekday = 7
		}
	}
	weekdays := rule.Weekdays
	if len(weekdays) == 0 {
		_ = json.Unmarshal([]byte(rule.WeekdaysJSON), &weekdays)
	}
	selected := false
	for _, item := range weekdays {
		if item == weekday {
			selected = true
			break
		}
	}
	if !selected {
		return false
	}
	if endSecond > startSecond {
		return nowSecond >= startSecond && nowSecond < endSecond
	}
	return nowSecond >= startSecond || nowSecond < endSecond
}

func silenceRulePeriodEnd(rule models.AlertSilenceRule, now time.Time) time.Time {
	if rule.SilenceType != "weekly" {
		if rule.EndAt != nil {
			return *rule.EndAt
		}
		return now
	}
	startClock, startErr := time.Parse("15:04", rule.StartTime)
	endClock, endErr := time.Parse("15:04", rule.EndTime)
	if startErr != nil || endErr != nil {
		return now
	}
	periodEnd := time.Date(now.Year(), now.Month(), now.Day(), endClock.Hour(), endClock.Minute(), 0, 0, now.Location())
	startSecond, endSecond := startClock.Hour()*3600+startClock.Minute()*60, endClock.Hour()*3600+endClock.Minute()*60
	nowSecond := now.Hour()*3600 + now.Minute()*60 + now.Second()
	if endSecond <= startSecond && nowSecond >= startSecond {
		periodEnd = periodEnd.AddDate(0, 0, 1)
	}
	return periodEnd
}

func activeSilenceRuleForAlert(centerID uint, alert models.AlertInstance, now time.Time) (*models.AlertSilenceRule, time.Time) {
	var rules []models.AlertSilenceRule
	database.DB.Where("fault_center_id = ? AND status = 1", centerID).Order("id DESC").Find(&rules)
	for i := range rules {
		if silenceRuleActiveAt(rules[i], now) && silenceRuleMatchesAlert(rules[i], alert) {
			return &rules[i], silenceRulePeriodEnd(rules[i], now)
		}
	}
	return nil, time.Time{}
}

func applyActiveSilenceRule(alert *models.AlertInstance, now time.Time) {
	if alert == nil || alert.ID == 0 {
		return
	}
	if alert.SilenceRuleID == nil && alert.SilencedUntil != nil && alert.SilencedUntil.After(now) {
		return
	}
	rule, until := activeSilenceRuleForAlert(alert.FaultCenterID, *alert, now)
	if rule == nil {
		if alert.SilenceRuleID != nil {
			database.DB.Model(alert).Updates(map[string]interface{}{"silenced_until": nil, "silence_reason": "", "silence_rule_id": nil})
			alert.SilencedUntil, alert.SilenceRuleID, alert.SilenceReason = nil, nil, ""
		}
		return
	}
	database.DB.Model(alert).Updates(map[string]interface{}{"silenced_until": until, "silence_reason": rule.Name, "silence_rule_id": rule.ID})
	alert.SilencedUntil, alert.SilenceRuleID, alert.SilenceReason = &until, &rule.ID, rule.Name
}

func reconcileFaultCenterSilences(centerID uint, now time.Time) {
	reconcileFaultCenterNotificationSuppressions(centerID, now)
}

func reconcileFaultCenterNotificationSuppressions(centerID uint, now time.Time) {
	var alerts []models.AlertInstance
	database.DB.Where("fault_center_id = ? AND status IN ?", centerID, []string{alertStatusPending, alertStatusFiring, alertStatusRecovering}).Find(&alerts)
	for i := range alerts {
		reconcileAlertNotificationSuppression(&alerts[i], now)
	}
}

func reconcileAlertNotificationSuppression(alert *models.AlertInstance, now time.Time) {
	if alert == nil || alert.ID == 0 {
		return
	}
	previousStatus := alert.NotificationStatus
	if status, reason := alertNotificationSuppression(alert, now); status != "" {
		updateAlertNotificationState(alert, status, reason, alert.NotificationGroupKey, nil)
		return
	}
	if previousStatus != notificationStatusSilenced && previousStatus != notificationStatusInhibited {
		return
	}
	if alert.Status == alertStatusFiring || alert.Status == alertStatusRecovering {
		config := loadAlertRuleNotificationConfig(alert.FaultCenterID)
		if queueGroupedFiringAlert(alert, config, now) {
			return
		}
	}
	status, reason := "", "等待下一次告警评估"
	if alert.Status == alertStatusPending {
		reason = "尚未达到告警持续时间"
	} else if alert.LastNotifiedAt != nil {
		status, reason = notificationStatusSent, "最近一次通知已发送"
	}
	updateAlertNotificationState(alert, status, reason, alert.NotificationGroupKey, nil)
}

func nextInspectionAlertState(current string, success bool, failures, threshold int, recoveryDue bool) string {
	if success {
		if current == alertStatusPending || ((current == alertStatusFiring || current == alertStatusRecovering) && recoveryDue) {
			return alertStatusResolved
		}
		if current == alertStatusFiring || current == alertStatusRecovering {
			return alertStatusRecovering
		}
		return current
	}
	if current == alertStatusRecovering {
		return alertStatusFiring
	}
	if failures >= normalizeFailureThreshold(threshold) {
		return alertStatusFiring
	}
	return alertStatusPending
}

func applyAlertStateTransition(updates map[string]interface{}, previous, next string, now time.Time) {
	if previous == next {
		return
	}
	updates["state_changed_at"] = now
	if next == alertStatusFiring && previous != alertStatusRecovering {
		updates["fired_at"] = now
	}
	if next == alertStatusResolved {
		updates["resolved_at"] = now
	}
}

func inspectionAlertFingerprint(itemID uint, node inspectionExecutionNode) string {
	return fmt.Sprintf("probe-task:%d:%s", itemID, inspectionNodeKey(node))
}

func reconcileInspectionNodeAlerts(item models.InspectionItem, nodes []inspectionExecutionNode, now time.Time) {
	expectedFingerprints, expectedNodeKeys := map[string]struct{}{}, make([]string, 0, len(nodes))
	for _, node := range nodes {
		expectedFingerprints[inspectionAlertFingerprint(item.ID, node)] = struct{}{}
		expectedNodeKeys = append(expectedNodeKeys, inspectionNodeKey(node))
	}
	var alerts []models.AlertInstance
	database.DB.Where("source = ? AND source_id = ? AND status IN ?", "probe-task", item.ID, []string{alertStatusPending, alertStatusFiring, alertStatusRecovering}).Find(&alerts)
	for index := range alerts {
		alert := &alerts[index]
		if _, keep := expectedFingerprints[alert.Fingerprint]; keep {
			continue
		}
		previous := alert.Status
		database.DB.Model(alert).Updates(map[string]interface{}{"status": alertStatusTerminated, "resolved_at": now, "state_changed_at": now, "last_event_at": now, "notification_status": notificationStatusTerminated, "notification_reason": "执行节点已移除", "next_notification_at": nil})
		alert.Status, alert.ResolvedAt = alertStatusTerminated, &now
		_ = recordAlertEvent(database.DB, *alert, "state_changed", previous, alertStatusTerminated, alert.Severity, alert.Severity, "probe_node_removed", "", alert.LastProbeStatus, now)
	}
	query := database.DB.Where("item_id = ?", item.ID)
	if len(expectedNodeKeys) > 0 {
		query = query.Where("node_key NOT IN ?", expectedNodeKeys)
	}
	query.Delete(&models.InspectionNodeState{})
}

func buildInspectionNodeAlertSnapshot(item models.InspectionItem, node inspectionExecutionNode, probeStatus, result string) (string, string) {
	labels := map[string]string{"alertname": item.Name, "task_name": item.Name, "task_type": item.Type, "target": item.Target, "probe_node": node.NodeName, "probe_node_type": node.NodeType}
	encodedLabels, _ := json.Marshal(labels)
	snapshot, _ := json.Marshal(gin.H{"alertname": item.Name, "source": "probe-task", "status": probeStatus, "result": result, "labels": labels, "datasource_labels": map[string]string{}, "external_labels": parseInspectionLabels(item.Labels)})
	return string(encodedLabels), string(snapshot)
}

func syncInspectionAlert(item models.InspectionItem, node inspectionExecutionNode, probeStatus, result string, failures int, recoveryDue bool, now time.Time) (string, string, error) {
	runtimeLabels, snapshot := buildInspectionNodeAlertSnapshot(item, node, probeStatus, textutil.Ellipsis(result, 4000))
	fingerprint := inspectionAlertFingerprint(item.ID, node)
	var alert models.AlertInstance
	tx := database.DB.Where("source = ? AND source_id = ? AND fingerprint = ? AND status IN ?", "probe-task", item.ID, fingerprint, []string{alertStatusPending, alertStatusFiring, alertStatusRecovering}).
		Order("id DESC").Limit(1).Find(&alert)
	if tx.Error != nil {
		return "", "", tx.Error
	}

	previous := alert.Status
	success := probeStatus == "success"
	if tx.RowsAffected == 0 {
		if success {
			return "", "", nil
		}
		state := nextInspectionAlertState("", false, failures, item.FailureThreshold, false)
		alert = models.AlertInstance{
			FaultCenterID: item.FaultCenterID, Source: "probe-task", SourceID: item.ID, Fingerprint: fingerprint, Title: fmt.Sprintf("%s · %s", item.Name, node.NodeName), Target: item.Target,
			Severity: normalizeInspectionSeverity(item.Severity), Status: state, Labels: runtimeLabels,
			FailureCount: failures, FailureThreshold: normalizeFailureThreshold(item.FailureThreshold),
			LastProbeStatus: probeStatus, LastResult: snapshot,
			StartedAt: now, StateChangedAt: now, LastEventAt: now,
		}
		if state == alertStatusFiring {
			alert.FiredAt = &now
		}
		if err := database.DB.Create(&alert).Error; err != nil {
			return previous, state, err
		}
		_ = recordAlertEvent(database.DB, alert, "created", "", state, "", alert.Severity, "probe_failed", result, probeStatus, now)
		applyActiveSilenceRule(&alert, now)
		return previous, state, nil
	}

	state := nextInspectionAlertState(previous, success, failures, item.FailureThreshold, recoveryDue)
	updates := map[string]interface{}{
		"fault_center_id": item.FaultCenterID, "title": fmt.Sprintf("%s · %s", item.Name, node.NodeName), "target": item.Target, "severity": normalizeInspectionSeverity(item.Severity), "labels": runtimeLabels,
		"status": state, "failure_count": failures,
		"failure_threshold": normalizeFailureThreshold(item.FailureThreshold),
		"last_probe_status": probeStatus, "last_result": snapshot,
		"last_event_at": now,
	}
	applyAlertStateTransition(updates, previous, state, now)
	if err := database.DB.Model(&alert).Updates(updates).Error; err != nil {
		return previous, state, err
	}
	alert.Labels, alert.LastResult, alert.FaultCenterID = runtimeLabels, snapshot, item.FaultCenterID
	alert.Severity = normalizeInspectionSeverity(item.Severity)
	alert.Status, alert.LastEventAt = state, now
	if state != previous {
		alert.StateChangedAt = now
		reason := "probe_state_changed"
		if state == alertStatusResolved {
			reason = "probe_recovered"
		}
		_ = recordAlertEvent(database.DB, alert, "state_changed", previous, state, alert.Severity, alert.Severity, reason, result, probeStatus, now)
	}
	applyActiveSilenceRule(&alert, now)
	return previous, state, nil
}
