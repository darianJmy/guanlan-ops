package handlers

import (
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type slsDashboardProjectView struct {
	CreatedAt             time.Time `json:"created_at"`
	ID                    uint      `json:"id"`
	Provider              string    `json:"provider"`
	Name                  string    `json:"name"`
	Project               string    `json:"project"`
	Region                string    `json:"region"`
	RoleArn               string    `json:"role_arn"`
	Description           string    `json:"description"`
	Sort                  int       `json:"sort"`
	CredentialsConfigured bool      `json:"credentials_configured"`
}

type slsDashboardProjectReq struct {
	ID              uint   `json:"id"`
	Provider        string `json:"provider"`
	Name            string `json:"name"`
	Project         string `json:"project"`
	Region          string `json:"region"`
	RoleArn         string `json:"role_arn"`
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	Description     string `json:"description"`
}

func CreateSLSDashboardProject(c *gin.Context) {
	var req slsDashboardProjectReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	if message := validateSLSDashboardProjectReq(req, true); message != "" {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, message)
		return
	}
	accessKeyIDCipher, err := encryptSecret(req.AccessKeyID)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "AccessKey 加密失败")
		return
	}
	accessKeySecretCipher, err := encryptSecret(req.AccessKeySecret)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "AccessKey 加密失败")
		return
	}
	project := models.SLSDashboardProject{
		Provider: normalizeLogDashboardProvider(req.Provider), Name: strings.TrimSpace(req.Name), Project: strings.TrimSpace(req.Project), Region: strings.TrimSpace(req.Region),
		RoleArn:           strings.TrimSpace(req.RoleArn),
		AccessKeyIDCipher: accessKeyIDCipher, AccessKeySecretCipher: accessKeySecretCipher, Description: strings.TrimSpace(req.Description),
	}
	if err := createDashboardAtFront(&project); err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "创建 SLS 项目失败")
		return
	}
	resp.Success(c, http.StatusOK, "创建成功", gin.H{"id": project.ID})
}

type reorderSLSDashboardProjectsReq struct {
	IDs []uint `json:"ids"`
}

func ReorderSLSDashboardProjects(c *gin.Context) {
	var req reorderSLSDashboardProjectsReq
	if err := c.ShouldBindJSON(&req); err != nil || !validSLSDashboardProjectOrder(req.IDs) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "项目排序参数不正确")
		return
	}

	var count int64
	if err := database.DB.Model(&models.SLSDashboardProject{}).Where("id IN ?", req.IDs).Count(&count).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "校验 SLS 项目排序失败")
		return
	}
	if count != int64(len(req.IDs)) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "排序中包含不存在的 SLS 项目")
		return
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		for index, id := range req.IDs {
			if err := tx.Model(&models.SLSDashboardProject{}).Where("id = ?", id).Update("sort", index).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "保存 SLS 项目排序失败")
		return
	}
	resp.Success(c, http.StatusOK, "排序已保存")
}

func validSLSDashboardProjectOrder(ids []uint) bool {
	if len(ids) == 0 {
		return false
	}
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			return false
		}
		if _, exists := seen[id]; exists {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}

func UpdateSLSDashboardProject(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "SLS 项目 ID 不正确")
		return
	}
	var project models.SLSDashboardProject
	if err := database.DB.First(&project, id).Error; err != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "SLS 项目不存在")
		return
	}
	var req slsDashboardProjectReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	if message := validateSLSDashboardProjectReq(req, false); message != "" {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, message)
		return
	}
	updates := map[string]interface{}{
		"provider": normalizeLogDashboardProvider(req.Provider), "name": strings.TrimSpace(req.Name), "project": strings.TrimSpace(req.Project),
		"region": strings.TrimSpace(req.Region), "role_arn": strings.TrimSpace(req.RoleArn), "description": strings.TrimSpace(req.Description),
	}
	if value := strings.TrimSpace(req.AccessKeyID); value != "" {
		ciphertext, err := encryptSecret(value)
		if err != nil {
			resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "AccessKey ID 加密失败")
			return
		}
		updates["access_key_id_cipher"] = ciphertext
	}
	if value := strings.TrimSpace(req.AccessKeySecret); value != "" {
		ciphertext, err := encryptSecret(value)
		if err != nil {
			resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "AccessKey Secret 加密失败")
			return
		}
		updates["access_key_secret_cipher"] = ciphertext
	}
	if err := database.DB.Model(&project).Updates(updates).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "更新 SLS 项目失败")
		return
	}
	resp.Success(c, http.StatusOK, "更新成功")
}

func DeleteSLSDashboardProject(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "SLS 项目 ID 不正确")
		return
	}
	result := database.DB.Delete(&models.SLSDashboardProject{}, id)
	if result.Error != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "删除 SLS 项目失败")
		return
	}
	if result.RowsAffected == 0 {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "SLS 项目不存在")
		return
	}
	resp.Success(c, http.StatusOK, "删除成功")
}

func TestSLSDashboardProject(c *gin.Context) {
	var req slsDashboardProjectReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	if message := validateSLSDashboardProjectReq(req, req.ID == 0); message != "" {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, message)
		return
	}
	auth, err := resolveSLSDashboardCredentials(req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if normalizeLogDashboardProvider(req.Provider) == "tencent_cls" {
		_, err = fetchTencentCLSTopics(auth, req.Region)
		if err == nil {
			_, err = assumeTencentCLSRole(auth, req.Region, req.RoleArn)
		}
	} else {
		slsClient, clientErr := newSLSSDKClient(auth, slsRegionEndpoint(req.Region))
		err = clientErr
		if err == nil {
			_, err = fetchSLSLogstores(slsClient, req.Project)
		}
	}
	if err != nil {
		resp.Error(c, http.StatusBadGateway, resp.CodeBadGateway, "连接失败: "+err.Error())
		return
	}
	resp.Success(c, http.StatusOK, "连接成功")
}

func validateSLSDashboardProjectReq(req slsDashboardProjectReq, requireCredentials bool) string {
	provider := normalizeLogDashboardProvider(req.Provider)
	if provider == "" {
		return "日志服务类型不正确"
	}
	if strings.TrimSpace(req.Name) == "" {
		return "请填写配置名称"
	}
	if provider == "aliyun_sls" && strings.TrimSpace(req.Project) == "" {
		return "请填写 Project"
	}
	if strings.TrimSpace(req.Region) == "" {
		return "请填写 Region"
	}
	if provider == "tencent_cls" && strings.TrimSpace(req.RoleArn) == "" {
		return "请填写 RoleArn"
	}
	if requireCredentials && (strings.TrimSpace(req.AccessKeyID) == "" || strings.TrimSpace(req.AccessKeySecret) == "") {
		if provider == "tencent_cls" {
			return "请填写 SecretId 和 SecretKey"
		}
		return "请填写 AccessKey ID 和 AccessKey Secret"
	}
	return ""
}

func normalizeLogDashboardProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "aliyun", "aliyun_sls", "sls":
		return "aliyun_sls"
	case "tencent", "tencent_cls", "cls":
		return "tencent_cls"
	default:
		return ""
	}
}

func resolveSLSDashboardCredentials(req slsDashboardProjectReq) (slsAuth, error) {
	auth := slsAuth{AccessKeyID: strings.TrimSpace(req.AccessKeyID), AccessKeySecret: strings.TrimSpace(req.AccessKeySecret)}
	if req.ID > 0 && (auth.AccessKeyID == "" || auth.AccessKeySecret == "") {
		var project models.SLSDashboardProject
		if err := database.DB.First(&project, req.ID).Error; err != nil {
			return slsAuth{}, fmt.Errorf("SLS 项目不存在")
		}
		stored, err := decryptSLSDashboardCredentials(project)
		if err != nil {
			return slsAuth{}, err
		}
		if auth.AccessKeyID == "" {
			auth.AccessKeyID = stored.AccessKeyID
		}
		if auth.AccessKeySecret == "" {
			auth.AccessKeySecret = stored.AccessKeySecret
		}
	}
	if auth.AccessKeyID == "" || auth.AccessKeySecret == "" {
		return slsAuth{}, fmt.Errorf("请填写 AccessKey ID 和 AccessKey Secret")
	}
	return auth, nil
}

func decryptSLSDashboardCredentials(project models.SLSDashboardProject) (slsAuth, error) {
	accessKeyID, err := decryptSecret(project.AccessKeyIDCipher)
	if err != nil {
		return slsAuth{}, err
	}
	accessKeySecret, err := decryptSecret(project.AccessKeySecretCipher)
	if err != nil {
		return slsAuth{}, err
	}
	return slsAuth{AccessKeyID: accessKeyID, AccessKeySecret: accessKeySecret}, nil
}
