package app

import (
	"fmt"
	"guanlan/internal/config"
	"guanlan/internal/database"
	"guanlan/internal/handlers"
	"guanlan/internal/routes"
	"guanlan/pkg/logger"
	"io/fs"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// App holds the initialized application and its runtime dependencies.
type App struct {
	config config.Config
	router *gin.Engine
}

func newApplication(cfg config.Config, router *gin.Engine) *App {
	return &App{config: cfg, router: router}
}

// The database dependency ensures handlers have a validated connection
// before the router is made available.
func newRouter(_ *gorm.DB, assets fs.FS) (*gin.Engine, error) {
	// Vite serves the UI during development; no frontend build is required.
	if os.Getenv("APP_WEB_DEV") == "1" {
		gin.SetMode(gin.ReleaseMode)
		return routes.SetupRouter(), nil
	}
	webFiles, err := fs.Sub(assets, "web/dist")
	if err != nil {
		return nil, fmt.Errorf("load embedded web files: %w", err)
	}
	gin.SetMode(gin.ReleaseMode)
	router := routes.SetupRouter()
	if err := routes.RegisterStatic(router, webFiles); err != nil {
		return nil, fmt.Errorf("register embedded web files: %w", err)
	}
	return router, nil
}

func (app *App) Run() error {
	handlers.StartScheduler()
	logger.Info("server starting", "addr", app.config.ServerPort)
	if err := app.router.Run(app.config.ServerPort); err != nil {
		return fmt.Errorf("start server: %w", err)
	}
	return nil
}

// Make logger initialization an explicit dependency of database startup.
func newDatabase(cfg config.Config, _ *slog.Logger) (*gorm.DB, func(), error) {
	return database.New(cfg)
}

func newLogger(cfg config.Config) (*slog.Logger, func(), error) { return logger.New(cfg.Log) }
