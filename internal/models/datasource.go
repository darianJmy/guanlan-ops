package models

import "time"

type Datasource struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"size:100;not null" json:"name"`
	Category     string    `gorm:"size:50;default:observability;index" json:"category"` // Category is assigned by the datasource adapter registry.
	Type         string    `gorm:"size:50;not null;index" json:"type"`                  // Product type; supported types/capabilities are defined by the adapter registry
	URL          string    `gorm:"size:500;not null" json:"url"`                        // URL, endpoint, host, or DSN
	AuthType     string    `gorm:"size:20;default:none" json:"auth_type"`               // Auth methods are validated by the datasource adapter.
	AuthConfig   string    `gorm:"type:text" json:"auth_config"`                        // JSON: {"username":"","password":"","token":""}
	Labels       string    `gorm:"type:text" json:"labels"`                             // JSON object with user-defined labels
	Region       string    `gorm:"size:50;index" json:"region"`                         // cn, eu, us, etc.
	Env          string    `gorm:"size:50;index" json:"env"`                            // prod, test, staging
	Domain       string    `gorm:"size:100;index" json:"domain"`                        // iot, message, user, etc.
	Capabilities string    `gorm:"size:255" json:"capabilities"`                        // Server-owned comma-separated capabilities: metrics, logs, traces, sql, k8s, redis, kafka
	Priority     int       `gorm:"default:100" json:"priority"`                         // lower value means higher priority
	Status       int       `gorm:"default:1" json:"status"`                             // 1=启用 0=禁用
	TunnelID     uint      `gorm:"default:0" json:"tunnel_id"`                          // 隧道节点 ID, 0=直连
	TunnelName   string    `gorm:"-" json:"tunnel_name"`                                // 填充用
	Config       string    `gorm:"type:text" json:"config"`                             // 类型差异化配置 JSON
	Remark       string    `gorm:"size:500" json:"remark"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
