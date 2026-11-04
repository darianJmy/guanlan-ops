package app

import (
	"context"
	"errors"
	"fmt"
	"guanlan/internal/config"
	"guanlan/internal/database"
	"guanlan/internal/handlers"
	"guanlan/internal/routes"
	"guanlan/pkg/logger"
	"guanlan/pkg/tracing"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Addr: app.config.ServerPort, Handler: app.router, ReadHeaderTimeout: 10 * time.Second}
	done := make(chan struct{})
	finished := make(chan struct{})
	defer func() { close(done); <-finished }()
	go func() {
		defer close(finished)
		select {
		case <-ctx.Done():
		case <-done:
			return
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("HTTP shutdown failed", "error", err)
			_ = server.Close()
		}
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("start server: %w", err)
	}
	return nil
}

// Make logger initialization an explicit dependency of database startup.
func newDatabase(cfg config.Config, _ *slog.Logger) (*gorm.DB, func(), error) {
	return database.New(cfg)
}

func newLogger(cfg config.Config) (*slog.Logger, func(), error) {
	log, cleanup, err := logger.New(cfg.Log)
	if err != nil {
		return nil, nil, err
	}
	shutdown, err := tracing.Init(context.Background())
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return log, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdown(ctx); err != nil {
			logger.Error("trace export shutdown failed", "error", err)
		}
		cleanup()
	}, nil
}
