package models

import "time"

// ServicePortalResource is the durable service definition. Runtime fields are
// materialized by the service-portal process and are not its persistence store.
type ServicePortalResource struct {
	ID        string `gorm:"size:128;primaryKey" json:"id"`
	Name      string `gorm:"size:128;not null" json:"name"`
	Type      string `gorm:"size:16;not null;default:kubernetes" json:"type"`
	TargetURL string `gorm:"size:2048;not null;default:''" json:"targetURL"`
	// DatasourceID references the shared access-center Datasource (type=k8s).
	// ClusterID is kept as a wire alias for the existing portal API and is
	// populated from DatasourceID when resources are serialized.
	DatasourceID uint   `gorm:"not null;index" json:"datasource_id"`
	ClusterID    string `gorm:"-" json:"clusterId,omitempty"`
	Namespace    string `gorm:"size:63;not null" json:"namespace"`
	Service      string `gorm:"size:253;not null" json:"service"`
	Port         string `gorm:"size:32;not null" json:"port"`
	PublicPort   int    `gorm:"not null" json:"publicPort"`
	Path         string `gorm:"size:1000;not null" json:"path"`
	// Desired persists the administrator's enabled/disabled intent across
	// runtime restarts. Actual listener status belongs to the portal process.
	Desired   bool      `gorm:"not null;default:false" json:"desired"`
	OnDemand  bool      `gorm:"not null;default:false" json:"onDemand"`
	Status    string    `gorm:"size:32" json:"status,omitempty"`
	LastError string    `gorm:"size:500" json:"lastError,omitempty"`
	Recovered string    `gorm:"size:64" json:"recovered,omitempty"`
	FolderID  uint      `gorm:"not null;default:0;index" json:"folder_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ServicePortalGrant is retained for ungrouped services. Grouped services use
// DashboardFolderRolePermission and do not need a per-user row here.
type ServicePortalGrant struct {
	UserID     string    `gorm:"size:128;primaryKey" json:"userId"`
	ResourceID string    `gorm:"size:128;primaryKey;index" json:"resourceId"`
	CreatedAt  time.Time `json:"created_at"`
}
