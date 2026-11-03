package models

import "time"

// AlertAIAnalysisRule contains no notification settings. Execution always uses its creator's permissions.
type AlertAIAnalysisRule struct {
	ModelName                string              `gorm:"-" json:"model_name"`
	LatestAnalysisAt         *time.Time          `gorm:"-" json:"latest_analysis_at"`
	ID                       uint                `gorm:"primaryKey" json:"id"`
	FaultCenterID            uint                `gorm:"not null;uniqueIndex:idx_ai_rule_name" json:"fault_center_id"`
	Name                     string              `gorm:"size:128;not null;uniqueIndex:idx_ai_rule_name" json:"name"`
	MatchersJSON             string              `gorm:"type:text" json:"-"`
	Matchers                 []AlertLabelMatcher `gorm:"-" json:"matchers"`
	ModelID                  uint                `json:"model_id"`
	Priority                 int                 `json:"priority"`
	Status                   int                 `gorm:"not null;index" json:"status"`
	Version                  uint                `json:"version"`
	ExecutionUserID          uint                `json:"execution_user_id"`
	AllowedDatasourceIDsJSON string              `gorm:"type:text" json:"-"`
	AllowedDatasourceIDs     []uint              `gorm:"-" json:"allowed_datasource_ids"`
	CreatedAt                time.Time           `json:"created_at"`
	UpdatedAt                time.Time           `json:"updated_at"`
}

// EpisodeKey uses the persisted firing timestamp.
// Recovering/firing oscillations and rule edits do not change this binding.
type AlertAIAnalysisJob struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	FaultCenterID     uint       `gorm:"not null;index" json:"fault_center_id"`
	AlertID           uint       `gorm:"not null;uniqueIndex:idx_ai_alert_episode" json:"alert_id"`
	EpisodeKey        string     `gorm:"size:80;not null;uniqueIndex:idx_ai_alert_episode" json:"episode_key"`
	RuleID            uint       `gorm:"index" json:"rule_id"`
	RuleName          string     `json:"rule_name"`
	RuleVersion       uint       `json:"rule_version"`
	ExecutionUserID   uint       `json:"-"`
	ModelID           uint       `json:"model_id"`
	ModelName         string     `json:"model_name"`
	ModelFingerprint  string     `json:"-"`
	DatasourceIDsJSON string     `gorm:"type:text" json:"-"`
	InputJSON         string     `gorm:"type:text" json:"-"`
	Status            string     `gorm:"size:32;index" json:"status"`
	ErrorMessage      string     `json:"error_message"`
	ResultJSON        string     `gorm:"type:text" json:"-"`
	EvidenceJSON      string     `gorm:"type:text" json:"-"`
	TraceJSON         string     `gorm:"type:text" json:"-"`
	LeaseToken        string     `json:"-"`
	LeaseUntil        *time.Time `json:"-"`
	StartedAt         *time.Time `json:"started_at"`
	FinishedAt        *time.Time `json:"finished_at"`
	CreatedAt         time.Time  `gorm:"index" json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}
