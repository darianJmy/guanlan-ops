package handlers

import (
	"encoding/json"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestInspectionHistoryRetentionAndPagination(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	pool.SetMaxOpenConns(1)
	previous := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previous; _ = pool.Close() })
	if err := db.AutoMigrate(&models.InspectionItem{}, &models.InspectionRecord{}); err != nil {
		t.Fatal(err)
	}
	item := models.InspectionItem{ID: 1, Name: "history", Type: "http", GroupID: 1}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	cutoff := now.Add(-72 * time.Hour)
	rows := []models.InspectionRecord{
		{ItemID: 1, CreatedAt: cutoff.Add(-time.Second), Status: "success"},
		{ItemID: 2, CreatedAt: cutoff.Add(-time.Hour), Status: "fail"},
		{ItemID: 1, CreatedAt: cutoff, Status: "success"},
	}
	for i := 0; i < 25; i++ {
		rows = append(rows, models.InspectionRecord{ItemID: 1, NodeType: "direct", Status: "success", CreatedAt: now.Add(-time.Duration(i) * time.Minute)})
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/records", ListInspectionRecords)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/records?item_id=1&page=2&page_size=20", nil))
	var body struct {
		Code int `json:"code"`
		Data struct {
			List  []models.InspectionRecord `json:"list"`
			Total int                       `json:"total"`
			Item  models.InspectionItem     `json:"item"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Total != 25 || len(body.Data.List) != 5 || body.Data.Item.Name != "history" {
		t.Fatalf("unexpected history response: %s", response.Body.String())
	}
	for _, record := range body.Data.List {
		if record.CreatedAt.Before(cutoff) {
			t.Fatal("expired record returned")
		}
	}
	if err := db.Model(&models.InspectionRecord{}).Where("id = ?", rows[3].ID).Updates(map[string]interface{}{"node_name": "上海节点", "result": "HTTP 500: 100%_failed"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		params url.Values
		want   int
		status int
	}{
		{"node search", url.Values{"keyword": {"上海"}}, 1, 200},
		{"result search", url.Values{"keyword": {"HTTP 500"}}, 1, 200},
		{"literal wildcard", url.Values{"keyword": {"%_"}}, 1, 200},
		{"combined filters", url.Values{"keyword": {"上海"}, "end_time": {now.Add(-time.Minute).Format(time.RFC3339Nano)}}, 0, 200},
		{"time range", url.Values{"start_time": {now.Add(-5 * time.Minute).Format(time.RFC3339Nano)}, "end_time": {now.Add(-2 * time.Minute).Format(time.RFC3339Nano)}}, 4, 200},
		{"retention bound", url.Values{"start_time": {now.Add(-7 * 24 * time.Hour).Format(time.RFC3339Nano)}}, 25, 200},
		{"invalid time", url.Values{"start_time": {"invalid"}}, 0, 400},
		{"reversed range", url.Values{"start_time": {now.Format(time.RFC3339Nano)}, "end_time": {now.Add(-time.Hour).Format(time.RFC3339Nano)}}, 0, 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.params.Set("item_id", "1")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("GET", "/records?"+test.params.Encode(), nil))
			if response.Code != test.status {
				t.Fatalf("unexpected status: %s", response.Body.String())
			}
			if test.status == 200 {
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.Data.Total != test.want {
					t.Fatalf("expected %d records: %s", test.want, response.Body.String())
				}
			}
		})
	}
	cleanupExpiredInspectionRecords(now)
	var remaining []models.InspectionRecord
	if err := db.Order("created_at ASC").Find(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 26 {
		t.Fatalf("expected 26 retained rows (not capped at 12), got %d", len(remaining))
	}
	if !remaining[0].CreatedAt.Equal(cutoff) {
		t.Fatal("record at cutoff must be retained")
	}
	cleanupExpiredInspectionRecords(now.Add(73 * time.Hour))
	var count int64
	db.Model(&models.InspectionRecord{}).Count(&count)
	if count != 0 {
		t.Fatalf("inactive task records must expire too: %d", count)
	}
}
