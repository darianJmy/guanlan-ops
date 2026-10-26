package models

import "time"

type DatasourceGroup struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	ParentID    uint      `gorm:"not null;default:0;index;uniqueIndex:idx_datasource_group_parent_name" json:"parent_id"`
	Name        string    `gorm:"size:128;not null;uniqueIndex:idx_datasource_group_parent_name" json:"name"`
	Description string    `gorm:"size:500" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type DatasourceGroupMember struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	GroupID      uint      `gorm:"not null;uniqueIndex:idx_datasource_group_member;index" json:"group_id"`
	DatasourceID uint      `gorm:"not null;uniqueIndex:idx_datasource_group_member;index" json:"datasource_id"`
	CreatedAt    time.Time `json:"created_at"`
}
