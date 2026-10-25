package handlers

import (
	"encoding/json"
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
	inspectionConfigAPIVersion = "aiops/v1"
	inspectionConfigKind       = "InspectionTaskList"
	inspectionConfigMaxTasks   = 500
)

type inspectionTaskConfigDocument struct {
	APIVersion string                      `yaml:"apiVersion"`
	Kind       string                      `yaml:"kind"`
	Tasks      []inspectionTaskConfigEntry `yaml:"tasks"`
}

type inspectionTaskConfigEntry struct {
	GroupID           uint                   `yaml:"group_id,omitempty"`
	Name              string                 `yaml:"name"`
	Type              string                 `yaml:"type"`
	Target            string                 `yaml:"target"`
	Config            map[string]interface{} `yaml:"config"`
	ExternalLabels    map[string]string      `yaml:"external_labels,omitempty"`
	ExpectedResult    string                 `yaml:"expected_result,omitempty"`
	IntervalSeconds   int                    `yaml:"interval_seconds"`
	FailureThreshold  int                    `yaml:"failure_threshold"`
	RecoveryThreshold int                    `yaml:"recovery_threshold"`
	Severity          string                 `yaml:"severity"`
	FaultCenterID     uint                   `yaml:"fault_center_id"`
	Status            *int                   `yaml:"status,omitempty"`
}

type importInspectionTaskConfigReq struct {
	GroupID   uint   `json:"group_id"`
	Content   string `json:"content" binding:"required"`
	Overwrite bool   `json:"overwrite"`
	DryRun    bool   `json:"dry_run"`
}

type inspectionTaskImportResult struct {
	Total   int `json:"total"`
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

type preparedInspectionTaskImport struct {
	Request  CreateInspectionItemReq
	Existing *models.InspectionItem
}

func ExportInspectionItems(c *gin.Context) {
	var items []models.InspectionItem
	query, ok := inspectionGroupQuery(c, database.DB.Order("id ASC"))
	if !ok {
		return
	}
	if err := query.Find(&items).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "读取拨测任务失败")
		return
	}
	document, err := inspectionTaskConfigDocumentFromItems(items)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, err.Error())
		return
	}

	content, err := yaml.Marshal(document)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "生成导出文件失败")
		return
	}
	filename := fmt.Sprintf("inspection-tasks-%s.yaml", time.Now().Format("20060102-150405"))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(http.StatusOK, "application/yaml; charset=utf-8", content)
}

func ImportInspectionItems(c *gin.Context) {
	var payload importInspectionTaskConfigReq
	if err := c.ShouldBindJSON(&payload); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请粘贴 YAML 配置")
		return
	}
	document, err := parseInspectionTaskConfigDocument(payload.Content)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if payload.GroupID > 0 {
		for i := range document.Tasks {
			document.Tasks[i].GroupID = payload.GroupID
		}
	}
	prepared, result, err := prepareInspectionTaskImport(document, payload.Overwrite)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if payload.DryRun {
		resp.Success(c, http.StatusOK, "配置校验通过", result)
		return
	}

	updated := make([]preparedInspectionTaskImport, 0, result.Updated)
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		for _, item := range prepared {
			if item.Existing != nil {
				if !payload.Overwrite {
					continue
				}
				if err := tx.Model(&models.InspectionItem{}).Where("id = ?", item.Existing.ID).Updates(inspectionItemConfigUpdates(item.Request)).Error; err != nil {
					return err
				}
				updated = append(updated, item)
				continue
			}
			created := inspectionItemFromRequest(item.Request)
			if err := tx.Create(&created).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "导入拨测任务失败，未写入任何配置")
		return
	}

	now := time.Now()
	for _, item := range updated {
		reconcileInspectionNodeAlerts(*item.Existing, resolveInspectionExecutionNodes(item.Request.Config), now)
		if item.Request.Status == 0 {
			terminateTaskAlerts("probe-task", item.Existing.ID, "task_disabled", now)
			database.DB.Model(&models.InspectionNodeState{}).Where("item_id = ?", item.Existing.ID).Updates(map[string]interface{}{"consecutive_failures": 0, "consecutive_successes": 0, "alert_active": false})
			database.DB.Model(&models.InspectionItem{}).Where("id = ?", item.Existing.ID).Updates(map[string]interface{}{"consecutive_failures": 0, "consecutive_successes": 0, "alert_active": false})
		}
	}
	resp.Success(c, http.StatusOK, "导入完成", result)
}

func parseInspectionTaskConfigDocument(content string) (inspectionTaskConfigDocument, error) {
	document := inspectionTaskConfigDocument{}
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
	if document.APIVersion != inspectionConfigAPIVersion || document.Kind != inspectionConfigKind {
		return document, fmt.Errorf("配置类型不正确，应为 apiVersion: %s、kind: %s", inspectionConfigAPIVersion, inspectionConfigKind)
	}
	if len(document.Tasks) == 0 {
		return document, fmt.Errorf("配置中至少需要一个拨测任务")
	}
	if len(document.Tasks) > inspectionConfigMaxTasks {
		return document, fmt.Errorf("单次最多导入 %d 个拨测任务", inspectionConfigMaxTasks)
	}
	return document, nil
}

func prepareInspectionTaskImport(document inspectionTaskConfigDocument, overwrite bool) ([]preparedInspectionTaskImport, inspectionTaskImportResult, error) {
	names := make([]string, 0, len(document.Tasks))
	seen := make(map[string]struct{}, len(document.Tasks))
	requests := make([]CreateInspectionItemReq, 0, len(document.Tasks))
	for index, task := range document.Tasks {
		request, err := inspectionTaskConfigRequest(task)
		if err != nil {
			return nil, inspectionTaskImportResult{}, fmt.Errorf("第 %d 个任务配置错误：%v", index+1, err)
		}
		nameKey := fmt.Sprintf("%d/%s", request.GroupID, inspectionTaskNameKey(request.Name))
		if _, exists := seen[nameKey]; exists {
			return nil, inspectionTaskImportResult{}, fmt.Errorf("配置中存在重复任务名称：%s", request.Name)
		}
		seen[nameKey] = struct{}{}
		names = append(names, request.Name)
		requests = append(requests, request)
	}

	var existingItems []models.InspectionItem
	if err := database.DB.Where("name IN ?", names).Order("id ASC").Find(&existingItems).Error; err != nil {
		return nil, inspectionTaskImportResult{}, fmt.Errorf("检查同名任务失败")
	}
	existingByName := make(map[string]*models.InspectionItem, len(existingItems))
	for index := range existingItems {
		item := &existingItems[index]
		nameKey := fmt.Sprintf("%d/%s", item.GroupID, inspectionTaskNameKey(item.Name))
		if _, requested := seen[nameKey]; !requested {
			continue
		}
		if _, exists := existingByName[nameKey]; exists {
			return nil, inspectionTaskImportResult{}, fmt.Errorf("数据库中存在多个同名任务：%s，请先处理重名数据", item.Name)
		}
		existingByName[nameKey] = item
	}

	result := inspectionTaskImportResult{Total: len(requests)}
	prepared := make([]preparedInspectionTaskImport, 0, len(requests))
	for _, request := range requests {
		existing := existingByName[fmt.Sprintf("%d/%s", request.GroupID, inspectionTaskNameKey(request.Name))]
		prepared = append(prepared, preparedInspectionTaskImport{Request: request, Existing: existing})
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

func inspectionTaskNameKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func inspectionTaskConfigRequest(task inspectionTaskConfigEntry) (CreateInspectionItemReq, error) {
	status := 1
	if task.Status != nil {
		status = *task.Status
	}
	if task.IntervalSeconds == 0 {
		task.IntervalSeconds = 300
		if strings.TrimSpace(task.Type) == "certificate_expiry" {
			task.IntervalSeconds = 21600
		}
	}
	if task.FailureThreshold == 0 {
		task.FailureThreshold = 3
		if strings.TrimSpace(task.Type) == "certificate_expiry" {
			task.FailureThreshold = 1
		}
	}
	if task.RecoveryThreshold == 0 {
		task.RecoveryThreshold = 3
		if strings.TrimSpace(task.Type) == "certificate_expiry" {
			task.RecoveryThreshold = 1
		}
	}
	if task.ExpectedResult == "" {
		task.ExpectedResult = "拨测成功"
	}
	name := strings.TrimSpace(task.Name)
	if name == "" || len([]rune(name)) > 100 {
		return CreateInspectionItemReq{}, fmt.Errorf("任务名称不能为空且不能超过 100 个字符")
	}
	config, err := json.Marshal(task.Config)
	if err != nil {
		return CreateInspectionItemReq{}, fmt.Errorf("拨测配置无法转换为内部格式")
	}
	if task.ExternalLabels == nil {
		task.ExternalLabels = map[string]string{}
	}
	labels, err := json.Marshal(task.ExternalLabels)
	if err != nil {
		return CreateInspectionItemReq{}, fmt.Errorf("额外标签无法转换为内部格式")
	}
	if task.GroupID == 0 {
		return CreateInspectionItemReq{}, fmt.Errorf("请指定拨测规则组 group_id，或选择导入目标规则组")
	}
	request := CreateInspectionItemReq{
		GroupID: task.GroupID,
		Name:    name, Type: task.Type, Target: task.Target, Config: string(config), ExternalLabels: string(labels),
		ExpectedResult: task.ExpectedResult, IntervalSeconds: task.IntervalSeconds, FailureThreshold: task.FailureThreshold,
		RecoveryThreshold: task.RecoveryThreshold, Severity: normalizeInspectionSeverity(task.Severity), FaultCenterID: task.FaultCenterID, Status: status,
	}
	normalizedLabels, err := normalizeInspectionLabels(request.ExternalLabels)
	if err != nil {
		return CreateInspectionItemReq{}, err
	}
	request.ExternalLabels = normalizedLabels
	if err := validateInspectionItemReq(request); err != nil {
		return CreateInspectionItemReq{}, err
	}
	return request, nil
}

func inspectionTaskConfigDocumentFromItems(items []models.InspectionItem) (inspectionTaskConfigDocument, error) {
	document := inspectionTaskConfigDocument{APIVersion: inspectionConfigAPIVersion, Kind: inspectionConfigKind, Tasks: make([]inspectionTaskConfigEntry, 0, len(items))}
	for _, item := range items {
		config := map[string]interface{}{}
		if err := json.Unmarshal([]byte(item.Config), &config); err != nil {
			return document, fmt.Errorf("任务 %s 的拨测配置损坏，无法导出", item.Name)
		}
		status := item.Status
		document.Tasks = append(document.Tasks, inspectionTaskConfigEntry{
			GroupID: item.GroupID, Name: item.Name, Type: item.Type, Target: item.Target, Config: config, ExternalLabels: parseInspectionLabels(item.Labels),
			ExpectedResult: item.ExpectedResult, IntervalSeconds: item.IntervalSeconds, FailureThreshold: item.FailureThreshold,
			RecoveryThreshold: item.RecoveryThreshold, Severity: item.Severity, FaultCenterID: item.FaultCenterID, Status: &status,
		})
	}
	sort.SliceStable(document.Tasks, func(i, j int) bool { return document.Tasks[i].Name < document.Tasks[j].Name })
	return document, nil
}

func inspectionItemFromRequest(req CreateInspectionItemReq) models.InspectionItem {
	return models.InspectionItem{
		GroupID: req.GroupID, Name: strings.TrimSpace(req.Name), Type: strings.ToLower(strings.TrimSpace(req.Type)), Target: strings.TrimSpace(req.Target),
		Config: req.Config, Labels: req.ExternalLabels, ExpectedResult: strings.TrimSpace(req.ExpectedResult), IntervalSeconds: req.IntervalSeconds,
		FailureThreshold: req.FailureThreshold, RecoveryThreshold: req.RecoveryThreshold, Severity: req.Severity, FaultCenterID: req.FaultCenterID, Status: req.Status,
	}
}

func inspectionItemConfigUpdates(req CreateInspectionItemReq) map[string]interface{} {
	item := inspectionItemFromRequest(req)
	return map[string]interface{}{
		"group_id": item.GroupID, "name": item.Name, "type": item.Type, "target": item.Target, "config": item.Config, "labels": item.Labels,
		"expected_result": item.ExpectedResult, "interval_seconds": item.IntervalSeconds, "failure_threshold": item.FailureThreshold,
		"recovery_threshold": item.RecoveryThreshold, "severity": item.Severity, "fault_center_id": item.FaultCenterID, "status": item.Status,
	}
}
