package models

import "time"

// DashboardFolder groups dashboards within one dashboard section.
type DashboardFolder struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Scope          string    `gorm:"size:32;not null;uniqueIndex:idx_dashboard_folder_name;index" json:"scope"`
	ParentID       uint      `gorm:"not null;default:0;uniqueIndex:idx_dashboard_folder_name;index" json:"parent_id"`
	Name           string    `gorm:"size:100;not null;uniqueIndex:idx_dashboard_folder_name" json:"name"`
	Sort           int       `gorm:"not null;default:0;index" json:"sort"`
	PermissionMode string    `gorm:"size:16;not null;default:inherit" json:"permission_mode"`
	AccessLevel    string    `gorm:"-" json:"access_level"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// DashboardFolderRolePermission grants a role access within a restricted folder subtree.
type DashboardFolderRolePermission struct {
	ID          uint            `gorm:"primaryKey" json:"id"`
	FolderID    uint            `gorm:"not null;uniqueIndex:idx_dashboard_folder_role;index" json:"folder_id"`
	RoleID      uint            `gorm:"not null;uniqueIndex:idx_dashboard_folder_role;index" json:"role_id"`
	AccessLevel string          `gorm:"size:16;not null" json:"access_level"`
	Folder      DashboardFolder `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	Role        Role            `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}
