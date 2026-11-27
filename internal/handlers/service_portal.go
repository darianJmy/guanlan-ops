package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const serviceDashboardList = "service-dashboard:list"
const serviceDashboardManage = "service-dashboard:manage"

// Serialize configuration writes across resource and folder handlers on this SQLite instance.
var serviceDashboardWrites sync.Mutex

func LockServiceDashboardWrites(c *gin.Context) {
	if c.Request.Method == http.MethodGet || strings.HasSuffix(c.Request.URL.Path, "/access") {
		c.Next()
		return
	}
	serviceDashboardWrites.Lock()
	defer serviceDashboardWrites.Unlock()
	c.Next()
}

type serviceAccessUser struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Admin        bool            `json:"admin"`
	Enabled      bool            `json:"enabled"`
	FolderAccess map[uint]string `json:"folderAccess,omitempty"`
}

// Resolve current account and role state on every request, including requests
// from resource sessions in the independent portal. JWT claims alone are not
// sufficient to keep a disabled account or revoked role out of an open service.
func currentServiceAccess(c *gin.Context) (serviceAccessUser, bool) {
	var user models.User
	if database.DB == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "认证服务暂时不可用"})
		return serviceAccessUser{}, false
	}
	err := database.DB.Preload("Roles.Permissions").First(&user, c.GetUint("user_id")).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "认证服务暂时不可用"})
		return serviceAccessUser{}, false
	}
	if err != nil || user.Status != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "账号不存在或已停用"})
		return serviceAccessUser{}, false
	}
	permissions := map[string]bool{}
	for _, role := range user.Roles {
		if role.Status != 1 {
			continue
		}
		for _, permission := range role.Permissions {
			if permission.Status == 1 {
				permissions[permission.Code] = true
			}
		}
	}
	if !permissions[serviceDashboardList] {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "没有服务仪表盘访问权限"})
		return serviceAccessUser{}, false
	}
	resolver, err := newDashboardFolderAccessResolver(c)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "目录权限服务暂时不可用"})
		return serviceAccessUser{}, false
	}
	folderAccess := make(map[uint]string)
	for id, folder := range resolver.folders {
		if folder.Scope == "services" {
			folderAccess[id] = resolver.access(id)
		}
	}
	return serviceAccessUser{ID: fmt.Sprintf("guanlan:%d", user.ID), Name: user.Username, Admin: permissions[serviceDashboardManage], Enabled: true, FolderAccess: folderAccess}, true
}

func ServiceAccessIdentity(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if user, ok := currentServiceAccess(c); ok {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"user": user}})
	}
}

// Only identity options are returned. Passwords and the system user-management
// API are never exposed to the portal service.
func ServiceAccessUsers(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	user, ok := currentServiceAccess(c)
	if !ok {
		return
	}
	if !user.Admin {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "没有服务配置管理权限"})
		return
	}
	var users []models.User
	if err := database.DB.Select("id", "username", "status").Preload("Roles.Permissions").Order("username, id").Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "读取用户失败"})
		return
	}
	items := make([]serviceAccessUser, 0, len(users))
	for _, item := range users {
		admin := false
		for _, role := range item.Roles {
			if role.Status != 1 {
				continue
			}
			for _, permission := range role.Permissions {
				if permission.Status == 1 && permission.Code == serviceDashboardManage {
					admin = true
				}
			}
		}
		items = append(items, serviceAccessUser{ID: fmt.Sprintf("guanlan:%d", item.ID), Name: item.Username, Admin: admin, Enabled: item.Status == 1})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": items})
}

var servicePortalHTTP = &http.Client{
	Timeout:       45 * time.Second,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
}

var servicePortalItemPath = regexp.MustCompile(`^/resources/[A-Za-z0-9_-]+$`)
var servicePortalClusterQuery = regexp.MustCompile(`^/clusters/[A-Za-z0-9_-]+/(namespaces|services)$`)
var servicePortalResourceAction = regexp.MustCompile(`^/resources/[A-Za-z0-9_-]+/(start|stop|access)$`)
var servicePortalResourceFolder = regexp.MustCompile(`^/resources/[A-Za-z0-9_-]+/folder$`)

func servicePortalOperation(method, path string) (allowed, manage bool) {
	switch path {
	case "/me", "/resources":
		if method == http.MethodGet {
			return true, false
		}
		return path == "/resources" && method == http.MethodPost, true
	case "/users":
		return method == http.MethodGet, true
	case "/permissions":
		return method == http.MethodGet || method == http.MethodPost || method == http.MethodDelete, true
	}
	if servicePortalItemPath.MatchString(path) {
		return method == http.MethodDelete, true
	}
	if servicePortalClusterQuery.MatchString(path) {
		return method == http.MethodGet, true
	}
	if servicePortalResourceAction.MatchString(path) {
		return method == http.MethodPost, !strings.HasSuffix(path, "/access")
	}
	if servicePortalResourceFolder.MatchString(path) {
		return method == http.MethodPost, true
	}
	return false, false
}

func servicePortalTarget(path string) (*url.URL, error) {
	target, err := url.Parse(strings.TrimSpace(os.Getenv("SERVICE_PORTAL_URL")))
	if err != nil || target == nil || (target.Scheme != "http" && target.Scheme != "https") || target.Hostname() == "" || target.User != nil || (target.Path != "" && target.Path != "/") || target.RawQuery != "" || target.Fragment != "" {
		return nil, fmt.Errorf("服务访问后端地址未正确配置")
	}
	target.Path = "/api" + path
	return target, nil
}

// 观澜 owns directory records and service definitions. A directory can only
// be deleted when its resource count is zero.
func servicePortalFolderItemCount(folderID uint) (int64, error) {
	var count int64
	if err := database.DB.Model(&models.ServicePortalResource{}).Where("folder_id = ?", folderID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// 观澜 owns service definitions, directory ACLs, grants and Kubernetes
// Datasource records. The independent portal receives only runtime requests.
func ServicePortalProxy(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	user, ok := currentServiceAccess(c)
	if !ok {
		return
	}
	path := c.Param("path")
	allowed, manage := servicePortalOperation(c.Request.Method, path)
	if !allowed {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "服务接口不存在"})
		return
	}
	if manage && !user.Admin {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "没有服务配置管理权限"})
		return
	}
	// The browser gateway registers a wildcard path, while local resource
	// handlers use the same :id parameter as a direct resource route.
	if strings.HasPrefix(path, "/resources/") {
		parts := strings.Split(strings.Trim(path, "/"), "/")
		c.Params = append(c.Params, gin.Param{Key: "id", Value: parts[1]})
	}
	// Configuration mutations stay in the 观澜 process and its SQLite database.
	// The independent process is deliberately limited to discovery, lifecycle,
	// tickets and business proxying.
	switch {
	case c.Request.Method == http.MethodGet && (path == "/me" || path == "/resources"):
		listServicePortalLocal(c, user, path)
		return
	case path == "/resources" && c.Request.Method == http.MethodPost:
		SaveServicePortalResource(c)
		return
	case servicePortalResourceAction.MatchString(path) && !strings.HasSuffix(path, "/access"):
		parts := strings.Split(strings.Trim(path, "/"), "/")
		setServicePortalResourceDesired(c, parts[1], parts[2] == "start")
		return
	case servicePortalItemPath.MatchString(path) && c.Request.Method == http.MethodDelete:
		DeleteServicePortalResource(c)
		return
	case strings.HasPrefix(path, "/resources/") && strings.HasSuffix(path, "/folder") && c.Request.Method == http.MethodPost:
		MoveServicePortalResource(c)
		return
	case path == "/users" && c.Request.Method == http.MethodGet:
		ServiceAccessUsers(c)
		return
	case path == "/permissions" && c.Request.Method == http.MethodGet:
		ListServicePortalGrants(c)
		return
	case path == "/permissions" && (c.Request.Method == http.MethodPost || c.Request.Method == http.MethodDelete):
		SetServicePortalGrant(c)
		return
	}
	target, err := servicePortalTarget(path)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "服务访问后端地址未正确配置"})
		return
	}
	target.RawQuery = c.Request.URL.RawQuery
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, target.String(), c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请求参数错误"})
		return
	}
	req.Header.Set("Authorization", c.GetHeader("Authorization"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.Request.Method == http.MethodPost && strings.HasSuffix(path, "/access") {
		req.Header.Set("X-Service-Portal-Internal", strings.TrimSpace(os.Getenv("SERVICE_PORTAL_INTERNAL_TOKEN")))
		if ip := c.RemoteIP(); ip != "" {
			req.Header.Set("X-Service-Portal-Client-IP", ip)
		}
	}
	response, err := servicePortalHTTP.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": "服务访问后端暂时不可用，请检查独立服务的运行状态"})
		return
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if err != nil || len(body) > 8<<20 {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": "服务访问后端响应无效"})
		return
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		status := response.StatusCode
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &failure)
		message := failure.Error
		if message == "" {
			message = "服务访问请求失败"
		}
		c.JSON(status, gin.H{"code": status, "message": message})
		return
	}
	var data any
	if len(strings.TrimSpace(string(body))) != 0 {
		if err := json.Unmarshal(body, &data); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": "服务访问后端响应无效"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": data})
}
