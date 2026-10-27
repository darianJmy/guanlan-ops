package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
	aicore "guanlan/internal/ai"
	"guanlan/internal/database"
	"guanlan/internal/models"
)

func datasourceMCPTools() []mcpTool {
	createSchema := mcpObjectSchema(map[string]interface{}{
		"name":      map[string]interface{}{"type": "string", "description": "用户指定的数据源名称"},
		"type":      mcpStringEnum(append(metricDatasourceTypes(), "loki")...),
		"url":       map[string]interface{}{"type": "string", "description": "用户提供的 HTTP/HTTPS 服务地址，不包含凭据或查询参数"},
		"auth_type": mcpStringEnum("none"),
		"remark":    map[string]interface{}{"type": "string", "description": "可选备注"},
	})
	createSchema["required"] = []string{"name", "type", "url", "auth_type"}
	testSchema := mcpObjectSchema(map[string]interface{}{"datasource_id": map[string]interface{}{"type": "integer", "minimum": 1, "description": "已保存的数据源 ID"}})
	testSchema["required"] = []string{"datasource_id"}
	return []mcpTool{
		{Name: "query_datasources", Title: "查询数据源", Permission: "datasource:list", Description: "查询平台全部受支持的数据源实例（不按当前分组筛选），返回 ID、名称、类型、状态和认证方式，不返回地址或密钥。可用于检查是否已经创建。", InputSchema: mcpObjectSchema(map[string]interface{}{"keyword": map[string]interface{}{"type": "string"}, "page": mcpPageSchema(1), "page_size": mcpPageSizeSchema()}), Annotations: mcpReadOnlyAnnotations()},
		{Name: "create_datasource", AssistantWritable: true, Title: "创建数据源", Permission: "datasource:create", Description: "按用户明确的创建请求新增启用的数据源。仅支持直连、无认证的 Prometheus/VictoriaMetrics/Loki；Elasticsearch/ClickHouse 需在接入中心配置日志字段映射；需用户提供名称、类型、URL，并明确无需认证。缺少信息先询问，不得编造；需认证或隧道请引导页面配置。返回实际 ID，不代表连接成功；不要因未知结果盲目重试。", InputSchema: createSchema, Annotations: map[string]interface{}{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": false, "openWorldHint": false}},
		{Name: "test_datasource_connection", Title: "测试数据源连接", Permission: "datasource:list", Description: "测试已保存数据源的连接，使用平台保存的认证信息；不接收新的 URL 或凭据。", InputSchema: testSchema, Annotations: mcpReadOnlyAnnotations()},
	}
}

func mcpRequiredFields(tool mcpTool) []string {
	values, _ := tool.InputSchema["required"].([]string)
	return values
}
func isDatasourceMCPTool(name string) bool {
	return name == "query_datasources" || name == "create_datasource" || name == "test_datasource_connection"
}

func decodeDatasourceToolArgs(args map[string]interface{}, target interface{}) error {
	data, err := json.Marshal(args)
	if err != nil {
		return errors.New("工具参数格式不正确")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("工具参数类型不正确或包含不支持的字段")
	}
	return nil
}

func datasourceToolView(ds models.Datasource) map[string]interface{} {
	return map[string]interface{}{"id": ds.ID, "name": ds.Name, "type": ds.Type, "status": ds.Status, "auth_type": ds.AuthType}
}

// Both local assistant calls and remote MCP calls use the same permission and business checks.
func executeDatasourceMCPTool(ctx context.Context, userID uint, name string, args map[string]interface{}) (interface{}, error) {
	tool, ok := findMCPTool(name)
	if !ok || !isDatasourceMCPTool(name) {
		return nil, errors.New("工具不存在")
	}
	if err := authorizeAIPermission(ctx, userID, tool.Permission); err != nil {
		return nil, errors.New("没有调用该工具的权限")
	}
	switch name {
	case "query_datasources":
		var req struct {
			Keyword  string `json:"keyword"`
			Page     int    `json:"page"`
			PageSize int    `json:"page_size"`
		}
		if err := decodeDatasourceToolArgs(args, &req); err != nil {
			return nil, err
		}
		page, size := mcpPagination(args)
		query := database.DB.WithContext(ctx).Model(&models.Datasource{}).Where("type IN ?", supportedDatasourceTypes())
		if req.Keyword != "" {
			query = query.Where("name LIKE ?", "%"+req.Keyword+"%")
		}
		var count int64
		if err := query.Count(&count).Error; err != nil {
			return nil, errors.New("查询数据源失败")
		}
		var rows []models.Datasource
		if err := query.Select("id", "name", "type", "status", "auth_type").Order("id ASC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
			return nil, errors.New("查询数据源失败")
		}
		list := make([]map[string]interface{}, 0, len(rows))
		for _, row := range rows {
			list = append(list, datasourceToolView(row))
		}
		return map[string]interface{}{"list": list, "total": count, "page": page, "page_size": size}, nil
	case "create_datasource":
		var req struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			URL      string `json:"url"`
			AuthType string `json:"auth_type"`
			Remark   string `json:"remark"`
		}
		if err := decodeDatasourceToolArgs(args, &req); err != nil {
			return nil, err
		}
		if req.AuthType != "none" {
			return nil, errors.New("请明确数据源无需认证；需要认证时请在接入中心创建")
		}
		if len([]rune(req.Remark)) > 500 {
			return nil, errors.New("备注不能超过 500 个字符")
		}
		ds, err := prepareDatasourceForCreate(CreateDatasourceWithTunnelReq{Name: req.Name, Type: req.Type, URL: req.URL, AuthType: "none", Status: 1, Remark: strings.TrimSpace(req.Remark)})
		if err != nil {
			return nil, err
		}
		duplicate := errors.New("同名数据源已存在，请先查询确认，避免重复创建")
		err = database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var count int64
			if err := tx.Model(&models.Datasource{}).Where("name = ?", ds.Name).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return duplicate
			}
			if err := persistNewDatasource(tx, &ds, 1); err != nil {
				return err
			}
			return tx.Create(&models.AuditLog{UserID: userID, Module: "接入中心", Action: "create", ResourceType: "datasource", ResourceID: strconv.FormatUint(uint64(ds.ID), 10), ResourceName: ds.Name, Method: "MCP", Path: "tools/create_datasource", RequestSummary: "通过 AI/MCP 创建数据源", Result: "success", StatusCode: 200, ResponseMessage: "创建成功"}).Error
		})
		if errors.Is(err, duplicate) {
			return nil, err
		}
		if err != nil {
			return nil, errors.New("创建数据源失败，未完成创建")
		}
		return map[string]interface{}{"datasource": datasourceToolView(ds), "message": "创建成功，尚未测试连接"}, nil
	case "test_datasource_connection":
		var req struct {
			ID uint `json:"datasource_id"`
		}
		if err := decodeDatasourceToolArgs(args, &req); err != nil {
			return nil, err
		}
		if req.ID == 0 {
			return nil, errors.New("请指定有效的数据源 ID")
		}
		var ds models.Datasource
		if err := database.DB.WithContext(ctx).Where("type IN ?", supportedDatasourceTypes()).First(&ds, req.ID).Error; err != nil {
			return nil, errors.New("数据源不存在或类型不受支持")
		}
		ok, _ := testConnectionForDatasource(ds)
		message := "连接成功"
		if !ok {
			message = "连接失败，请检查服务地址、网络和认证配置"
		}
		return map[string]interface{}{"datasource_id": ds.ID, "connected": ok, "message": message}, nil
	}
	return nil, fmt.Errorf("工具不存在")
}

func assistantDatasourceTool(ctx context.Context, userID uint, call aicore.ToolCall) aicore.ToolResult {
	data, err := executeDatasourceMCPTool(ctx, userID, call.Tool, call.Params)
	if err != nil {
		return aicore.ToolResult{Tool: call.Tool, ErrorCode: "DATASOURCE_OPERATION_FAILED", Message: err.Error()}
	}
	return aicore.ToolResult{Tool: call.Tool, OK: true, Summary: "数据源操作完成", Data: data}
}
