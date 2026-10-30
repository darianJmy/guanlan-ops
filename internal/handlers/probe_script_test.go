package handlers

import (
	"encoding/json"
	"fmt"
	"guanlan/internal/config"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func scriptProbeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previous := database.DB
	db, cleanup, err := database.New(config.Config{DBDSN: filepath.Join(t.TempDir(), "probe.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.DB = previous; cleanup() })
	if err := db.Create(&models.InspectionRuleGroup{ID: 1, Name: "script probes"}).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func TestScriptProbeExecution(t *testing.T) {
	db := scriptProbeTestDB(t)
	for _, test := range []struct {
		name, kind, content, status string
		code                        int
		timeout                     int
	}{
		{"shell success", "shell", "printf 'healthy'; printf 'diagnostic' >&2", "success", 0, 2000},
		{"shell failure", "shell", "printf 'broken' >&2; exit 7", "fail", 7, 2000},
		{"python success", "python", "print('healthy')", "success", 0, 2000},
		{"python failure", "python", "import sys; print('broken'); sys.exit(3)", "fail", 3, 2000},
		{"timeout", "shell", "printf 'started'; sleep 5; printf 'should-not-run'", "fail", 124, 1000},
	} {
		t.Run(test.name, func(t *testing.T) {
			script := models.Script{Name: test.name, Type: test.kind, Content: test.content, Status: 1}
			if err := db.Create(&script).Error; err != nil {
				t.Fatal(err)
			}
			task := createProbeAutomationTask(t, db, script, test.timeout/1000)
			start := time.Now()
			status, raw, _ := executeInspection(models.InspectionItem{Type: "script", Config: fmt.Sprintf(`{"automation_task_id":%d}`, task.ID)})
			var result scriptProbeResult
			if err := json.Unmarshal([]byte(raw), &result); err != nil {
				t.Fatal(err)
			}
			if status != test.status || result.ExitCode != test.code || result.AutomationTaskID != task.ID || result.ExecutionID == 0 || result.Output == "" {
				t.Fatalf("status=%s result=%+v", status, result)
			}
			if test.name == "shell success" && (!strings.Contains(result.Output, "healthy") || !strings.Contains(result.Output, "diagnostic")) {
				t.Fatalf("stdout/stderr missing: %q", result.Output)
			}
			if test.name == "timeout" && (time.Since(start) > 3*time.Second || !strings.Contains(result.Error, "超时") || strings.Contains(result.Output, "should-not-run")) {
				t.Fatalf("timeout failed: %+v", result)
			}
		})
	}
	script := models.Script{Name: "latest", Type: "shell", Content: "exit 1", Status: 1}
	db.Create(&script)
	task := createProbeAutomationTask(t, db, script, 2)
	req := ProbeReq{AutomationTaskID: task.ID}
	db.Model(&script).Update("content", "printf 'latest'")
	if status, raw, _ := executeScriptProbe(req); status != "success" || !strings.Contains(raw, "latest") {
		t.Fatalf("did not use latest library content: %s", raw)
	}
	db.Model(&script).Update("status", 0)
	if status, raw, _ := executeScriptProbe(req); status != "fail" || !strings.Contains(raw, "停用") {
		t.Fatalf("disabled script ran: %s", raw)
	}
	db.Delete(&script)
	if status, raw, _ := executeScriptProbe(req); status != "fail" || !strings.Contains(raw, "不存在") {
		t.Fatalf("deleted script ran: %s", raw)
	}
	if _, err := loadProbeAutomationTask(ProbeReq{TimeoutMS: 1000}); err == nil {
		t.Fatal("missing script accepted")
	}
	if _, err := loadProbeAutomationTask(ProbeReq{ScriptID: 1, TimeoutMS: 300001}); err == nil {
		t.Fatal("invalid timeout accepted")
	}
}

func TestScriptProbeAPIAndAlertLifecycle(t *testing.T) {
	db := scriptProbeTestDB(t)
	script := models.Script{Name: "health script", Type: "shell", Content: "printf 'unhealthy'; exit 2", Status: 1}
	if err := db.Create(&script).Error; err != nil {
		t.Fatal(err)
	}
	task := createProbeAutomationTask(t, db, script, 2)
	center := models.FaultCenter{Name: "script center", Status: 1}
	if err := db.Create(&center).Error; err != nil {
		t.Fatal(err)
	}
	req := CreateInspectionItemReq{GroupID: 1, Name: "script health", Type: "script", Target: script.Name, Config: fmt.Sprintf(`{"automation_task_id":%d}`, task.ID), IntervalSeconds: 10, FailureThreshold: 2, RecoveryThreshold: 2, Severity: "P2", FaultCenterID: center.ID, Status: 1}
	if err := validateInspectionItemReq(req); err != nil {
		t.Fatal(err)
	}
	// The YAML import path must accept the exact same configuration.
	item := inspectionItemFromRequest(req)
	doc, err := inspectionTaskConfigDocumentFromItems([]models.InspectionItem{item})
	if err != nil {
		t.Fatal(err)
	}
	imported, err := inspectionTaskConfigRequest(doc.Tasks[0])
	if err != nil || imported.Type != "script" {
		t.Fatalf("import: %+v %v", imported, err)
	}
	router := gin.New()
	router.POST("/items", CreateInspectionItem)
	router.PUT("/items/:id", UpdateInspectionItem)
	router.POST("/instant", InstantProbe)
	router.DELETE("/tasks/:id", DeleteAutomationTask)
	call := func(method, path string, payload interface{}) *httptest.ResponseRecorder {
		encoded, _ := json.Marshal(payload)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, path, strings.NewReader(string(encoded)))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		return recorder
	}
	response := call(http.MethodPost, "/items", req)
	if response.Code != 200 {
		t.Fatalf("create: %s", response.Body.String())
	}
	if err := db.Where("name = ?", req.Name).First(&item).Error; err != nil {
		t.Fatal(err)
	}
	req.Name = "updated script health"
	response = call(http.MethodPut, fmt.Sprintf("/items/%d", item.ID), req)
	if response.Code != 200 {
		t.Fatalf("update: %s", response.Body.String())
	}
	db.First(&item, item.ID)
	if item.Name != req.Name {
		t.Fatal("update did not persist")
	}
	response = call(http.MethodDelete, fmt.Sprintf("/tasks/%03d", task.ID), nil)
	if response.Code != 400 || !strings.Contains(response.Body.String(), "拨测任务引用") {
		t.Fatalf("referenced script deletion: %s", response.Body.String())
	}
	// Trial execution must not create records or alerts.
	response = call(http.MethodPost, "/instant", ProbeReq{Type: "script", AutomationTaskID: task.ID})
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"status":"fail"`) {
		t.Fatalf("instant: %s", response.Body.String())
	}
	var records int64
	db.Model(&models.InspectionRecord{}).Count(&records)
	if records != 0 {
		t.Fatal("trial created inspection records")
	}
	var alert models.AlertInstance
	for _, expected := range []string{alertStatusPending, alertStatusFiring, alertStatusRecovering, alertStatusResolved} {
		if expected == alertStatusRecovering {
			db.Model(&script).Update("content", "printf 'healthy'; exit 0")
		}
		record := executeAndRecordInspection(&item)
		if record.Status == "error" {
			t.Fatalf("execution failed: %s", record.Result)
		}
		if err := db.Where("source = ? AND source_id = ?", "probe-task", item.ID).Order("id DESC").First(&alert).Error; err != nil {
			t.Fatal(err)
		}
		if alert.Status != expected {
			t.Fatalf("state=%s want=%s result=%s", alert.Status, expected, record.Result)
		}
	}
	var latest models.InspectionRecord
	db.Where("item_id = ?", item.ID).Order("id DESC").First(&latest)
	var result scriptProbeResult
	json.Unmarshal([]byte(latest.Result), &result)
	if latest.NodeType != "direct" || result.ExitCode != 0 || result.Output != "healthy\n" && result.Output != "healthy" {
		t.Fatalf("stored record=%+v result=%+v", latest, result)
	}
	if !inspectionTaskDue(models.InspectionItem{Status: 1, IntervalSeconds: 10, LastRunAt: ptrTime(time.Now().Add(-11 * time.Second))}, time.Now()) {
		t.Fatal("10-second schedule ignored")
	}
}

func ptrTime(value time.Time) *time.Time { return &value }

func TestScriptProbeUnavailableNodesAndTruncation(t *testing.T) {
	db := scriptProbeTestDB(t)
	script := models.Script{Name: "node test", Type: "shell", Content: "printf 'healthy'", Status: 1}
	db.Create(&script)
	task := createProbeAutomationTask(t, db, script, 2)
	for _, nodeType := range []string{"ssh", "agent", "invalid"} {
		db.Model(&task).Updates(map[string]interface{}{"node_type": nodeType, "node_id": 999})
		status, raw, _ := executeScriptProbe(ProbeReq{AutomationTaskID: task.ID})
		var result scriptProbeResult
		json.Unmarshal([]byte(raw), &result)
		if status != "fail" || result.Error == "" || result.ExitCode == 0 {
			t.Fatalf("unavailable %s: %s", nodeType, raw)
		}
	}
	db.Model(&script).Updates(map[string]interface{}{"type": "python", "content": "print('x' * 100000)"})
	db.Model(&task).Updates(map[string]interface{}{"node_type": "direct", "node_id": 0})
	status, raw, _ := executeScriptProbe(ProbeReq{AutomationTaskID: task.ID})
	var result scriptProbeResult
	json.Unmarshal([]byte(raw), &result)
	if status != "success" || !result.Truncated || len(result.Output) > 65540 {
		t.Fatalf("truncation failed: %s, %d", status, len(result.Output))
	}
}

func TestScriptProbeScheduler(t *testing.T) {
	db := scriptProbeTestDB(t)
	script := models.Script{Name: "scheduled probe", Type: "shell", Content: "printf 'scheduled'", Status: 1}
	db.Create(&script)
	task := createProbeAutomationTask(t, db, script, 2)
	item := models.InspectionItem{GroupID: 1, Name: "scheduled probe", Type: "script", Target: script.Name, Config: fmt.Sprintf(`{"automation_task_id":%d}`, task.ID), IntervalSeconds: 10, FailureThreshold: 1, RecoveryThreshold: 1, Status: 1}
	db.Create(&item)
	now := time.Now()
	runDueInspectionItems(now)
	deadline := time.Now().Add(5 * time.Second)
	for len(inspectionWorkers) > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(inspectionWorkers) > 0 {
		t.Fatal("scheduler worker did not finish")
	}
	db.First(&item, item.ID)
	if item.LastStatus != "success" || item.LastRunAt == nil {
		t.Fatalf("script probe was not scheduled: %+v", item)
	}
	var records int64
	db.Model(&models.InspectionRecord{}).Where("item_id = ?", item.ID).Count(&records)
	if records != 1 {
		t.Fatalf("records=%d", records)
	}
	runDueInspectionItems(now.Add(time.Second))
	if len(inspectionWorkers) != 0 {
		t.Fatal("task re-scheduled before interval elapsed")
	}
	// A concurrent/manual invocation must not run the same script twice.
	runningInspections.Store(item.ID, struct{}{})
	record := executeAndRecordInspection(&item)
	runningInspections.Delete(item.ID)
	if record.Status != "error" || !strings.Contains(record.Result, "正在执行") {
		t.Fatalf("duplicate invocation allowed: %+v", record)
	}
}

func createProbeAutomationTask(t *testing.T, db *gorm.DB, script models.Script, timeout int) models.AutomationTask {
	t.Helper()
	task := models.AutomationTask{Name: script.Name, ScriptID: script.ID, SourceType: "script", NodeType: "direct", TimeoutSeconds: timeout, Status: 1}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	return task
}

func TestScriptProbeCannotOverrideAutomationTask(t *testing.T) {
	db := scriptProbeTestDB(t)
	script := models.Script{Name: "central config", Type: "shell", Content: "printf healthy", Status: 1}
	db.Create(&script)
	task := createProbeAutomationTask(t, db, script, 2)
	for _, req := range []ProbeReq{
		{ScriptID: script.ID},
		{AutomationTaskID: task.ID, ScriptID: script.ID},
		{AutomationTaskID: task.ID, NodeType: "direct"},
		{AutomationTaskID: task.ID, TimeoutMS: 5000},
		{AutomationTaskID: task.ID, ExecutionNodes: []inspectionExecutionNode{{NodeType: "direct"}}},
	} {
		if _, err := loadProbeAutomationTask(req); err == nil {
			t.Fatalf("override accepted: %+v", req)
		}
	}
	db.Model(&task).Updates(map[string]interface{}{"node_type": "ssh", "node_id": 999})
	config := fmt.Sprintf(`{"automation_task_id":%d}`, task.ID)
	nodes := inspectionExecutionNodes(config)
	if len(nodes) != 1 || nodes[0].NodeType != "ssh" || nodes[0].NodeID != 999 {
		t.Fatalf("not using current task node: %+v", nodes)
	}
	if status, _, _ := executeInspectionOnNode(models.InspectionItem{ID: 42, Type: "script", Config: config}, inspectionExecutionNode{NodeType: "direct"}); status != "fail" {
		t.Fatal("caller bypassed configured SSH node")
	}
	var record models.AutomationExecution
	db.Order("id DESC").First(&record)
	if record.TaskID != task.ID || record.TriggerType != "probe" || record.TriggerID != 42 {
		t.Fatalf("missing invocation audit: %+v", record)
	}
	db.Model(&task).Update("status", 0)
	if status, _, _ := executeScriptProbe(ProbeReq{AutomationTaskID: task.ID}); status != "fail" {
		t.Fatal("disabled task executed")
	}
}

func TestAlertActionOnlyInvokesAutomationTask(t *testing.T) {
	db := scriptProbeTestDB(t)
	script := models.Script{Name: "action task", Type: "shell", Content: "printf centralized", Status: 1}
	db.Create(&script)
	task := createProbeAutomationTask(t, db, script, 2)
	rule := models.AlertActionRule{ActionType: "automation_task", TargetID: task.ID, NodeType: "ssh", NodeID: 999, TimeoutSeconds: 0}
	out, err := executeAlertAction(rule, models.AlertInstance{ID: 9}, "firing", time.Now())
	if err != nil || out != "centralized" {
		t.Fatalf("task settings not used: %s %v", out, err)
	}
	for _, kind := range []string{"script", "workflow", "webhook"} {
		rule.ActionType = kind
		if _, err := executeAlertAction(rule, models.AlertInstance{}, "firing", time.Now()); err == nil {
			t.Fatalf("legacy %s action executed", kind)
		}
	}
}
