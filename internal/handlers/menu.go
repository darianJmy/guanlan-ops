package handlers

import (
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func ListMenus(c *gin.Context) {
	var menus []models.Menu
	database.DB.Order("sort ASC").Find(&menus)

	menuMap := make(map[uint]*models.Menu)
	for i := range menus {
		menuMap[menus[i].ID] = &menus[i]
	}

	tree := make([]models.Menu, 0)
	for _, menu := range menus {
		if menu.ParentID == 0 {
			buildTree(menuMap, menu.ID)
			tree = append(tree, *menuMap[menu.ID])
		}
	}

	resp.Success(c, http.StatusOK, "成功", tree)
}

func buildTree(menuMap map[uint]*models.Menu, id uint) {
	menu := menuMap[id]
	for _, m := range menuMap {
		if m.ParentID == id {
			menu.Children = append(menu.Children, *m)
			buildTree(menuMap, m.ID)
		}
	}
	for i := 0; i < len(menu.Children); i++ {
		for j := i + 1; j < len(menu.Children); j++ {
			if menu.Children[i].Sort > menu.Children[j].Sort {
				menu.Children[i], menu.Children[j] = menu.Children[j], menu.Children[i]
			}
		}
	}
}

type CreateMenuReq struct {
	ParentID   uint   `json:"parent_id"`
	Name       string `json:"name" binding:"required"`
	Path       string `json:"path"`
	Component  string `json:"component"`
	Icon       string `json:"icon"`
	Sort       int    `json:"sort"`
	Type       int    `json:"type" binding:"required"`
	Permission string `json:"permission"`
	Status     int    `json:"status"`
}

func CreateMenu(c *gin.Context) {
	var req CreateMenuReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	if req.Status == 0 {
		req.Status = 1
	}

	menu := models.Menu{
		ParentID:   req.ParentID,
		Name:       req.Name,
		Path:       req.Path,
		Component:  req.Component,
		Icon:       req.Icon,
		Sort:       req.Sort,
		Type:       req.Type,
		Permission: req.Permission,
		Status:     req.Status,
	}

	database.DB.Create(&menu)
	resp.Success(c, http.StatusOK, "创建成功")
}

func UpdateMenu(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req CreateMenuReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}

	database.DB.Model(&models.Menu{}).Where("id = ?", id).Updates(map[string]interface{}{
		"parent_id":  req.ParentID,
		"name":       req.Name,
		"path":       req.Path,
		"component":  req.Component,
		"icon":       req.Icon,
		"sort":       req.Sort,
		"type":       req.Type,
		"permission": req.Permission,
		"status":     req.Status,
	})

	resp.Success(c, http.StatusOK, "更新成功")
}

func DeleteMenu(c *gin.Context) {
	id := c.Param("id")
	var count int64
	database.DB.Model(&models.Menu{}).Where("parent_id = ?", id).Count(&count)
	if count > 0 {
		resp.Error(c, http.StatusOK, resp.CodeBadRequest, "请先删除子菜单")
		return
	}
	database.DB.Delete(&models.Menu{}, id)
	resp.Success(c, http.StatusOK, "删除成功")
}

func GetMenuOptions(c *gin.Context) {
	var menus []models.Menu
	database.DB.Select("id, parent_id, name, sort, type, path, component, permission").Order("sort ASC").Find(&menus)

	menuMap := make(map[uint]*models.Menu)
	for i := range menus {
		menuMap[menus[i].ID] = &menus[i]
	}

	tree := make([]models.Menu, 0)
	for _, menu := range menus {
		if menu.ParentID == 0 {
			buildTree(menuMap, menu.ID)
			tree = append(tree, *menuMap[menu.ID])
		}
	}

	resp.Success(c, http.StatusOK, "成功", tree)
}
