package handlers

import (
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func ListDashboards(c *gin.Context) {
	var dashboards []models.Dashboard
	tx := database.DB.Order("sort ASC, id ASC")
	if provider := strings.TrimSpace(c.Query("provider")); provider != "" {
		normalized, err := normalizeDashboardProvider(provider)
		if err != nil {
			resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
			return
		}
		tx = tx.Where("provider = ?", normalized)
	}
	tx.Find(&dashboards)
	resp.Success(c, http.StatusOK, "成功", dashboards)
}

type CreateDashboardReq struct {
	Name        string `json:"name" binding:"required"`
	Provider    string `json:"provider"`
	URL         string `json:"url" binding:"required"`
	Description string `json:"description"`
	Sort        int    `json:"sort"`
	Status      int    `json:"status"`
}

func CreateDashboard(c *gin.Context) {
	var req CreateDashboardReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	if req.Status == 0 {
		req.Status = 1
	}

	provider, err := normalizeDashboardProvider(req.Provider)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	d := models.Dashboard{
		Name:        req.Name,
		Provider:    provider,
		URL:         req.URL,
		Description: req.Description,
		Sort:        req.Sort,
		Status:      req.Status,
	}
	if err := createDashboardAtFront(&d); err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "创建仪表盘失败")
		return
	}
	resp.Success(c, http.StatusOK, "创建成功")
}

func UpdateDashboard(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req UpdateDashboardReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	updates, err := dashboardUpdateFields(req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if len(updates) == 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "没有可更新的字段")
		return
	}

	database.DB.Model(&models.Dashboard{}).Where("id = ?", id).Updates(updates)

	resp.Success(c, http.StatusOK, "更新成功")
}

type UpdateDashboardReq struct {
	Name        *string `json:"name"`
	Provider    *string `json:"provider"`
	URL         *string `json:"url"`
	Description *string `json:"description"`
	Sort        *int    `json:"sort"`
	Status      *int    `json:"status"`
}

func dashboardUpdateFields(req UpdateDashboardReq) (map[string]interface{}, error) {
	updates := make(map[string]interface{})
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			return nil, fmt.Errorf("名称不能为空")
		}
		updates["name"] = *req.Name
	}
	if req.Provider != nil {
		provider, err := normalizeDashboardProvider(*req.Provider)
		if err != nil {
			return nil, err
		}
		updates["provider"] = provider
	}
	if req.URL != nil {
		if strings.TrimSpace(*req.URL) == "" {
			return nil, fmt.Errorf("URL 不能为空")
		}
		updates["url"] = *req.URL
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Sort != nil {
		updates["sort"] = *req.Sort
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	return updates, nil
}

func normalizeDashboardProvider(value string) (string, error) {
	provider := strings.ToLower(strings.TrimSpace(value))
	if provider == "" {
		return "grafana", nil
	}
	if provider != "grafana" && provider != "aliyun_sls" {
		return "", fmt.Errorf("不支持的仪表盘类型")
	}
	return provider, nil
}

func DeleteDashboard(c *gin.Context) {
	id := c.Param("id")
	database.DB.Delete(&models.Dashboard{}, id)
	resp.Success(c, http.StatusOK, "删除成功")
}
