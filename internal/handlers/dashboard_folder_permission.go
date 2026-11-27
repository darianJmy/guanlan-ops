package handlers

import (
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	dashboardFolderAccessNone   = "none"
	dashboardFolderAccessView   = "view"
	dashboardFolderAccessEdit   = "edit"
	dashboardFolderAccessManage = "manage"
)

var dashboardFolderAccessRanks = map[string]int{
	dashboardFolderAccessNone: 0, dashboardFolderAccessView: 1,
	dashboardFolderAccessEdit: 2, dashboardFolderAccessManage: 3,
}

type dashboardFolderAccessResolver struct {
	isAdmin  bool
	folders  map[uint]models.DashboardFolder
	grants   map[uint]string
	cache    map[uint]string
	visiting map[uint]bool
}

func newDashboardFolderAccessResolver(c *gin.Context) (*dashboardFolderAccessResolver, error) {
	resolver := &dashboardFolderAccessResolver{
		folders: make(map[uint]models.DashboardFolder), grants: make(map[uint]string),
		cache: make(map[uint]string), visiting: make(map[uint]bool),
	}
	var folders []models.DashboardFolder
	if err := database.DB.Find(&folders).Error; err != nil {
		return nil, err
	}
	for _, folder := range folders {
		resolver.folders[folder.ID] = folder
	}

	var roles []models.Role
	if err := database.DB.Table("roles AS r").
		Select("r.id, r.code, r.status").
		Joins("JOIN user_roles AS ur ON ur.role_id = r.id").
		Where("ur.user_id = ? AND r.status = 1", c.GetUint("user_id")).
		Find(&roles).Error; err != nil {
		return nil, err
	}
	roleIDs := make([]uint, 0, len(roles))
	for _, role := range roles {
		if role.Code == "admin" {
			resolver.isAdmin = true
		}
		roleIDs = append(roleIDs, role.ID)
	}
	if resolver.isAdmin || len(roleIDs) == 0 {
		return resolver, nil
	}

	var grants []models.DashboardFolderRolePermission
	if err := database.DB.Where("role_id IN ?", roleIDs).Find(&grants).Error; err != nil {
		return nil, err
	}
	for _, grant := range grants {
		level := normalizeDashboardFolderAccess(grant.AccessLevel)
		if dashboardFolderAccessRanks[level] > dashboardFolderAccessRanks[resolver.grants[grant.FolderID]] {
			resolver.grants[grant.FolderID] = level
		}
	}
	return resolver, nil
}

func (r *dashboardFolderAccessResolver) access(folderID uint) string {
	if folderID == 0 || r.isAdmin {
		return dashboardFolderAccessManage
	}
	if value, ok := r.cache[folderID]; ok {
		return value
	}
	folder, ok := r.folders[folderID]
	if !ok || r.visiting[folderID] {
		return dashboardFolderAccessNone
	}
	r.visiting[folderID] = true
	level := r.access(folder.ParentID)
	delete(r.visiting, folderID)
	if folder.PermissionMode == "restricted" {
		grant := r.grants[folderID]
		if dashboardFolderAccessRanks[grant] < dashboardFolderAccessRanks[level] {
			level = normalizeDashboardFolderAccess(grant)
		}
	}
	r.cache[folderID] = level
	return level
}

func (r *dashboardFolderAccessResolver) require(folderID uint, required string) error {
	if folderID > 0 {
		if _, ok := r.folders[folderID]; !ok {
			return gorm.ErrRecordNotFound
		}
	}
	if dashboardFolderAccessRanks[r.access(folderID)] < dashboardFolderAccessRanks[required] {
		return fmt.Errorf("没有该目录的%s权限", dashboardFolderAccessLabel(required))
	}
	return nil
}

func normalizeDashboardFolderAccess(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if _, ok := dashboardFolderAccessRanks[value]; ok {
		return value
	}
	return dashboardFolderAccessNone
}

func dashboardFolderAccessLabel(value string) string {
	switch value {
	case dashboardFolderAccessManage:
		return "管理"
	case dashboardFolderAccessEdit:
		return "编辑"
	default:
		return "查看"
	}
}

func requireDashboardFolderAccess(c *gin.Context, folderID uint, required string) (*dashboardFolderAccessResolver, bool) {
	resolver, err := newDashboardFolderAccessResolver(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "检查目录权限失败"})
		return nil, false
	}
	if err := resolver.require(folderID, required); err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "仪表盘目录不存在"})
		} else {
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": err.Error()})
		}
		return nil, false
	}
	return resolver, true
}

type dashboardFolderPermissionGrantReq struct {
	RoleID      uint   `json:"role_id"`
	AccessLevel string `json:"access_level"`
}

type dashboardFolderPermissionsReq struct {
	PermissionMode string                              `json:"permission_mode"`
	Permissions    []dashboardFolderPermissionGrantReq `json:"permissions"`
}

type dashboardFolderPermissionRoleView struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Code        string `json:"code"`
	AccessLevel string `json:"access_level"`
}

func GetDashboardFolderPermissions(c *gin.Context) {
	id, err := parseDashboardFolderID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	resolver, ok := requireDashboardFolderAccess(c, id, dashboardFolderAccessManage)
	if !ok {
		return
	}
	folder := resolver.folders[id]
	if !requireDashboardFolderScopePermission(c, folder.Scope, "dashboard:edit") {
		return
	}

	var roles []models.Role
	if err := database.DB.Where("status = 1").Order("id ASC").Find(&roles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "读取角色失败"})
		return
	}
	var grants []models.DashboardFolderRolePermission
	if err := database.DB.Where("folder_id = ?", id).Find(&grants).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "读取目录权限失败"})
		return
	}
	grantByRole := make(map[uint]string, len(grants))
	for _, grant := range grants {
		grantByRole[grant.RoleID] = normalizeDashboardFolderAccess(grant.AccessLevel)
	}
	result := make([]dashboardFolderPermissionRoleView, 0, len(roles))
	for _, role := range roles {
		result = append(result, dashboardFolderPermissionRoleView{
			ID: role.ID, Name: role.Name, Code: role.Code, AccessLevel: grantByRole[role.ID],
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "成功", "data": gin.H{
		"folder_id": id, "folder_name": folder.Name, "permission_mode": folder.PermissionMode, "roles": result,
	}})
}

func UpdateDashboardFolderPermissions(c *gin.Context) {
	id, err := parseDashboardFolderID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	resolver, ok := requireDashboardFolderAccess(c, id, dashboardFolderAccessManage)
	if !ok {
		return
	}
	if !requireDashboardFolderScopePermission(c, resolver.folders[id].Scope, "dashboard:edit") {
		return
	}
	var req dashboardFolderPermissionsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	mode := strings.ToLower(strings.TrimSpace(req.PermissionMode))
	if mode != "inherit" && mode != "restricted" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "目录权限模式不正确"})
		return
	}

	grants := make([]models.DashboardFolderRolePermission, 0, len(req.Permissions))
	roleIDs := make([]uint, 0, len(req.Permissions))
	seen := make(map[uint]struct{}, len(req.Permissions))
	if mode == "restricted" {
		for _, item := range req.Permissions {
			level := normalizeDashboardFolderAccess(item.AccessLevel)
			requestedLevel := strings.ToLower(strings.TrimSpace(item.AccessLevel))
			if requestedLevel != dashboardFolderAccessNone && level == dashboardFolderAccessNone {
				c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "目录访问级别不正确"})
				return
			}
			if item.RoleID == 0 || level == dashboardFolderAccessNone {
				continue
			}
			if _, exists := seen[item.RoleID]; exists {
				c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "角色权限不能重复"})
				return
			}
			seen[item.RoleID] = struct{}{}
			roleIDs = append(roleIDs, item.RoleID)
			grants = append(grants, models.DashboardFolderRolePermission{FolderID: id, RoleID: item.RoleID, AccessLevel: level})
		}
	}
	if len(roleIDs) > 0 {
		var roleCount int64
		if err := database.DB.Model(&models.Role{}).Where("id IN ? AND status = 1", roleIDs).Count(&roleCount).Error; err != nil || roleCount != int64(len(roleIDs)) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "包含不存在或已停用的角色"})
			return
		}
	}
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.DashboardFolder{}).Where("id = ?", id).Update("permission_mode", mode).Error; err != nil {
			return err
		}
		if err := tx.Where("folder_id = ?", id).Delete(&models.DashboardFolderRolePermission{}).Error; err != nil {
			return err
		}
		if len(grants) > 0 {
			return tx.Create(&grants).Error
		}
		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "保存目录权限失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "目录权限已保存"})
}
