package handlers

import (
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	sls "github.com/alibabacloud-go/sls-20201230/v6/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const cloudMonitorTicketTTL = int64(3600)

var cloudMonitorRegions = map[string]string{
	"cn-hangzhou":    "华东 1（杭州）",
	"cn-shanghai":    "华东 2（上海）",
	"cn-beijing":     "华北 2（北京）",
	"cn-shenzhen":    "华南 1（深圳）",
	"ap-southeast-1": "新加坡",
	"eu-central-1":   "德国（法兰克福）",
	"us-west-1":      "美国（硅谷）",
}

type cloudMonitorWorkspaceReq struct {
	ID              uint   `json:"id"`
	Name            string `json:"name"`
	Workspace       string `json:"workspace"`
	Region          string `json:"region"`
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	Description     string `json:"description"`
}

type cloudMonitorWorkspaceView struct {
	ID                    uint      `json:"id"`
	Name                  string    `json:"name"`
	Workspace             string    `json:"workspace"`
	Region                string    `json:"region"`
	RegionName            string    `json:"region_name"`
	Description           string    `json:"description"`
	CredentialsConfigured bool      `json:"credentials_configured"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

func ListCloudMonitorWorkspaces(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	query := database.DB.Model(&models.CloudMonitorWorkspace{})
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("name LIKE ?", like)
	}
	var items []models.CloudMonitorWorkspace
	if err := query.Order("id DESC").Find(&items).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "读取云监控 Workspace 失败")
		return
	}
	data := make([]cloudMonitorWorkspaceView, 0, len(items))
	for _, item := range items {
		data = append(data, cloudMonitorWorkspaceView{
			ID: item.ID, Name: item.Name, Workspace: item.Workspace, Region: item.Region, RegionName: cloudMonitorRegions[item.Region],
			Description: item.Description, CredentialsConfigured: item.AccessKeyIDCipher != "" && item.AccessKeySecretCipher != "",
			CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		})
	}
	resp.Success(c, http.StatusOK, "成功", data)
}

func CreateCloudMonitorWorkspace(c *gin.Context) {
	var req cloudMonitorWorkspaceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	if message := validateCloudMonitorWorkspaceReq(req, true); message != "" {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, message)
		return
	}
	accessKeyIDCipher, err := encryptSecret(strings.TrimSpace(req.AccessKeyID))
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "AccessKey ID 加密失败")
		return
	}
	accessKeySecretCipher, err := encryptSecret(strings.TrimSpace(req.AccessKeySecret))
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "AccessKey Secret 加密失败")
		return
	}
	item := models.CloudMonitorWorkspace{
		Name: strings.TrimSpace(req.Name), Workspace: strings.TrimSpace(req.Workspace), Region: strings.TrimSpace(req.Region),
		Endpoint: slsRegionEndpoint(req.Region), AccessKeyIDCipher: accessKeyIDCipher,
		AccessKeySecretCipher: accessKeySecretCipher, Description: strings.TrimSpace(req.Description),
	}
	if err := createDashboardAtFront(&item); err != nil {
		if isCloudMonitorDuplicate(err) {
			resp.Error(c, http.StatusConflict, resp.CodeConflict, "该地域下的 Workspace 已存在")
			return
		}
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "创建云监控 Workspace 失败")
		return
	}
	resp.Success(c, http.StatusOK, "创建成功", gin.H{"id": item.ID})
}

func UpdateCloudMonitorWorkspace(c *gin.Context) {
	item, ok := loadCloudMonitorWorkspace(c, "id")
	if !ok {
		return
	}
	var req cloudMonitorWorkspaceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	if message := validateCloudMonitorWorkspaceReq(req, false); message != "" {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, message)
		return
	}
	updates := map[string]interface{}{
		"name": strings.TrimSpace(req.Name), "workspace": strings.TrimSpace(req.Workspace), "region": strings.TrimSpace(req.Region),
		"endpoint": slsRegionEndpoint(req.Region), "description": strings.TrimSpace(req.Description),
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
	if err := database.DB.Model(&item).Updates(updates).Error; err != nil {
		if isCloudMonitorDuplicate(err) {
			resp.Error(c, http.StatusConflict, resp.CodeConflict, "该地域下的 Workspace 已存在")
			return
		}
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "更新云监控 Workspace 失败")
		return
	}
	resp.Success(c, http.StatusOK, "更新成功")
}

func DeleteCloudMonitorWorkspace(c *gin.Context) {
	item, ok := loadCloudMonitorWorkspace(c, "id")
	if !ok {
		return
	}
	if err := database.DB.Delete(&item).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "删除云监控 Workspace 失败")
		return
	}
	resp.Success(c, http.StatusOK, "删除成功")
}

func TestCloudMonitorWorkspace(c *gin.Context) {
	var req cloudMonitorWorkspaceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	if message := validateCloudMonitorWorkspaceReq(req, req.ID == 0); message != "" {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, message)
		return
	}
	auth, err := resolveCloudMonitorCredentials(req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if _, err := createCloudMonitorTicket(auth); err != nil {
		resp.Error(c, http.StatusBadGateway, resp.CodeBadGateway, "连接失败: "+err.Error())
		return
	}
	resp.Success(c, http.StatusOK, "连接成功")
}

func CreateCloudMonitorSession(c *gin.Context) {
	item, ok := loadCloudMonitorWorkspace(c, "id")
	if !ok {
		return
	}
	auth, err := decryptCloudMonitorCredentials(item)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "云监控凭证读取失败")
		return
	}
	ticket, err := createCloudMonitorTicket(auth)
	if err != nil {
		resp.Error(c, http.StatusBadGateway, resp.CodeBadGateway, "创建云监控免登录会话失败: "+err.Error())
		return
	}
	c.Header("Cache-Control", "no-store")
	resp.Success(c, http.StatusOK, "成功", gin.H{
		"url": buildCloudMonitorConsoleURL(item.Region, item.Workspace, ticket), "expires_in": cloudMonitorTicketTTL,
	})
}

func createCloudMonitorTicket(auth slsAuth) (string, error) {
	client, err := newSLSSDKClient(auth, slsTicketEndpoint)
	if err != nil {
		return "", err
	}
	request := (&sls.CreateTicketRequest{}).SetExpirationTime(cloudMonitorTicketTTL).SetAccessTokenExpirationTime(cloudMonitorTicketTTL)
	response, err := client.CreateTicketWithOptions(request, map[string]*string{}, newSLSRuntimeOptions())
	if err != nil {
		return "", err
	}
	if response == nil || response.Body == nil || strings.TrimSpace(tea.StringValue(response.Body.Ticket)) == "" {
		return "", fmt.Errorf("阿里云未返回有效票据")
	}
	return strings.TrimSpace(tea.StringValue(response.Body.Ticket)), nil
}

func buildCloudMonitorConsoleURL(region, workspace, ticket string) string {
	query := url.Values{}
	query.Set("sls_ticket", strings.TrimSpace(ticket))
	return "https://cmsnext.console.aliyun.com/next/region/" + url.PathEscape(strings.TrimSpace(region)) +
		"/workspace/" + url.PathEscape(strings.TrimSpace(workspace)) + "/app/apm/application-list?" + query.Encode()
}

func validateCloudMonitorWorkspaceReq(req cloudMonitorWorkspaceReq, requireCredentials bool) string {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return "请填写名称"
	}
	if len(name) > 128 {
		return "名称不能超过 128 个字符"
	}
	workspace := strings.TrimSpace(req.Workspace)
	if workspace == "" {
		return "请填写 Workspace"
	}
	if len(workspace) > 128 || strings.ContainsAny(workspace, "/?#") {
		return "Workspace 格式不正确"
	}
	if _, ok := cloudMonitorRegions[strings.TrimSpace(req.Region)]; !ok {
		return "请选择支持的地域"
	}
	if len(strings.TrimSpace(req.Description)) > 500 {
		return "描述不能超过 500 个字符"
	}
	if requireCredentials && (strings.TrimSpace(req.AccessKeyID) == "" || strings.TrimSpace(req.AccessKeySecret) == "") {
		return "请填写 AccessKey ID 和 AccessKey Secret"
	}
	return ""
}

func resolveCloudMonitorCredentials(req cloudMonitorWorkspaceReq) (slsAuth, error) {
	auth := slsAuth{AccessKeyID: strings.TrimSpace(req.AccessKeyID), AccessKeySecret: strings.TrimSpace(req.AccessKeySecret)}
	if req.ID > 0 && (auth.AccessKeyID == "" || auth.AccessKeySecret == "") {
		var item models.CloudMonitorWorkspace
		if err := database.DB.First(&item, req.ID).Error; err != nil {
			return slsAuth{}, fmt.Errorf("云监控 Workspace 不存在")
		}
		stored, err := decryptCloudMonitorCredentials(item)
		if err != nil {
			return slsAuth{}, fmt.Errorf("云监控凭证读取失败")
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

func decryptCloudMonitorCredentials(item models.CloudMonitorWorkspace) (slsAuth, error) {
	accessKeyID, err := decryptSecret(item.AccessKeyIDCipher)
	if err != nil {
		return slsAuth{}, err
	}
	accessKeySecret, err := decryptSecret(item.AccessKeySecretCipher)
	if err != nil {
		return slsAuth{}, err
	}
	return slsAuth{AccessKeyID: accessKeyID, AccessKeySecret: accessKeySecret}, nil
}

func loadCloudMonitorWorkspace(c *gin.Context, param string) (models.CloudMonitorWorkspace, bool) {
	id, err := strconv.ParseUint(c.Param(param), 10, 64)
	if err != nil || id == 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "云监控 Workspace ID 不正确")
		return models.CloudMonitorWorkspace{}, false
	}
	var item models.CloudMonitorWorkspace
	if err := database.DB.First(&item, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "云监控 Workspace 不存在")
		} else {
			resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "读取云监控 Workspace 失败")
		}
		return models.CloudMonitorWorkspace{}, false
	}
	return item, true
}

func isCloudMonitorDuplicate(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate")
}
