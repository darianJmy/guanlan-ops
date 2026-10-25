package handlers

import (
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const promQueryHistoryLimit = 20

func recordPromQueryHistory(c *gin.Context, datasource models.Datasource, query, mode string) error {
	userID := currentUserID(c)
	if userID == 0 || query == "" {
		return nil
	}

	return database.DB.Transaction(func(tx *gorm.DB) error {
		history := models.PromQueryHistory{
			UserID:         userID,
			DatasourceID:   datasource.ID,
			DatasourceName: datasource.Name,
			Query:          query,
			Mode:           mode,
		}
		if err := tx.Create(&history).Error; err != nil {
			return err
		}

		var count int64
		if err := tx.Model(&models.PromQueryHistory{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
			return err
		}
		excess := count - promQueryHistoryLimit
		if excess <= 0 {
			return nil
		}

		var expiredIDs []uint
		if err := tx.Model(&models.PromQueryHistory{}).
			Where("user_id = ?", userID).
			Order("created_at ASC, id ASC").
			Limit(int(excess)).
			Pluck("id", &expiredIDs).Error; err != nil {
			return err
		}
		return tx.Where("id IN ?", expiredIDs).Delete(&models.PromQueryHistory{}).Error
	})
}

func ListPromQueryHistory(c *gin.Context) {
	userID := currentUserID(c)
	var histories []models.PromQueryHistory
	if err := database.DB.Where("user_id = ?", userID).
		Order("created_at DESC, id DESC").
		Limit(promQueryHistoryLimit).
		Find(&histories).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "查询历史读取失败")
		return
	}
	resp.Success(c, http.StatusOK, "成功", histories)
}

func ClearPromQueryHistory(c *gin.Context) {
	userID := currentUserID(c)
	if err := database.DB.Where("user_id = ?", userID).Delete(&models.PromQueryHistory{}).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "查询历史清空失败")
		return
	}
	resp.Success(c, http.StatusOK, "清空成功")
}
