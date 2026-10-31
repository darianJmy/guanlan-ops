package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"guanlan/internal/config"
	"guanlan/internal/database"
	"guanlan/internal/models"

	"github.com/gin-gonic/gin"
)

func TestCreateUserPreservesStatusAndRejectsInvalidPasswords(t *testing.T) {
	previous := database.DB
	db, cleanup, err := database.New(config.Config{DBDSN: filepath.Join(t.TempDir(), "users.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanup(); database.DB = previous })
	router := gin.New()
	router.POST("/users", CreateUser)
	for _, tc := range []struct {
		name, password string
		status, want   int
	}{
		{"disabled", "valid-password", 0, http.StatusOK},
		{"enabled", "valid-password", 1, http.StatusOK},
		{"long", strings.Repeat("密", 25), 1, http.StatusBadRequest},
		{"invalid-status", "valid-password", 2, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{"username": tc.name, "password": tc.password, "status": tc.status})
			req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
			}
			var user models.User
			err := db.Where("username = ?", tc.name).First(&user).Error
			if tc.want == http.StatusOK {
				if err != nil || user.Status != tc.status || user.Password == "" {
					t.Fatalf("invalid created user: %+v, %v", user, err)
				}
			} else if err == nil {
				t.Fatal("invalid input created a user")
			}
		})
	}
}
