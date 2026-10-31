package models

import "time"

type AlertRuleGroup struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:128;not null;uniqueIndex" json:"name"`
	Description string    `gorm:"size:500" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AlertRule struct {
	ID                        uint       `gorm:"primaryKey" json:"id"`
	GroupID                   uint       `gorm:"not null;default:0;index;uniqueIndex:idx_alert_rule_group_name" json:"group_id"`
	Name                      string     `gorm:"size:128;not null;uniqueIndex:idx_alert_rule_group_name" json:"name"`
	Description               string     `gorm:"size:500" json:"description"`
	Expression                string     `gorm:"type:text;not null" json:"expression"`
	ForSeconds                int        `gorm:"not null;default:0" json:"for_seconds"`
	RecoveryWaitSeconds       int        `gorm:"not null;default:0" json:"recovery_wait_seconds"`
	NoDataPolicy              string     `gorm:"size:20;not null;default:keep" json:"no_data_policy"`
	EvaluationIntervalSeconds int        `gorm:"not null;default:60" json:"evaluation_interval_seconds"`
	Severity                  string     `gorm:"size:20;not null;default:P1" json:"severity"`
	Labels                    string     `gorm:"type:text" json:"-"`
	Annotations               string     `gorm:"type:text" json:"-"`
	FaultCenterID             uint       `gorm:"not null;index" json:"fault_center_id"`
	Status                    int        `gorm:"not null;default:1;index" json:"status"`
	LastEvaluatedAt           *time.Time `gorm:"index" json:"last_evaluated_at"`
	LastStatus                string     `gorm:"size:20" json:"last_status"`
	LastError                 string     `gorm:"size:1000" json:"last_error"`
	CreatedAt                 time.Time  `json:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`
}

type AlertRuleBinding struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	RuleID           uint       `gorm:"not null;uniqueIndex:idx_alert_rule_datasource;index" json:"rule_id"`
	DatasourceID     uint       `gorm:"not null;uniqueIndex:idx_alert_rule_datasource;index" json:"datasource_id"`
	NextEvaluationAt *time.Time `gorm:"index" json:"next_evaluation_at"`
	LeaseUntil       *time.Time `gorm:"index" json:"-"`
	LastEvaluatedAt  *time.Time `gorm:"index" json:"last_evaluated_at"`
	LastStatus       string     `gorm:"size:20" json:"last_status"`
	LastError        string     `gorm:"size:1000" json:"last_error"`
	LastResultCount  int        `gorm:"not null;default:0" json:"last_result_count"`
	LastDurationMs   int64      `gorm:"not null;default:0" json:"last_duration_ms"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type AlertRuleDatasourceGroup struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	RuleID            uint      `gorm:"not null;uniqueIndex:idx_alert_rule_datasource_group;index" json:"rule_id"`
	DatasourceGroupID uint      `gorm:"not null;uniqueIndex:idx_alert_rule_datasource_group;index" json:"datasource_group_id"`
	CreatedAt         time.Time `json:"created_at"`
}

// AlertRuleCondition defines one severity threshold branch for a rule's
// PromQL result. Evaluation uses the parent rule's shared duration.
type AlertRuleCondition struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	RuleID    uint      `gorm:"not null;index" json:"rule_id"`
	Severity  string    `gorm:"size:20;not null" json:"severity"`
	Operator  string    `gorm:"size:16;not null" json:"operator"`
	Threshold float64   `gorm:"not null;default:0" json:"threshold"`
	Sort      int       `gorm:"not null;default:0" json:"sort"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AlertEvaluationRecord struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	RuleID       uint      `gorm:"not null;index" json:"rule_id"`
	DatasourceID uint      `gorm:"not null;index" json:"datasource_id"`
	Status       string    `gorm:"size:20;not null;index" json:"status"`
	ResultCount  int       `gorm:"not null;default:0" json:"result_count"`
	DurationMs   int64     `gorm:"not null;default:0" json:"duration_ms"`
	Message      string    `gorm:"size:2000" json:"message"`
	EvaluatedAt  time.Time `gorm:"not null;index" json:"evaluated_at"`
	CreatedAt    time.Time `json:"created_at"`
}
