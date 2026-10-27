package handlers

import (
	"errors"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

type saveGitCredentialReq struct {
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	BaseURL     string `json:"base_url"`
	AuthType    string `json:"auth_type"`
	Username    string `json:"username"`
	Secret      string `json:"secret"`
	Description string `json:"description"`
	Status      int    `json:"status"`
}

func ListGitCredentials(c *gin.Context) {
	var rows []models.GitCredential
	database.DB.Order("id DESC").Find(&rows)
	result := make([]gin.H, 0, len(rows))
	for i := range rows {
		result = append(result, gitCredentialResponse(rows[i]))
	}
	resp.Success(c, http.StatusOK, "成功", result)
}

func CreateGitCredential(c *gin.Context) {
	var req saveGitCredentialReq
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "Git 认证格式不正确")
		return
	}
	item, err := buildGitCredential(req, nil)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if err := database.DB.Create(&item).Error; err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "Git 认证名称已存在")
		return
	}
	resp.Success(c, http.StatusOK, "Git 认证已创建", gitCredentialResponse(item))
}

func UpdateGitCredential(c *gin.Context) {
	var existing models.GitCredential
	if database.DB.First(&existing, c.Param("id")).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "Git 认证不存在")
		return
	}
	var req saveGitCredentialReq
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "Git 认证格式不正确")
		return
	}
	item, err := buildGitCredential(req, &existing)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if err := database.DB.Model(&existing).Updates(map[string]interface{}{
		"name": item.Name, "provider": item.Provider, "base_url": item.BaseURL, "auth_type": item.AuthType,
		"username": item.Username, "secret_cipher": item.SecretCipher, "description": item.Description, "status": item.Status,
	}).Error; err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "Git 认证名称已存在")
		return
	}
	database.DB.First(&existing, existing.ID)
	resp.Success(c, http.StatusOK, "Git 认证已更新", gitCredentialResponse(existing))
}

func DeleteGitCredential(c *gin.Context) {
	var item models.GitCredential
	if database.DB.First(&item, c.Param("id")).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "Git 认证不存在")
		return
	}
	var count int64
	database.DB.Model(&models.GitRepository{}).Where("credential_id = ?", item.ID).Count(&count)
	if count > 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "Git 认证已被仓库使用，请先更换仓库认证")
		return
	}
	database.DB.Delete(&item)
	resp.Success(c, http.StatusOK, "Git 认证已删除")
}

func buildGitCredential(req saveGitCredentialReq, existing *models.GitCredential) (models.GitCredential, error) {
	item := models.GitCredential{
		Name: strings.TrimSpace(req.Name), Provider: strings.ToLower(strings.TrimSpace(req.Provider)),
		BaseURL: strings.TrimRight(strings.TrimSpace(req.BaseURL), "/"), AuthType: strings.ToLower(strings.TrimSpace(req.AuthType)),
		Username: strings.TrimSpace(req.Username), Description: strings.TrimSpace(req.Description), Status: req.Status,
	}
	if item.Name == "" || len([]rune(item.Name)) > 128 {
		return item, errors.New("请输入不超过 128 个字符的认证名称")
	}
	if item.Provider == "" {
		item.Provider = "other"
	}
	if item.Provider != "github" && item.Provider != "gitlab" && item.Provider != "gitee" && item.Provider != "other" {
		return item, errors.New("Git 服务类型不正确")
	}
	parsed, err := url.Parse(item.BaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return item, errors.New("Git 服务地址必须是有效的 HTTP 或 HTTPS 地址")
	}
	if item.AuthType == "" {
		item.AuthType = "none"
	}
	if item.AuthType != "none" && item.AuthType != "token" && item.AuthType != "ssh_key" {
		return item, errors.New("Git 认证方式不正确")
	}
	if item.Status != 0 && item.Status != 1 {
		return item, errors.New("启用状态不正确")
	}
	if item.AuthType == "token" && item.Username == "" {
		item.Username = "git"
	}
	if existing != nil {
		item.SecretCipher = existing.SecretCipher
		if item.AuthType != "none" && item.AuthType != existing.AuthType && strings.TrimSpace(req.Secret) == "" {
			return item, errors.New("切换认证方式时请重新填写访问凭据")
		}
	}
	if item.AuthType == "none" {
		item.Username = ""
		item.SecretCipher = ""
	} else if strings.TrimSpace(req.Secret) != "" {
		ciphertext, err := encryptSecret(req.Secret)
		if err != nil {
			return item, errors.New("Git 凭据加密失败")
		}
		item.SecretCipher = ciphertext
	}
	if item.AuthType != "none" && item.SecretCipher == "" {
		return item, errors.New("请填写 Git 访问凭据")
	}
	return item, nil
}

func gitCredentialResponse(item models.GitCredential) gin.H {
	var repositoryCount int64
	database.DB.Model(&models.GitRepository{}).Where("credential_id = ?", item.ID).Count(&repositoryCount)
	return gin.H{
		"id": item.ID, "name": item.Name, "provider": item.Provider, "base_url": item.BaseURL,
		"auth_type": item.AuthType, "username": item.Username, "secret_configured": item.SecretCipher != "",
		"description": item.Description, "status": item.Status, "repository_count": repositoryCount,
		"created_at": item.CreatedAt, "updated_at": item.UpdatedAt,
	}
}
