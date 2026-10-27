package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"guanlan/internal/database"
	"guanlan/internal/models"
)

func TestCustomWebhookWithoutTemplate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	old, transport := database.DB, http.DefaultTransport
	database.DB = db
	t.Cleanup(func() { database.DB = old; http.DefaultTransport = transport; sqlDB, _ := db.DB(); sqlDB.Close() })
	if err := db.AutoMigrate(&models.NotificationChannel{}, &models.NotificationTemplate{}, &models.NotificationRecord{}); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/channels", CreateNotificationChannel)
	router.PUT("/channels/:id", UpdateNotificationChannel)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		return w
	}
	w := call("POST", "/channels", `{"name":"hook","type":"webhook","webhook_url":"https://example.com/hook","status":1}`)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var channel models.NotificationChannel
	if err := db.First(&channel).Error; err != nil {
		t.Fatal(err)
	}
	if channel.TemplateID != 0 {
		t.Fatal("unexpected template")
	}
	w = call("PUT", "/channels/1", `{"name":"hook","type":"webhook","template_id":123,"status":1}`)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	db.First(&channel)
	if channel.TemplateID != 0 {
		t.Fatal("template retained on update")
	}
	if w := call("POST", "/channels", `{"name":"robot","type":"feishu","webhook_url":"https://example.com/hook"}`); w.Code != 400 {
		t.Fatal("robot no longer requires template")
	}
	payload := map[string]interface{}{"title": "original", "content": "raw event", "status": "firing", "labels": map[string]interface{}{"app": "nginx"}}
	tpl := models.NotificationTemplate{Name: "old", Type: "webhook", Content: "should not render"}
	db.Create(&tpl)
	channel.TemplateID = tpl.ID
	http.DefaultTransport = weComTestTransport(func(r *http.Request) (*http.Response, error) {
		var got map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got["title"] != "original" || got["content"] != "raw event" || got["status"] != "firing" || got["labels"].(map[string]interface{})["app"] != "nginx" {
			t.Fatalf("event altered: %+v", got)
		}
		return &http.Response{StatusCode: 204, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	if err := sendNotificationToChannel(channel, "alert", "event", "original", payload); err != nil {
		t.Fatal(err)
	}
	if payload["title"] != "original" || payload["content"] != "raw event" {
		t.Fatal("original event mutated")
	}
	var record models.NotificationRecord
	if err := db.First(&record).Error; err != nil || record.Status != "success" {
		t.Fatal("delivery not recorded", err)
	}
}
