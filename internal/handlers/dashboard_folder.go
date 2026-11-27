package handlers

import (
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const maxDashboardFolderDepth = 8

type dashboardFolderReq struct {
	Scope    string `json:"scope"`
	ParentID uint   `json:"parent_id"`
	Name     string `json:"name"`
	Sort     int    `json:"sort"`
}

func ListDashboardFolders(c *gin.Context) {
	scope, err := normalizeDashboardFolderScope(c.Query("scope"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if !requireDashboardFolderScopePermission(c, scope, "dashboard:list") {
		return
	}
	var folders []models.DashboardFolder
	if err := database.DB.Where("scope = ?", scope).Order("sort ASC, name ASC, id ASC").Find(&folders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "读取仪表盘目录失败"})
		return
	}
	resolver, err := newDashboardFolderAccessResolver(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "检查目录权限失败"})
		return
	}
	visible := make([]models.DashboardFolder, 0, len(folders))
	for _, folder := range folders {
		folder.AccessLevel = resolver.access(folder.ID)
		if folder.AccessLevel != dashboardFolderAccessNone {
			visible = append(visible, folder)
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "成功", "data": visible})
}

func CreateDashboardFolder(c *gin.Context) {
	var req dashboardFolderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	scope, err := normalizeDashboardFolderScope(req.Scope)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if !requireDashboardFolderScopePermission(c, scope, "dashboard:create") {
		return
	}
	name, err := validateDashboardFolderName(req.Name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if err := validateDashboardFolderParent(scope, req.ParentID, 0); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if _, ok := requireDashboardFolderAccess(c, req.ParentID, dashboardFolderAccessEdit); !ok {
		return
	}
	folder := models.DashboardFolder{Scope: scope, ParentID: req.ParentID, Name: name, Sort: req.Sort}
	if err := database.DB.Create(&folder).Error; err != nil {
		if isDuplicateDatabaseError(err) {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "同级目录名称已存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "创建仪表盘目录失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "创建成功", "data": folder})
}

func UpdateDashboardFolder(c *gin.Context) {
	id, err := parseDashboardFolderID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	var folder models.DashboardFolder
	if err := database.DB.First(&folder, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "仪表盘目录不存在"})
		return
	}
	if !requireDashboardFolderScopePermission(c, folder.Scope, "dashboard:edit") {
		return
	}
	resolver, ok := requireDashboardFolderAccess(c, id, dashboardFolderAccessManage)
	if !ok {
		return
	}
	var req dashboardFolderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	name, err := validateDashboardFolderName(req.Name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if err := validateDashboardFolderParent(folder.Scope, req.ParentID, folder.ID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if req.ParentID != folder.ParentID {
		if err := resolver.require(req.ParentID, dashboardFolderAccessEdit); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "没有目标目录的编辑权限"})
			return
		}
	}
	if err := database.DB.Model(&folder).Updates(map[string]interface{}{"name": name, "parent_id": req.ParentID, "sort": req.Sort}).Error; err != nil {
		if isDuplicateDatabaseError(err) {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "同级目录名称已存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "更新仪表盘目录失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "更新成功"})
}

func DeleteDashboardFolder(c *gin.Context) {
	id, err := parseDashboardFolderID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	var folder models.DashboardFolder
	if err := database.DB.First(&folder, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "仪表盘目录不存在"})
		return
	}
	if !requireDashboardFolderScopePermission(c, folder.Scope, "dashboard:delete") {
		return
	}
	if _, ok := requireDashboardFolderAccess(c, id, dashboardFolderAccessManage); !ok {
		return
	}
	var children int64
	if err := database.DB.Model(&models.DashboardFolder{}).Where("parent_id = ?", id).Count(&children).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "检查子目录失败"})
		return
	}
	items, err := servicePortalFolderItemCount(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "检查目录内容失败"})
		return
	}
	if children > 0 || items > 0 {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "目录非空，请先移动或删除其中的内容"})
		return
	}
	if err := database.DB.Delete(&folder).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "删除仪表盘目录失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "删除成功"})
}

func normalizeDashboardFolderScope(value string) (string, error) {
	if strings.EqualFold(strings.TrimSpace(value), "services") {
		return "services", nil
	}
	return "", fmt.Errorf("仪表盘目录类型不正确")
}

func requireDashboardFolderScopePermission(c *gin.Context, scope, permission string) bool {
	if scope != "services" {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "目录不存在"})
		return false
	}
	if permission == "dashboard:list" {
		_, ok := currentServiceAccess(c)
		return ok
	}
	_, ok := servicePortalAdmin(c)
	return ok
}

func validateDashboardFolderName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", fmt.Errorf("请填写目录名称")
	}
	if len([]rune(name)) > 100 {
		return "", fmt.Errorf("目录名称不能超过 100 个字符")
	}
	return name, nil
}

func validateDashboardFolderAssignment(scope string, folderID uint) error {
	if folderID == 0 {
		return nil
	}
	var count int64
	if err := database.DB.Model(&models.DashboardFolder{}).Where("id = ? AND scope = ?", folderID, scope).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("选择的仪表盘目录不存在")
	}
	return nil
}

func validateDashboardFolderParent(scope string, parentID, movingID uint) error {
	if parentID == 0 {
		return nil
	}
	if parentID == movingID {
		return fmt.Errorf("目录不能作为自己的上级目录")
	}
	currentID := parentID
	for depth := 1; depth <= maxDashboardFolderDepth; depth++ {
		var parent models.DashboardFolder
		if err := database.DB.Where("id = ? AND scope = ?", currentID, scope).First(&parent).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("上级目录不存在")
			}
			return err
		}
		if parent.ID == movingID {
			return fmt.Errorf("不能把目录移动到自己的子目录")
		}
		if parent.ParentID == 0 {
			if depth >= maxDashboardFolderDepth {
				return fmt.Errorf("目录最多支持 %d 级", maxDashboardFolderDepth)
			}
			return nil
		}
		currentID = parent.ParentID
	}
	return fmt.Errorf("目录最多支持 %d 级", maxDashboardFolderDepth)
}

func parseDashboardFolderID(value string) (uint, error) {
	id, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("仪表盘目录 ID 不正确")
	}
	return uint(id), nil
}

func isDuplicateDatabaseError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
}
