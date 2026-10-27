package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/logger"
	"guanlan/pkg/resp"
	"guanlan/pkg/textutil"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const alertRuleSource = "prometheus-rule"

var (
	alertRuleQueue                     = make(chan uint, 4096)
	alertRuleWorkerOnce                sync.Once
	alertRuleInFlight                  sync.Map
	promLabelTemplatePattern           = regexp.MustCompile(`(?:\{\{\s*\$labels\.([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}|\$\{labels\.([a-zA-Z_][a-zA-Z0-9_]*)\})`)
	promDatasourceLabelTemplatePattern = regexp.MustCompile(`(?:\{\{\s*\$datasourceLabels\.([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}|\$\{datasource_labels\.([a-zA-Z_][a-zA-Z0-9_]*)\})`)
	promExternalLabelTemplatePattern   = regexp.MustCompile(`(?:\{\{\s*\$externalLabels\.([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}|\$\{external_labels\.([a-zA-Z_][a-zA-Z0-9_]*)\})`)
)

const alertRuleWorkerCount = 8

type saveAlertRuleReq struct {
	GroupID                   uint                        `json:"group_id"`
	Name                      string                      `json:"name"`
	Description               string                      `json:"description"`
	Expression                string                      `json:"expression"`
	DatasourceIDs             []uint                      `json:"datasource_ids"`
	FaultCenterID             uint                        `json:"fault_center_id"`
	ForSeconds                int                         `json:"for_seconds"`
	RecoveryWaitSeconds       int                         `json:"recovery_wait_seconds"`
	EvaluationIntervalSeconds int                         `json:"evaluation_interval_seconds"`
	Severity                  string                      `json:"severity"`
	ExternalLabels            map[string]string           `json:"external_labels"`
	Annotations               map[string]string           `json:"annotations"`
	DatasourceGroupIDs        []uint                      `json:"datasource_group_ids"`
	Status                    int                         `json:"status"`
	Conditions                []saveAlertRuleConditionReq `json:"conditions"`
}

type saveAlertRuleConditionReq struct {
	Severity  string   `json:"severity"`
	Operator  string   `json:"operator"`
	Threshold *float64 `json:"threshold"`
}

type alertRuleSelection struct {
	DirectDatasourceIDs []uint
	DatasourceGroupIDs  []uint
	EffectiveIDs        []uint
	Conditions          []models.AlertRuleCondition
}

type promVectorSeries struct {
	Labels       map[string]string
	Value        string
	ConditionID  uint
	ConditionKey string
	Severity     string
	ForSeconds   int
	Condition    string
}

type alertEvaluationAction string

const (
	alertEvaluationSync alertEvaluationAction = "sync"
	alertEvaluationKeep alertEvaluationAction = "keep"
)

func alertRuleEvaluationAction(status string) alertEvaluationAction {
	if status == "success" || status == "no_data" {
		return alertEvaluationSync
	}
	return alertEvaluationKeep
}

func ListAlertRules(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	status := c.Query("status")
	datasourceID, _ := strconv.Atoi(c.Query("datasource_id"))
	groupID, _ := strconv.Atoi(c.Query("group_id"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	query := database.DB.Model(&models.AlertRule{})
	if groupID > 0 {
		query = query.Where("group_id = ?", groupID)
	}
	if keyword != "" {
		query = query.Where("name LIKE ? OR expression LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if status == "enabled" {
		query = query.Where("status = 1")
	} else if status == "disabled" {
		query = query.Where("status = 0")
	}
	if datasourceID > 0 {
		query = query.Where("id IN (?)", database.DB.Model(&models.AlertRuleBinding{}).Select("rule_id").Where("datasource_id = ?", datasourceID))
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "查询告警规则失败")
		return
	}
	var rows []models.AlertRule
	if err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "查询告警规则失败")
		return
	}
	list := make([]gin.H, 0, len(rows))
	for i := range rows {
		list = append(list, alertRuleResponse(rows[i]))
	}
	resp.Success(c, http.StatusOK, "成功", gin.H{
		"list": list, "total": total, "page": page, "page_size": pageSize,
	})
}

func GetAlertRule(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var rule models.AlertRule
	if id < 1 || database.DB.First(&rule, id).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "告警规则不存在")
		return
	}
	resp.Success(c, http.StatusOK, "成功", alertRuleResponse(rule))
}

func CreateAlertRule(c *gin.Context) {
	var req saveAlertRuleReq
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "告警规则格式不正确")
		return
	}
	rule, selection, err := buildAlertRule(req, 0)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	requestedStatus := rule.Status
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&rule).Error; err != nil {
			return err
		}
		// GORM applies the model's default tag to zero values on Create. Restore an
		// explicitly requested disabled state before creating scheduler bindings.
		if requestedStatus == 0 {
			if err := tx.Model(&rule).UpdateColumn("status", 0).Error; err != nil {
				return err
			}
			rule.Status = 0
		}
		if err := replaceAlertRuleSelections(tx, rule.ID, selection); err != nil {
			return err
		}
		if err := replaceAlertRuleConditions(tx, rule.ID, selection.Conditions); err != nil {
			return err
		}
		return replaceAlertRuleBindings(tx, rule.ID, selection.EffectiveIDs)
	})
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "创建告警规则失败")
		return
	}
	resp.Success(c, http.StatusOK, "告警规则已创建", alertRuleResponse(rule))
}

func UpdateAlertRule(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var existing models.AlertRule
	if id < 1 || database.DB.First(&existing, id).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "告警规则不存在")
		return
	}
	var req saveAlertRuleReq
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "告警规则格式不正确")
		return
	}
	rule, selection, err := buildAlertRule(req, existing.ID)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		values := map[string]interface{}{
			"group_id": rule.GroupID, "name": rule.Name, "description": rule.Description, "expression": rule.Expression, "for_seconds": rule.ForSeconds,
			"recovery_wait_seconds": rule.RecoveryWaitSeconds, "no_data_policy": "resolve", "evaluation_interval_seconds": rule.EvaluationIntervalSeconds,
			"severity": rule.Severity, "labels": rule.Labels, "annotations": rule.Annotations,
			"fault_center_id": rule.FaultCenterID, "status": rule.Status,
		}
		if err := tx.Model(&existing).Updates(values).Error; err != nil {
			return err
		}
		if err := replaceAlertRuleSelections(tx, existing.ID, selection); err != nil {
			return err
		}
		if err := replaceAlertRuleConditions(tx, existing.ID, selection.Conditions); err != nil {
			return err
		}
		if err := replaceAlertRuleBindings(tx, existing.ID, selection.EffectiveIDs); err != nil {
			return err
		}
		if rule.Status == 0 {
			now := time.Now()
			return resolvePromRuleAlerts(tx, existing.ID, 0, now, "rule_disabled")
		}
		return nil
	})
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "更新告警规则失败")
		return
	}
	database.DB.First(&existing, existing.ID)
	resp.Success(c, http.StatusOK, "告警规则已更新", alertRuleResponse(existing))
}

func UpdateAlertRuleStatus(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var rule models.AlertRule
	if id < 1 || database.DB.First(&rule, id).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "告警规则不存在")
		return
	}
	var req struct {
		Status *int `json:"status"`
	}
	if c.ShouldBindJSON(&req) != nil || req.Status == nil || (*req.Status != 0 && *req.Status != 1) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "启用状态不正确")
		return
	}
	now := time.Now()
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&rule).Update("status", *req.Status).Error; err != nil {
			return err
		}
		if *req.Status == 0 {
			return resolvePromRuleAlerts(tx, rule.ID, 0, now, "rule_disabled")
		}
		return tx.Model(&models.AlertRuleBinding{}).Where("rule_id = ?", rule.ID).Updates(map[string]interface{}{
			"next_evaluation_at": now,
			"lease_until":        nil,
		}).Error
	}); err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "更新告警规则状态失败")
		return
	}
	rule.Status = *req.Status
	resp.Success(c, http.StatusOK, "告警规则状态已更新", gin.H{"id": rule.ID, "status": rule.Status})
}

type batchAlertRuleStatusReq struct {
	IDs    []uint `json:"ids"`
	Status *int   `json:"status"`
}

func BatchUpdateAlertRuleStatus(c *gin.Context) {
	var req batchAlertRuleStatusReq
	if c.ShouldBindJSON(&req) != nil || len(req.IDs) == 0 || req.Status == nil || (*req.Status != 0 && *req.Status != 1) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请选择告警规则并指定正确状态")
		return
	}
	var rules []models.AlertRule
	if err := database.DB.Where("id IN ?", req.IDs).Find(&rules).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "查询告警规则失败")
		return
	}
	if len(rules) == 0 {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "告警规则不存在")
		return
	}
	ruleIDs := make([]uint, 0, len(rules))
	for _, rule := range rules {
		ruleIDs = append(ruleIDs, rule.ID)
	}
	now := time.Now()
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.AlertRule{}).Where("id IN ?", ruleIDs).Update("status", *req.Status).Error; err != nil {
			return err
		}
		if *req.Status == 0 {
			for _, id := range ruleIDs {
				if err := resolvePromRuleAlerts(tx, id, 0, now, "rule_disabled"); err != nil {
					return err
				}
			}
			return nil
		}
		return tx.Model(&models.AlertRuleBinding{}).Where("rule_id IN ?", ruleIDs).Updates(map[string]interface{}{
			"next_evaluation_at": now,
			"lease_until":        nil,
		}).Error
	})
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "批量更新告警规则状态失败")
		return
	}
	resp.Success(c, http.StatusOK, "批量更新成功", gin.H{"updated": len(ruleIDs)})
}

type batchAlertRuleIDsReq struct {
	IDs []uint `json:"ids"`
}

func BatchDeleteAlertRules(c *gin.Context) {
	var req batchAlertRuleIDsReq
	if c.ShouldBindJSON(&req) != nil || len(req.IDs) == 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请选择要删除的告警规则")
		return
	}
	var rules []models.AlertRule
	if err := database.DB.Where("id IN ?", req.IDs).Find(&rules).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "查询告警规则失败")
		return
	}
	if len(rules) == 0 {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "告警规则不存在")
		return
	}
	ruleIDs := make([]uint, 0, len(rules))
	for _, rule := range rules {
		ruleIDs = append(ruleIDs, rule.ID)
	}
	now := time.Now()
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		for _, id := range ruleIDs {
			if err := resolvePromRuleAlerts(tx, id, 0, now, "rule_deleted"); err != nil {
				return err
			}
		}
		if err := tx.Where("rule_id IN ?", ruleIDs).Delete(&models.AlertRuleBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Where("rule_id IN ?", ruleIDs).Delete(&models.AlertRuleDatasource{}).Error; err != nil {
			return err
		}
		if err := tx.Where("rule_id IN ?", ruleIDs).Delete(&models.AlertRuleDatasourceGroup{}).Error; err != nil {
			return err
		}
		if err := tx.Where("rule_id IN ?", ruleIDs).Delete(&models.AlertRuleCondition{}).Error; err != nil {
			return err
		}
		return tx.Where("id IN ?", ruleIDs).Delete(&models.AlertRule{}).Error
	})
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "批量删除告警规则失败")
		return
	}
	resp.Success(c, http.StatusOK, "批量删除成功", gin.H{"deleted": len(ruleIDs)})
}

func DeleteAlertRule(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var rule models.AlertRule
	if id < 1 || database.DB.First(&rule, id).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "告警规则不存在")
		return
	}
	now := time.Now()
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := resolvePromRuleAlerts(tx, rule.ID, 0, now, "rule_deleted"); err != nil {
			return err
		}
		if err := tx.Where("rule_id = ?", rule.ID).Delete(&models.AlertRuleBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Where("rule_id = ?", rule.ID).Delete(&models.AlertRuleDatasource{}).Error; err != nil {
			return err
		}
		if err := tx.Where("rule_id = ?", rule.ID).Delete(&models.AlertRuleDatasourceGroup{}).Error; err != nil {
			return err
		}
		if err := tx.Where("rule_id = ?", rule.ID).Delete(&models.AlertRuleCondition{}).Error; err != nil {
			return err
		}
		return tx.Delete(&rule).Error
	})
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "删除告警规则失败")
		return
	}
	resp.Success(c, http.StatusOK, "告警规则已删除")
}

func RunAlertRule(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var rule models.AlertRule
	if id < 1 || database.DB.First(&rule, id).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "告警规则不存在")
		return
	}
	if rule.Status != 1 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请先启用告警规则")
		return
	}
	now := time.Now()
	result := database.DB.Model(&models.AlertRuleBinding{}).Where("rule_id = ?", rule.ID).Updates(map[string]interface{}{"next_evaluation_at": now, "lease_until": nil})
	if result.RowsAffected == 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "告警规则未绑定数据源")
		return
	}
	go runDueAlertRuleBindings(now)
	resp.Success(c, http.StatusOK, "规则评估已提交")
}

func ListAlertEvaluationRecords(c *gin.Context) {
	ruleID, _ := strconv.Atoi(c.Query("rule_id"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	query := database.DB.Model(&models.AlertEvaluationRecord{})
	if ruleID > 0 {
		query = query.Where("rule_id = ?", ruleID)
	}
	var total int64
	var rows []models.AlertEvaluationRecord
	query.Count(&total)
	query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows)
	data := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		var rule models.AlertRule
		var datasource models.Datasource
		database.DB.Select("id", "name").First(&rule, row.RuleID)
		database.DB.Select("id", "name").First(&datasource, row.DatasourceID)
		data = append(data, gin.H{"id": row.ID, "rule_id": row.RuleID, "rule_name": rule.Name, "datasource_id": row.DatasourceID, "datasource_name": datasource.Name, "status": row.Status, "result_count": row.ResultCount, "duration_ms": row.DurationMs, "message": row.Message, "evaluated_at": row.EvaluatedAt})
	}
	resp.Success(c, http.StatusOK, "成功", gin.H{"list": data, "total": total, "page": page, "page_size": pageSize})
}

func buildAlertRule(req saveAlertRuleReq, excludeID uint) (models.AlertRule, alertRuleSelection, error) {
	emptySelection := alertRuleSelection{}
	req.Name, req.Description, req.Expression = strings.TrimSpace(req.Name), strings.TrimSpace(req.Description), strings.TrimSpace(req.Expression)
	if req.Name == "" || len([]rune(req.Name)) > 128 || req.Expression == "" {
		return models.AlertRule{}, emptySelection, fmt.Errorf("规则名称和 PromQL 不能为空，名称不能超过 128 个字符")
	}
	if len([]rune(req.Description)) > 500 {
		return models.AlertRule{}, emptySelection, fmt.Errorf("规则描述不能超过 500 个字符")
	}
	var groupCount int64
	if req.GroupID > 0 {
		database.DB.Model(&models.AlertRuleGroup{}).Where("id = ?", req.GroupID).Count(&groupCount)
	}
	if groupCount == 0 {
		return models.AlertRule{}, emptySelection, fmt.Errorf("请选择有效的规则组")
	}
	var duplicate int64
	nameQuery := database.DB.Model(&models.AlertRule{}).Where("group_id = ? AND name = ?", req.GroupID, req.Name)
	if excludeID > 0 {
		nameQuery = nameQuery.Where("id <> ?", excludeID)
	}
	nameQuery.Count(&duplicate)
	if duplicate > 0 {
		return models.AlertRule{}, emptySelection, fmt.Errorf("告警规则名称已存在")
	}
	if err := validateAlertRuleTiming(req.EvaluationIntervalSeconds, req.ForSeconds, req.RecoveryWaitSeconds); err != nil {
		return models.AlertRule{}, emptySelection, err
	}
	if req.Status != 0 && req.Status != 1 {
		return models.AlertRule{}, emptySelection, fmt.Errorf("启用状态不正确")
	}
	var centerCount int64
	if req.FaultCenterID > 0 {
		database.DB.Model(&models.FaultCenter{}).Where("id = ? AND status = 1", req.FaultCenterID).Count(&centerCount)
	}
	if centerCount == 0 {
		return models.AlertRule{}, emptySelection, fmt.Errorf("请选择启用的故障中心")
	}
	datasourceGroupIDs := append([]uint{}, req.DatasourceGroupIDs...)
	datasourceGroupIDs = normalizeChannelIDs(datasourceGroupIDs)
	directDatasourceIDs := normalizeChannelIDs(req.DatasourceIDs)
	effectiveIDs, err := datasourceIDsForSelection(database.DB, datasourceGroupIDs, directDatasourceIDs)
	if err != nil {
		return models.AlertRule{}, emptySelection, err
	}
	if len(datasourceGroupIDs) == 0 && len(directDatasourceIDs) == 0 {
		return models.AlertRule{}, emptySelection, fmt.Errorf("请至少选择一个数据源或数据源分组")
	}
	severity := strings.ToUpper(strings.TrimSpace(req.Severity))
	if severity == "" {
		severity = "P1"
	}
	if severity != "P0" && severity != "P1" && severity != "P2" {
		return models.AlertRule{}, emptySelection, fmt.Errorf("告警等级仅支持 P0、P1、P2")
	}
	conditions, err := normalizeAlertRuleConditions(req.Conditions, req.ForSeconds)
	if err != nil {
		return models.AlertRule{}, emptySelection, err
	}
	severity = conditions[0].Severity
	labels, err := normalizeRuleTextMap(req.ExternalLabels, 30)
	if err != nil {
		return models.AlertRule{}, emptySelection, err
	}
	annotations, err := normalizeAlertRuleAnnotations(req.Annotations)
	if err != nil {
		return models.AlertRule{}, emptySelection, err
	}
	delete(labels, "severity")
	encodedLabels, _ := json.Marshal(labels)
	encodedAnnotations, _ := json.Marshal(annotations)
	selection := alertRuleSelection{DirectDatasourceIDs: directDatasourceIDs, DatasourceGroupIDs: datasourceGroupIDs, EffectiveIDs: effectiveIDs, Conditions: conditions}
	return models.AlertRule{GroupID: req.GroupID, Name: req.Name, Description: req.Description, Expression: req.Expression, ForSeconds: req.ForSeconds, RecoveryWaitSeconds: req.RecoveryWaitSeconds, NoDataPolicy: "resolve", EvaluationIntervalSeconds: req.EvaluationIntervalSeconds, Severity: severity, Labels: string(encodedLabels), Annotations: string(encodedAnnotations), FaultCenterID: req.FaultCenterID, Status: req.Status}, selection, nil
}

func validateAlertRuleTiming(evaluationInterval, forSeconds, recoverySeconds int) error {
	if evaluationInterval < 30 || evaluationInterval > 86400 {
		return fmt.Errorf("执行频率必须在 30 到 86400 秒之间")
	}
	if forSeconds < 0 || forSeconds > 604800 {
		return fmt.Errorf("持续时间必须在 0 到 604800 秒之间")
	}
	if recoverySeconds < 0 || recoverySeconds > 86400 {
		return fmt.Errorf("恢复时间必须在 0 到 86400 秒之间")
	}
	if forSeconds > 0 && forSeconds < evaluationInterval {
		return fmt.Errorf("持续时间不能小于执行频率；如需首次命中立即触发，请设置为 0")
	}
	if recoverySeconds > 0 && recoverySeconds < evaluationInterval {
		return fmt.Errorf("恢复时间不能小于执行频率；如需首次不命中立即恢复，请设置为 0")
	}
	return nil
}

func normalizeRuleTextMap(values map[string]string, limit int) (map[string]string, error) {
	result := make(map[string]string)
	for rawKey, rawValue := range values {
		key, value := strings.TrimSpace(rawKey), strings.TrimSpace(rawValue)
		if key == "" || value == "" || len(key) > 128 || len(value) > 2000 {
			return nil, fmt.Errorf("标签和注解的 Key、Value 不能为空且不能过长")
		}
		result[key] = value
	}
	if len(result) > limit {
		return nil, fmt.Errorf("标签或注解数量过多")
	}
	return result, nil
}

func normalizeAlertRuleAnnotations(values map[string]string) (map[string]string, error) {
	for key := range values {
		if strings.TrimSpace(key) != "description" {
			return nil, fmt.Errorf("告警注解仅支持 description")
		}
	}
	return normalizeRuleTextMap(values, 1)
}

func normalizeAlertRuleConditions(values []saveAlertRuleConditionReq, forSeconds int) ([]models.AlertRuleCondition, error) {
	if forSeconds < 0 || forSeconds > 604800 {
		return nil, fmt.Errorf("持续时间必须在 0 到 604800 秒之间")
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("请至少配置一个告警条件")
	}
	if len(values) > 3 {
		return nil, fmt.Errorf("一条规则最多添加 3 个告警条件")
	}
	allowedOperators := map[string]bool{">": true, ">=": true, "<": true, "<=": true, "==": true, "!=": true}
	usedSeverities := make(map[string]bool, len(values))
	conditions := make([]models.AlertRuleCondition, 0, len(values))
	for index, item := range values {
		severity := strings.ToUpper(strings.TrimSpace(item.Severity))
		operator := strings.TrimSpace(item.Operator)
		if severity != "P0" && severity != "P1" && severity != "P2" {
			return nil, fmt.Errorf("第 %d 个告警条件的等级不正确", index+1)
		}
		if usedSeverities[severity] {
			return nil, fmt.Errorf("P0、P1、P2 每个等级只能配置一条")
		}
		usedSeverities[severity] = true
		if !allowedOperators[operator] {
			return nil, fmt.Errorf("第 %d 个告警条件的比较符不正确", index+1)
		}
		if item.Threshold == nil || math.IsNaN(*item.Threshold) || math.IsInf(*item.Threshold, 0) {
			return nil, fmt.Errorf("第 %d 个告警条件缺少阈值", index+1)
		}
		threshold := float64(0)
		if item.Threshold != nil {
			threshold = *item.Threshold
		}
		conditions = append(conditions, models.AlertRuleCondition{Severity: severity, Operator: operator, Threshold: threshold, Sort: index})
	}
	return conditions, nil
}

func replaceAlertRuleConditions(tx *gorm.DB, ruleID uint, conditions []models.AlertRuleCondition) error {
	if err := tx.Where("rule_id = ?", ruleID).Delete(&models.AlertRuleCondition{}).Error; err != nil {
		return err
	}
	for index := range conditions {
		conditions[index].ID = 0
		conditions[index].RuleID = ruleID
		conditions[index].Sort = index
		if err := tx.Create(&conditions[index]).Error; err != nil {
			return err
		}
	}
	return nil
}

func datasourceIDsForSelection(tx *gorm.DB, datasourceGroupIDs, directDatasourceIDs []uint) ([]uint, error) {
	if len(datasourceGroupIDs) > 0 {
		var groupCount int64
		if err := tx.Model(&models.DatasourceGroup{}).Where("id IN ?", datasourceGroupIDs).Count(&groupCount).Error; err != nil {
			return nil, err
		}
		if groupCount != int64(len(datasourceGroupIDs)) {
			return nil, fmt.Errorf("数据源分组不存在")
		}
	}
	if len(directDatasourceIDs) > 0 {
		var datasourceCount int64
		if err := tx.Model(&models.Datasource{}).Where("id IN ? AND type IN ? AND status = 1", directDatasourceIDs, metricDatasourceTypes()).Count(&datasourceCount).Error; err != nil {
			return nil, err
		}
		if datasourceCount != int64(len(directDatasourceIDs)) {
			return nil, fmt.Errorf("数据源不存在、已停用或不支持指标查询")
		}
	}
	effectiveIDs := append([]uint{}, directDatasourceIDs...)
	if len(datasourceGroupIDs) > 0 {
		expandedIDs, err := expandedDatasourceGroupIDs(tx, datasourceGroupIDs)
		if err != nil {
			return nil, err
		}
		var members []models.DatasourceGroupMember
		if err := tx.Where("group_id IN ?", expandedIDs).Where("datasource_id IN (?)", tx.Model(&models.Datasource{}).Select("id").Where("type IN ? AND status = 1", metricDatasourceTypes())).Find(&members).Error; err != nil {
			return nil, err
		}
		for _, member := range members {
			effectiveIDs = append(effectiveIDs, member.DatasourceID)
		}
	}
	return normalizeChannelIDs(effectiveIDs), nil
}

func replaceAlertRuleSelections(tx *gorm.DB, ruleID uint, selection alertRuleSelection) error {
	if err := tx.Where("rule_id = ?", ruleID).Delete(&models.AlertRuleDatasource{}).Error; err != nil {
		return err
	}
	if err := tx.Where("rule_id = ?", ruleID).Delete(&models.AlertRuleDatasourceGroup{}).Error; err != nil {
		return err
	}
	for _, datasourceID := range selection.DirectDatasourceIDs {
		if err := tx.Create(&models.AlertRuleDatasource{RuleID: ruleID, DatasourceID: datasourceID}).Error; err != nil {
			return err
		}
	}
	for _, datasourceGroupID := range selection.DatasourceGroupIDs {
		if err := tx.Create(&models.AlertRuleDatasourceGroup{RuleID: ruleID, DatasourceGroupID: datasourceGroupID}).Error; err != nil {
			return err
		}
	}
	return nil
}

func effectiveAlertRuleDatasourceIDs(tx *gorm.DB, ruleID uint) ([]uint, error) {
	var directSelections []models.AlertRuleDatasource
	if err := tx.Where("rule_id = ?", ruleID).Find(&directSelections).Error; err != nil {
		return nil, err
	}
	directIDs := make([]uint, 0, len(directSelections))
	for _, selection := range directSelections {
		directIDs = append(directIDs, selection.DatasourceID)
	}
	var groupSelections []models.AlertRuleDatasourceGroup
	if err := tx.Where("rule_id = ?", ruleID).Find(&groupSelections).Error; err != nil {
		return nil, err
	}
	groupIDs := make([]uint, 0, len(groupSelections))
	for _, selection := range groupSelections {
		groupIDs = append(groupIDs, selection.DatasourceGroupID)
	}
	var activeDirectIDs []uint
	if len(directIDs) > 0 {
		if err := tx.Model(&models.Datasource{}).Where("id IN ? AND type IN ? AND status = 1", directIDs, metricDatasourceTypes()).Pluck("id", &activeDirectIDs).Error; err != nil {
			return nil, err
		}
	}
	return datasourceIDsForSelection(tx, groupIDs, activeDirectIDs)
}

func replaceAlertRuleBindings(tx *gorm.DB, ruleID uint, datasourceIDs []uint) error {
	var current []models.AlertRuleBinding
	if err := tx.Where("rule_id = ?", ruleID).Find(&current).Error; err != nil {
		return err
	}
	wanted := make(map[uint]bool, len(datasourceIDs))
	for _, id := range datasourceIDs {
		wanted[id] = true
	}
	for _, binding := range current {
		if !wanted[binding.DatasourceID] {
			if err := resolvePromRuleAlerts(tx, ruleID, binding.DatasourceID, time.Now(), "datasource_unbound"); err != nil {
				return err
			}
			if err := tx.Delete(&binding).Error; err != nil {
				return err
			}
		} else {
			delete(wanted, binding.DatasourceID)
		}
	}
	now := time.Now()
	for datasourceID := range wanted {
		if err := tx.Create(&models.AlertRuleBinding{RuleID: ruleID, DatasourceID: datasourceID, NextEvaluationAt: &now}).Error; err != nil {
			return err
		}
	}
	return nil
}

func resolvePromRuleAlerts(tx *gorm.DB, ruleID, datasourceID uint, now time.Time, reason string) error {
	query := tx.Where(
		"source = ? AND source_id = ? AND status IN ?",
		alertRuleSource, ruleID, []string{alertStatusPending, alertStatusFiring, alertStatusRecovering},
	)
	if datasourceID > 0 {
		query = query.Where("datasource_id = ?", datasourceID)
	}
	var alerts []models.AlertInstance
	if err := query.Find(&alerts).Error; err != nil {
		return err
	}
	for i := range alerts {
		alert := &alerts[i]
		previous := alert.Status
		endStatus := alertEndStatus(reason)
		if err := tx.Model(alert).Updates(map[string]interface{}{
			"status": endStatus, "resolved_at": now, "state_changed_at": now, "last_event_at": now,
			"notification_status": notificationStatusTerminated, "notification_reason": alertTerminationReason(reason), "next_notification_at": nil,
		}).Error; err != nil {
			return err
		}
		alert.Status, alert.StateChangedAt, alert.LastEventAt, alert.ResolvedAt = endStatus, now, now, &now
		alert.NotificationStatus, alert.NotificationReason, alert.NextNotificationAt = notificationStatusTerminated, alertTerminationReason(reason), nil
		if err := recordAlertEvent(tx, *alert, "state_changed", previous, endStatus, alert.Severity, alert.Severity, reason, "", alert.LastProbeStatus, now); err != nil {
			return err
		}
	}
	return nil
}

func alertEndStatus(reason string) string {
	switch reason {
	case "rule_disabled", "rule_deleted", "datasource_unbound":
		return alertStatusTerminated
	default:
		return alertStatusResolved
	}
}

func alertTerminationReason(reason string) string {
	return map[string]string{
		"rule_disabled":      "告警规则已停用",
		"rule_deleted":       "告警规则已删除",
		"datasource_unbound": "数据源已解除绑定",
	}[reason]
}

func alertRuleResponse(rule models.AlertRule) gin.H {
	var bindings []models.AlertRuleBinding
	database.DB.Where("rule_id = ?", rule.ID).Order("id ASC").Find(&bindings)
	datasourceIDs := make([]uint, 0, len(bindings))
	datasourceNames := make([]string, 0, len(bindings))
	datasourceResults := make([]gin.H, 0, len(bindings))
	aggregateStatus, aggregateError := "", ""
	var lastEvaluatedAt *time.Time
	for _, binding := range bindings {
		datasourceIDs = append(datasourceIDs, binding.DatasourceID)
		var datasource models.Datasource
		database.DB.Select("id", "name").First(&datasource, binding.DatasourceID)
		datasourceNames = append(datasourceNames, datasource.Name)
		datasourceResults = append(datasourceResults, gin.H{"datasource_id": datasource.ID, "datasource_name": datasource.Name, "last_evaluated_at": binding.LastEvaluatedAt, "last_status": binding.LastStatus, "last_error": binding.LastError, "result_count": binding.LastResultCount, "duration_ms": binding.LastDurationMs})
		if binding.LastEvaluatedAt != nil && (lastEvaluatedAt == nil || binding.LastEvaluatedAt.After(*lastEvaluatedAt)) {
			value := *binding.LastEvaluatedAt
			lastEvaluatedAt = &value
		}
		switch binding.LastStatus {
		case "error":
			aggregateStatus = "error"
			if aggregateError == "" {
				aggregateError = binding.LastError
			}
		case "success":
			if aggregateStatus != "error" {
				aggregateStatus = "success"
			}
		case "no_data":
			if aggregateStatus == "" {
				aggregateStatus = "no_data"
			}
		}
	}
	var center models.FaultCenter
	database.DB.Select("id", "name").First(&center, rule.FaultCenterID)
	var group models.AlertRuleGroup
	database.DB.Select("id", "name").First(&group, rule.GroupID)
	var directSelections []models.AlertRuleDatasource
	database.DB.Where("rule_id = ?", rule.ID).Order("id ASC").Find(&directSelections)
	directDatasourceIDs := make([]uint, 0, len(directSelections))
	for _, selection := range directSelections {
		directDatasourceIDs = append(directDatasourceIDs, selection.DatasourceID)
	}
	var datasourceGroupSelections []models.AlertRuleDatasourceGroup
	database.DB.Where("rule_id = ?", rule.ID).Order("id ASC").Find(&datasourceGroupSelections)
	datasourceGroupIDs := make([]uint, 0, len(datasourceGroupSelections))
	datasourceGroupNames := make([]string, 0, len(datasourceGroupSelections))
	datasourceGroupTree, _ := loadDatasourceGroupTree(database.DB)
	for _, selection := range datasourceGroupSelections {
		var datasourceGroup models.DatasourceGroup
		database.DB.Select("id", "name").First(&datasourceGroup, selection.DatasourceGroupID)
		datasourceGroupIDs = append(datasourceGroupIDs, selection.DatasourceGroupID)
		path, _ := datasourceGroupPath(datasourceGroupTree, datasourceGroup.ID)
		datasourceGroupNames = append(datasourceGroupNames, path)
	}
	var activeCount int64
	database.DB.Model(&models.AlertInstance{}).Where("source = ? AND source_id = ? AND status IN ?", alertRuleSource, rule.ID, []string{alertStatusPending, alertStatusFiring, alertStatusRecovering}).Count(&activeCount)
	var conditions []models.AlertRuleCondition
	database.DB.Where("rule_id = ?", rule.ID).Order("sort ASC, id ASC").Find(&conditions)
	return gin.H{"id": rule.ID, "group_id": rule.GroupID, "group_name": group.Name, "name": rule.Name, "description": rule.Description, "expression": rule.Expression, "datasource_group_ids": datasourceGroupIDs, "datasource_group_names": datasourceGroupNames, "direct_datasource_ids": directDatasourceIDs, "datasource_ids": datasourceIDs, "datasource_names": datasourceNames, "datasource_results": datasourceResults, "fault_center_id": rule.FaultCenterID, "fault_center_name": center.Name, "for_seconds": rule.ForSeconds, "recovery_wait_seconds": rule.RecoveryWaitSeconds, "no_data_policy": "resolve", "evaluation_interval_seconds": rule.EvaluationIntervalSeconds, "severity": rule.Severity, "conditions": conditions, "external_labels": decodeAlertRuleExternalLabels(rule.Labels), "annotations": decodeAlertRuleAnnotations(rule.Annotations), "status": rule.Status, "last_evaluated_at": lastEvaluatedAt, "last_status": aggregateStatus, "last_error": aggregateError, "active_count": activeCount, "created_at": rule.CreatedAt, "updated_at": rule.UpdatedAt}
}

func decodeRuleTextMap(value string) map[string]string {
	result := map[string]string{}
	_ = json.Unmarshal([]byte(value), &result)
	return result
}

func decodeAlertRuleExternalLabels(value string) map[string]string {
	labels := decodeRuleTextMap(value)
	delete(labels, "severity")
	return labels
}

func decodeAlertRuleAnnotations(value string) map[string]string {
	values := decodeRuleTextMap(value)
	description := strings.TrimSpace(values["description"])
	if description == "" {
		return map[string]string{}
	}
	return map[string]string{"description": description}
}

func runDueAlertRuleBindings(now time.Time) {
	startAlertRuleWorkers()
	var bindings []models.AlertRuleBinding
	database.DB.Table("alert_rule_bindings").Select("alert_rule_bindings.*").Joins("JOIN alert_rules ON alert_rules.id = alert_rule_bindings.rule_id").Joins("JOIN datasources ON datasources.id = alert_rule_bindings.datasource_id").Where("alert_rules.status = 1 AND datasources.status = 1 AND datasources.type IN ? AND (alert_rule_bindings.next_evaluation_at IS NULL OR alert_rule_bindings.next_evaluation_at <= ?)", metricDatasourceTypes(), now).Order("alert_rule_bindings.next_evaluation_at ASC").Limit(5000).Scan(&bindings)
	for _, binding := range bindings {
		if _, loaded := alertRuleInFlight.LoadOrStore(binding.ID, struct{}{}); loaded {
			continue
		}
		select {
		case alertRuleQueue <- binding.ID:
		default:
			alertRuleInFlight.Delete(binding.ID)
			return
		}
	}
}

func startAlertRuleWorkers() {
	alertRuleWorkerOnce.Do(func() {
		for i := 0; i < alertRuleWorkerCount; i++ {
			go func() {
				for bindingID := range alertRuleQueue {
					runAlertRuleBinding(bindingID)
				}
			}()
		}
	})
}

func runAlertRuleBinding(bindingID uint) {
	defer func() {
		alertRuleInFlight.Delete(bindingID)
		if recovered := recover(); recovered != nil {
			logger.Error("alert rule worker panic", "binding_id", bindingID, "panic", recovered, "stack", string(debug.Stack()))
		}
	}()
	evaluateAlertRuleBinding(bindingID, time.Now())
}

func evaluateAlertRuleBinding(bindingID uint, evaluatedAt time.Time) {
	started := time.Now()
	var binding models.AlertRuleBinding
	var rule models.AlertRule
	var datasource models.Datasource
	if database.DB.First(&binding, bindingID).Error != nil || database.DB.First(&rule, binding.RuleID).Error != nil || database.DB.First(&datasource, binding.DatasourceID).Error != nil {
		return
	}
	if rule.Status != 1 || datasource.Status != 1 || !isMetricDatasourceType(datasource.Type) {
		return
	}
	series, err := queryPromRuleDatasource(datasource, rule.Expression)
	durationMS := time.Since(started).Milliseconds()
	status, message := "success", ""
	if err != nil {
		status, message = "error", textutil.Ellipsis(err.Error(), 1900)
	} else if len(series) == 0 {
		status = "no_data"
	}
	if err == nil && alertRuleEvaluationAction(status) == alertEvaluationSync {
		var conditions []models.AlertRuleCondition
		database.DB.Where("rule_id = ?", rule.ID).Order("sort ASC, id ASC").Find(&conditions)
		syncPromRuleAlerts(rule, datasource, applyAlertRuleConditions(series, conditions, rule), evaluatedAt)
	} else {
		markPromRuleEvaluationOutcome(rule.ID, datasource.ID, status, message, evaluatedAt)
	}
	next := evaluatedAt.Add(time.Duration(rule.EvaluationIntervalSeconds) * time.Second)
	recordEvaluation := binding.LastEvaluatedAt == nil || binding.LastStatus != status || binding.LastError != message
	updates := map[string]interface{}{"next_evaluation_at": next, "lease_until": nil, "last_evaluated_at": evaluatedAt, "last_status": status, "last_error": message, "last_result_count": len(series), "last_duration_ms": durationMS}
	database.DB.Model(&binding).Updates(updates)
	if recordEvaluation {
		database.DB.Create(&models.AlertEvaluationRecord{RuleID: rule.ID, DatasourceID: datasource.ID, Status: status, ResultCount: len(series), DurationMs: durationMS, Message: message, EvaluatedAt: evaluatedAt})
	}
}

func queryPromRuleDatasource(datasource models.Datasource, expression string) ([]promVectorSeries, error) {
	params := url.Values{}
	params.Set("query", expression)
	// Let the datasource evaluate at its own current time. Local evaluation
	// timestamps are only used for scheduling and alert lifecycle durations.
	queryPath := "/api/v1/query"
	var config struct {
		QueryPath   string            `json:"query_path"`
		QueryParams map[string]string `json:"query_params"`
	}
	if json.Unmarshal([]byte(datasource.Config), &config) == nil {
		if strings.TrimSpace(config.QueryPath) != "" {
			queryPath = "/" + strings.TrimLeft(strings.TrimSpace(config.QueryPath), "/")
		}
		for key, value := range config.QueryParams {
			params.Set(key, value)
		}
	}
	apiURL := strings.TrimRight(datasource.URL, "/") + queryPath + "?" + params.Encode()
	body, err := doPromHTTP(datasource, apiURL)
	if err != nil {
		return nil, err
	}
	return parsePromVectorResponse(body)
}

func parsePromVectorResponse(body []byte) ([]promVectorSeries, error) {
	var response struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  []interface{}     `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("Prometheus 返回格式不正确: %w", err)
	}
	if response.Status != "success" {
		if response.Error == "" {
			response.Error = string(body)
		}
		return nil, fmt.Errorf("Prometheus 查询失败: %s", response.Error)
	}
	if response.Data.ResultType != "vector" {
		return nil, fmt.Errorf("告警规则只支持 instant vector，当前结果类型为 %s", response.Data.ResultType)
	}
	series := make([]promVectorSeries, 0, len(response.Data.Result))
	for _, item := range response.Data.Result {
		value := ""
		if len(item.Value) > 1 {
			value = fmt.Sprint(item.Value[1])
		}
		series = append(series, promVectorSeries{Labels: item.Metric, Value: value})
	}
	return series, nil
}

func applyAlertRuleConditions(series []promVectorSeries, conditions []models.AlertRuleCondition, rule models.AlertRule) []promVectorSeries {
	if len(conditions) == 0 {
		return []promVectorSeries{}
	}
	matched := make([]promVectorSeries, 0, len(series))
	for _, item := range series {
		value, err := strconv.ParseFloat(item.Value, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		var selected *promVectorSeries
		for _, condition := range conditions {
			if !alertRuleConditionMatches(value, condition.Operator, condition.Threshold) {
				continue
			}
			copy := item
			copy.ConditionID = condition.ID
			if condition.Operator != "active" {
				copy.ConditionKey = strings.Join([]string{condition.Severity, condition.Operator, strconv.FormatFloat(condition.Threshold, 'g', -1, 64)}, "|")
			}
			copy.Severity = condition.Severity
			copy.ForSeconds = rule.ForSeconds
			copy.Condition = fmt.Sprintf("%s %s", condition.Operator, strconv.FormatFloat(condition.Threshold, 'f', -1, 64))
			if condition.Operator == "active" {
				copy.Condition = "查询结果非空"
			}
			if selected == nil || alertSeverityRank(copy.Severity) > alertSeverityRank(selected.Severity) {
				candidate := copy
				selected = &candidate
			}
		}
		if selected != nil {
			matched = append(matched, *selected)
		}
	}
	return matched
}

func alertSeverityRank(severity string) int {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "P0":
		return 3
	case "P1":
		return 2
	case "P2":
		return 1
	default:
		return 0
	}
}

func alertRuleConditionMatches(value float64, operator string, threshold float64) bool {
	switch operator {
	case "active":
		return true
	case ">":
		return value > threshold
	case ">=":
		return value >= threshold
	case "<":
		return value < threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	case "!=":
		return value != threshold
	default:
		return false
	}
}

func promSeriesFingerprint(datasourceID, ruleID uint, labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := []string{strconv.FormatUint(uint64(datasourceID), 10), strconv.FormatUint(uint64(ruleID), 10)}
	for _, key := range keys {
		parts = append(parts, key+"="+labels[key])
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func promAlertIdentity(alert models.AlertInstance) string {
	return strings.TrimSpace(alert.Fingerprint)
}

func nextPromRuleAlertState(current string, conditionActive bool, stateSince time.Time, forSeconds, recoverySeconds int, now time.Time) string {
	if conditionActive {
		switch current {
		case alertStatusPending:
			if forSeconds <= 0 || !now.Before(stateSince.Add(time.Duration(forSeconds)*time.Second)) {
				return alertStatusFiring
			}
			return alertStatusPending
		case alertStatusRecovering:
			return alertStatusFiring
		case alertStatusFiring:
			return alertStatusFiring
		default:
			if forSeconds > 0 {
				return alertStatusPending
			}
			return alertStatusFiring
		}
	}
	switch current {
	case alertStatusPending:
		return alertStatusResolved
	case alertStatusFiring:
		if recoverySeconds > 0 {
			return alertStatusRecovering
		}
		return alertStatusResolved
	case alertStatusRecovering:
		if recoverySeconds <= 0 || !now.Before(stateSince.Add(time.Duration(recoverySeconds)*time.Second)) {
			return alertStatusResolved
		}
		return alertStatusRecovering
	default:
		return current
	}
}

func syncPromRuleAlerts(rule models.AlertRule, datasource models.Datasource, series []promVectorSeries, now time.Time) {
	var activeAlerts []models.AlertInstance
	database.DB.Where("source = ? AND source_id = ? AND datasource_id = ? AND status IN ?", alertRuleSource, rule.ID, datasource.ID, []string{alertStatusPending, alertStatusFiring, alertStatusRecovering}).Find(&activeAlerts)
	existing := make(map[string]models.AlertInstance, len(activeAlerts))
	for _, alert := range activeAlerts {
		identity := promAlertIdentity(alert)
		if identity == "" {
			continue
		}
		current, exists := existing[identity]
		if !exists || alertSeverityRank(alert.Severity) > alertSeverityRank(current.Severity) {
			existing[identity] = alert
		}
	}
	matchedAlertIDs := make(map[uint]bool, len(series))
	ruleLabels := decodeAlertRuleExternalLabels(rule.Labels)
	ruleAnnotations := decodeAlertRuleAnnotations(rule.Annotations)
	for _, item := range series {
		metricLabels := make(map[string]string, len(item.Labels))
		for key, value := range item.Labels {
			metricLabels[key] = value
		}
		datasourceLabels := promDatasourceLabels(datasource)
		externalLabels := make(map[string]string, len(ruleLabels))
		for key, value := range ruleLabels {
			externalLabels[key] = renderPromRuleTemplate(value, metricLabels, datasourceLabels, externalLabels, item.Value)
		}
		fingerprint := promSeriesFingerprint(datasource.ID, rule.ID, metricLabels)
		alert, found := existing[fingerprint]
		if found {
			matchedAlertIDs[alert.ID] = true
		}
		previous := alert.Status
		previousSeverity := alert.Severity
		severity := item.Severity
		if severity == "" {
			severity = rule.Severity
		}
		stateSince := alert.StateChangedAt
		if stateSince.IsZero() {
			stateSince = alert.StartedAt
		}
		forSeconds := item.ForSeconds
		if item.ConditionID == 0 && item.Severity == "" {
			forSeconds = rule.ForSeconds
		}
		state := nextPromRuleAlertState(previous, true, stateSince, forSeconds, rule.RecoveryWaitSeconds, now)
		annotations := make(map[string]string, len(ruleAnnotations))
		for key, value := range ruleAnnotations {
			annotations[key] = renderPromRuleTemplate(value, metricLabels, datasourceLabels, externalLabels, item.Value)
		}
		encodedLabels, _ := json.Marshal(metricLabels)
		encodedResult, _ := json.Marshal(buildPromAlertResultSnapshot(rule, datasource, fingerprint, item, metricLabels, datasourceLabels, externalLabels, annotations))
		title := rule.Name
		target := datasource.Name
		if summary := inspectionLabelSummary(metricLabels); summary != "" {
			target += " · " + summary
		}
		if !found {
			alert = models.AlertInstance{FaultCenterID: rule.FaultCenterID, Source: alertRuleSource, SourceID: rule.ID, DatasourceID: datasource.ID, Fingerprint: fingerprint, Title: title, Target: textutil.Ellipsis(target, 500), Severity: severity, Status: state, Labels: string(encodedLabels), FailureCount: 1, FailureThreshold: 1, LastProbeStatus: "success", LastResult: string(encodedResult), StartedAt: now, StateChangedAt: now, LastEventAt: now}
			if state == alertStatusFiring {
				alert.FiredAt = &now
			}
			if database.DB.Create(&alert).Error != nil {
				continue
			}
			_ = recordAlertEvent(database.DB, alert, "created", "", state, "", severity, "condition_detected", item.Value, "success", now)
		} else {
			target = textutil.Ellipsis(target, 500)
			encodedDatasourceLabels, _ := json.Marshal(datasourceLabels)
			metadata := promAlertPersistenceMetadata{FaultCenterID: rule.FaultCenterID, Fingerprint: fingerprint, Title: title, Target: target, Severity: severity, Status: state, Labels: string(encodedLabels), DatasourceLabels: string(encodedDatasourceLabels), ProbeStatus: "success"}
			if promAlertNeedsPersistence(alert, metadata) {
				updates := map[string]interface{}{"fault_center_id": rule.FaultCenterID, "fingerprint": fingerprint, "title": title, "target": target, "severity": severity, "status": state, "labels": string(encodedLabels), "last_probe_status": "success", "last_result": string(encodedResult)}
				if state != previous || previousSeverity != severity || alert.LastProbeStatus != "success" {
					updates["last_event_at"] = now
				}
				applyAlertStateTransition(updates, previous, state, now)
				if database.DB.Model(&alert).Updates(updates).Error != nil {
					continue
				}
			}
			alert.Title, alert.Target, alert.Severity = title, textutil.Ellipsis(target, 500), severity
			alert.FaultCenterID = rule.FaultCenterID
			alert.Fingerprint = fingerprint
			alert.Status, alert.Labels, alert.LastProbeStatus, alert.LastResult = state, string(encodedLabels), "success", string(encodedResult)
			if state != previous {
				alert.StateChangedAt = now
				alert.LastEventAt = now
			}
			if state == alertStatusFiring && previous != alertStatusRecovering && previous != alertStatusFiring {
				alert.FiredAt = &now
			}
			if eventType := alertTransitionEventType(previous, state, previousSeverity, severity); eventType != "" {
				reason := "condition_active"
				if previous == alertStatusPending && state == alertStatusFiring {
					reason = "for_duration_reached"
				} else if previous == alertStatusRecovering && state == alertStatusFiring {
					reason = "condition_retriggered"
				} else if previousSeverity != severity {
					reason = "severity_condition_changed"
				}
				_ = recordAlertEvent(database.DB, alert, eventType, previous, state, previousSeverity, severity, reason, item.Value, "success", now)
			}
		}
		applyActiveSilenceRule(&alert, now)
		if state == alertStatusFiring {
			sendPromRuleNotification(rule, datasource, &alert, false, now)
		}
	}
	for i := range activeAlerts {
		alert := &activeAlerts[i]
		if matchedAlertIDs[alert.ID] {
			continue
		}
		stateSince := alert.StateChangedAt
		if stateSince.IsZero() {
			stateSince = alert.StartedAt
		}
		previous := alert.Status
		state := nextPromRuleAlertState(previous, false, stateSince, rule.ForSeconds, rule.RecoveryWaitSeconds, now)
		if state != previous || alert.LastProbeStatus != "success" {
			updates := map[string]interface{}{"status": state, "last_probe_status": "success", "last_event_at": now}
			applyAlertStateTransition(updates, previous, state, now)
			database.DB.Model(alert).Updates(updates)
			alert.LastEventAt = now
		}
		alert.Status = state
		if state != previous {
			alert.StateChangedAt = now
		}
		if state == alertStatusResolved {
			alert.ResolvedAt = &now
		}
		alert.LastProbeStatus = "success"
		if previous != state {
			reason := "condition_cleared"
			if previous == alertStatusRecovering && state == alertStatusResolved {
				reason = "recovery_wait_reached"
			} else if previous == alertStatusPending {
				reason = "condition_cleared_before_firing"
			}
			_ = recordAlertEvent(database.DB, *alert, "state_changed", previous, state, alert.Severity, alert.Severity, reason, "", "success", now)
		}
		if state == alertStatusResolved && (previous == alertStatusFiring || previous == alertStatusRecovering) {
			sendPromRuleNotification(rule, datasource, alert, true, now)
		}
	}
}

type promAlertPersistenceMetadata struct {
	FaultCenterID    uint
	Fingerprint      string
	Title            string
	Target           string
	Severity         string
	Status           string
	Labels           string
	DatasourceLabels string
	ProbeStatus      string
}

func promAlertNeedsPersistence(alert models.AlertInstance, next promAlertPersistenceMetadata) bool {
	return alert.FaultCenterID != next.FaultCenterID ||
		promAlertSnapshotDatasourceLabels(alert.LastResult) != next.DatasourceLabels ||
		alert.Fingerprint != next.Fingerprint ||
		alert.Title != next.Title ||
		alert.Target != next.Target ||
		alert.Severity != next.Severity ||
		alert.Status != next.Status ||
		alert.Labels != next.Labels ||
		alert.LastProbeStatus != next.ProbeStatus
}

func promAlertSnapshotDatasourceLabels(value string) string {
	var snapshot struct {
		DatasourceLabels map[string]string `json:"datasource_labels"`
	}
	_ = json.Unmarshal([]byte(value), &snapshot)
	if snapshot.DatasourceLabels == nil {
		snapshot.DatasourceLabels = map[string]string{}
	}
	encoded, _ := json.Marshal(snapshot.DatasourceLabels)
	return string(encoded)
}

func promDatasourceLabels(datasource models.Datasource) map[string]string {
	return parseInspectionLabels(datasource.Labels)
}

func buildPromAlertResultSnapshot(rule models.AlertRule, datasource models.Datasource, fingerprint string, item promVectorSeries, labels, datasourceLabels, externalLabels, annotations map[string]string) gin.H {
	snapshotLabels := make(map[string]string, len(labels))
	for key, value := range labels {
		snapshotLabels[key] = value
	}
	severity := item.Severity
	if severity == "" {
		severity = rule.Severity
	}
	return gin.H{
		"alertname":         rule.Name,
		"datasource":        datasource.Name,
		"fingerprint":       fingerprint,
		"severity":          severity,
		"description":       rule.Description,
		"labels":            snapshotLabels,
		"datasource_labels": datasourceLabels,
		"external_labels":   externalLabels,
		"annotations":       annotations,
		"expression":        rule.Expression,
		"condition":         item.Condition,
		"value":             item.Value,
	}
}

func markPromRuleEvaluationOutcome(ruleID, datasourceID uint, status, message string, now time.Time) {
	var alerts []models.AlertInstance
	database.DB.Where("source = ? AND source_id = ? AND datasource_id = ? AND status IN ?", alertRuleSource, ruleID, datasourceID, []string{alertStatusPending, alertStatusFiring, alertStatusRecovering}).Find(&alerts)
	for i := range alerts {
		alert := &alerts[i]
		if alert.LastProbeStatus == status {
			continue
		}
		if database.DB.Model(alert).Updates(map[string]interface{}{"last_probe_status": status, "last_event_at": now}).Error != nil {
			continue
		}
		alert.LastProbeStatus, alert.LastEventAt = status, now
		_ = recordAlertEvent(database.DB, *alert, "evaluation_status_changed", alert.Status, alert.Status, alert.Severity, alert.Severity, status, message, status, now)
	}
}

func renderPromRuleTemplate(value string, labels, datasourceLabels, externalLabels map[string]string, sampleValue string) string {
	value = strings.ReplaceAll(value, "{{ $value }}", sampleValue)
	value = promLabelTemplatePattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := promLabelTemplatePattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		labelName := parts[1]
		if labelName == "" {
			labelName = parts[2]
		}
		return labels[labelName]
	})
	value = promDatasourceLabelTemplatePattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := promDatasourceLabelTemplatePattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		labelName := parts[1]
		if labelName == "" {
			labelName = parts[2]
		}
		return datasourceLabels[labelName]
	})
	return promExternalLabelTemplatePattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := promExternalLabelTemplatePattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		labelName := parts[1]
		if labelName == "" {
			labelName = parts[2]
		}
		return externalLabels[labelName]
	})
}

type alertRuleNotificationConfig struct {
	ChannelIDs           []uint
	RepeatSeconds        int
	RecoveryNotification bool
	Routes               []models.AlertNotificationRoute
}

func loadAlertRuleNotificationConfig(centerID uint) alertRuleNotificationConfig {
	result := alertRuleNotificationConfig{RepeatSeconds: 3600, RecoveryNotification: true}
	var config models.AlertNotificationConfig
	if database.DB.Where("fault_center_id = ?", centerID).Limit(1).Find(&config).RowsAffected > 0 {
		result.ChannelIDs = parseChannelIDs(config.DefaultChannelIDs)
		result.RepeatSeconds = config.RepeatIntervalSeconds
		result.RecoveryNotification = config.RecoveryNotification
	}
	database.DB.Where("fault_center_id = ? AND status = 1", centerID).Order("id ASC").Find(&result.Routes)
	return result
}

func sendPromRuleNotification(rule models.AlertRule, datasource models.Datasource, alert *models.AlertInstance, recovery bool, now time.Time) {
	if alert == nil || alert.ID == 0 {
		return
	}
	config := loadAlertRuleNotificationConfig(rule.FaultCenterID)
	if recovery {
		if queueGroupedRecoveryAlert(alert, config, now) {
			return
		}
	} else if queueGroupedFiringAlert(alert, config, now) {
		// A zero-wait/P0 group is delivered immediately; all other groups are
		// picked up independently by the notification scheduler.
		dispatchDueAlertGroups(now)
		return
	}
	if recovery && (!config.RecoveryNotification || alert.LastNotifiedAt == nil) {
		status := resolvedAlertNotificationStatus(alert.LastNotifiedAt)
		reason := "在首次通知前恢复"
		if !config.RecoveryNotification && alert.LastNotifiedAt != nil {
			status, reason = notificationStatusRecoveryDisabled, "恢复通知未启用"
		}
		updateAlertNotificationState(alert, status, reason, alert.NotificationGroupKey, nil)
		return
	}
	if !recovery && alert.LastNotifiedAt != nil && now.Before(alert.LastNotifiedAt.Add(time.Duration(config.RepeatSeconds)*time.Second)) {
		return
	}
	channelIDs := notificationRouteChannelIDs(*alert, config.Routes, config.ChannelIDs)
	if len(channelIDs) == 0 {
		updateAlertNotificationState(alert, notificationStatusNoReceiver, "没有匹配到可用通知对象", "", nil)
		return
	}
	groupKey := ""
	if !recovery {
		allowed, key := alertInstanceNotificationAllowed(alert, alert.LastNotifiedAt != nil, now)
		if !allowed {
			return
		}
		groupKey = key
	}
	title := "告警触发：" + alert.Title
	eventType, titleColor, status := "alert.rule.firing", alertNotificationTitleColor(alert.Severity, false), "firing"
	if alert.LastNotifiedAt != nil && !recovery {
		title = "告警持续：" + alert.Title
	}
	if recovery {
		title, eventType, titleColor, status = "告警恢复："+alert.Title, "alert.rule.recovery", alertNotificationTitleColor(alert.Severity, true), "resolved"
	}
	description := ""
	var result struct {
		AlertName        string            `json:"alertname"`
		Condition        string            `json:"condition"`
		Datasource       string            `json:"datasource"`
		Description      string            `json:"description"`
		Labels           map[string]string `json:"labels"`
		DatasourceLabels map[string]string `json:"datasource_labels"`
		ExternalLabels   map[string]string `json:"external_labels"`
		Annotations      map[string]string `json:"annotations"`
		Value            string            `json:"value"`
	}
	_ = json.Unmarshal([]byte(alert.LastResult), &result)
	if strings.TrimSpace(result.AlertName) == "" {
		result.AlertName = rule.Name
	}
	if strings.TrimSpace(result.Datasource) == "" {
		result.Datasource = datasource.Name
	}
	description = result.Annotations["description"]
	content := fmt.Sprintf("规则：%s\n数据源：%s\n状态：%s\n当前值：%s", rule.Name, datasource.Name, status, result.Value)
	if description != "" {
		content += "\n说明：" + description
	}
	payload := map[string]interface{}{"title": title, "content": content, "title_color": titleColor, "source": alertRuleSource, "fault_center_id": rule.FaultCenterID, "rule_id": rule.ID, "rule_name": rule.Name, "alertname": result.AlertName, "datasource_id": datasource.ID, "datasource_name": datasource.Name, "datasource": result.Datasource, "expression": rule.Expression, "condition": result.Condition, "description": result.Description, "value": result.Value, "status": status, "severity": alert.Severity, "level": alert.Severity, "fingerprint": alert.Fingerprint, "fired_at": alert.FiredAt, "recovered_at": alert.ResolvedAt, "annotations": result.Annotations, "evaluated_at": now}
	addNotificationLabelNamespace(payload, "system_labels", map[string]string{"alertname": rule.Name, "severity": alert.Severity, "datasource": datasource.Name})
	addNotificationLabelNamespace(payload, "labels", result.Labels)
	addNotificationLabelNamespace(payload, "datasource_labels", result.DatasourceLabels)
	addNotificationLabelNamespace(payload, "external_labels", result.ExternalLabels)
	sent, attempted := false, false
	for _, channelID := range channelIDs {
		var channel models.NotificationChannel
		if database.DB.Where("id = ? AND status = 1", channelID).First(&channel).Error != nil {
			continue
		}
		attempted = true
		eventKey := fmt.Sprintf("prometheus-rule:%d:%s:%s", rule.ID, alert.Fingerprint, status)
		if sendNotificationToChannel(channel, eventType, eventKey, title, payload) == nil {
			sent = true
		}
	}
	if sent {
		directStatus := notificationStatusSent
		if recovery {
			directStatus = notificationStatusRecoverySent
		}
		updateAlertNotificationState(alert, directStatus, "通知已发送", "", nil)
		_ = recordAlertEvent(database.DB, *alert, "notification_sent", alert.Status, alert.Status, alert.Severity, alert.Severity, status, result.Value, alert.LastProbeStatus, now)
	} else if attempted {
		updateAlertNotificationState(alert, notificationStatusSendFailed, "通知发送失败", "", nil)
		_ = recordAlertEvent(database.DB, *alert, "notification_failed", alert.Status, alert.Status, alert.Severity, alert.Severity, status, result.Value, alert.LastProbeStatus, now)
	}
	if sent && !recovery {
		database.DB.Model(alert).Update("last_notified_at", now)
		alert.LastNotifiedAt = &now
		markAlertGroupNotified(rule.FaultCenterID, groupKey, now)
	}
}

func addNotificationLabelNamespace(payload map[string]interface{}, namespace string, labels map[string]string) {
	values := make(map[string]string, len(labels))
	for key, value := range labels {
		values[key] = value
	}
	payload[namespace] = values
}
