package handlers

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"guanlan/internal/database"
	"guanlan/internal/models"
)

func TestQueryHistoryUserIsolationAndRetention(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	previous := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previous; sqlDB, _ := db.DB(); sqlDB.Close() })
	if err := db.AutoMigrate(&models.PromQueryHistory{}); err != nil {
		t.Fatal(err)
	}
	contextFor := func(user uint) (*gin.Context, *httptest.ResponseRecorder) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("user_id", user)
		c.Request = httptest.NewRequest("GET", "/prometheus/query-history?datasource_id=999&user_id=2", nil)
		return c, w
	}
	for _, user := range []uint{1, 2} {
		c, _ := contextFor(user)
		for i := 0; i < 55; i++ {
			mode := "table"
			if i%2 == 0 {
				mode = "logs"
			}
			ds := models.Datasource{ID: uint(i%3 + 1), Name: "removed-or-active-source"}
			if err := recordPromQueryHistory(c, ds, fmt.Sprintf("user-%d-query-%d", user, i), mode); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, user := range []uint{1, 2} {
		c, w := contextFor(user)
		ListPromQueryHistory(c)
		var response struct {
			Data []models.PromQueryHistory `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || len(response.Data) != 50 {
			t.Fatalf("user %d: %d %s", user, w.Code, w.Body.String())
		}
		for i, item := range response.Data {
			if item.Query != fmt.Sprintf("user-%d-query-%d", user, 54-i) {
				t.Fatalf("unexpected query: %s", item.Query)
			}
		}
		var count int64
		db.Model(&models.PromQueryHistory{}).Where("user_id = ?", user).Count(&count)
		if count != 50 {
			t.Fatalf("stored count: %d", count)
		}
	}
	c, w := contextFor(1)
	ClearPromQueryHistory(c)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var remaining []models.PromQueryHistory
	db.Find(&remaining)
	if len(remaining) != 50 {
		t.Fatalf("remaining: %d", len(remaining))
	}
	for _, item := range remaining {
		if item.UserID != 2 {
			t.Fatal("cleared wrong user's records")
		}
	}
}
