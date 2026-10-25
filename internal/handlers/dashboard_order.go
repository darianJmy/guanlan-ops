package handlers

import (
	"errors"
	"fmt"
	"guanlan/pkg/resp"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"guanlan/internal/database"
	"guanlan/internal/models"
)

// Only non-secret configuration fields are included in the shared card list.
const dashboardCardsSQL = `SELECT id, 'grafana' AS kind, sort, name, description, provider, url,
 '' AS project, '' AS workspace, '' AS region, '' AS role_arn, status FROM dashboards WHERE provider = 'grafana'
 UNION ALL SELECT id, 'logs', sort, name, description, provider, '', project, '', region, role_arn, 1 FROM sls_dashboard_projects
 UNION ALL SELECT id, 'cloud', sort, name, description, '', '', '', workspace, region, '', 1 FROM cloud_monitor_workspaces`

type dashboardCard struct {
	ID          uint   `json:"id"`
	Kind        string `json:"kind"`
	Sort        int    `json:"sort"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Provider    string `json:"provider"`
	URL         string `json:"url,omitempty"`
	Project     string `json:"project,omitempty"`
	Workspace   string `json:"workspace,omitempty"`
	Region      string `json:"region,omitempty"`
	RoleArn     string `json:"role_arn,omitempty"`
	Status      int    `json:"status"`
}

type dashboardOrderItem struct {
	Kind string `json:"kind"`
	ID   uint   `json:"id"`
}

func dashboardOrderModel(kind string) interface{} {
	switch kind {
	case "grafana":
		return &models.Dashboard{}
	case "logs":
		return &models.SLSDashboardProject{}
	case "cloud":
		return &models.CloudMonitorWorkspace{}
	default:
		return nil
	}
}

func ListDashboardCards(c *gin.Context) {
	cards := make([]dashboardCard, 0)
	if err := database.DB.Raw("SELECT * FROM (" + dashboardCardsSQL + ") ORDER BY sort ASC, kind ASC, id ASC").Scan(&cards).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "读取仪表盘失败")
		return
	}
	resp.SuccessData(c, http.StatusOK, cards)
}

// Creation and position allocation use one transaction across all provider tables.
func createDashboardAtFront(item interface{}) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		var next int
		if err := tx.Raw("SELECT COALESCE(MIN(sort), 0) - 1 FROM (" + dashboardCardsSQL + ")").Scan(&next).Error; err != nil {
			return err
		}
		switch value := item.(type) {
		case *models.Dashboard:
			value.Sort = next
		case *models.SLSDashboardProject:
			value.Sort = next
		case *models.CloudMonitorWorkspace:
			value.Sort = next
		default:
			return fmt.Errorf("unsupported dashboard model")
		}
		return tx.Create(item).Error
	})
}

func ReorderDashboardCards(c *gin.Context) {
	var req struct {
		Items []dashboardOrderItem `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Items) == 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "排序参数不正确")
		return
	}
	seen := make(map[dashboardOrderItem]bool, len(req.Items))
	for _, item := range req.Items {
		if item.ID == 0 || dashboardOrderModel(item.Kind) == nil || seen[item] {
			resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "排序包含重复或无效的仪表盘")
			return
		}
		seen[item] = true
	}
	changed := errors.New("dashboard list changed")
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var current []dashboardOrderItem
		if err := tx.Raw("SELECT kind, id FROM (" + dashboardCardsSQL + ")").Scan(&current).Error; err != nil {
			return err
		}
		if len(current) != len(req.Items) {
			return changed
		}
		for _, item := range current {
			if !seen[item] {
				return changed
			}
		}
		for index, item := range req.Items {
			if err := tx.Model(dashboardOrderModel(item.Kind)).Where("id = ?", item.ID).Update("sort", index).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, changed) {
		resp.Error(c, http.StatusConflict, resp.CodeConflict, "仪表盘列表已变化，请刷新后重新排序")
		return
	}
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "保存仪表盘排序失败")
		return
	}
	resp.Success(c, http.StatusOK, "排序已保存")
}
