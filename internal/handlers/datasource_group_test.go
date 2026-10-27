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

func TestDatasourceGroupCreationAndFiltering(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old; sqlDB.Close() })
	if err := db.AutoMigrate(&models.Datasource{}, &models.DatasourceGroup{}, &models.DatasourceGroupMember{}, &models.Tunnel{}, &models.AlertRule{}, &models.AlertRuleDatasourceGroup{}, &models.AlertRuleDatasource{}, &models.AlertRuleBinding{}); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/groups", CreateDatasourceGroup)
	router.POST("/sources", CreateDatasourceWithTunnel)
	router.GET("/sources", ListDatasourcesWithTunnel)
	call := func(method, path, body string) map[string]interface{} {
		t.Helper()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)
		var result map[string]interface{}
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	result := call("POST", "/groups", `{"name":"生产环境"}`)
	if result["code"] != float64(0) {
		t.Fatalf("empty group: %v", result)
	}
	groupID := uint(result["data"].(map[string]interface{})["id"].(float64))
	// A rule already using this group must gain new metrics, but never log sources.
	rule := models.AlertRule{}
	if err := db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AlertRuleDatasourceGroup{RuleID: rule.ID, DatasourceGroupID: groupID}).Error; err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"prometheus", "loki"} {
		result = call("POST", "/sources", fmt.Sprintf(`{"name":%q,"type":%q,"url":"http://example.test:9090","group_id":%d}`, kind, kind, groupID))
		if result["code"] != float64(0) {
			t.Fatalf("create in group: %v", result)
		}
	}
	var members []models.DatasourceGroupMember
	db.Where("group_id = ?", groupID).Find(&members)
	if len(members) != 2 {
		t.Fatalf("members: %v", members)
	}
	var bindings []models.AlertRuleBinding
	db.Where("rule_id = ?", rule.ID).Find(&bindings)
	if len(bindings) != 1 || bindings[0].DatasourceID != members[0].DatasourceID {
		t.Fatalf("metric bindings: %v", bindings)
	}
	result = call("POST", "/sources", `{"name":"outside","type":"prometheus","url":"http://example.test:9090"}`)
	if result["code"] != float64(0) {
		t.Fatal(result)
	}
	for _, test := range []struct {
		query string
		total float64
	}{
		{"", 3}, {fmt.Sprintf("?group_id=%d&page_size=1", groupID), 2}, {fmt.Sprintf("?group_id=%d&type=loki", groupID), 1}, {"?group_id=999", 0},
	} {
		result = call("GET", "/sources"+test.query, "")
		if result["code"] != float64(0) || result["data"].(map[string]interface{})["total"] != test.total {
			t.Fatalf("filter %s: %v", test.query, result)
		}
	}
	result = call("POST", "/sources", `{"name":"invalid-group","type":"loki","url":"http://example.test:9090","group_id":999}`)
	if result["code"] == float64(0) {
		t.Fatal("accepted missing group")
	}
	var count int64
	db.Model(&models.Datasource{}).Where("name = ?", "invalid-group").Count(&count)
	if count != 0 {
		t.Fatal("created an orphan instance")
	}
	// Failure to persist membership must also roll back the instance itself.
	if err := db.Exec("CREATE TRIGGER reject_group_member BEFORE INSERT ON datasource_group_members BEGIN SELECT RAISE(ABORT, 'membership rejected'); END").Error; err != nil {
		t.Fatal(err)
	}
	result = call("POST", "/sources", fmt.Sprintf(`{"name":"rollback","type":"loki","url":"http://example.test:9090","group_id":%d}`, groupID))
	if result["code"] == float64(0) {
		t.Fatal("membership failure accepted")
	}
	db.Model(&models.Datasource{}).Where("name = ?", "rollback").Count(&count)
	if count != 0 {
		t.Fatal("instance survived membership failure")
	}
}
