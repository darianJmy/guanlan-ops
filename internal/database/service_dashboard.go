package database

import (
	"errors"
	"fmt"

	"guanlan/internal/models"
	sqldb "guanlan/pkg/database"

	"gorm.io/gorm"
)

func createServiceDashboardSchema(db *gorm.DB) error {
	if err := sqldb.AutoMigrate(db,
		&models.DashboardFolder{},
		&models.DashboardFolderRolePermission{},
		&models.ServicePortalResource{},
		&models.ServicePortalGrant{},
	); err != nil {
		return err
	}
	return db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_service_portal_public_port ON service_portal_resources(public_port) WHERE public_port > 0").Error
}

func seedServiceDashboard(db *gorm.DB) error {
	permissions := []models.Permission{
		{Name: "查看服务仪表盘", Code: "service-dashboard:list", Status: 1},
		{Name: "管理服务仪表盘", Code: "service-dashboard:manage", Status: 1},
	}
	for i := range permissions {
		permission := &permissions[i]
		if err := db.Where("code = ?", permission.Code).FirstOrCreate(permission).Error; err != nil {
			return fmt.Errorf("seed service dashboard permission: %w", err)
		}
	}
	var admin models.Role
	if err := db.Where("code = ?", "admin").First(&admin).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if err := db.Model(&admin).Association("Permissions").Append(permissions); err != nil {
		return err
	}
	menu, err := unifiedDashboardMenu(db)
	if err != nil {
		return err
	}
	return db.Model(&admin).Association("Menus").Append(&menu)
}

func unifiedDashboardMenu(db *gorm.DB) (models.Menu, error) {
	menu := models.Menu{
		Name: "仪表盘", Path: "/dashboards", Component: "monitor/DashboardHub",
		Icon: "DataAnalysis", Sort: 1, Type: 2, Permission: "dashboard:list", Status: 1,
	}
	err := db.Where("path = ?", menu.Path).FirstOrCreate(&menu).Error
	return menu, err
}

func mergeServiceDashboardMenu(db *gorm.DB) error {
	legacyMenus := db.Model(&models.Menu{}).Select("id").
		Where("path = ? AND component = ?", "/dashboards/services", "monitor/ServiceDashboardManage")
	menu, err := unifiedDashboardMenu(db)
	if err != nil {
		return err
	}
	if err := db.Exec("INSERT OR IGNORE INTO role_menus (role_id, menu_id) SELECT role_id, ? FROM role_menus WHERE menu_id IN (?)", menu.ID, legacyMenus).Error; err != nil {
		return fmt.Errorf("transfer service dashboard menu assignments: %w", err)
	}
	if err := db.Exec("DELETE FROM role_menus WHERE menu_id IN (?)", legacyMenus).Error; err != nil {
		return err
	}
	return db.Where("id IN (?)", legacyMenus).Delete(&models.Menu{}).Error
}
