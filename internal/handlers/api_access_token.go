package handlers

import (
	authutil "guanlan/internal/auth"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type createAPIAccessTokenReq struct {
	Name          string `json:"name"`
	UserID        uint   `json:"user_id"`
	ExpiresInDays *int   `json:"expires_in_days"`
	Description   string `json:"description"`
}

type updateAPIAccessTokenStatusReq struct {
	Status int `json:"status"`
}

type updateAPIAccessTokenReq struct {
	Description string `json:"description"`
}

type regenerateAPIAccessTokenReq struct {
	ExpiresInDays *int `json:"expires_in_days"`
}

const permanentAPITokenYear = 9999

var allowedAPITokenExpiryDays = map[int]struct{}{0: {}, 7: {}, 30: {}, 60: {}, 90: {}}

func ListAPIAccessTokens(c *gin.Context) {
	var items []models.APIAccessToken
	if err := database.DB.Preload("User").Order("created_at DESC").Find(&items).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "读取 API Token 失败")
		return
	}
	var clients []models.MCPClient
	_ = database.DB.Select("id", "api_token_id").Find(&clients).Error
	clientByToken := make(map[string]string, len(clients))
	for _, client := range clients {
		clientByToken[client.APITokenID] = client.ID
	}
	result := make([]gin.H, 0, len(items))
	for i := range items {
		view := apiAccessTokenResponse(items[i])
		view["mcp_client_id"] = clientByToken[items[i].ID]
		result = append(result, view)
	}
	resp.Success(c, http.StatusOK, "成功", result)
}

func CreateAPIAccessToken(c *gin.Context) {
	var req createAPIAccessTokenReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "API Token 格式不正确")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if req.Name == "" || len([]rune(req.Name)) > 100 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请输入不超过 100 个字符的 Token 名称")
		return
	}
	if req.UserID == 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请选择绑定用户")
		return
	}
	var expiresAt time.Time
	validExpiry := req.ExpiresInDays != nil
	if validExpiry {
		expiresAt, validExpiry = apiTokenExpiresAt(*req.ExpiresInDays, time.Now())
	}
	if !validExpiry {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "有效期仅支持 7 天、30 天、60 天、90 天或永久")
		return
	}
	if len([]rune(req.Description)) > 500 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "描述不能超过 500 个字符")
		return
	}
	var user models.User
	if err := database.DB.Where("id = ? AND status = 1", req.UserID).First(&user).Error; err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "绑定用户不存在或已禁用")
		return
	}
	id, token, prefix, err := authutil.NewAPIToken()
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "生成 API Token 失败")
		return
	}
	item := models.APIAccessToken{
		ID: id, Name: req.Name, TokenHash: authutil.HashAPIToken(token), TokenPrefix: prefix, UserID: user.ID, User: user,
		Description: req.Description, Status: 1, ExpiresAt: expiresAt, CreatedBy: c.GetUint("user_id"),
	}
	if err := database.DB.Create(&item).Error; err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "Token 名称已存在")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	resp.Success(c, http.StatusOK, "API Token 已创建", gin.H{"token": token, "item": apiAccessTokenResponse(item)})
}

func UpdateAPIAccessToken(c *gin.Context) {
	var req updateAPIAccessTokenReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "访问令牌格式不正确")
		return
	}
	req.Description = strings.TrimSpace(req.Description)
	if len([]rune(req.Description)) > 500 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "备注不能超过 500 个字符")
		return
	}
	result := database.DB.Model(&models.APIAccessToken{}).Where("id = ?", c.Param("id")).Update("description", req.Description)
	if result.Error != nil || result.RowsAffected == 0 {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "访问令牌不存在")
		return
	}
	resp.Success(c, http.StatusOK, "访问令牌备注已更新")
}

func RegenerateAPIAccessToken(c *gin.Context) {
	var req regenerateAPIAccessTokenReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "访问令牌格式不正确")
		return
	}
	var expiresAt time.Time
	validExpiry := req.ExpiresInDays != nil
	if validExpiry {
		expiresAt, validExpiry = apiTokenExpiresAt(*req.ExpiresInDays, time.Now())
	}
	if !validExpiry {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "有效期仅支持 7 天、30 天、60 天、90 天或永久")
		return
	}
	var item models.APIAccessToken
	if err := database.DB.Preload("User").First(&item, "id = ?", c.Param("id")).Error; err != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "访问令牌不存在")
		return
	}
	_, token, prefix, err := authutil.NewAPIToken()
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "重新生成访问令牌失败")
		return
	}
	if err := database.DB.Model(&item).Updates(map[string]interface{}{
		"token_hash": authutil.HashAPIToken(token), "token_prefix": prefix, "expires_at": expiresAt,
		"last_used_at": nil, "last_used_ip": "",
	}).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "重新生成访问令牌失败")
		return
	}
	item.TokenHash = authutil.HashAPIToken(token)
	item.TokenPrefix = prefix
	item.ExpiresAt = expiresAt
	item.LastUsedAt = nil
	item.LastUsedIP = ""
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	resp.Success(c, http.StatusOK, "访问令牌已重新生成", gin.H{"token": token, "item": apiAccessTokenResponse(item)})
}

func UpdateAPIAccessTokenStatus(c *gin.Context) {
	var req updateAPIAccessTokenStatusReq
	if err := c.ShouldBindJSON(&req); err != nil || (req.Status != 0 && req.Status != 1) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "启用状态不正确")
		return
	}
	result := database.DB.Model(&models.APIAccessToken{}).Where("id = ?", c.Param("id")).Update("status", req.Status)
	if result.Error != nil || result.RowsAffected == 0 {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "API Token 不存在")
		return
	}
	resp.Success(c, http.StatusOK, "API Token 状态已更新")
}

func DeleteAPIAccessToken(c *gin.Context) {
	var clientCount int64
	if err := database.DB.Model(&models.MCPClient{}).Where("api_token_id = ?", c.Param("id")).Count(&clientCount).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "检查访问令牌关联失败")
		return
	}
	if clientCount > 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "访问令牌已用于 MCP 连接，请先删除连接或更换访问令牌")
		return
	}
	result := database.DB.Where("id = ?", c.Param("id")).Delete(&models.APIAccessToken{})
	if result.Error != nil || result.RowsAffected == 0 {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "API Token 不存在")
		return
	}
	resp.Success(c, http.StatusOK, "API Token 已删除")
}

func apiAccessTokenResponse(item models.APIAccessToken) gin.H {
	return gin.H{
		"id": item.ID, "name": item.Name, "token_prefix": item.TokenPrefix, "user_id": item.UserID,
		"username": item.User.Username, "nickname": item.User.Nickname, "description": item.Description,
		"status": item.Status, "expires_at": item.ExpiresAt, "permanent": item.ExpiresAt.Year() >= permanentAPITokenYear, "last_used_at": item.LastUsedAt,
		"last_used_ip": item.LastUsedIP, "created_by": item.CreatedBy, "created_at": item.CreatedAt,
	}
}

func apiTokenExpiresAt(days int, now time.Time) (time.Time, bool) {
	if _, ok := allowedAPITokenExpiryDays[days]; !ok {
		return time.Time{}, false
	}
	if days == 0 {
		return time.Date(permanentAPITokenYear, 12, 31, 23, 59, 59, 0, time.UTC), true
	}
	return now.AddDate(0, 0, days), true
}
