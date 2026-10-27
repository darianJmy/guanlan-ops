package handlers

import (
	"encoding/json"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestInspectionRuleGroupLifecycle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:?_pragma=foreign_keys(1)"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(&models.InspectionRuleGroup{}, &models.InspectionItem{}, &models.InspectionNodeState{}, &models.FaultCenter{}, &models.AlertInstance{}, &models.InspectionRecord{}); err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	old := database.DB
	database.DB = tx
	defer func() { tx.Rollback(); database.DB = old }()
	if err := tx.Create(&models.InspectionRuleGroup{ID: 1, Name: "默认规则组"}).Error; err != nil {
		t.Fatal(err)
	}
	center := models.FaultCenter{Name: "test", Status: 1}
	if err := tx.Create(&center).Error; err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.GET("/groups", ListInspectionRuleGroups)
	r.POST("/groups", CreateInspectionRuleGroup)
	r.PUT("/groups/:id", UpdateInspectionRuleGroup)
	r.DELETE("/groups/:id", DeleteInspectionRuleGroup)
	r.GET("/items", ListInspectionItems)
	r.POST("/items", CreateInspectionItem)
	r.PUT("/items/:id", UpdateInspectionItem)
	r.GET("/export", ExportInspectionItems)
	r.POST("/import", ImportInspectionItems)
	call := func(method, url string, body any, status int) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, url, strings.NewReader(string(raw)))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("%s %s: %d %s", method, url, res.Code, res.Body.String())
		}
		return res
	}
	call("POST", "/groups", gin.H{"name": "  production  "}, 200)
	call("POST", "/groups", gin.H{"name": "production"}, 400)
	call("POST", "/groups", gin.H{"name": " "}, 400)
	var group models.InspectionRuleGroup
	if err := tx.Where("name = ?", "production").First(&group).Error; err != nil {
		t.Fatal(err)
	}
	call("PUT", fmt.Sprintf("/groups/%d", group.ID), gin.H{"name": "生产", "description": "production probes"}, 200)
	call("DELETE", "/groups/1", nil, 200)
	// ID 1 is an ordinary group and may be created again after deletion.
	if err := tx.Create(&models.InspectionRuleGroup{ID: 1, Name: "another group"}).Error; err != nil {
		t.Fatal(err)
	}
	req := CreateInspectionItemReq{GroupID: group.ID, Name: "same-name", Type: "http", Target: "https://example.invalid", Config: `{"target":"https://example.invalid","execution_nodes":[{"node_type":"direct"}]}`, IntervalSeconds: 60, FailureThreshold: 3, RecoveryThreshold: 3, Severity: "P2", FaultCenterID: center.ID, Status: 1}
	missingGroup := req
	missingGroup.GroupID = 0
	call("POST", "/items", missingGroup, 400)
	call("POST", "/items", req, 200)
	var item models.InspectionItem
	tx.Where("group_id = ?", group.ID).First(&item)
	if item.ID == 0 {
		t.Fatal("task was not persisted")
	}
	record := models.InspectionRecord{ItemID: item.ID, Status: "success", Result: `{}`}
	if err := tx.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	call("DELETE", fmt.Sprintf("/groups/%d", group.ID), nil, 400)
	res := call("GET", fmt.Sprintf("/items?group_id=%d&page=1", group.ID), nil, 200)
	if !strings.Contains(res.Body.String(), `"total":1`) {
		t.Fatal(res.Body.String())
	}
	res = call("GET", "/items?group_id=1&page=1", nil, 200)
	if !strings.Contains(res.Body.String(), `"total":0`) {
		t.Fatal(res.Body.String())
	}
	call("GET", "/items?group_id=invalid", nil, 400)
	exported := call("GET", fmt.Sprintf("/export?group_id=%d", group.ID), nil, 200).Body.String()
	if !strings.Contains(exported, fmt.Sprintf("group_id: %d", group.ID)) {
		t.Fatal(exported)
	}
	withoutGroup := strings.ReplaceAll(exported, fmt.Sprintf("group_id: %d", group.ID), "group_id: 0")
	call("POST", "/import", gin.H{"content": withoutGroup, "dry_run": true}, 400)
	call("POST", "/import", gin.H{"content": withoutGroup, "group_id": group.ID, "dry_run": true}, 200)
	// Duplicate names in an unrelated group must not block this group's import.
	for i := 0; i < 2; i++ {
		other := models.InspectionItem{GroupID: 1, Name: "same-name", Type: "http", Target: "unrelated"}
		if err := tx.Create(&other).Error; err != nil {
			t.Fatal(err)
		}
	}
	call("POST", "/import", gin.H{"content": exported, "group_id": group.ID, "overwrite": true, "dry_run": true}, 200)
	if err := tx.Where("group_id = ?", 1).Delete(&models.InspectionItem{}).Error; err != nil {
		t.Fatal(err)
	}
	call("POST", "/import", gin.H{"content": exported, "group_id": 1, "overwrite": true}, 200)
	var imported models.InspectionItem
	tx.Where("group_id = ?", 1).First(&imported)
	if imported.ID == 0 || imported.ID == item.ID {
		t.Fatal("import overwrote a different group's task")
	}
	call("DELETE", "/groups/1", nil, 400)
	req.GroupID = 999999
	call("POST", "/items", req, 400)
	req.GroupID = 1
	call("PUT", fmt.Sprintf("/items/%d", item.ID), req, 200)
	var recordCount int64
	tx.Model(&models.InspectionRecord{}).Where("id = ?", record.ID).Count(&recordCount)
	if recordCount != 1 {
		t.Fatal("moving task removed history")
	}
	call("DELETE", fmt.Sprintf("/groups/%d", group.ID), nil, 200)
	// Database constraint also blocks concurrent inserts into deleted groups.
	orphan := models.InspectionItem{GroupID: group.ID, Name: "orphan", Type: "http", Target: "test"}
	if err := tx.Create(&orphan).Error; err == nil {
		t.Fatal("orphaned task was accepted")
	}
}
