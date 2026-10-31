package middleware

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"guanlan/internal/auth"
	"guanlan/internal/config"
	"guanlan/internal/database"
	"guanlan/internal/models"

	"github.com/gin-gonic/gin"
)

func TestJWTRejectsDisabledAndDeletedUsers(t *testing.T) {
	previousDB, previousConfig := database.DB, config.AppConfig
	db, cleanup, err := database.New(config.Config{DBDSN: filepath.Join(t.TempDir(), "auth.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanup(); database.DB = previousDB; config.AppConfig = previousConfig })
	config.AppConfig.JWTSecret = "auth-test-secret"
	user := models.User{Username: "session-user", Status: 1}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	token, err := auth.GenerateToken(user.ID, user.Username)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/protected", Auth(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	check := func(want int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("status = %d, want %d: %s", rec.Code, want, rec.Body.String())
		}
	}
	check(http.StatusNoContent)
	if err := db.Model(&user).Update("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	check(http.StatusUnauthorized)
	if err := db.Model(&user).Update("status", 1).Error; err != nil {
		t.Fatal(err)
	}
	check(http.StatusNoContent)
	if err := db.Delete(&user).Error; err != nil {
		t.Fatal(err)
	}
	check(http.StatusUnauthorized)
}
