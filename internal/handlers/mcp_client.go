package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"guanlan/pkg/textutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	currentMCPProtocolVersion = "2025-11-25"
	maxMCPClientPageSize      = 100
)

var supportedMCPProtocolVersions = map[string]struct{}{
	"2025-03-26": {},
	"2025-06-18": {},
	"2025-11-25": {},
}

var allowedMCPClientTypes = map[string]struct{}{
	"codex": {}, "claude": {}, "cursor": {}, "custom": {},
}

type mcpClientReq struct {
	Name        string `json:"name"`
	ClientType  string `json:"client_type"`
	APITokenID  string `json:"api_token_id"`
	Description string `json:"description"`
	Status      int    `json:"status"`
}

func ListMCPClients(c *gin.Context) {
	page := mcpPositiveInt(c.Query("page"), 1)
	pageSize := mcpPositiveInt(c.Query("page_size"), 10)
	if pageSize > maxMCPClientPageSize {
		pageSize = maxMCPClientPageSize
	}

	tx := database.DB.Model(&models.MCPClient{}).
		Joins("LEFT JOIN api_access_tokens ON api_access_tokens.id = mcp_clients.api_token_id").
		Joins("LEFT JOIN users ON users.id = api_access_tokens.user_id")
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		like := "%" + keyword + "%"
		tx = tx.Where("mcp_clients.name LIKE ? OR api_access_tokens.name LIKE ? OR users.username LIKE ? OR users.nickname LIKE ?", like, like, like, like)
	}
	if clientType := strings.TrimSpace(c.Query("client_type")); clientType != "" {
		tx = tx.Where("mcp_clients.client_type = ?", clientType)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		if value, err := strconv.Atoi(status); err == nil && (value == 0 || value == 1) {
			tx = tx.Where("mcp_clients.status = ?", value)
		}
	}
	if compatibility := strings.TrimSpace(c.Query("compatibility")); compatibility != "" {
		switch compatibility {
		case "never_connected":
			tx = tx.Where("mcp_clients.last_seen_at IS NULL")
		case "compatible":
			tx = tx.Where("mcp_clients.last_seen_at IS NOT NULL AND mcp_clients.protocol_version = ?", currentMCPProtocolVersion)
		case "incompatible":
			tx = tx.Where("mcp_clients.last_seen_at IS NOT NULL AND (mcp_clients.protocol_version = '' OR mcp_clients.protocol_version <> ?)", currentMCPProtocolVersion)
		}
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "读取 MCP 连接失败")
		return
	}
	var clients []models.MCPClient
	if err := tx.Preload("APIToken.User").Order("mcp_clients.created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&clients).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "读取 MCP 连接失败")
		return
	}
	items := make([]gin.H, 0, len(clients))
	for i := range clients {
		items = append(items, mcpClientResponse(clients[i]))
	}
	resp.Success(c, http.StatusOK, "成功", gin.H{"list": items, "total": total, "page": page, "page_size": pageSize})
}

func CreateMCPClient(c *gin.Context) {
	var req mcpClientReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "MCP 连接格式不正确")
		return
	}
	req = normalizeMCPClientReq(req)
	if message := validateMCPClientReq(req); message != "" {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, message)
		return
	}
	token, message := availableMCPToken(req.APITokenID, "")
	if message != "" {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, message)
		return
	}
	id, err := newMCPClientID()
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "生成 MCP 连接标识失败")
		return
	}
	client := models.MCPClient{ID: id, Name: req.Name, ClientType: req.ClientType, APITokenID: token.ID, APIToken: token, Description: req.Description, Status: req.Status, CreatedBy: c.GetUint("user_id")}
	if err := database.DB.Create(&client).Error; err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "客户端名称或访问令牌已被使用")
		return
	}
	resp.Success(c, http.StatusOK, "MCP 连接已创建", mcpClientResponse(client))
}

func UpdateMCPClient(c *gin.Context) {
	var client models.MCPClient
	if err := database.DB.First(&client, "id = ?", c.Param("id")).Error; err != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "MCP 连接不存在")
		return
	}
	var req mcpClientReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "MCP 连接格式不正确")
		return
	}
	req = normalizeMCPClientReq(req)
	if message := validateMCPClientReq(req); message != "" {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, message)
		return
	}
	if _, message := availableMCPToken(req.APITokenID, client.ID); message != "" {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, message)
		return
	}
	if err := database.DB.Model(&client).Updates(map[string]interface{}{"name": req.Name, "client_type": req.ClientType, "api_token_id": req.APITokenID, "description": req.Description, "status": req.Status}).Error; err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "客户端名称或访问令牌已被使用")
		return
	}
	resp.Success(c, http.StatusOK, "MCP 连接已更新")
}

func UpdateMCPClientStatus(c *gin.Context) {
	var req struct {
		Status int `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || (req.Status != 0 && req.Status != 1) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "启用状态不正确")
		return
	}
	result := database.DB.Model(&models.MCPClient{}).Where("id = ?", c.Param("id")).Update("status", req.Status)
	if result.Error != nil || result.RowsAffected == 0 {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "MCP 连接不存在")
		return
	}
	resp.Success(c, http.StatusOK, "MCP 连接状态已更新")
}

func DeleteMCPClient(c *gin.Context) {
	result := database.DB.Where("id = ?", c.Param("id")).Delete(&models.MCPClient{})
	if result.Error != nil || result.RowsAffected == 0 {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "MCP 连接不存在")
		return
	}
	resp.Success(c, http.StatusOK, "MCP 连接已删除")
}

func normalizeMCPClientReq(req mcpClientReq) mcpClientReq {
	req.Name = strings.TrimSpace(req.Name)
	req.ClientType = strings.ToLower(strings.TrimSpace(req.ClientType))
	req.APITokenID = strings.TrimSpace(req.APITokenID)
	req.Description = strings.TrimSpace(req.Description)
	if req.Status != 0 {
		req.Status = 1
	}
	return req
}

func validateMCPClientReq(req mcpClientReq) string {
	if req.Name == "" || len([]rune(req.Name)) > 100 {
		return "请输入不超过 100 个字符的客户端名称"
	}
	if _, ok := allowedMCPClientTypes[req.ClientType]; !ok {
		return "客户端类型不正确"
	}
	if req.APITokenID == "" {
		return "请选择访问令牌"
	}
	if len([]rune(req.Description)) > 500 {
		return "备注不能超过 500 个字符"
	}
	return ""
}

func availableMCPToken(tokenID, currentClientID string) (models.APIAccessToken, string) {
	var token models.APIAccessToken
	if err := database.DB.Preload("User").Where("api_access_tokens.id = ?", tokenID).First(&token).Error; err != nil || token.Status != 1 || token.User.Status != 1 || !token.ExpiresAt.After(time.Now()) {
		return token, "访问令牌不存在、已停用或已过期"
	}
	var count int64
	tx := database.DB.Model(&models.MCPClient{}).Where("api_token_id = ?", tokenID)
	if currentClientID != "" {
		tx = tx.Where("id <> ?", currentClientID)
	}
	if tx.Count(&count).Error != nil || count > 0 {
		return token, "该访问令牌已用于其他 MCP 连接"
	}
	return token, ""
}

func mcpClientResponse(client models.MCPClient) gin.H {
	return gin.H{
		"id": client.ID, "name": client.Name, "client_type": client.ClientType, "api_token_id": client.APITokenID,
		"reported_name":  client.ReportedName,
		"api_token_name": client.APIToken.Name, "token_prefix": client.APIToken.TokenPrefix, "user_id": client.APIToken.UserID,
		"username": client.APIToken.User.Username, "nickname": client.APIToken.User.Nickname,
		"client_version": client.ClientVersion, "protocol_version": client.ProtocolVersion, "platform_protocol_version": currentMCPProtocolVersion,
		"compatibility": mcpCompatibilityStatus(client), "description": client.Description, "status": client.Status,
		"last_seen_at": client.LastSeenAt, "last_ip": client.LastIP, "last_user_agent": client.LastUserAgent,
		"created_by": client.CreatedBy, "created_at": client.CreatedAt, "updated_at": client.UpdatedAt,
	}
}

func mcpCompatibilityStatus(client models.MCPClient) string {
	if client.LastSeenAt == nil {
		return "never_connected"
	}
	if _, ok := supportedMCPProtocolVersions[client.ProtocolVersion]; ok {
		return "compatible"
	}
	return "incompatible"
}

func newMCPClientID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func mcpPositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func truncateMCPText(value string, limit int) string {
	return textutil.Clip(strings.TrimSpace(value), limit)
}
