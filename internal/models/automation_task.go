package models

import "time"

// AutomationTask binds one reusable script to one execution node.
type AutomationTask struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	Name             string    `gorm:"size:128;not null;uniqueIndex" json:"name"`
	Description      string    `gorm:"size:500" json:"description"`
	ScriptID         uint      `gorm:"not null;index" json:"script_id"`
	SourceType       string    `gorm:"size:20;not null;default:script;index" json:"source_type"` // script, repository
	RepositoryID     uint      `gorm:"index" json:"repository_id"`
	GitRef           string    `gorm:"size:255" json:"git_ref"`
	WorkingDirectory string    `gorm:"size:500" json:"working_directory"`
	Command          string    `gorm:"type:text" json:"command"`
	NodeType         string    `gorm:"size:20;not null;default:direct" json:"node_type"`
	NodeID           uint      `gorm:"index" json:"node_id"`
	TimeoutSeconds   int       `gorm:"not null;default:300" json:"timeout_seconds"`
	MaxRetries       int       `gorm:"not null;default:0" json:"max_retries"`
	Status           int       `gorm:"not null;default:1;index" json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// AutomationExecution records every manual or business-triggered task run.
type AutomationExecution struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	TaskID         uint      `gorm:"not null;index" json:"task_id"`
	TriggerType    string    `gorm:"size:32;not null;index" json:"trigger_type"`
	TriggerID      uint      `gorm:"index" json:"trigger_id"`
	Status         string    `gorm:"size:20;not null;index" json:"status"`
	ExitCode       int       `json:"exit_code"`
	Input          string    `gorm:"type:longtext" json:"input"`
	Output         string    `gorm:"type:longtext" json:"output"`
	Error          string    `gorm:"type:text" json:"error"`
	DurationMs     int64     `json:"duration_ms"`
	Revision       string    `gorm:"size:64;index" json:"revision"`
	ArtifactSHA256 string    `gorm:"size:64;index" json:"artifact_sha256"`
	CreatedAt      time.Time `json:"created_at"`
}
