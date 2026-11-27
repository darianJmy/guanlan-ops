package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/internal/serviceportal/targetconfig"
	"io"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// servicePortalState is the private wire format consumed by the port-forward
// runtime. Kubernetes connections are the shared Datasource records managed
// by 接入中心; this endpoint only materializes their kubeconfig server-side.
type servicePortalState struct {
	Clusters  []servicePortalClusterState `json:"clusters"`
	Resources []servicePortalResourceView `json:"resources"`
	Grants    map[string][]string         `json:"grants"`
}

type servicePortalClusterState struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Context    string `json:"context"`
	Kubeconfig string `json:"kubeconfig"`
}

type servicePortalResourceView struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	TargetURL    string `json:"targetURL"`
	ClusterID    string `json:"clusterId"`
	DatasourceID uint   `json:"datasource_id"`
	Namespace    string `json:"namespace"`
	Service      string `json:"service"`
	Port         string `json:"port"`
	PublicPort   int    `json:"publicPort"`
	Path         string `json:"path"`
	Desired      bool   `json:"desired"`
	OnDemand     bool   `json:"onDemand"`
	Status       string `json:"status,omitempty"`
	LastError    string `json:"lastError,omitempty"`
	Recovered    string `json:"recovered,omitempty"`
	FolderID     uint   `json:"folder_id"`
}

type servicePortalResourceRequest struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	TargetURL    string `json:"targetURL"`
	ClusterID    string `json:"clusterId"`
	DatasourceID uint   `json:"datasource_id"`
	Namespace    string `json:"namespace"`
	Service      string `json:"service"`
	Port         string `json:"port"`
	PublicPort   int    `json:"publicPort"`
	Path         string `json:"path"`
	OnDemand     *bool  `json:"onDemand"`
	FolderID     uint   `json:"folder_id"`
}

var servicePortalConfigID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var servicePortalNamespace = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
var servicePortalService = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`)
var servicePortalPortName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

func portalInternal(c *gin.Context) bool {
	token := strings.TrimSpace(os.Getenv("SERVICE_PORTAL_INTERNAL_TOKEN"))
	if token == "" || subtle.ConstantTimeCompare([]byte(c.GetHeader("X-Service-Portal-Internal")), []byte(token)) != 1 {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "接口不存在"})
		return false
	}
	return true
}

func servicePortalNewID() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func servicePortalAdmin(c *gin.Context) (serviceAccessUser, bool) {
	user, ok := currentServiceAccess(c)
	if !ok {
		return user, false
	}
	if !user.Admin {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "没有服务配置管理权限"})
		return user, false
	}
	return user, true
}

func portalCluster(ds models.Datasource) (servicePortalClusterState, error) {
	var cluster servicePortalClusterState
	if ds.TunnelID != 0 {
		return cluster, errors.New("服务代理进程需要能够直连 Kubernetes API Server")
	}
	if err := validateKubernetesConnection(ds); err != nil {
		return cluster, err
	}
	cfg, err := kubernetesConfig(ds)
	if err != nil {
		return cluster, err
	}
	var auth DatasourceAuthConfig
	if err := json.Unmarshal([]byte(ds.AuthConfig), &auth); err != nil {
		return cluster, errors.New("Kubernetes 认证配置格式不正确")
	}
	if ds.AuthType == "bearer" && strings.TrimSpace(auth.Token) == "" {
		return cluster, errors.New("Kubernetes 接入缺少 Bearer Token")
	}
	const contextName = "default"
	config := clientcmdapi.NewConfig()
	config.Clusters[contextName] = &clientcmdapi.Cluster{Server: ds.URL, TLSServerName: cfg.TLSServerName, InsecureSkipTLSVerify: cfg.InsecureSkipVerify}
	if !cfg.InsecureSkipVerify {
		config.Clusters[contextName].CertificateAuthorityData = []byte(cfg.CACertificate)
	}
	config.AuthInfos[contextName] = &clientcmdapi.AuthInfo{Token: auth.Token, ClientCertificateData: []byte(auth.ClientCertificate), ClientKeyData: []byte(auth.ClientKey)}
	config.Contexts[contextName] = &clientcmdapi.Context{Cluster: contextName, AuthInfo: contextName, Namespace: cfg.Namespace}
	config.CurrentContext = contextName
	data, err := clientcmd.Write(*config)
	if err != nil {
		return cluster, errors.New("生成 Kubernetes 连接配置失败")
	}
	return servicePortalClusterState{ID: strconv.FormatUint(uint64(ds.ID), 10), Name: ds.Name, Context: contextName, Kubeconfig: string(data)}, nil
}

func servicePortalDatasourceState() ([]servicePortalClusterState, error) {
	var rows []models.Datasource
	if err := database.DB.Where("type = ? AND status = 1", "k8s").Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]servicePortalClusterState, 0, len(rows))
	for _, ds := range rows {
		if cluster, err := portalCluster(ds); err == nil {
			result = append(result, cluster)
		}
	}
	return result, nil
}

func toServicePortalResource(item models.ServicePortalResource) servicePortalResourceView {
	typeName, _ := targetconfig.Type(item.Type)
	clusterID := ""
	if typeName == targetconfig.Kubernetes || typeName == targetconfig.KubernetesTCP {
		clusterID = strconv.FormatUint(uint64(item.DatasourceID), 10)
	}
	return servicePortalResourceView{ID: item.ID, Name: item.Name, Type: typeName, TargetURL: item.TargetURL, ClusterID: clusterID, DatasourceID: item.DatasourceID, Namespace: item.Namespace, Service: item.Service, Port: item.Port, PublicPort: item.PublicPort, Path: item.Path, Desired: item.Desired, OnDemand: item.OnDemand, Status: item.Status, LastError: item.LastError, Recovered: item.Recovered, FolderID: item.FolderID}
}

// ServicePortalState is only used by the separate runtime. It is not exposed
// through the browser-facing /service-portal proxy.
func ServicePortalState(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !portalInternal(c) {
		return
	}
	if _, ok := currentServiceAccess(c); !ok {
		return
	}
	writeServicePortalState(c)
}

// ServicePortalRuntimeState lets the runtime reconcile enabled services without
// depending on a browser session. The internal token only authorizes this
// private configuration read; business requests still require 观澜 identity.
func ServicePortalRuntimeState(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !portalInternal(c) {
		return
	}
	writeServicePortalState(c)
}

func writeServicePortalState(c *gin.Context) {
	clusters, err := servicePortalDatasourceState()
	if err != nil {
		c.JSON(503, gin.H{"error": "读取 Kubernetes 接入失败"})
		return
	}
	var resources []models.ServicePortalResource
	var grants []models.ServicePortalGrant
	if err = database.DB.Find(&resources).Error; err != nil {
		c.JSON(503, gin.H{"error": "读取服务配置失败"})
		return
	}
	if err = database.DB.Find(&grants).Error; err != nil {
		c.JSON(503, gin.H{"error": "读取服务授权失败"})
		return
	}
	state := servicePortalState{Clusters: clusters, Resources: make([]servicePortalResourceView, 0, len(resources)), Grants: map[string][]string{}}
	for _, item := range resources {
		if item.Type != targetconfig.HTTPDirect {
			state.Resources = append(state.Resources, toServicePortalResource(item))
		}
	}
	for _, grant := range grants {
		state.Grants[grant.UserID] = append(state.Grants[grant.UserID], grant.ResourceID)
	}
	c.JSON(http.StatusOK, state)
}

func normalizePortalResourceRequest(req *servicePortalResourceRequest) error {
	typeName, err := targetconfig.Type(req.Type)
	if err != nil {
		return err
	}
	req.Type = typeName
	req.Name = strings.TrimSpace(req.Name)
	req.Namespace = strings.TrimSpace(req.Namespace)
	req.Service = strings.TrimSpace(req.Service)
	req.Port = strings.TrimSpace(req.Port)
	req.Path = strings.TrimSpace(req.Path)
	if req.Name == "" || len(req.Name) > 128 {
		return errors.New("请填写有效的服务名称")
	}
	if typeName == targetconfig.HTTP || typeName == targetconfig.HTTPDirect {
		if typeName == targetconfig.HTTPDirect {
			req.PublicPort = 0
		}
		u, err := targetconfig.ParseHTTPURL(req.TargetURL)
		if err != nil {
			return err
		}
		req.TargetURL, req.Path = u.String(), targetconfig.OpenPath(u)
		req.ClusterID, req.DatasourceID = "", 0
		req.Namespace, req.Service, req.Port = "", "", ""
		return nil
	}
	req.TargetURL = ""
	if req.Namespace == "" || len(req.Namespace) > 63 || !servicePortalNamespace.MatchString(req.Namespace) || req.Service == "" || len(req.Service) > 253 || !servicePortalService.MatchString(req.Service) || req.Port == "" {
		return errors.New("请填写服务名称、命名空间、Service 和端口")
	}
	if port, err := strconv.Atoi(req.Port); err == nil {
		if port < 1 || port > 65535 {
			return errors.New("Service 端口无效")
		}
	} else if len(req.Port) > 15 || !servicePortalPortName.MatchString(req.Port) {
		return errors.New("Service 端口名称无效")
	}
	if req.DatasourceID == 0 && req.ClusterID != "" {
		id, err := strconv.ParseUint(req.ClusterID, 10, 64)
		if err != nil || id == 0 || id > uint64(^uint(0)) {
			return errors.New("Kubernetes 接入无效")
		}
		req.DatasourceID = uint(id)
	}
	if req.DatasourceID == 0 {
		return errors.New("请选择 Kubernetes 接入")
	}
	if req.Path == "" {
		req.Path = "/"
	}
	path, err := url.Parse(req.Path)
	if err != nil || path.IsAbs() || path.Host != "" || !strings.HasPrefix(req.Path, "/") || strings.HasPrefix(req.Path, "//") || strings.ContainsAny(req.Path, "\\\r\n") || path.Fragment != "" || strings.HasPrefix(path.Path, "/_portal/") {
		return errors.New("打开路径必须是站内绝对路径，例如 /nacos/")
	}
	if len(req.Path) > 1000 {
		return errors.New("打开路径过长")
	}
	return nil
}

func loadPortalDatasource(id uint) (models.Datasource, error) {
	var ds models.Datasource
	if err := database.DB.Where("id = ? AND type = ? AND status = 1", id, "k8s").First(&ds).Error; err != nil {
		return ds, errors.New("Kubernetes 接入不存在或已停用")
	}
	if _, err := portalCluster(ds); err != nil {
		return ds, err
	}
	return ds, nil
}

func SaveServicePortalResource(c *gin.Context) {
	if _, ok := servicePortalAdmin(c); !ok {
		return
	}
	var req servicePortalResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if err := normalizePortalResourceRequest(&req); err != nil {
		c.JSON(400, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if req.Type != targetconfig.HTTPDirect {
		if _, err := servicePortalTarget("/me"); err != nil || strings.TrimSpace(os.Getenv("SERVICE_PORTAL_INTERNAL_TOKEN")) == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "请先配置服务代理进程"})
			return
		}
	}
	if req.Path == "" {
		req.Path = "/"
	}
	if err := validateDashboardFolderAssignment("services", req.FolderID); err != nil {
		c.JSON(400, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if _, ok := requireDashboardFolderAccess(c, req.FolderID, dashboardFolderAccessEdit); !ok {
		return
	}
	if req.Type == targetconfig.Kubernetes || req.Type == targetconfig.KubernetesTCP {
		if _, err := loadPortalDatasource(req.DatasourceID); err != nil {
			c.JSON(400, gin.H{"code": 400, "message": err.Error()})
			return
		}
	}
	if req.ID == "" {
		req.ID = servicePortalNewID()
	}
	if !servicePortalConfigID.MatchString(req.ID) || (req.Type != targetconfig.HTTPDirect && (req.PublicPort < 1024 || req.PublicPort > 65535)) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "服务标识或访问端口无效"})
		return
	}
	var existing models.ServicePortalResource
	if err := database.DB.First(&existing, "id = ?", req.ID).Error; err == nil {
		if _, ok := requireDashboardFolderAccess(c, existing.FolderID, dashboardFolderAccessEdit); !ok {
			return
		}
		if existing.Type != targetconfig.HTTPDirect {
			if status, runtimeErr := servicePortalRuntimeStatus(c, req.ID); runtimeErr != nil {
				c.JSON(http.StatusBadGateway, gin.H{"code": http.StatusBadGateway, "message": runtimeErr.Error()})
				return
			} else if status == "running" || status == "connecting" || status == "retrying" {
				c.JSON(http.StatusConflict, gin.H{"code": http.StatusConflict, "message": "运行中的资源请先停止后再修改"})
				return
			}
		}
		if existing.Desired {
			c.JSON(409, gin.H{"code": 409, "message": "运行中的资源请先停止后再修改"})
			return
		}
		var portCount int64
		if err := database.DB.Model(&models.ServicePortalResource{}).Where("public_port = ? AND id <> ?", req.PublicPort, req.ID).Count(&portCount).Error; err != nil {
			c.JSON(500, gin.H{"code": 500, "message": "检查对外端口失败"})
			return
		}
		if req.Type != targetconfig.HTTPDirect && portCount > 0 {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "对外端口已被其他服务使用"})
			return
		}
		updates := map[string]any{"name": strings.TrimSpace(req.Name), "type": req.Type, "target_url": req.TargetURL, "datasource_id": req.DatasourceID, "namespace": strings.TrimSpace(req.Namespace), "service": strings.TrimSpace(req.Service), "port": strings.TrimSpace(req.Port), "public_port": req.PublicPort, "path": req.Path, "folder_id": req.FolderID}
		if req.OnDemand != nil {
			updates["on_demand"] = *req.OnDemand
		}
		if req.Type == targetconfig.HTTPDirect {
			updates["on_demand"] = false
			updates["desired"] = false
			updates["status"] = "direct"
			updates["last_error"] = ""
		} else if existing.Type == targetconfig.HTTPDirect {
			updates["status"] = "stopped"
		}
		if err := database.DB.Model(&existing).Updates(updates).Error; err != nil {
			c.JSON(500, gin.H{"code": 500, "message": "更新服务失败"})
			return
		}
		existing.Name, existing.DatasourceID, existing.Namespace, existing.Service, existing.Port, existing.PublicPort, existing.Path, existing.FolderID = req.Name, req.DatasourceID, req.Namespace, req.Service, req.Port, req.PublicPort, req.Path, req.FolderID
		if req.OnDemand != nil {
			existing.OnDemand = *req.OnDemand
		}
		if req.Type == targetconfig.HTTPDirect {
			existing.OnDemand = false
			existing.Desired, existing.Status, existing.LastError = false, "direct", ""
		} else if existing.Type == targetconfig.HTTPDirect {
			existing.Status = "stopped"
		}
		existing.Type, existing.TargetURL = req.Type, req.TargetURL
		c.JSON(200, gin.H{"code": 0, "data": toServicePortalResource(existing)})
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(500, gin.H{"code": 500, "message": "读取服务失败"})
		return
	}
	var portCount int64
	if err := database.DB.Model(&models.ServicePortalResource{}).Where("public_port = ?", req.PublicPort).Count(&portCount).Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "message": "检查对外端口失败"})
		return
	}
	if req.Type != targetconfig.HTTPDirect && portCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "对外端口已被其他服务使用"})
		return
	}
	item := models.ServicePortalResource{ID: req.ID, Name: strings.TrimSpace(req.Name), Type: req.Type, TargetURL: req.TargetURL, DatasourceID: req.DatasourceID, Namespace: strings.TrimSpace(req.Namespace), Service: strings.TrimSpace(req.Service), Port: strings.TrimSpace(req.Port), PublicPort: req.PublicPort, Path: req.Path, FolderID: req.FolderID, Desired: true, Status: "connecting"}
	if req.Type == targetconfig.HTTPDirect {
		item.Desired, item.Status = false, "direct"
	} else if req.OnDemand != nil && *req.OnDemand {
		item.OnDemand, item.Status = true, "idle"
	}
	if err := database.DB.Create(&item).Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "message": "创建服务失败"})
		return
	}
	c.JSON(200, gin.H{"code": 0, "data": toServicePortalResource(item)})
}

// servicePortalRuntimeStatus asks the process that owns listeners for a
// current status before changing a durable definition. Desired is the durable
// intent, while only the runtime can confirm whether its listener has closed.
func servicePortalRuntimeStatus(c *gin.Context, id string) (string, error) {
	target, err := servicePortalTarget("/resources")
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, target.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", c.GetHeader("Authorization"))
	req.Header.Set("Accept", "application/json")
	response, err := servicePortalHTTP.Do(req)
	if err != nil {
		return "", errors.New("服务访问后端暂不可用，无法确认运行状态")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", errors.New("服务运行状态同步失败")
	}
	var rows []struct {
		Resource struct {
			ID string `json:"id"`
		} `json:"resource"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&rows); err != nil {
		return "", errors.New("服务运行状态响应无效")
	}
	for _, row := range rows {
		if row.Resource.ID == id {
			return row.Status, nil
		}
	}
	return "", nil
}

func DeleteServicePortalResource(c *gin.Context) {
	if _, ok := servicePortalAdmin(c); !ok {
		return
	}
	id := c.Param("id")
	var item models.ServicePortalResource
	if err := database.DB.First(&item, "id = ?", id).Error; err != nil {
		c.JSON(404, gin.H{"code": 404, "message": "资源不存在"})
		return
	}
	if _, ok := requireDashboardFolderAccess(c, item.FolderID, dashboardFolderAccessEdit); !ok {
		return
	}
	// Disable before stopping so a concurrent runtime reconciliation cannot
	// reopen the listener between the stop request and deleting its definition.
	if err := database.DB.Model(&item).Update("desired", false).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "停用服务失败"})
		return
	}
	if item.Type != targetconfig.HTTPDirect {
		if err := stopServicePortalRuntime(c, id); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"code": http.StatusBadGateway, "message": err.Error()})
			return
		}
	}
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("resource_id = ?", id).Delete(&models.ServicePortalGrant{}).Error; err != nil {
			return err
		}
		return tx.Delete(&item).Error
	}); err != nil {
		c.JSON(500, gin.H{"code": 500, "message": "删除服务失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": nil})
}

// setServicePortalResourceDesired records lifecycle intent in 观澜 even while
// the runtime is unavailable. Its reconciliation loop applies the change and
// keeps retrying an enabled service until connectivity recovers.
func setServicePortalResourceDesired(c *gin.Context, id string, desired bool) {
	if _, ok := servicePortalAdmin(c); !ok {
		return
	}
	var item models.ServicePortalResource
	if err := database.DB.First(&item, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "资源不存在"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "读取服务失败"})
		}
		return
	}
	if _, ok := requireDashboardFolderAccess(c, item.FolderID, dashboardFolderAccessEdit); !ok {
		return
	}
	if item.Type == targetconfig.HTTPDirect {
		c.JSON(400, gin.H{"code": 400, "message": "直连服务无需启停"})
		return
	}
	status := "stopping"
	if desired {
		status = "connecting"
		if item.OnDemand {
			status = "idle"
		}
	}
	if err := database.DB.Model(&item).Updates(map[string]any{"desired": desired, "status": status, "last_error": ""}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "更新服务启用状态失败"})
		return
	}
	item.Desired, item.Status, item.LastError = desired, status, ""
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": toServicePortalResource(item)})
}

func stopServicePortalRuntime(c *gin.Context, id string) error {
	target, err := servicePortalTarget("/resources/" + url.PathEscape(id) + "/stop")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, target.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.GetHeader("Authorization"))
	resp, err := servicePortalHTTP.Do(req)
	if err != nil {
		return errors.New("服务访问后端暂不可用，无法停止运行中的资源")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("服务运行状态同步失败")
	}
	return nil
}

func MoveServicePortalResource(c *gin.Context) {
	if _, ok := servicePortalAdmin(c); !ok {
		return
	}
	id := c.Param("id")
	var item models.ServicePortalResource
	if err := database.DB.First(&item, "id = ?", id).Error; err != nil {
		c.JSON(404, gin.H{"code": 404, "message": "资源不存在"})
		return
	}
	var req struct {
		FolderID uint `json:"folder_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if _, ok := requireDashboardFolderAccess(c, item.FolderID, dashboardFolderAccessEdit); !ok {
		return
	}
	if err := validateDashboardFolderAssignment("services", req.FolderID); err != nil {
		c.JSON(400, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if _, ok := requireDashboardFolderAccess(c, req.FolderID, dashboardFolderAccessEdit); !ok {
		return
	}
	if err := database.DB.Model(&item).Update("folder_id", req.FolderID).Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "message": "移动服务失败"})
		return
	}
	c.JSON(200, gin.H{"code": 0, "message": "移动成功"})
}

func ListServicePortalGrants(c *gin.Context) {
	if _, ok := servicePortalAdmin(c); !ok {
		return
	}
	var rows []models.ServicePortalGrant
	if err := database.DB.Find(&rows).Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "message": "读取授权失败"})
		return
	}
	result := map[string][]string{}
	for _, row := range rows {
		result[row.UserID] = append(result[row.UserID], row.ResourceID)
	}
	c.JSON(200, gin.H{"code": 0, "data": result})
}

func SetServicePortalGrant(c *gin.Context) {
	if _, ok := servicePortalAdmin(c); !ok {
		return
	}
	var req struct {
		UserID     string `json:"userId"`
		ResourceID string `json:"resourceId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.UserID == "" || req.ResourceID == "" {
		c.JSON(400, gin.H{"code": 400, "message": "用户和资源不能为空"})
		return
	}
	if c.Request.Method == http.MethodPost {
		if !strings.HasPrefix(req.UserID, "guanlan:") {
			c.JSON(400, gin.H{"code": 400, "message": "请选择有效的 观澜 用户"})
			return
		}
		userID, err := strconv.ParseUint(strings.TrimPrefix(req.UserID, "guanlan:"), 10, 64)
		if err != nil || userID == 0 {
			c.JSON(400, gin.H{"code": 400, "message": "请选择有效的 观澜 用户"})
			return
		}
		var targetUser models.User
		if err := database.DB.Where("id = ? AND status = 1", userID).First(&targetUser).Error; err != nil {
			c.JSON(400, gin.H{"code": 400, "message": "请选择有效的 观澜 用户"})
			return
		}
	}
	var resource models.ServicePortalResource
	if err := database.DB.First(&resource, "id = ?", req.ResourceID).Error; err != nil {
		c.JSON(404, gin.H{"code": 404, "message": "资源不存在"})
		return
	}
	if resource.FolderID != 0 {
		c.JSON(400, gin.H{"code": 400, "message": "分组内服务请配置目录角色权限"})
		return
	}
	if c.Request.Method == http.MethodDelete {
		if err := database.DB.Where("user_id = ? AND resource_id = ?", req.UserID, req.ResourceID).Delete(&models.ServicePortalGrant{}).Error; err != nil {
			c.JSON(500, gin.H{"code": 500, "message": "撤销授权失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": nil})
		return
	}
	if err := database.DB.Where("user_id = ? AND resource_id = ?", req.UserID, req.ResourceID).FirstOrCreate(&models.ServicePortalGrant{UserID: req.UserID, ResourceID: req.ResourceID}).Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "message": "保存授权失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": nil})
}
