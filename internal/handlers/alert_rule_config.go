package handlers

import (
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

const (
	alertRuleConfigAPIVersion = "aiops/v1"
	alertRuleConfigKind       = "AlertRuleList"
	alertRuleConfigMaxRules   = 500
)

type alertRuleConfigDocument struct {
	APIVersion string                 `yaml:"apiVersion"`
	Kind       string                 `yaml:"kind"`
	Rules      []alertRuleConfigEntry `yaml:"rules"`
}

type alertRuleConfigEntry struct {
	Name                      string                          `yaml:"name"`
	Description               string                          `yaml:"description,omitempty"`
	Expression                string                          `yaml:"expression"`
	DatasourceIDs             []uint                          `yaml:"datasource_ids,omitempty"`
	DatasourceGroupIDs        []uint                          `yaml:"datasource_group_ids,omitempty"`
	FaultCenterID             uint                            `yaml:"fault_center_id"`
	ForSeconds                int                             `yaml:"for_seconds"`
	RecoveryWaitSeconds       int                             `yaml:"recovery_wait_seconds"`
	EvaluationIntervalSeconds int                             `yaml:"evaluation_interval_seconds"`
	ExternalLabels            map[string]string               `yaml:"external_labels,omitempty"`
	Annotations               map[string]string               `yaml:"annotations,omitempty"`
	Conditions                []alertRuleConfigConditionEntry `yaml:"conditions"`
	Status                    *int                            `yaml:"status,omitempty"`
}

type alertRuleConfigConditionEntry struct {
	Severity  string   `yaml:"severity"`
	Operator  string   `yaml:"operator"`
	Threshold *float64 `yaml:"threshold"`
}

type importAlertRuleConfigReq struct {
	GroupID   uint   `json:"group_id" binding:"required"`
	Content   string `json:"content" binding:"required"`
	Overwrite bool   `json:"overwrite"`
	DryRun    bool   `json:"dry_run"`
}

type alertRuleImportResult struct {
	Total   int `json:"total"`
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

type preparedAlertRuleImport struct {
	Rule      models.AlertRule
	Selection alertRuleSelection
	Existing  *models.AlertRule
}

func ExportAlertRules(c *gin.Context) {
	groupID, _ := parsePositiveUint(c.Query("group_id"))
	if groupID == 0 || !alertRuleGroupExists(groupID) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请选择有效的规则组")
		return
	}

	var rules []models.AlertRule
	if err := database.DB.Where("group_id = ?", groupID).Order("name ASC, id ASC").Find(&rules).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "读取告警规则失败")
		return
	}
	document, err := alertRuleConfigDocumentFromRules(rules)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, err.Error())
		return
	}
	content, err := yaml.Marshal(document)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "生成导出文件失败")
		return
	}
	filename := fmt.Sprintf("alert-rules-%d-%s.yaml", groupID, time.Now().Format("20060102-150405"))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(http.StatusOK, "application/yaml; charset=utf-8", content)
}

func ImportAlertRules(c *gin.Context) {
	var payload importAlertRuleConfigReq
	if err := c.ShouldBindJSON(&payload); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请选择规则组并粘贴 YAML 配置")
		return
	}
	if !alertRuleGroupExists(payload.GroupID) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请选择有效的规则组")
		return
	}
	document, err := parseAlertRuleConfigDocument(payload.Content)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	prepared, result, err := prepareAlertRuleImport(payload.GroupID, document, payload.Overwrite)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if payload.DryRun {
		resp.Success(c, http.StatusOK, "配置校验通过", result)
		return
	}

	err = database.DB.Transaction(func(tx *gorm.DB) error {
		for _, item := range prepared {
			if item.Existing != nil && !payload.Overwrite {
				continue
			}
			if item.Existing == nil {
				if err := createImportedAlertRule(tx, &item.Rule, item.Selection); err != nil {
					return err
				}
				continue
			}
			if err := updateImportedAlertRule(tx, item.Existing, item.Rule, item.Selection); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "导入告警规则失败，未写入任何配置")
		return
	}
	resp.Success(c, http.StatusOK, "导入完成", result)
}

func parseAlertRuleConfigDocument(content string) (alertRuleConfigDocument, error) {
	document := alertRuleConfigDocument{}
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return document, fmt.Errorf("配置内容不能为空")
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return document, fmt.Errorf("仅支持 YAML 配置，请将 JSON 转换为 YAML 后再导入")
	}
	decoder := yaml.NewDecoder(strings.NewReader(trimmed))
	decoder.KnownFields(true)
	if err := decoder.Decode(&document); err != nil {
		return document, fmt.Errorf("YAML 配置解析失败：%v", err)
	}
	if document.APIVersion != alertRuleConfigAPIVersion || document.Kind != alertRuleConfigKind {
		return document, fmt.Errorf("配置类型不正确，应为 apiVersion: %s、kind: %s", alertRuleConfigAPIVersion, alertRuleConfigKind)
	}
	if len(document.Rules) == 0 {
		return document, fmt.Errorf("配置中至少需要一条告警规则")
	}
	if len(document.Rules) > alertRuleConfigMaxRules {
		return document, fmt.Errorf("单次最多导入 %d 条告警规则", alertRuleConfigMaxRules)
	}
	return document, nil
}

func prepareAlertRuleImport(groupID uint, document alertRuleConfigDocument, overwrite bool) ([]preparedAlertRuleImport, alertRuleImportResult, error) {
	names := make([]string, 0, len(document.Rules))
	seen := make(map[string]struct{}, len(document.Rules))
	for index := range document.Rules {
		name := strings.TrimSpace(document.Rules[index].Name)
		if name == "" {
			return nil, alertRuleImportResult{}, fmt.Errorf("第 %d 条规则名称不能为空", index+1)
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			return nil, alertRuleImportResult{}, fmt.Errorf("配置中存在重复规则名称：%s", name)
		}
		seen[key] = struct{}{}
		names = append(names, name)
	}

	var existingRules []models.AlertRule
	if err := database.DB.Where("group_id = ? AND name IN ?", groupID, names).Order("id ASC").Find(&existingRules).Error; err != nil {
		return nil, alertRuleImportResult{}, fmt.Errorf("检查同名规则失败")
	}
	existingByName := make(map[string]*models.AlertRule, len(existingRules))
	for index := range existingRules {
		rule := &existingRules[index]
		key := strings.ToLower(strings.TrimSpace(rule.Name))
		if _, exists := existingByName[key]; exists {
			return nil, alertRuleImportResult{}, fmt.Errorf("当前规则组存在多个同名规则：%s，请先处理重名数据", rule.Name)
		}
		existingByName[key] = rule
	}

	result := alertRuleImportResult{Total: len(document.Rules)}
	prepared := make([]preparedAlertRuleImport, 0, len(document.Rules))
	for index, entry := range document.Rules {
		existing := existingByName[strings.ToLower(strings.TrimSpace(entry.Name))]
		req := alertRuleConfigRequest(groupID, entry)
		excludeID := uint(0)
		if existing != nil {
			excludeID = existing.ID
		}
		rule, selection, err := buildAlertRule(req, excludeID)
		if err != nil {
			return nil, alertRuleImportResult{}, fmt.Errorf("第 %d 条规则“%s”配置错误：%v", index+1, strings.TrimSpace(entry.Name), err)
		}
		prepared = append(prepared, preparedAlertRuleImport{Rule: rule, Selection: selection, Existing: existing})
		if existing == nil {
			result.Created++
		} else if overwrite {
			result.Updated++
		} else {
			result.Skipped++
		}
	}
	return prepared, result, nil
}

func alertRuleConfigRequest(groupID uint, entry alertRuleConfigEntry) saveAlertRuleReq {
	status := 1
	if entry.Status != nil {
		status = *entry.Status
	}
	interval := entry.EvaluationIntervalSeconds
	if interval == 0 {
		interval = 60
	}
	conditions := make([]saveAlertRuleConditionReq, 0, len(entry.Conditions))
	for _, condition := range entry.Conditions {
		conditions = append(conditions, saveAlertRuleConditionReq{Severity: condition.Severity, Operator: condition.Operator, Threshold: condition.Threshold})
	}
	return saveAlertRuleReq{
		GroupID: groupID, Name: entry.Name, Description: entry.Description, Expression: entry.Expression,
		DatasourceIDs: entry.DatasourceIDs, DatasourceGroupIDs: entry.DatasourceGroupIDs, FaultCenterID: entry.FaultCenterID,
		ForSeconds: entry.ForSeconds, RecoveryWaitSeconds: entry.RecoveryWaitSeconds, EvaluationIntervalSeconds: interval,
		ExternalLabels: entry.ExternalLabels, Annotations: entry.Annotations, Status: status, Conditions: conditions,
	}
}

func alertRuleConfigDocumentFromRules(rules []models.AlertRule) (alertRuleConfigDocument, error) {
	document := alertRuleConfigDocument{APIVersion: alertRuleConfigAPIVersion, Kind: alertRuleConfigKind, Rules: make([]alertRuleConfigEntry, 0, len(rules))}
	if len(rules) == 0 {
		return document, nil
	}
	ruleIDs := make([]uint, 0, len(rules))
	for _, rule := range rules {
		ruleIDs = append(ruleIDs, rule.ID)
	}
	var directSelections []models.AlertRuleDatasource
	if err := database.DB.Where("rule_id IN ?", ruleIDs).Order("rule_id ASC, datasource_id ASC").Find(&directSelections).Error; err != nil {
		return document, fmt.Errorf("读取规则数据源失败")
	}
	var groupSelections []models.AlertRuleDatasourceGroup
	if err := database.DB.Where("rule_id IN ?", ruleIDs).Order("rule_id ASC, datasource_group_id ASC").Find(&groupSelections).Error; err != nil {
		return document, fmt.Errorf("读取规则数据源分组失败")
	}
	var conditions []models.AlertRuleCondition
	if err := database.DB.Where("rule_id IN ?", ruleIDs).Order("rule_id ASC, sort ASC, id ASC").Find(&conditions).Error; err != nil {
		return document, fmt.Errorf("读取规则告警条件失败")
	}
	directIDsByRule := make(map[uint][]uint, len(rules))
	for _, selection := range directSelections {
		directIDsByRule[selection.RuleID] = append(directIDsByRule[selection.RuleID], selection.DatasourceID)
	}
	groupIDsByRule := make(map[uint][]uint, len(rules))
	for _, selection := range groupSelections {
		groupIDsByRule[selection.RuleID] = append(groupIDsByRule[selection.RuleID], selection.DatasourceGroupID)
	}
	conditionsByRule := make(map[uint][]alertRuleConfigConditionEntry, len(rules))
	for _, condition := range conditions {
		threshold := condition.Threshold
		conditionsByRule[condition.RuleID] = append(conditionsByRule[condition.RuleID], alertRuleConfigConditionEntry{Severity: condition.Severity, Operator: condition.Operator, Threshold: &threshold})
	}
	for _, rule := range rules {
		status := rule.Status
		document.Rules = append(document.Rules, alertRuleConfigEntry{
			Name: rule.Name, Description: rule.Description, Expression: rule.Expression,
			DatasourceIDs: directIDsByRule[rule.ID], DatasourceGroupIDs: groupIDsByRule[rule.ID], FaultCenterID: rule.FaultCenterID,
			ForSeconds: rule.ForSeconds, RecoveryWaitSeconds: rule.RecoveryWaitSeconds, EvaluationIntervalSeconds: rule.EvaluationIntervalSeconds,
			ExternalLabels: decodeAlertRuleExternalLabels(rule.Labels), Annotations: decodeAlertRuleAnnotations(rule.Annotations),
			Conditions: conditionsByRule[rule.ID], Status: &status,
		})
	}
	sort.SliceStable(document.Rules, func(i, j int) bool { return document.Rules[i].Name < document.Rules[j].Name })
	return document, nil
}

func createImportedAlertRule(tx *gorm.DB, rule *models.AlertRule, selection alertRuleSelection) error {
	requestedStatus := rule.Status
	if err := tx.Create(rule).Error; err != nil {
		return err
	}
	if requestedStatus == 0 {
		if err := tx.Model(rule).UpdateColumn("status", 0).Error; err != nil {
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
}

func updateImportedAlertRule(tx *gorm.DB, existing *models.AlertRule, rule models.AlertRule, selection alertRuleSelection) error {
	values := map[string]interface{}{
		"group_id": rule.GroupID, "name": rule.Name, "description": rule.Description, "expression": rule.Expression, "for_seconds": rule.ForSeconds,
		"recovery_wait_seconds": rule.RecoveryWaitSeconds, "no_data_policy": "resolve", "evaluation_interval_seconds": rule.EvaluationIntervalSeconds,
		"severity": rule.Severity, "labels": rule.Labels, "annotations": rule.Annotations, "fault_center_id": rule.FaultCenterID,
		"status": rule.Status,
	}
	if err := tx.Model(existing).Updates(values).Error; err != nil {
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
		return resolvePromRuleAlerts(tx, existing.ID, 0, time.Now(), "rule_disabled")
	}
	return nil
}

func alertRuleGroupExists(groupID uint) bool {
	var count int64
	database.DB.Model(&models.AlertRuleGroup{}).Where("id = ?", groupID).Count(&count)
	return count > 0
}

func parsePositiveUint(value string) (uint, error) {
	var parsed uint
	_, err := fmt.Sscan(strings.TrimSpace(value), &parsed)
	return parsed, err
}
