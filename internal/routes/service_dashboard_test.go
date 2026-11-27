package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"guanlan/internal/auth"
	"guanlan/internal/config"
	"guanlan/internal/database"
	"guanlan/internal/models"
)

func TestServiceDashboardDirectLinkAndFolderAccess(t *testing.T) {
	t.Setenv("SERVICE_PORTAL_URL", "")
	t.Setenv("SERVICE_PORTAL_INTERNAL_TOKEN", "test-service-portal-secret")

	previousDB, previousConfig := database.DB, config.AppConfig
	config.AppConfig.JWTSecret = "test-service-dashboard-jwt"
	db, closeDB, err := database.New(config.Config{DBDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		closeDB()
		database.DB, config.AppConfig = previousDB, previousConfig
	})

	var listPermission models.Permission
	if err := db.Where("code = ?", "service-dashboard:list").First(&listPermission).Error; err != nil {
		t.Fatal(err)
	}
	viewRole := models.Role{Code: "service-viewer", Name: "Viewer", Status: 1, Permissions: []models.Permission{listPermission}}
	viewer := models.User{Username: "viewer", Password: "unused", Status: 1, Roles: []models.Role{viewRole}}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatal(err)
	}
	adminToken, err := auth.GenerateToken(1, "admin")
	if err != nil {
		t.Fatal(err)
	}
	viewerToken, err := auth.GenerateToken(viewer.ID, viewer.Username)
	if err != nil {
		t.Fatal(err)
	}

	router := SetupRouter()
	send := func(method, path, token, payload string, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		if out.Code != status {
			t.Fatalf("%s %s returned %d, want %d: %s", method, path, out.Code, status, out.Body.String())
		}
		return out
	}

	const resources = "/api/service-portal/resources"
	folderResponse := send(http.MethodPost, "/api/dashboard-folders", adminToken, `{"scope":"services","name":"Team"}`, http.StatusOK)
	var folder struct {
		Data models.DashboardFolder
	}
	if err := json.Unmarshal(folderResponse.Body.Bytes(), &folder); err != nil || folder.Data.ID == 0 {
		t.Fatalf("invalid folder response: %s (%v)", folderResponse.Body.String(), err)
	}
	folderID := folder.Data.ID
	permissionsPath := fmt.Sprintf("/api/dashboard-folders/%d/permissions", folderID)
	send(http.MethodPost, resources, viewerToken, "{}", http.StatusForbidden)
	send(http.MethodPost, resources, adminToken, fmt.Sprintf(`{"name":"Example","type":"http_direct","targetURL":"https://app.example.test/","folder_id":%d}`, folderID), http.StatusOK)
	out := send(http.MethodGet, resources, viewerToken, "", http.StatusOK)
	if !strings.Contains(out.Body.String(), "https://app.example.test/") {
		t.Fatal("viewer could not see service in inherited folder")
	}
	send(http.MethodPut, permissionsPath, adminToken, `{"permission_mode":"restricted","permissions":[]}`, http.StatusOK)
	out = send(http.MethodGet, resources, viewerToken, "", http.StatusOK)
	if strings.Contains(out.Body.String(), "app.example.test") {
		t.Fatal("restricted service was exposed")
	}
	send(http.MethodPut, permissionsPath, adminToken, fmt.Sprintf(`{"permission_mode":"restricted","permissions":[{"role_id":%d,"access_level":"view"}]}`, viewer.Roles[0].ID), http.StatusOK)
	out = send(http.MethodGet, resources, viewerToken, "", http.StatusOK)
	if !strings.Contains(out.Body.String(), "https://app.example.test/") {
		t.Fatal("service was not visible after granting view access")
	}

	send(http.MethodGet, "/api/service-access/portal/state", adminToken, "", http.StatusNotFound)
	send(http.MethodGet, "/api/service-access/portal/runtime-state", "", "", http.StatusNotFound)
}
