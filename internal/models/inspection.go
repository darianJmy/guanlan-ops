package models

import "time"

type InspectionRuleGroup struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:128;not null;uniqueIndex" json:"name"`
	Description string    `gorm:"size:500" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type InspectionItem struct {
	GroupID              uint                 `gorm:"not null;index" json:"group_id"`
	Group                *InspectionRuleGroup `gorm:"foreignKey:GroupID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-"`
	ID                   uint                 `gorm:"primaryKey" json:"id"`
	Name                 string               `gorm:"size:100;not null" json:"name"`
	Type                 string               `gorm:"size:50;not null" json:"type"`     // http, icmp, tcp, certificate_expiry, script
	Target               string               `gorm:"size:500;not null" json:"target"`  // 便于列表展示的 URL 或 Host
	Config               string               `gorm:"type:text" json:"config"`          // JSON: type-specific config
	Labels               string               `gorm:"type:text" json:"external_labels"` // JSON: user-defined labels; exposed as external_labels in alerts
	ExpectedResult       string               `gorm:"size:200" json:"expected_result"`
	IntervalSeconds      int                  `gorm:"not null;default:300" json:"interval_seconds"`
	FailureThreshold     int                  `gorm:"not null;default:3" json:"failure_threshold"`
	ConsecutiveFailures  int                  `gorm:"not null;default:0" json:"consecutive_failures"`
	RecoveryThreshold    int                  `gorm:"not null;default:3" json:"recovery_threshold"`
	ConsecutiveSuccesses int                  `gorm:"not null;default:0" json:"consecutive_successes"`
	Severity             string               `gorm:"size:20;not null;default:P2;index" json:"severity"`
	FaultCenterID        uint                 `gorm:"not null;default:0;index" json:"fault_center_id"`
	AlertActive          bool                 `gorm:"not null;default:false" json:"alert_active"`
	LastRunAt            *time.Time           `json:"last_run_at"`
	LastStatus           string               `gorm:"size:20" json:"last_status"`
	Status               int                  `gorm:"default:1" json:"status"` // 1=启用 0=禁用
	CreatedAt            time.Time            `json:"created_at"`
	UpdatedAt            time.Time            `json:"updated_at"`
}

type InspectionRecord struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ItemID     uint      `gorm:"index;not null" json:"item_id"`
	ItemName   string    `gorm:"-" json:"item_name"` // 填充用
	NodeType   string    `gorm:"size:20;not null;default:direct;index" json:"node_type"`
	NodeID     uint      `gorm:"not null;default:0;index" json:"node_id"`
	NodeName   string    `gorm:"size:100" json:"node_name"`
	Status     string    `gorm:"size:20;not null" json:"status"` // success, fail, error
	Result     string    `gorm:"type:text" json:"result"`        // JSON 详情
	DurationMs int64     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

type InspectionNodeState struct {
	ID                   uint       `gorm:"primaryKey" json:"id"`
	ItemID               uint       `gorm:"not null;uniqueIndex:idx_inspection_node_state" json:"item_id"`
	NodeKey              string     `gorm:"size:64;not null;uniqueIndex:idx_inspection_node_state" json:"node_key"`
	NodeType             string     `gorm:"size:20;not null" json:"node_type"`
	NodeID               uint       `gorm:"not null;default:0" json:"node_id"`
	NodeName             string     `gorm:"size:100" json:"node_name"`
	ConsecutiveFailures  int        `gorm:"not null;default:0" json:"consecutive_failures"`
	ConsecutiveSuccesses int        `gorm:"not null;default:0" json:"consecutive_successes"`
	AlertActive          bool       `gorm:"not null;default:false" json:"alert_active"`
	LastRunAt            *time.Time `json:"last_run_at"`
	LastStatus           string     `gorm:"size:20" json:"last_status"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// InspectionTrendBucket stores compact five-minute aggregates for charts.
// Raw InspectionRecord rows are intentionally limited to the latest 12 per node.
type InspectionTrendBucket struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	ItemID         uint      `gorm:"not null;uniqueIndex:idx_inspection_trend_bucket" json:"item_id"`
	NodeType       string    `gorm:"size:20;not null;uniqueIndex:idx_inspection_trend_bucket" json:"node_type"`
	NodeID         uint      `gorm:"not null;default:0;uniqueIndex:idx_inspection_trend_bucket" json:"node_id"`
	NodeName       string    `gorm:"size:100" json:"node_name"`
	BucketAt       time.Time `gorm:"not null;uniqueIndex:idx_inspection_trend_bucket;index" json:"bucket_at"`
	TotalCount     int       `gorm:"not null;default:0" json:"total_count"`
	SuccessCount   int       `gorm:"not null;default:0" json:"success_count"`
	FailureCount   int       `gorm:"not null;default:0" json:"failure_count"`
	DurationSumMs  int64     `gorm:"not null;default:0" json:"duration_sum_ms"`
	DurationMinMs  int64     `gorm:"not null;default:0" json:"duration_min_ms"`
	DurationMaxMs  int64     `gorm:"not null;default:0" json:"duration_max_ms"`
	Status2xxCount int       `gorm:"not null;default:0" json:"status_2xx_count"`
	Status3xxCount int       `gorm:"not null;default:0" json:"status_3xx_count"`
	Status4xxCount int       `gorm:"not null;default:0" json:"status_4xx_count"`
	Status5xxCount int       `gorm:"not null;default:0" json:"status_5xx_count"`
	StatusOther    int       `gorm:"not null;default:0" json:"status_other_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
