package models

import "time"

// CloudMonitorWorkspace stores an Aliyun Cloud Monitor 2.0 workspace and its
// encrypted access credentials. Credentials are deliberately excluded from JSON.
type CloudMonitorWorkspace struct {
	Sort                  int       `gorm:"not null;default:0;index" json:"sort"`
	ID                    uint      `gorm:"primaryKey" json:"id"`
	Name                  string    `gorm:"size:128;not null;default:''" json:"name"`
	Workspace             string    `gorm:"size:128;not null;uniqueIndex:idx_cloud_monitor_region_workspace" json:"workspace"`
	Region                string    `gorm:"size:50;not null;uniqueIndex:idx_cloud_monitor_region_workspace" json:"region"`
	Endpoint              string    `gorm:"size:255;not null" json:"-"`
	AccessKeyIDCipher     string    `gorm:"type:text;not null" json:"-"`
	AccessKeySecretCipher string    `gorm:"type:text;not null" json:"-"`
	Description           string    `gorm:"size:500" json:"description"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}
