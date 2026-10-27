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

const CurrentSchemaVersion = "2026102601"

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
	return db.Transaction(func(tx *gorm.DB) error {
		if err := sqldb.RequireSchemaVersion(tx, "2026102201"); err == nil && sqldb.RequireSchemaVersion(tx, CurrentSchemaVersion) != nil {
			if err := tx.Migrator().AddColumn(&models.DatasourceGroup{}, "ParentID"); err != nil {
				return err
			}
			if err := tx.Migrator().DropIndex(&models.DatasourceGroup{}, "idx_datasource_groups_name"); err != nil {
				return err
			}
			if err := tx.Migrator().CreateIndex(&models.DatasourceGroup{}, "idx_datasource_group_parent_name"); err != nil {
				return err
			}
			if err := tx.Migrator().CreateIndex(&models.DatasourceGroup{}, "ParentID"); err != nil {
				return err
			}
			if err := sqldb.RecordSchemaVersion(tx, CurrentSchemaVersion); err != nil {
				return err
			}
		}
		return sqldb.Initialize(tx, CurrentSchemaVersion, createCurrentSchema, seedInitialData)
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
		&models.Certificate{},
		&models.CertificateVersion{},
		&models.CertificateDomain{},
		&models.AgentConnection{},
		&models.CertificateDeployScript{},
		&models.CertificateExecutor{},
		&models.CertificateDeploymentRecord{},
		&models.CertificateRotationRecord{},
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
		&models.AlertActionRule{},
		&models.AlertActionExecution{},
		&models.AlertRuleGroup{},
		&models.AlertRule{},
		&models.AlertRuleDatasource{},
		&models.AlertRuleDatasourceGroup{},
		&models.AlertRuleCondition{},
		&models.AlertRuleBinding{},
		&models.AlertEvaluationRecord{},
		&models.AuditLog{},
	); err != nil {
		return fmt.Errorf("create database schema: %w", err)
	}
	return nil
}
