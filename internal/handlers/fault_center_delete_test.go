package handlers

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeleteFaultCenterDefaultProtection(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	previous := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previous; _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.FaultCenter{}, &models.InspectionItem{}, &models.AlertInstance{}, &models.AlertGroupState{}, &models.AlertGroupingRule{}, &models.AlertActionExecution{}, &models.AlertActionRule{}); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.DELETE("/fault-centers/:id", DeleteFaultCenter)
	for _, tc := range []struct {
		name      string
		isDefault bool
		want      int
	}{
		{"默认故障中心", true, http.StatusBadRequest},
		{"renamed default", true, http.StatusBadRequest},
		{"custom", false, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			center := models.FaultCenter{Name: tc.name, IsDefault: tc.isDefault}
			if err := db.Create(&center).Error; err != nil {
				t.Fatal(err)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/fault-centers/%d", center.ID), nil))
			if recorder.Code != tc.want {
				t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
			}
			var count int64
			db.Model(&models.FaultCenter{}).Where("id = ?", center.ID).Count(&count)
			if (count == 1) != tc.isDefault {
				t.Fatalf("unexpected remaining count: %d", count)
			}
		})
	}
}
