package database

import (
	"fmt"
	"log/slog"

	"guanlan/internal/config"
	"guanlan/internal/models"
	sqldb "guanlan/pkg/database"
	"guanlan/pkg/logger"

	"gorm.io/gorm"
)

var DB *gorm.DB

const (
	CurrentSchemaVersion        = "2026112702"
	serviceDashboardMenuVersion = "2026112701"
	previousSchemaVersion       = "2026110301"
)

// New opens SQLite and atomically initializes a fresh database on first startup.
// DB is the shared connection used by the HTTP and tool handlers.
func New(cfg config.Config) (*gorm.DB, func(), error) {
	db, err := openDatabaseDSN(cfg.DBDSN)
	if err != nil {
		return nil, nil, fmt.Errorf("connect database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("access database pool: %w", err)
	}
	cleanup := func() { _ = sqlDB.Close() }
	if err := prepareDatabase(db); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("initialize database: %w", err)
	}
	DB = db
	logger.Info("database ready", "schema_version", CurrentSchemaVersion)
	return db, cleanup, nil
}

func prepareDatabase(db *gorm.DB) error {
	if !db.Migrator().HasTable("schema_migrations") {
		return sqldb.Initialize(db, CurrentSchemaVersion, createCurrentSchema, seedInitialData)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := sqldb.RequireSchemaVersion(tx, CurrentSchemaVersion); err == nil {
			return nil
		}
		if err := sqldb.RequireSchemaVersion(tx, serviceDashboardMenuVersion); err == nil {
			if err := mergeServiceDashboardMenu(tx); err != nil {
				return err
			}
		} else {
			if err := sqldb.RequireSchemaVersion(tx, previousSchemaVersion); err != nil {
				return err
			}
			if err := createServiceDashboardSchema(tx); err != nil {
				return err
			}
			if err := seedServiceDashboard(tx); err != nil {
				return err
			}
		}
		return sqldb.RecordSchemaVersion(tx, CurrentSchemaVersion)
	})
}

func openDatabaseDSN(dsn string) (*gorm.DB, error) {
	return sqldb.OpenSQLite(dsn, slog.Default())
}

func createCurrentSchema(db *gorm.DB) error {
	if err := sqldb.AutoMigrate(db,
		&models.User{},
		&models.Role{},
		&models.Permission{},
		&models.Menu{},
		&models.Datasource{},
		&models.DatasourceGroup{},
		&models.DatasourceGroupMember{},
		&models.Dashboard{},
		&models.SLSDashboardProject{},
		&models.CloudMonitorWorkspace{},
		&models.InspectionRuleGroup{},
		&models.InspectionItem{},
		&models.InspectionRecord{},
		&models.InspectionNodeState{},
		&models.InspectionTrendBucket{},
		&models.Script{},
		&models.ScriptExecution{},
		&models.Workflow{},
		&models.WorkflowExecution{},
		&models.AutomationTask{},
		&models.AutomationExecution{},
		&models.GitCredential{},
		&models.GitRepository{},
		&models.SystemSetting{},
		&models.AuthSetting{},
		&models.APIAccessToken{},
		&models.MCPClient{},
		&models.Tunnel{},
		&models.AIModel{},
		&models.ChatConversation{},
		&models.ChatMessage{},
		&models.ChatMemory{},
		&models.AgentConnection{},
		&models.CertificateExecutor{},
		&models.NotificationChannel{},
		&models.NotificationTemplate{},
		&models.NotificationRecord{},
		&models.PromQueryHistory{},
		&models.FaultCenter{},
		&models.AlertInstance{},
		&models.AlertEvent{},
		&models.AlertSilenceRule{},
		&models.AlertInhibitionRule{},
		&models.AlertNotificationConfig{},
		&models.AlertNotificationRoute{},
		&models.AlertGroupingRule{},
		&models.AlertGroupState{},
		&models.AlertAIAnalysisRule{},
		&models.AlertAIAnalysisJob{},
		&models.AlertActionRule{},
		&models.AlertActionExecution{},
		&models.AlertRuleGroup{},
		&models.AlertRule{},
		&models.AlertRuleDatasourceGroup{},
		&models.AlertRuleCondition{},
		&models.AlertRuleBinding{},
		&models.AlertEvaluationRecord{},
		&models.AuditLog{},
	); err != nil {
		return fmt.Errorf("create database schema: %w", err)
	}
	if err := createServiceDashboardSchema(db); err != nil {
		return err
	}
	return db.Exec("CREATE UNIQUE INDEX idx_ai_running_center ON alert_ai_analysis_jobs(fault_center_id) WHERE status = 'running'").Error
}
