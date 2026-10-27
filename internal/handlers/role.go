package handlers

import (
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func ListRoles(c *gin.Context) {
	var roles []models.Role
	database.DB.Preload("Permissions").Preload("Menus").Order("id ASC").Find(&roles)
	resp.Success(c, http.StatusOK, "成功", roles)
}

type CreateRoleReq struct {
	Name        string `json:"name" binding:"required"`
	Code        string `json:"code" binding:"required"`
	Description string `json:"description"`
	Status      int    `json:"status"`
}

func CreateRole(c *gin.Context) {
	var req CreateRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}

	if req.Status == 0 {
		req.Status = 1
	}

	role := models.Role{
		Name:        req.Name,
		Code:        req.Code,
		Description: req.Description,
		Status:      req.Status,
	}

	if err := database.DB.Create(&role).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeBadRequest, "角色编码已存在")
		return
	}

	resp.Success(c, http.StatusOK, "创建成功")
}

type UpdateRoleReq struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Status      int    `json:"status"`
}

func UpdateRole(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req UpdateRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}

	database.DB.Model(&models.Role{}).Where("id = ?", id).Updates(map[string]interface{}{
		"name":        req.Name,
		"description": req.Description,
		"status":      req.Status,
	})

	resp.Success(c, http.StatusOK, "更新成功")
}

func DeleteRole(c *gin.Context) {
	id := c.Param("id")
	var role models.Role
	if err := database.DB.First(&role, id).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "角色不存在")
		return
	}
	database.DB.Select("Permissions", "Menus").Delete(&role)
	resp.Success(c, http.StatusOK, "删除成功")
}

type AssignPermissionsReq struct {
	PermissionIDs []uint `json:"permission_ids"`
}

func AssignRolePermissions(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req AssignPermissionsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}

	var role models.Role
	if err := database.DB.First(&role, id).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "角色不存在")
		return
	}

	var perms []models.Permission
	if len(req.PermissionIDs) > 0 {
		database.DB.Find(&perms, req.PermissionIDs)
	}
	database.DB.Model(&role).Association("Permissions").Replace(perms)

	resp.Success(c, http.StatusOK, "分配成功")
}

type AssignMenusReq struct {
	MenuIDs []uint `json:"menu_ids"`
}

func AssignRoleMenus(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req AssignMenusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}

	var role models.Role
	if err := database.DB.First(&role, id).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "角色不存在")
		return
	}

	var menus []models.Menu
	if len(req.MenuIDs) > 0 {
		database.DB.Find(&menus, req.MenuIDs)
	}
	database.DB.Model(&role).Association("Menus").Replace(menus)

	resp.Success(c, http.StatusOK, "分配成功")
}
