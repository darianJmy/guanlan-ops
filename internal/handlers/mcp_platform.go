package handlers

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"guanlan/internal/database"
	"guanlan/internal/models"
)

func executePlatformMCPTool(ctx context.Context, userID uint, name string, args map[string]interface{}) (interface{}, error) {
	tool, ok := findMCPTool(name)
	if !ok || !isPlatformMCPTool(name) {
		return nil, errors.New("工具不存在")
	}
	if err := authorizeAIPermission(ctx, userID, tool.Permission); err != nil {
		return nil, errors.New("没有调用该工具的权限")
	}
	normalized, err := validatePlatformMCPArgs(tool, args)
	if err != nil {
		return nil, err
	}
	for _, query := range platformMCPQueries() {
		if query.name == name {
			return queryPlatformMCP(ctx, query, normalized)
		}
	}
	return createPlatformMCPResource(ctx, userID, name, normalized)
}

func queryPlatformMCP(ctx context.Context, definition platformMCPQuery, args map[string]interface{}) (interface{}, error) {
	page, size := mcpPagination(args)
	query := database.DB.WithContext(ctx).Model(definition.model)
	if id, ok := args["id"]; ok {
		query = query.Where("id = ?", id)
	}
	if keyword := strings.TrimSpace(mcpStringArg(args, "keyword")); keyword != "" {
		conditions := []string{}
		values := []interface{}{}
		for _, column := range definition.keywordColumns {
			conditions = append(conditions, column+" LIKE ?")
			values = append(values, "%"+keyword+"%")
		}
		query = query.Where(strings.Join(conditions, " OR "), values...)
	}
	for key, column := range definition.filters {
		if value, ok := args[key]; ok {
			query = query.Where(column+" = ?", value)
		}
	}
	if enabled, ok := args["enabled"].(bool); ok {
		status := 0
		if enabled {
			status = 1
		}
		query = query.Where("status = ?", status)
	}
	var start, end time.Time
	for _, key := range []string{"start_time", "end_time"} {
		if raw, ok := args[key]; ok {
			value, err := time.Parse(time.RFC3339, raw.(string))
			if err != nil {
				return nil, fmt.Errorf("%s 必须为带时区的 RFC3339 时间", key)
			}
			if key == "start_time" {
				start = value
				query = query.Where(definition.timeColumn+" >= ?", value)
			} else {
				end = value
				query = query.Where(definition.timeColumn+" <= ?", value)
			}
		}
	}
	if !start.IsZero() && !end.IsZero() && start.After(end) {
		return nil, errors.New("开始时间不能晚于结束时间")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, errors.New("查询平台记录失败")
	}
	rows := []map[string]interface{}{}
	order := "id DESC"
	if definition.timeColumn != "" {
		order = definition.timeColumn + " DESC, id DESC"
	}
	if err := query.Select(definition.columns).Order(order).Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, errors.New("查询平台记录失败")
	}
	for _, row := range rows {
		// Keep the model result budget bounded without silently presenting partial content as complete.
		for key, value := range row {
			if text, ok := value.(string); ok && len([]rune(text)) > 4000 {
				row[key] = string([]rune(text)[:4000])
				row[key+"_truncated"] = true
			}
		}
		if definition.name == "query_datasource_groups" {
			ids := []uint{}
			if err := database.DB.WithContext(ctx).Model(&models.DatasourceGroupMember{}).Where("group_id = ?", row["id"]).Order("id ASC").Pluck("datasource_id", &ids).Error; err != nil {
				return nil, errors.New("查询数据源分组成员失败")
			}
			row["datasource_ids"] = ids
			row["member_count"] = len(ids)
		}
	}
	return map[string]interface{}{"list": rows, "total": total, "page": page, "page_size": size}, nil
}

type platformInputError struct{ message string }

func (e *platformInputError) Error() string { return e.message }

type createPlatformReq struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	DatasourceIDs   []uint   `json:"datasource_ids"`
	Type            string   `json:"type"`
	Content         string   `json:"content"`
	TemplateID      uint     `json:"template_id"`
	EmailRecipients []string `json:"email_recipients"`
}

func createPlatformMCPResource(ctx context.Context, userID uint, toolName string, args map[string]interface{}) (interface{}, error) {
	var req createPlatformReq
	if err := decodeDatasourceToolArgs(args, &req); err != nil {
		return nil, err
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if req.Name == "" {
		return nil, errors.New("名称不能为空")
	}
	var model interface{}
	var id *uint
	var resource string
	after := func(*gorm.DB) error { return nil }
	before := func(*gorm.DB) error { return nil }
	switch toolName {
	case "create_fault_center":
		if err := validateFaultCenterReq(saveFaultCenterReq{Name: req.Name, Description: req.Description, Status: 1}, 0); err != nil {
			return nil, err
		}
		row := &models.FaultCenter{Name: req.Name, Description: req.Description, Status: 1}
		model, id, resource = row, &row.ID, "fault_center"
	case "create_datasource_group":
		name, description, ids, err := normalizeDatasourceGroupInput(saveDatasourceGroupReq{Name: req.Name, Description: req.Description, DatasourceIDs: req.DatasourceIDs})
		if err != nil {
			return nil, err
		}
		row := &models.DatasourceGroup{Name: name, Description: description}
		model, id, resource = row, &row.ID, "workspace"
		after = func(tx *gorm.DB) error { return replaceDatasourceGroupMembers(tx, row.ID, ids) }
	case "create_alert_rule_group":
		name, description, err := normalizeAlertRuleGroupInput(req.Name, req.Description)
		if err != nil {
			return nil, err
		}
		row := &models.AlertRuleGroup{Name: name, Description: description}
		model, id, resource = row, &row.ID, "alert_rule_group"
	case "create_probe_rule_group":
		name, description, err := normalizeInspectionRuleGroupInput(req.Name, req.Description)
		if err != nil {
			return nil, err
		}
		row := &models.InspectionRuleGroup{Name: name, Description: description}
		model, id, resource = row, &row.ID, "inspection_rule_group"
	case "create_notification_template":
		content := strings.TrimSpace(req.Content)
		if content == "" {
			return nil, errors.New("通知模板正文不能为空")
		}
		if err := validateNotificationTemplateContent(content); err != nil {
			return nil, errors.New("通知模板语法不正确，请检查模板变量和语法")
		}
		row := &models.NotificationTemplate{Name: req.Name, Description: req.Description, Type: req.Type, Content: content, AdvancedCard: req.Type == "feishu", Status: 1}
		model, id, resource = row, &row.ID, "notification_template"
	case "create_email_notification_channel":
		prepared, err := prepareNotificationChannel(saveNotificationChannelReq{Type: "email", EmailRecipients: req.EmailRecipients}, nil)
		if err != nil {
			return nil, err
		}
		row := &models.NotificationChannel{Name: req.Name, Description: req.Description, Type: "email", TemplateID: req.TemplateID, EmailRecipientsJSON: prepared.EmailRecipientsJSON, Status: 1}
		model, id, resource = row, &row.ID, "notification_channel"
		before = func(tx *gorm.DB) error {
			var count int64
			if err := tx.Model(&models.NotificationTemplate{}).Where("id = ? AND type = ? AND status = 1", req.TemplateID, "email").Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return &platformInputError{"请选择已启用的邮件通知模板"}
			}
			return nil
		}
	default:
		return nil, errors.New("未开放该创建操作")
	}
	err := database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(model).Where("name = ?", req.Name).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return &platformInputError{"同名资源已存在，请先查询确认，避免重复创建"}
		}
		if err := before(tx); err != nil {
			return err
		}
		if err := tx.Create(model).Error; err != nil {
			return err
		}
		if err := after(tx); err != nil {
			return err
		}
		return tx.Create(&models.AuditLog{UserID: userID, Module: "AI/MCP", Action: "create", ResourceType: resource, ResourceID: strconv.FormatUint(uint64(*id), 10), ResourceName: req.Name, Method: "MCP", Path: "tools/" + toolName, RequestSummary: "通过 AI/MCP 创建平台资源", Result: "success", StatusCode: 200, ResponseMessage: "创建成功"}).Error
	})
	if err != nil {
		var input *platformInputError
		if errors.As(err, &input) {
			return nil, input
		}
		return nil, errors.New("创建失败，数据库操作未完成")
	}
	result := map[string]interface{}{"id": *id, "name": req.Name, "resource_type": resource, "message": "创建成功"}
	if toolName == "create_email_notification_channel" {
		result["message"] = "邮件通知对象已创建，未发送邮件"
	}
	return result, nil
}
