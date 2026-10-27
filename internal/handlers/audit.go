package handlers

import (
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

type auditLogQuery struct {
	Page        int    `form:"page"`
	PageSize    int    `form:"page_size"`
	Keyword     string `form:"keyword"`
	Username    string `form:"username"`
	Module      string `form:"module"`
	Action      string `form:"action"`
	Result      string `form:"result"`
	WorkspaceID uint   `form:"workspace_id"`
	StartTime   string `form:"start_time"`
	EndTime     string `form:"end_time"`
}

func ListAuditLogs(c *gin.Context) {
	var query auditLogQuery
	_ = c.ShouldBindQuery(&query)
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}

	tx := applyAuditLogFilters(database.DB.Model(&models.AuditLog{}), query)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "查询审计日志失败")
		return
	}
	var rows []models.AuditLog
	if err := tx.Order("id DESC").Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&rows).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "查询审计日志失败")
		return
	}
	resp.Success(c, http.StatusOK, "成功", gin.H{
		"list": rows, "total": total, "page": query.Page, "page_size": query.PageSize,
	})
}

func GetAuditLogStats(c *gin.Context) {
	startOfDay := time.Now().Truncate(24 * time.Hour)
	if location := time.Now().Location(); location != time.UTC {
		now := time.Now().In(location)
		startOfDay = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	}
	var today, success, failed, users int64
	base := database.DB.Model(&models.AuditLog{}).Where("created_at >= ?", startOfDay)
	base.Count(&today)
	database.DB.Model(&models.AuditLog{}).Where("created_at >= ? AND result = ?", startOfDay, "成功").Count(&success)
	database.DB.Model(&models.AuditLog{}).Where("created_at >= ? AND result = ?", startOfDay, "失败").Count(&failed)
	database.DB.Model(&models.AuditLog{}).Where("created_at >= ?", startOfDay).Distinct("username").Count(&users)

	var modules, actions []string
	database.DB.Model(&models.AuditLog{}).Where("module <> ''").Distinct().Order("module").Pluck("module", &modules)
	database.DB.Model(&models.AuditLog{}).Where("action <> ''").Distinct().Order("action").Pluck("action", &actions)
	resp.Success(c, http.StatusOK, "成功", gin.H{
		"today": today, "success": success, "failed": failed, "users": users, "modules": modules, "actions": actions,
	})
}

func applyAuditLogFilters(tx *gorm.DB, query auditLogQuery) *gorm.DB {
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		tx = tx.Where("username LIKE ? OR resource_name LIKE ? OR resource_id LIKE ? OR path LIKE ?", like, like, like, like)
	}
	if username := strings.TrimSpace(query.Username); username != "" {
		tx = tx.Where("username = ?", username)
	}
	if module := strings.TrimSpace(query.Module); module != "" {
		tx = tx.Where("module = ?", module)
	}
	if action := strings.TrimSpace(query.Action); action != "" {
		tx = tx.Where("action = ?", action)
	}
	if result := strings.TrimSpace(query.Result); result != "" {
		tx = tx.Where("result = ?", result)
	}
	if query.WorkspaceID > 0 {
		tx = tx.Where("workspace_id = ?", query.WorkspaceID)
	}
	if value, ok := parseAuditTime(query.StartTime, false); ok {
		tx = tx.Where("created_at >= ?", value)
	}
	if value, ok := parseAuditTime(query.EndTime, true); ok {
		tx = tx.Where("created_at <= ?", value)
	}
	return tx
}

func parseAuditTime(value string, endOfDay bool) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			if endOfDay && layout == "2006-01-02" {
				parsed = parsed.Add(24*time.Hour - time.Nanosecond)
			}
			return parsed, true
		}
	}
	if unix, err := strconv.ParseInt(value, 10, 64); err == nil {
		return time.UnixMilli(unix), true
	}
	return time.Time{}, false
}
