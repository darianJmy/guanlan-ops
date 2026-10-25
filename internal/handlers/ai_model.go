package handlers

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"guanlan/internal/ai/provider"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"
	"strconv"
	"strings"
)

type aiModelRequest struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
	APIKey   string `json:"api_key"`
	Status   int    `json:"status"`
}

func aiModelPermission(c *gin.Context, edit bool) bool {
	permission := "system-setting:list"
	if edit {
		permission = "system-setting:edit"
	}
	if authorizeAIPermission(c.Request.Context(), currentUserID(c), permission) != nil {
		resp.Error(c, http.StatusForbidden, resp.CodeForbidden, "没有管理 AI 模型的权限")
		return false
	}
	return true
}
func ListAIModels(c *gin.Context) {
	if !aiModelPermission(c, false) {
		return
	}
	var rows []models.AIModel
	if database.DB.WithContext(c.Request.Context()).Order("id ASC").Find(&rows).Error != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "读取模型失败")
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		items = append(items, aiModelResponse(row))
	}
	resp.SuccessData(c, http.StatusOK, items)
}
func ListChatModels(c *gin.Context) {
	var rows []models.AIModel
	if database.DB.WithContext(c.Request.Context()).Where("status = 1").Order("id ASC").Find(&rows).Error != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "读取可用模型失败")
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		items = append(items, gin.H{"id": row.ID, "name": row.Name, "provider": row.Provider, "model": row.Model})
	}
	resp.SuccessData(c, http.StatusOK, items)
}
func aiModelResponse(row models.AIModel) gin.H {
	return gin.H{"id": row.ID, "name": row.Name, "provider": row.Provider, "base_url": row.BaseURL, "model": row.Model, "status": row.Status, "key_configured": row.SecretCipher != "", "updated_at": row.UpdatedAt}
}
func SaveAIModel(c *gin.Context) {
	if !aiModelPermission(c, true) {
		return
	}
	var row models.AIModel
	if c.Param("id") != "" {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil || id == 0 || database.DB.First(&row, id).Error != nil {
			resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "模型不存在")
			return
		}
	}
	var req aiModelRequest
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "模型配置格式不正确")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Model = strings.TrimSpace(req.Model)
	req.Provider = strings.ToLower(strings.TrimSpace(req.Provider))
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	if req.Name == "" || len([]rune(req.Name)) > 128 || req.Model == "" || len(req.Model) > 200 || len(req.BaseURL) > 1000 || (req.Status != 0 && req.Status != 1) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请填写有效的名称、模型标识和状态")
		return
	}
	if req.Provider != "newapi" && req.Provider != "openai" && req.Provider != "anthropic" {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请选择模型 Provider")
		return
	}
	if _, err := provider.BaseURL(req.BaseURL, req.Provider); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "模型地址必须为有效的 HTTP(S) 地址")
		return
	}
	if row.ID != 0 && (row.BaseURL != req.BaseURL || row.Provider != req.Provider) && strings.TrimSpace(req.APIKey) == "" {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "更换地址或 Provider 时请重新填写 API Key")
		return
	}
	if strings.TrimSpace(req.APIKey) != "" {
		if len(req.APIKey) > 8192 {
			resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "API Key 过长")
			return
		}
		encrypted, err := encryptSecret(strings.TrimSpace(req.APIKey))
		if err != nil {
			resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "密钥保存失败")
			return
		}
		row.SecretCipher = encrypted
	}
	if row.SecretCipher == "" {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请填写 API Key")
		return
	}
	row.Name = req.Name
	row.Provider = req.Provider
	row.BaseURL = req.BaseURL
	row.Model = req.Model
	row.Status = req.Status
	if err := database.DB.WithContext(c.Request.Context()).Save(&row).Error; err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "保存失败，请检查模型名称是否重复")
		return
	}
	resp.Success(c, http.StatusOK, "模型已保存", aiModelResponse(row))
}
func DeleteAIModel(c *gin.Context) {
	if !aiModelPermission(c, true) {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "模型 ID 无效")
		return
	}
	result := database.DB.WithContext(c.Request.Context()).Delete(&models.AIModel{}, id)
	if result.Error != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "删除失败")
		return
	}
	if result.RowsAffected == 0 {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "模型不存在")
		return
	}
	resp.Success(c, http.StatusOK, "模型已删除，原会话需重新选择模型")
}
func resolveAIModel(ctx context.Context, id uint) (models.AIModel, error) {
	var row models.AIModel
	query := database.DB.WithContext(ctx).Where("status = 1")
	if id != 0 {
		query = query.Where("id = ?", id)
	}
	if query.Order("id ASC").First(&row).Error != nil {
		return row, errors.New("所选模型不可用，请在系统设置添加或启用模型后重新选择")
	}
	return row, nil
}
func aiModelConfig(row models.AIModel) (provider.Config, error) {
	key, err := decryptSecret(row.SecretCipher)
	if err != nil {
		return provider.Config{}, errors.New("模型密钥无法读取，请重新保存 API Key")
	}
	return provider.Config{Provider: row.Provider, BaseURL: row.BaseURL, APIKey: key, Model: row.Model}, nil
}
