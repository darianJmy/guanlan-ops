package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"guanlan/pkg/resp"
	"guanlan/pkg/textutil"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	aicore "guanlan/internal/ai"
	"guanlan/internal/database"
	"guanlan/internal/models"

	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ── Conversation CRUD ──

func ListConversations(c *gin.Context) {
	userID := currentUserID(c)
	if userID == 0 {
		resp.Error(c, http.StatusUnauthorized, resp.CodeUnauthorized, "用户身份无效")
		return
	}
	var convos []models.ChatConversation
	if err := chatConversationScope(database.DB, userID).Order("updated_at DESC").Find(&convos).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "读取对话失败")
		return
	}
	resp.Success(c, http.StatusOK, "成功", convos)
}

func CreateConversation(c *gin.Context) {
	userID := currentUserID(c)
	if userID == 0 {
		resp.Error(c, http.StatusUnauthorized, resp.CodeUnauthorized, "用户身份无效")
		return
	}
	var req struct {
		Title       string `json:"title"`
		WorkspaceID uint   `json:"workspace_id"`
	}
	_ = c.ShouldBindJSON(&req)
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		req.Title = "新对话"
	}
	convo := models.ChatConversation{UserID: userID, Title: textutil.Ellipsis(req.Title, 100), WorkspaceID: req.WorkspaceID}
	if err := database.DB.Create(&convo).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "创建对话失败")
		return
	}
	resp.Success(c, http.StatusOK, "创建成功", convo)
}

func DeleteConversation(c *gin.Context) {
	convo, ok := findOwnedConversation(c, c.Param("id"))
	if !ok {
		return
	}
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("conversation_id = ?", convo.ID).Delete(&models.ChatMessage{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ? AND user_id = ?", convo.ID, convo.UserID).Delete(&models.ChatConversation{}).Error
	}); err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "删除对话失败")
		return
	}
	resp.Success(c, http.StatusOK, "删除成功")
}

func GetMessages(c *gin.Context) {
	convo, ok := findOwnedConversation(c, c.Param("id"))
	if !ok {
		return
	}
	var messages []models.ChatMessage
	if err := database.DB.Where("conversation_id = ?", convo.ID).Order("id ASC").Find(&messages).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "读取消息失败")
		return
	}
	resp.Success(c, http.StatusOK, "成功", messages)
}

func findOwnedConversation(c *gin.Context, rawID string) (models.ChatConversation, bool) {
	userID := currentUserID(c)
	id, err := strconv.ParseUint(strings.TrimSpace(rawID), 10, 64)
	if userID == 0 || err != nil || id == 0 {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "对话不存在")
		return models.ChatConversation{}, false
	}
	var convo models.ChatConversation
	err = chatConversationScope(database.DB, userID).Where("id = ?", uint(id)).First(&convo).Error
	if err == nil {
		return convo, true
	}
	if err == gorm.ErrRecordNotFound {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "对话不存在")
	} else {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "读取对话失败")
	}
	return models.ChatConversation{}, false
}

func chatConversationScope(db *gorm.DB, userID uint) *gorm.DB {
	return db.Where("user_id = ?", userID)
}

// ── Chat (核心) ──

type ChatReq struct {
	ModelID        uint   `json:"model_id"`
	ConversationID uint   `json:"conversation_id"`
	WorkspaceID    uint   `json:"workspace_id"`
	Message        string `json:"message" binding:"required"`
}

func SendMessage(c *gin.Context) {
	var req ChatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" || len([]rune(req.Message)) > 4000 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请输入不超过 4000 个字符的问题")
		return
	}
	userID := currentUserID(c)
	if userID == 0 {
		resp.Error(c, http.StatusUnauthorized, resp.CodeUnauthorized, "用户身份无效")
		return
	}

	// Validate ownership and chosen model before saving any messages.
	if req.ConversationID != 0 {
		convo, ok := findOwnedConversation(c, strconv.FormatUint(uint64(req.ConversationID), 10))
		if !ok {
			return
		}
		req.WorkspaceID = convo.WorkspaceID
		if req.ModelID == 0 {
			req.ModelID = convo.ModelID
		}
	}
	selectedModel, err := resolveAIModel(c.Request.Context(), req.ModelID)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	req.ModelID = selectedModel.ID
	// 确保会话存在
	if req.ConversationID == 0 {
		convo := models.ChatConversation{UserID: userID, Title: textutil.Ellipsis(req.Message, 30), WorkspaceID: req.WorkspaceID, ModelID: req.ModelID}
		if err := database.DB.Create(&convo).Error; err != nil {
			resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "创建对话失败")
			return
		}
		req.ConversationID = convo.ID
	} else {
		convo, ok := findOwnedConversation(c, strconv.FormatUint(uint64(req.ConversationID), 10))
		if !ok {
			return
		}
		req.WorkspaceID = convo.WorkspaceID
	}

	release, accepted := acquireChatExecution(userID, req.ConversationID)
	if !accepted {
		resp.Error(c, http.StatusTooManyRequests, resp.CodeTooManyRequests, "当前会话正在分析或服务繁忙，请稍后重试")
		return
	}
	defer release()
	if err := database.DB.Model(&models.ChatConversation{}).Where("id = ? AND user_id = ?", req.ConversationID, userID).Update("model_id", req.ModelID).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "保存模型选择失败")
		return
	}

	// 保存用户消息
	userMsg := models.ChatMessage{ConversationID: req.ConversationID, Role: "user", Content: req.Message}
	if err := database.DB.Create(&userMsg).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "保存消息失败")
		return
	}

	// 当前问题单独传入，历史仅包含此前已保存的消息。
	chatHistory := collectChatHistoryBefore(req.ConversationID, userMsg.ID)
	memoryContext := collectMemoryContext(userID, req.WorkspaceID)
	response, metadata := runAIOrchestrator(c.Request.Context(), userID, req.WorkspaceID, req.Message, chatHistory, memoryContext, selectedModel)

	// 保存助手消息
	metaJSON, _ := json.Marshal(metadata)
	assistantMsg := models.ChatMessage{
		ConversationID: req.ConversationID, Role: "assistant",
		Content: response, Metadata: string(metaJSON),
	}
	if err := database.DB.Create(&assistantMsg).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "保存回复失败")
		return
	}

	// 首条问题用于命名会话，其余消息只刷新排序时间。
	var userMessageCount int64
	database.DB.Model(&models.ChatMessage{}).
		Where("conversation_id = ? AND role = ?", req.ConversationID, "user").Count(&userMessageCount)
	if userMessageCount == 1 {
		database.DB.Model(&models.ChatConversation{}).Where("id = ? AND user_id = ?", req.ConversationID, userID).
			Update("title", textutil.Ellipsis(req.Message, 30))
	}
	database.DB.Model(&models.ChatConversation{}).Where("id = ? AND user_id = ?", req.ConversationID, userID).
		Update("updated_at", time.Now())

	resp.Success(c, http.StatusOK, "成功", assistantMsg)
}

func currentUserID(c *gin.Context) uint {
	value, exists := c.Get("user_id")
	if !exists {
		return 0
	}
	switch typed := value.(type) {
	case uint:
		return typed
	case int:
		return uint(typed)
	case float64:
		return uint(typed)
	default:
		return 0
	}
}

// ── 智能分析引擎 ──

func collectChatHistoryBefore(convoID, beforeID uint) string {
	var msgs []models.ChatMessage
	database.DB.Where("conversation_id = ? AND id < ?", convoID, beforeID).Order("id DESC").Limit(10).Find(&msgs)
	var sb strings.Builder
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		sb.WriteString(fmt.Sprintf("%s: %s\n", m.Role, m.Content))
	}
	return sb.String()
}

func collectMemoryContext(userID, workspaceID uint) string {
	if userID == 0 {
		return ""
	}
	var memories []models.ChatMemory
	tx := database.DB.Where("user_id = ?", userID)
	if workspaceID > 0 {
		tx = tx.Where("workspace_id = 0 OR workspace_id = ?", workspaceID)
	}
	tx.Order("updated_at DESC").Limit(20).Find(&memories)
	var sb strings.Builder
	for _, memory := range memories {
		sb.WriteString(fmt.Sprintf("- %s=%s\n", memory.Key, memory.Value))
	}
	return sb.String()
}

func collectDatasourceCatalog(workspaceID uint) string {
	var ds []models.Datasource
	tx := database.DB.Where("type IN ?", supportedDatasourceTypes())
	if workspaceID > 0 {
		ids, scoped := workspaceDatasourceIDs(workspaceID)
		if scoped && len(ids) == 0 {
			return "[]"
		}
		tx = tx.Where("id IN ?", ids)
	}
	tx.Where("status = 1").Order("priority ASC, id ASC").Find(&ds)
	items := make([]map[string]interface{}, 0, len(ds))
	for _, d := range ds {
		items = append(items, map[string]interface{}{
			"id":           d.ID,
			"name":         d.Name,
			"type":         d.Type,
			"category":     d.Category,
			"region":       d.Region,
			"env":          d.Env,
			"domain":       d.Domain,
			"labels":       datasourceLabels(d),
			"capabilities": effectiveCapabilities(d),
			"priority":     priorityOrDefault(d.Priority),
			"tools":        datasourceCapabilities(d.Type),
		})
	}
	body, _ := json.MarshalIndent(items, "", "  ")
	return string(body)
}

func workspaceDatasourceIDs(workspaceID uint) ([]uint, bool) {
	if workspaceID == 0 {
		return nil, false
	}
	var count int64
	if database.DB.Model(&models.DatasourceGroup{}).Where("id = ?", workspaceID).Count(&count); count == 0 {
		return nil, true
	}
	var members []models.DatasourceGroupMember
	database.DB.Where("group_id = ?", workspaceID).Order("id ASC").Find(&members)
	ids := make([]uint, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.DatasourceID)
	}
	return ids, true
}

func effectiveCapabilities(ds models.Datasource) string {
	if definition, ok := datasourceDefinitionFor(ds.Type); ok {
		return strings.Join(definition.Capabilities, ",")
	}
	return ""
}

func datasourceLabels(ds models.Datasource) map[string]string {
	return parseInspectionLabels(ds.Labels)
}

func datasourceCapabilities(dsType string) []string {
	if isMetricDatasourceType(dsType) {
		return []string{"query_prometheus_instant"}
	}
	return []string{}
}

func buildOpsToolRegistry(workspaceID uint) *aicore.Registry {
	registry := aicore.NewRegistry()
	mustRegisterTool(registry, aicore.ToolSpec{
		Name:           "query_prometheus_instant",
		Description:    "执行 Prometheus 或 VictoriaMetrics 即时 PromQL 查询。LLM 只传 datasource_id 和 promql，不传 URL。",
		DatasourceType: "prometheus",
		Risk:           aicore.RiskReadonlyLow,
		TimeoutSeconds: 10,
		ParamsSchema: map[string]*schema.ParameterInfo{
			"datasource_id": {Type: schema.Integer, Required: true, Desc: "数据源 ID"},
			"promql":        {Type: schema.String, Required: true, Desc: "PromQL 查询表达式"},
		},
	}, scopedToolHandler(workspaceID, handleQueryPrometheusInstant))
	return registry
}

func mustRegisterTool(registry *aicore.Registry, spec aicore.ToolSpec, handler aicore.ToolHandler) {
	if err := registry.Register(spec, handler); err != nil {
		panic(err)
	}
}

func scopedToolHandler(workspaceID uint, handler aicore.ToolHandler) aicore.ToolHandler {
	return func(ctx context.Context, call aicore.ToolCall) aicore.ToolResult {
		if call.Params == nil {
			call.Params = map[string]interface{}{}
		}
		call.Params["_workspace_id"] = float64(workspaceID)
		return handler(ctx, call)
	}
}

func handleQueryPrometheusInstant(ctx context.Context, call aicore.ToolCall) aicore.ToolResult {
	ds, err := loadDatasourceByToolParam(call.Params, "prometheus")
	if err != nil {
		return toolError(call.Tool, "DATASOURCE_ERROR", err)
	}
	promql := strings.TrimSpace(stringParam(call.Params, "promql"))
	if promql == "" {
		return aicore.ToolResult{Tool: call.Tool, OK: false, ErrorCode: "MISSING_PROMQL", Message: "缺少 promql 参数", Retryable: true}
	}
	value, err := executePromQLContext(ctx, ds, promql)
	if err != nil {
		return toolError(call.Tool, "PROMETHEUS_QUERY_FAILED", err)
	}
	return aicore.ToolResult{Tool: call.Tool, OK: true, Summary: "指标查询成功", Data: map[string]interface{}{"datasource": ds.Name, "promql": promql, "result": value}}
}

func loadDatasourceByToolParam(params map[string]interface{}, expectedType string) (models.Datasource, error) {
	if !isMetricDatasourceType(expectedType) {
		return models.Datasource{}, fmt.Errorf("仅支持具备 PromQL 查询能力的指标数据源")
	}
	id := uint(intParam(params, "datasource_id"))
	workspaceID := uint(intParam(params, "_workspace_id"))
	var ds models.Datasource
	tx := database.DB.Where("status = 1 AND type IN ?", metricDatasourceTypes())
	if workspaceID > 0 {
		ids, scoped := workspaceDatasourceIDs(workspaceID)
		if scoped && len(ids) == 0 {
			return ds, fmt.Errorf("当前数据源分组未关联数据源，请先在「数据源分组」中关联接入")
		}
		tx = tx.Where("id IN ?", ids)
	}
	if id > 0 {
		tx = tx.Where("id = ?", id)
	}
	if err := tx.Order("id ASC").First(&ds).Error; err != nil {
		return ds, fmt.Errorf("未找到可用的 %s 数据源", expectedType)
	}
	return ds, nil
}

func toolError(tool, code string, err error) aicore.ToolResult {
	return aicore.ToolResult{Tool: tool, OK: false, ErrorCode: code, Message: err.Error(), Retryable: true}
}

func atoiDefault(value string, fallback int) int {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func selectDatasourcesByQuestion(q string, datasources []models.Datasource) []models.Datasource {
	normalizedQuestion := normalizeDatasourceMatchText(q)
	var matched []models.Datasource
	for _, ds := range datasources {
		name := normalizeDatasourceMatchText(ds.Name)
		if name != "" && strings.Contains(normalizedQuestion, name) {
			matched = append(matched, ds)
		}
	}
	if len(matched) > 0 {
		return matched
	}
	return datasources
}

func stringParam(params map[string]interface{}, key string) string {
	if value, ok := params[key].(string); ok {
		return value
	}
	return ""
}

func intParam(params map[string]interface{}, key string) int {
	switch value := params[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return 0
	}
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func normalizeDatasourceMatchText(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, "\t", "")
	return value
}

func collectPlatformKnowledge() string {
	candidates := []string{
		"docs/aiops-agent-goal.md",
	}
	for _, path := range candidates {
		body, err := os.ReadFile(path)
		if err == nil {
			return textutil.Ellipsis(string(body), 5000)
		}
	}
	return `观澜运维平台包含数据源、数据源分组、Prometheus 查询、拨测、脚本、自动化任务、证书和用户权限等模块。助手只能通过后端白名单工具读取和操作平台资源；明确请求且参数齐全时可创建工具清单中开放的资源，包括无认证直连数据源、故障中心、各类分组、通知模板及邮件对象；可创建/修改具体告警规则，并通过 search_platform_operations、describe_platform_operation、execute_platform_operation 发现和执行平台目录操作，权限以实际执行检查为准。`
}

func executePromQLContext(ctx context.Context, ds models.Datasource, query string) (string, error) {
	client, cleanup, err := datasourceHTTPClient(ds, 10*time.Second)
	if err != nil {
		return "", err
	}
	defer cleanup()

	apiURL, err := url.Parse(strings.TrimRight(ds.URL, "/") + "/api/v1/query")
	if err != nil {
		return "", fmt.Errorf("Prometheus 地址无效: %w", err)
	}
	params := apiURL.Query()
	params.Set("query", query)
	apiURL.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("查询失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取 Prometheus 响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Prometheus HTTP %d: %s", resp.StatusCode, textutil.Ellipsis(string(body), 500))
	}

	var promResp struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  []interface{}     `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &promResp); err != nil {
		return "", fmt.Errorf("解析 Prometheus 响应失败: %w", err)
	}

	if promResp.Status != "success" {
		return "", fmt.Errorf("Prometheus 查询异常: %s", textutil.Ellipsis(string(body), 500))
	}

	var sb strings.Builder
	for _, r := range promResp.Data.Result {
		labels := ""
		for k, v := range r.Metric {
			if k != "__name__" && k != "job" {
				if labels != "" {
					labels += ","
				}
				labels += fmt.Sprintf("%s=%s", k, v)
			}
		}
		if len(r.Value) >= 2 {
			sb.WriteString(fmt.Sprintf("%s → %v\n", labels, r.Value[1]))
		}
	}
	return sb.String(), nil
}
