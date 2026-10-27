package app

import (
	"guanlan/internal/config"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"gorm.io/gorm"
)

func TestInitializeApplicationRejectsInvalidDatabase(t *testing.T) {
	t.Chdir(t.TempDir())
	previous := config.AppConfig
	t.Cleanup(func() { config.AppConfig = previous })
	t.Setenv("APP_DATABASE_DSN", "mysql://invalid")
	setArgs(t)
	app, cleanup, err := InitializeApp(fstest.MapFS{})
	if err == nil || !strings.Contains(err.Error(), "SQLite file path") {
		t.Fatalf("expected DSN error, got %v", err)
	}
	if app != nil || cleanup != nil {
		t.Fatal("failed initialization must not return an application or cleanup")
	}
}

func TestInitializeApplicationRejectsMissingConfig(t *testing.T) {
	previous := config.AppConfig
	setArgs(t, "--conf", filepath.Join(t.TempDir(), "missing.yaml"))
	app, cleanup, err := InitializeApp(fstest.MapFS{})
	if err == nil || !strings.Contains(err.Error(), "read configuration") {
		t.Fatalf("expected config error before database initialization, got %v", err)
	}
	if app != nil || cleanup != nil || config.AppConfig != previous {
		t.Fatal("failed configuration load must not publish configuration or create an application")
	}
}

func TestNewRouterRegistersEmbeddedAssets(t *testing.T) {
	assets := fstest.MapFS{"web/dist/index.html": {Data: []byte("<html>app</html>")}}
	router, err := newRouter(&gorm.DB{}, assets)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK || response.Body.String() != "<html>app</html>" {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
}

func TestNewRouterRejectsMissingAssets(t *testing.T) {
	router, err := newRouter(&gorm.DB{}, fstest.MapFS{})
	if err == nil || router != nil {
		t.Fatal("missing index must fail router initialization")
	}
}

func setArgs(t *testing.T, args ...string) {
	t.Helper()
	previous := os.Args
	os.Args = append([]string{"guanlan"}, args...)
	t.Cleanup(func() { os.Args = previous })
}

func TestNewRouterDevelopmentWithoutAssets(t *testing.T) {
	t.Setenv("APP_WEB_DEV", "1")
	router, err := newRouter(&gorm.DB{}, fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/datasource-groups", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("API must be registered: %d %s", response.Code, response.Body.String())
	}
}
