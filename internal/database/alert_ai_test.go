package database

import (
	"guanlan/internal/models"
	"testing"
)

func TestAlertAIFreshInitializationAndReopen(t *testing.T) {
	db := freshSQLite(t)
	db.Model(&models.User{}).Where("username = ?", "admin").Updates(map[string]interface{}{"password": "unchanged-hash", "nickname": "existing-admin"})
	center := models.FaultCenter{Name: "existing-center", Status: 1}
	db.Create(&center)
	channel := models.NotificationChannel{Name: "existing-channel", Type: "webhook", Status: 1}
	db.Create(&channel)
	if err := prepareDatabase(db); err != nil {
		t.Fatal(err)
	}
	if err := prepareDatabase(db); err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	var user models.User
	db.Preload("Roles.Permissions").Where("username = ?", "admin").First(&user)
	if user.Password != "unchanged-hash" || user.Nickname != "existing-admin" {
		t.Fatal("reopen reset administrator")
	}
	if db.First(&models.FaultCenter{}, center.ID).Error != nil || db.First(&models.NotificationChannel{}, channel.ID).Error != nil {
		t.Fatal("business data lost")
	}
	for _, code := range []string{"alert:ai-analysis:list", "alert:ai-analysis:edit"} {
		found := false
		for _, r := range user.Roles {
			for _, p := range r.Permissions {
				if p.Code == code {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("admin missing %s", code)
		}
	}
	if !db.Migrator().HasTable(&models.AlertAIAnalysisJob{}) || !db.Migrator().HasTable(&models.AlertAIAnalysisRule{}) {
		t.Fatal("analysis tables missing")
	}
}
