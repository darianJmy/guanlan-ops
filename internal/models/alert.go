package models

import "time"

type FaultCenter struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	Name            string    `gorm:"size:128;not null;uniqueIndex" json:"name"`
	Description     string    `gorm:"size:500" json:"description"`
	IsDefault       bool      `gorm:"not null;default:false" json:"is_default"`
	Status          int       `gorm:"not null;default:1;index" json:"status"`
	TaskCount       int64     `gorm:"-" json:"task_count"`
	PendingCount    int64     `gorm:"-" json:"pending_count"`
	FiringCount     int64     `gorm:"-" json:"firing_count"`
	RecoveringCount int64     `gorm:"-" json:"recovering_count"`
	ResolvedCount   int64     `gorm:"-" json:"resolved_count"`
	SilencedCount   int64     `gorm:"-" json:"silenced_count"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type AlertInstance struct {
	MatchingLabels        map[string]string `gorm:"-" json:"matching_labels"`
	ID                    uint              `gorm:"primaryKey" json:"id"`
	FaultCenterID         uint              `gorm:"not null;default:0;index" json:"fault_center_id"`
	Source                string            `gorm:"size:50;not null;index:idx_alert_source" json:"source"`
	SourceID              uint              `gorm:"not null;index:idx_alert_source" json:"source_id"`
	DatasourceID          uint              `gorm:"not null;default:0;index" json:"datasource_id"`
	Fingerprint           string            `gorm:"size:64;index" json:"fingerprint"`
	Title                 string            `gorm:"size:255;not null" json:"title"`
	Target                string            `gorm:"size:500" json:"target"`
	Severity              string            `gorm:"size:20;not null;default:warning;index" json:"severity"`
	Status                string            `gorm:"size:20;not null;index" json:"status"`
	Labels                string            `gorm:"type:text" json:"labels"`
	FailureCount          int               `gorm:"not null;default:0" json:"failure_count"`
	FailureThreshold      int               `gorm:"not null;default:1" json:"failure_threshold"`
	LastProbeStatus       string            `gorm:"size:20" json:"last_probe_status"`
	LastResult            string            `gorm:"type:text" json:"last_result"`
	NotificationChannelID uint              `gorm:"index" json:"notification_channel_id"`
	StartedAt             time.Time         `gorm:"not null;index" json:"started_at"`
	FiredAt               *time.Time        `json:"fired_at"`
	ResolvedAt            *time.Time        `gorm:"index" json:"resolved_at"`
	SilencedUntil         *time.Time        `gorm:"index" json:"silenced_until"`
	SilenceReason         string            `gorm:"size:500" json:"silence_reason"`
	SilenceRuleID         *uint             `gorm:"index" json:"silence_rule_id"`
	LastEventAt           time.Time         `gorm:"not null;index" json:"last_event_at"`
	StateChangedAt        time.Time         `gorm:"index" json:"state_changed_at"`
	LastNotifiedAt        *time.Time        `gorm:"index" json:"last_notified_at"`
	NotificationStatus    string            `gorm:"size:32;index" json:"notification_status"`
	NotificationReason    string            `gorm:"size:255" json:"notification_reason"`
	NotificationGroupKey  string            `gorm:"size:500;index" json:"notification_group_key"`
	NextNotificationAt    *time.Time        `gorm:"index" json:"next_notification_at"`
	RuleDescription       string            `gorm:"-" json:"rule_description,omitempty"`
	RuleGroupID           uint              `gorm:"-" json:"rule_group_id,omitempty"`
	RuleGroupName         string            `gorm:"-" json:"rule_group_name,omitempty"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
}

// AlertEvent is the immutable timeline of an alert instance. AlertInstance
// stores the latest snapshot; this model explains how that snapshot was reached.
type AlertEvent struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	AlertID          uint      `gorm:"not null;index" json:"alert_id"`
	EventType        string    `gorm:"size:50;not null;index" json:"event_type"`
	FromStatus       string    `gorm:"size:20" json:"from_status"`
	ToStatus         string    `gorm:"size:20" json:"to_status"`
	FromSeverity     string    `gorm:"size:20" json:"from_severity"`
	ToSeverity       string    `gorm:"size:20" json:"to_severity"`
	Reason           string    `gorm:"size:100" json:"reason"`
	Value            string    `gorm:"size:255" json:"value"`
	Labels           string    `gorm:"type:text" json:"labels"`
	EvaluationStatus string    `gorm:"size:20;index" json:"evaluation_status"`
	CreatedAt        time.Time `gorm:"not null;index" json:"created_at"`
}

type AlertSilenceRule struct {
	ID            uint                `gorm:"primaryKey" json:"id"`
	FaultCenterID uint                `gorm:"not null;index" json:"fault_center_id"`
	Name          string              `gorm:"size:128;not null" json:"name"`
	MatchersJSON  string              `gorm:"column:matchers;type:text" json:"-"`
	Matchers      []AlertLabelMatcher `gorm:"-" json:"matchers"`
	SilenceType   string              `gorm:"size:20;not null;default:once" json:"silence_type"`
	WeekdaysJSON  string              `gorm:"column:weekdays;size:32" json:"-"`
	Weekdays      []int               `gorm:"-" json:"weekdays"`
	StartTime     string              `gorm:"size:5" json:"start_time"`
	EndTime       string              `gorm:"size:5" json:"end_time"`
	StartAt       *time.Time          `gorm:"index" json:"start_at"`
	EndAt         *time.Time          `gorm:"index" json:"end_at"`
	Status        int                 `gorm:"not null;default:1;index" json:"status"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
}

type AlertInhibitionRule struct {
	ID                 uint                 `gorm:"primaryKey" json:"id"`
	FaultCenterID      uint                 `gorm:"not null;index" json:"fault_center_id"`
	Name               string               `gorm:"size:128;not null" json:"name"`
	SourceMatchersJSON string               `gorm:"column:source_matchers;type:text" json:"-"`
	TargetMatchersJSON string               `gorm:"column:target_matchers;type:text" json:"-"`
	EqualLabelsJSON    string               `gorm:"column:equal_labels_v2;type:text" json:"-"`
	SourceMatchers     []AlertLabelMatcher  `gorm:"-" json:"source_matchers"`
	TargetMatchers     []AlertLabelMatcher  `gorm:"-" json:"target_matchers"`
	EqualLabels        []AlertLabelSelector `gorm:"-" json:"equal_labels"`
	Status             int                  `gorm:"not null;default:1;index" json:"status"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
}

type AlertNotificationConfig struct {
	ID                    uint      `gorm:"primaryKey" json:"id"`
	FaultCenterID         uint      `gorm:"not null;uniqueIndex" json:"fault_center_id"`
	DefaultChannelIDs     string    `gorm:"size:500" json:"-"`
	RepeatIntervalSeconds int       `gorm:"not null;default:3600" json:"repeat_interval_seconds"`
	RecoveryNotification  bool      `gorm:"not null" json:"recovery_notification"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type AlertNotificationRoute struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	FaultCenterID  uint      `gorm:"not null;index" json:"fault_center_id"`
	LabelNamespace string    `gorm:"size:32;not null" json:"label_namespace"`
	LabelKey       string    `gorm:"size:128;not null" json:"label_key"`
	LabelValue     string    `gorm:"size:255;not null" json:"label_value"`
	ChannelIDs     string    `gorm:"size:500;not null" json:"-"`
	Status         int       `gorm:"not null;default:1;index" json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type AlertLabelMatcher struct {
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
	Value     string `json:"value"`
}

type AlertLabelSelector struct {
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
}

// AlertGroupingRule selects one alert population and aggregates matching
// alerts into notification groups. Rules are evaluated by Priority then ID;
// the first enabled matching rule owns the alert.
type AlertGroupingRule struct {
	ID              uint                 `gorm:"primaryKey" json:"id"`
	FaultCenterID   uint                 `gorm:"not null;index;uniqueIndex:idx_alert_grouping_rule_name" json:"fault_center_id"`
	Name            string               `gorm:"size:128;not null;uniqueIndex:idx_alert_grouping_rule_name" json:"name"`
	MatchersJSON    string               `gorm:"column:matchers;type:text" json:"-"`
	Matchers        []AlertLabelMatcher  `gorm:"-" json:"matchers"`
	GroupByJSON     string               `gorm:"column:group_by;type:text" json:"-"`
	GroupBy         []AlertLabelSelector `gorm:"-" json:"group_by"`
	WaitSeconds     int                  `gorm:"not null" json:"wait_seconds"`
	IntervalSeconds int                  `gorm:"not null;default:300" json:"interval_seconds"`
	Priority        int                  `gorm:"not null;default:100;index" json:"priority"`
	Status          int                  `gorm:"not null;index" json:"status"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
}

type AlertGroupState struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	FaultCenterID      uint       `gorm:"not null;uniqueIndex:idx_alert_group_state" json:"fault_center_id"`
	GroupingRuleID     uint       `gorm:"not null;default:0;index" json:"grouping_rule_id"`
	GroupKey           string     `gorm:"size:500;not null;uniqueIndex:idx_alert_group_state" json:"group_key"`
	FirstSeenAt        time.Time  `gorm:"index" json:"first_seen_at"`
	LastNotifiedAt     *time.Time `gorm:"index" json:"last_notified_at"`
	NextNotificationAt *time.Time `gorm:"index" json:"next_notification_at"`
	Pending            bool       `gorm:"not null;default:false;index" json:"pending"`
	LeaseUntil         *time.Time `gorm:"index" json:"-"`
	ClosedAt           *time.Time `gorm:"index" json:"closed_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}
