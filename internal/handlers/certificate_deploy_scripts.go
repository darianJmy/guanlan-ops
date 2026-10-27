package handlers

import (
	"errors"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type saveDeployScriptReq struct {
	Name        string `json:"name" binding:"required"`
	Type        string `json:"type"`
	TargetType  string `json:"target_type"`
	Content     string `json:"content" binding:"required"`
	Description string `json:"description"`
	Status      int    `json:"status"`
}

func ListCertificateDeployScripts(c *gin.Context) {
	var items []models.CertificateDeployScript
	database.DB.Order("id DESC").Find(&items)
	resp.Success(c, http.StatusOK, "成功", items)
}

func CreateCertificateDeployScript(c *gin.Context) {
	var req saveDeployScriptReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请填写脚本名称和脚本内容")
		return
	}
	item, err := buildDeployScript(req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if err := database.DB.Create(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "Duplicate") {
			resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "脚本名称已存在")
			return
		}
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "创建发布脚本失败")
		return
	}
	resp.Success(c, http.StatusOK, "创建成功", item)
}

func UpdateCertificateDeployScript(c *gin.Context) {
	var existing models.CertificateDeployScript
	if err := database.DB.First(&existing, c.Param("id")).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "发布脚本不存在")
		return
	}
	var req saveDeployScriptReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请填写脚本名称和脚本内容")
		return
	}
	item, err := buildDeployScript(req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if err := database.DB.Model(&existing).Updates(map[string]interface{}{
		"name":        item.Name,
		"type":        item.Type,
		"target_type": item.TargetType,
		"content":     item.Content,
		"description": item.Description,
		"status":      item.Status,
	}).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "更新发布脚本失败")
		return
	}
	resp.Success(c, http.StatusOK, "更新成功")
}

func DeleteCertificateDeployScript(c *gin.Context) {
	id := c.Param("id")
	if names := boundCertificateDomainNames("deploy_script_id", id); len(names) > 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, boundResourceMessage("发布脚本", names))
		return
	}
	if err := database.DB.Delete(&models.CertificateDeployScript{}, id).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "删除发布脚本失败")
		return
	}
	resp.Success(c, http.StatusOK, "删除成功")
}

func buildDeployScript(req saveDeployScriptReq) (models.CertificateDeployScript, error) {
	scriptType := certificateDefaultString(req.Type, "shell")
	if scriptType != "shell" && scriptType != "python" {
		return models.CertificateDeployScript{}, errors.New("脚本类型只支持 Shell 或 Python")
	}
	status := req.Status
	if status == 0 {
		status = 1
	}
	item := models.CertificateDeployScript{
		Name:        strings.TrimSpace(req.Name),
		Type:        scriptType,
		TargetType:  scriptType,
		Content:     strings.TrimSpace(req.Content),
		Description: strings.TrimSpace(req.Description),
		Status:      status,
	}
	if item.Name == "" {
		return models.CertificateDeployScript{}, errors.New("请填写脚本名称")
	}
	if item.Content == "" {
		return models.CertificateDeployScript{}, errors.New("请填写脚本内容")
	}
	return item, nil
}
