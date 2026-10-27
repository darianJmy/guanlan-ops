package handlers

import (
	"encoding/json"
	"fmt"
	"guanlan/internal/config"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDatasourceGroupHierarchyLifecycle(t *testing.T) {
	old := database.DB
	db, cleanup, err := database.New(config.Config{DBDSN: filepath.Join(t.TempDir(), "tree.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanup(); database.DB = old })
	router := gin.New()
	router.POST("/groups", CreateDatasourceGroup)
	router.PUT("/groups/:id", UpdateDatasourceGroup)
	router.DELETE("/groups/:id", DeleteDatasourceGroup)
	router.GET("/groups", ListDatasourceGroups)
	router.POST("/sources", CreateDatasourceWithTunnel)
	router.GET("/sources", ListDatasourcesWithTunnel)
	router.PUT("/sources/status", BatchUpdateDatasourceStatus)
	router.POST("/rules", CreateAlertRule)
	router.DELETE("/sources/batch", BatchDeleteDatasources)
	router.DELETE("/sources/:id", DeleteDatasource)
	call := func(method, path string, body interface{}, success bool) map[string]interface{} {
		t.Helper()
		encoded, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, strings.NewReader(string(encoded)))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		var result map[string]interface{}
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if (result["code"] == float64(0)) != success {
			t.Fatalf("%s %s: %s", method, path, recorder.Body.String())
		}
		return result
	}
	create := func(name string, parent uint) uint {
		t.Helper()
		result := call("POST", "/groups", map[string]interface{}{"name": name, "parent_id": parent}, true)
		return uint(result["data"].(map[string]interface{})["id"].(float64))
	}
	// Populated leaf groups cannot be removed, including disabled members.
	occupied := create("protected", 0)
	call("POST", "/sources", map[string]interface{}{"name": "disabled-member", "type": "prometheus", "url": "http://example.test", "group_id": occupied, "status": 0}, true)
	resultDelete := call("DELETE", fmt.Sprintf("/groups/%d", occupied), nil, false)
	if resultDelete["message"] != "分组内存在数据源，不能删除" {
		t.Fatal(resultDelete)
	}
	var remainingGroups, remainingMembers int64
	db.Model(&models.DatasourceGroup{}).Where("id = ?", occupied).Count(&remainingGroups)
	db.Model(&models.DatasourceGroupMember{}).Where("group_id = ?", occupied).Count(&remainingMembers)
	if remainingGroups != 1 || remainingMembers != 1 {
		t.Fatal("rejected deletion changed the group or its members")
	}
	call("PUT", fmt.Sprintf("/groups/%d", occupied), map[string]interface{}{"name": "protected", "datasource_ids": []uint{}}, true)
	call("DELETE", fmt.Sprintf("/groups/%d", occupied), nil, true)
	root := create("生产环境", 0)
	eu := create("eu", root)
	prom := create("prometheus", eu)
	us := create("us", root)
	create("prometheus", us)
	call("POST", "/groups", map[string]interface{}{"name": "prometheus", "parent_id": eu}, false)
	fourth := create("cluster", prom)
	fifth := create("node", fourth)
	call("POST", "/groups", map[string]interface{}{"name": "sixth", "parent_id": fifth}, false)
	call("POST", "/groups", map[string]interface{}{"name": "missing", "parent_id": 999999}, false)
	call("PUT", fmt.Sprintf("/groups/%d", root), map[string]interface{}{"name": "生产环境", "parent_id": prom}, false)
	call("PUT", fmt.Sprintf("/groups/%d", eu), map[string]interface{}{"name": "eu", "parent_id": us}, false) // subtree would exceed five levels
	call("DELETE", fmt.Sprintf("/groups/%d", eu), nil, false)
	rg := models.AlertRuleGroup{Name: "rules"}
	if err := db.Create(&rg).Error; err != nil {
		t.Fatal(err)
	}
	center := models.FaultCenter{Name: "center", Status: 1}
	if err := db.Create(&center).Error; err != nil {
		t.Fatal(err)
	}
	// Save a rule against empty groups before the first instance is connected.
	result := call("POST", "/rules", map[string]interface{}{"group_id": rg.ID, "name": "inherited", "expression": "up", "datasource_group_ids": []uint{eu, prom}, "fault_center_id": center.ID, "evaluation_interval_seconds": 60, "status": 1, "conditions": []map[string]interface{}{{"severity": "P2", "operator": "==", "threshold": 0}}}, true)
	ruleID := uint(result["data"].(map[string]interface{})["id"].(float64))
	assertBindings := func(want int64) {
		t.Helper()
		var count int64
		db.Model(&models.AlertRuleBinding{}).Where("rule_id = ?", ruleID).Count(&count)
		if count != want {
			t.Fatalf("bindings got %d want %d", count, want)
		}
	}
	assertBindings(0)
	for _, kind := range []string{"prometheus", "loki"} {
		call("POST", "/sources", map[string]interface{}{"name": kind, "type": kind, "url": "http://example.test", "group_id": prom}, true)
	}
	assertBindings(1) // overlap deduplicated; logs excluded
	var source models.Datasource
	db.Where("name = ?", "prometheus").First(&source)
	list := call("GET", fmt.Sprintf("/sources?group_id=%d", root), nil, true)
	if list["data"].(map[string]interface{})["total"] != float64(2) {
		t.Fatal("parent listing excludes descendants")
	}
	groups := call("GET", "/groups", nil, true)["data"].([]interface{})
	for _, value := range groups {
		group := value.(map[string]interface{})
		if uint(group["id"].(float64)) == prom && group["path"] != "生产环境 / eu / prometheus" {
			t.Fatal(group)
		}
	}
	alert := models.AlertInstance{Source: alertRuleSource, SourceID: ruleID, DatasourceID: source.ID, Status: alertStatusFiring, Severity: "P2", FaultCenterID: center.ID, Fingerprint: "tree-test"}
	if err := db.Create(&alert).Error; err != nil {
		t.Fatal(err)
	}
	call("PUT", "/sources/status", map[string]interface{}{"ids": []uint{source.ID}, "status": 0}, true)
	assertBindings(0)
	db.First(&alert, alert.ID)
	if alert.Status != alertStatusTerminated {
		t.Fatalf("stale alert: %s", alert.Status)
	}
	call("PUT", "/sources/status", map[string]interface{}{"ids": []uint{source.ID}, "status": 1}, true)
	assertBindings(1)
	// Remove the direct leaf selection so moving the subtree tests parent inheritance.
	db.Where("rule_id = ? AND datasource_group_id = ?", ruleID, prom).Delete(&models.AlertRuleDatasourceGroup{})
	var sourceIDs []uint
	db.Model(&models.DatasourceGroupMember{}).Where("group_id = ?", prom).Pluck("datasource_id", &sourceIDs)
	call("PUT", fmt.Sprintf("/groups/%d", prom), map[string]interface{}{"name": "metrics", "parent_id": us, "datasource_ids": sourceIDs}, true)
	assertBindings(0)
	call("PUT", fmt.Sprintf("/groups/%d", prom), map[string]interface{}{"name": "prometheus", "parent_id": eu, "datasource_ids": sourceIDs}, true)
	assertBindings(1)
	// Deleting a grouped source removes inherited bindings and terminates alerts.
	db.Model(&alert).Update("status", alertStatusFiring)
	if err := db.Exec("CREATE TRIGGER reject_datasource_delete BEFORE DELETE ON datasources BEGIN SELECT RAISE(ABORT, 'delete failed'); END").Error; err != nil {
		t.Fatal(err)
	}
	call("DELETE", fmt.Sprintf("/sources/%d", source.ID), nil, false)
	assertBindings(1)
	db.First(&alert, alert.ID)
	if alert.Status != alertStatusFiring {
		t.Fatal("failed delete changed alert state")
	}
	var preservedMembers int64
	db.Model(&models.DatasourceGroupMember{}).Where("datasource_id = ?", source.ID).Count(&preservedMembers)
	if preservedMembers != 1 {
		t.Fatal("failed delete changed membership")
	}
	if err := db.Exec("DROP TRIGGER reject_datasource_delete").Error; err != nil {
		t.Fatal(err)
	}
	call("DELETE", fmt.Sprintf("/sources/%d", source.ID), nil, true)
	assertBindings(0)
	db.First(&alert, alert.ID)
	if alert.Status != alertStatusTerminated {
		t.Fatal("deletion left active alert")
	}
	var membershipCount int64
	db.Model(&models.DatasourceGroupMember{}).Where("datasource_id = ?", source.ID).Count(&membershipCount)
	if membershipCount != 0 {
		t.Fatal("deletion left membership")
	}
	var logSource models.Datasource
	db.Where("name = ?", "loki").First(&logSource)
	// A batch with a direct rule reference must leave the source and membership intact.
	db.Create(&models.AlertRuleDatasource{RuleID: ruleID, DatasourceID: logSource.ID})
	call("DELETE", "/sources/batch", map[string]interface{}{"ids": []uint{logSource.ID}}, false)
	db.Model(&models.DatasourceGroupMember{}).Where("datasource_id = ?", logSource.ID).Count(&membershipCount)
	if membershipCount != 1 {
		t.Fatal("rejected batch removed membership")
	}
	db.Where("rule_id = ? AND datasource_id = ?", ruleID, logSource.ID).Delete(&models.AlertRuleDatasource{})
	call("DELETE", "/sources/batch", map[string]interface{}{"ids": []uint{logSource.ID}}, true)
	db.Model(&models.DatasourceGroupMember{}).Where("group_id = ?", prom).Count(&membershipCount)
	if membershipCount != 0 {
		t.Fatal("batch deletion left members")
	}
	call("PUT", fmt.Sprintf("/groups/%d", prom), map[string]interface{}{"name": "prometheus", "parent_id": eu, "datasource_ids": []uint{}}, true)
	assertBindings(0)
}
