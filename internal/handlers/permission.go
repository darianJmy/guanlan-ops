package handlers

import (
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"

	"github.com/gin-gonic/gin"
)

func ListPermissions(c *gin.Context) {
	var permissions []models.Permission
	database.DB.Where("code NOT LIKE ?", "task:%").Order("id ASC").Find(&permissions)
	resp.Success(c, http.StatusOK, "成功", permissions)
}
