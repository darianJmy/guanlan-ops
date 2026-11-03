package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	aicore "guanlan/internal/ai"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func setupAlertAITest(t *testing.T) (*gorm.DB, models.AlertInstance, models.AlertAIAnalysisRule) {
	t.Helper()
	db := setupAIModelTestDB(t)
	if err := db.AutoMigrate(&models.User{}, &models.Role{}, &models.Permission{}, &models.Datasource{}, &models.FaultCenter{}, &models.AlertInstance{}, &models.AlertEvent{}, &models.AlertSilenceRule{}, &models.AlertInhibitionRule{}, &models.AlertAIAnalysisRule{}, &models.AlertAIAnalysisJob{}, &models.AlertNotificationConfig{}, &models.AlertNotificationRoute{}, &models.AlertGroupingRule{}, &models.AlertGroupState{}, &models.NotificationChannel{}, &models.NotificationTemplate{}, &models.NotificationRecord{}, &models.AlertActionRule{}, &models.AlertActionExecution{}); err != nil {
		t.Fatal(err)
	}
	permissions := []models.Permission{}
	for _, code := range []string{"alert:event:list", "alert:ai-analysis:list", "alert:ai-analysis:edit", "datasource:list"} {
		permissions = append(permissions, models.Permission{Name: code, Code: code, Status: 1})
	}
	user := models.User{Username: "analyst", Status: 1, Roles: []models.Role{{Name: "analyst", Code: "analyst", Status: 1, Permissions: permissions}}}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	key, err := encryptSecret("test-key")
	if err != nil {
		t.Fatal(err)
	}
	model := models.AIModel{Name: "diagnostic", Provider: "openai", BaseURL: "http://unused.invalid", Model: "test", SecretCipher: key, Status: 1}
	db.Create(&model)
	source := models.Datasource{Name: "metrics", Type: "prometheus", URL: "http://unused.invalid", Status: 1}
	db.Create(&source)
	center := models.FaultCenter{Name: "production", Status: 1}
	db.Create(&center)
	now := time.Now().UTC().Truncate(time.Millisecond)
	a := models.AlertInstance{FaultCenterID: center.ID, Source: alertRuleSource, SourceID: 1, DatasourceID: source.ID, Title: "CPU 高", Severity: "P0", Status: alertStatusFiring, Labels: `{"instance":"node-1:9100","service":"payment"}`, LastResult: `{"expression":"cpu_usage","value":"95","labels":{"instance":"node-1:9100"},"token":"hidden-secret"}`, StartedAt: now, FiredAt: &now, LastEventAt: now}
	if err := db.Create(&a).Error; err != nil {
		t.Fatal(err)
	}
	r := models.AlertAIAnalysisRule{FaultCenterID: center.ID, Name: "CPU 分析", ModelID: model.ID, Priority: 100, Status: 1, Version: 1, ExecutionUserID: user.ID, MatchersJSON: `[{"namespace":"labels","key":"instance","value":"node-1:9100"}]`, AllowedDatasourceIDsJSON: "[]"}
	if err := db.Create(&r).Error; err != nil {
		t.Fatal(err)
	}
	return db, a, r
}
func queuedAIJob(t *testing.T, a models.AlertInstance, r models.AlertAIAnalysisRule) models.AlertAIAnalysisJob {
	t.Helper()
	if err := enqueueAlertAIAnalysis(a, r, time.Now()); err != nil {
		t.Fatal(err)
	}
	var j models.AlertAIAnalysisJob
	if database.DB.Where("alert_id = ?", a.ID).Order("id DESC").First(&j).Error != nil {
		t.Fatal("job not created")
	}
	return j
}
func startAIJob(t *testing.T, j *models.AlertAIAnalysisJob) {
	t.Helper()
	now := time.Now()
	until := now.Add(100 * time.Second)
	j.Status = "running"
	j.LeaseToken = "test-lease"
	j.StartedAt = &now
	j.LeaseUntil = &until
	if err := database.DB.Save(j).Error; err != nil {
		t.Fatal(err)
	}
}

func TestAlertAIQueriesPrometheusWithoutWaitingNotification(t *testing.T) {
	db, a, r := setupAlertAITest(t)
	var queries atomic.Int32
	prom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		queries.Add(1)
		if !strings.Contains(req.URL.Query().Get("query"), "node-1:9100") {
			t.Error("node scope missing")
		}
		if req.URL.Query().Get("limit") != "50" {
			t.Error("query limit missing")
		}
		kind := "vector"
		result := map[string]interface{}{"metric": map[string]string{"instance": "node-1:9100"}, "value": []interface{}{time.Now().Unix(), "95"}}
		if strings.HasSuffix(req.URL.Path, "query_range") {
			kind = "matrix"
			delete(result, "value")
			result["values"] = [][]interface{}{{time.Now().Unix(), "95"}}
			if req.URL.Query().Get("step") != "15" {
				t.Error("range step missing")
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "success", "data": map[string]interface{}{"resultType": kind, "result": []interface{}{result}}})
	}))
	defer prom.Close()
	db.Model(&models.Datasource{}).Where("id = ?", a.DatasourceID).Update("url", prom.URL)
	entered, release := make(chan struct{}), make(chan struct{})
	var rounds atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		n := rounds.Add(1)
		var input map[string]interface{}
		json.NewDecoder(req.Body).Decode(&input)
		raw := encodeJSON(input)
		if strings.Contains(raw, "execute_platform_operation") || strings.Contains(raw, "hidden-secret") {
			t.Error("unsafe tools or secrets exposed")
		}
		if n == 1 {
			close(entered)
			<-release
		}
		if n <= 2 {
			name := "query_analysis_metric_range"
			if n == 2 {
				name = "query_analysis_metric_instant"
				if !strings.Contains(raw, "ev-1") {
					t.Error("evidence not returned to model")
				}
			}
			args := encodeJSON(map[string]interface{}{"datasource_id": a.DatasourceID, "query": `sum by (mode) (rate(node_cpu_seconds_total{instance="node-1:9100"}[5m]))`})
			json.NewEncoder(w).Encode(map[string]interface{}{"choices": []interface{}{map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "tool_calls": []interface{}{map[string]interface{}{"id": fmt.Sprintf("call-%d", n), "type": "function", "function": map[string]interface{}{"name": name, "arguments": args}}}}, "finish_reason": "tool_calls"}}})
			return
		}
		report := `{"summary":"CPU 持续升高，需进一步定位进程","assessment":"hypothesis","possible_causes":[{"text":"计算负载升高","evidence_ids":["ev-1","ev-2"]}],"suggestions":["检查进程 CPU 指标"],"missing_information":["缺少进程指标"]}`
		json.NewEncoder(w).Encode(map[string]interface{}{"choices": []interface{}{map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "content": report}, "finish_reason": "stop"}}})
	}))
	defer model.Close()
	db.Model(&models.AIModel{}).Where("id = ?", r.ModelID).Update("base_url", model.URL)
	j := queuedAIJob(t, a, r)
	startAIJob(t, &j)
	done := make(chan struct{})
	go func() { defer close(done); runAlertAIAnalysis(j) }()
	released := false
	defer func() {
		if !released {
			close(release)
		}
		<-done
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("model not started")
	}
	var notifications atomic.Int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { notifications.Add(1); w.WriteHeader(200) }))
	defer receiver.Close()
	cipher, _ := encryptSecret(receiver.URL)
	channel := models.NotificationChannel{Name: "ops", Type: "webhook", WebhookURLCipher: cipher, Status: 1}
	db.Create(&channel)
	db.Create(&models.AlertNotificationConfig{FaultCenterID: a.FaultCenterID, DefaultChannelIDs: fmt.Sprint(channel.ID), RepeatIntervalSeconds: 3600, RecoveryNotification: true})
	sent := make(chan struct{})
	go func() {
		defer close(sent)
		sendPromRuleNotification(models.AlertRule{ID: 1, Name: "CPU 高", FaultCenterID: a.FaultCenterID}, models.Datasource{ID: a.DatasourceID, Name: "metrics"}, &a, false, time.Now())
	}()
	select {
	case <-sent:
	case <-time.After(3 * time.Second):
		t.Fatal("notification blocked by model")
	}
	if notifications.Load() != 1 {
		t.Fatalf("expected original alert immediately, got %d", notifications.Load())
	}
	last := a.LastNotifiedAt
	close(release)
	released = true
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("analysis did not finish")
	}
	db.First(&j, j.ID)
	if j.Status != "succeeded" || queries.Load() != 2 || !strings.Contains(j.EvidenceJSON, "node_cpu_seconds_total") {
		t.Fatalf("analysis failed: %+v queries %d", j, queries.Load())
	}
	if notifications.Load() != 1 {
		t.Fatal("AI sent extra notification")
	}
	var after models.AlertInstance
	db.First(&after, a.ID)
	if last == nil || after.LastNotifiedAt == nil || !after.LastNotifiedAt.Equal(*last) {
		t.Fatal("analysis changed notification state")
	}
}

func TestAlertAIEligibilityPriorityAndRounds(t *testing.T) {
	db, a, r := setupAlertAITest(t)
	higher := r
	higher.ID = 0
	higher.Name = "优先"
	higher.Priority = 1
	db.Create(&higher)
	var cursor uint
	if err := scanAlertAIAnalyses(time.Now(), &cursor); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		cursor = 0
		if err := scanAlertAIAnalyses(time.Now(), &cursor); err != nil {
			t.Fatal(err)
		}
	}
	var jobs []models.AlertAIAnalysisJob
	db.Find(&jobs)
	if len(jobs) != 1 || jobs[0].RuleID != higher.ID {
		t.Fatalf("dedup or priority failed: %+v", jobs)
	}
	db.Model(&a).Update("status", alertStatusRecovering)
	db.Model(&a).Update("status", alertStatusFiring)
	cursor = 0
	scanAlertAIAnalyses(time.Now(), &cursor)
	var count int64
	db.Model(&models.AlertAIAnalysisJob{}).Count(&count)
	if count != 1 {
		t.Fatal("recovery oscillation created new analysis")
	}
	next := a.FiredAt.Add(time.Minute)
	db.Model(&a).Updates(map[string]interface{}{"fired_at": next, "severity": "P1"})
	cursor = 0
	scanAlertAIAnalyses(time.Now(), &cursor)
	db.Model(&models.AlertAIAnalysisJob{}).Count(&count)
	if count != 2 {
		t.Fatal("new firing round not analyzed")
	}
	db.Model(&a).Updates(map[string]interface{}{"status": alertStatusResolved, "fired_at": next.Add(time.Minute)})
	cursor = 0
	scanAlertAIAnalyses(time.Now(), &cursor)
	db.Model(&models.AlertAIAnalysisJob{}).Count(&count)
	if count != 2 {
		t.Fatal("historical resolved alert analyzed")
	}
}

func TestAlertAIToolsRejectScopeAndRevocation(t *testing.T) {
	db, a, r := setupAlertAITest(t)
	j := queuedAIJob(t, a, r)
	runtime := &alertAIToolRuntime{job: j, started: time.Now()}
	tool, _ := runtime.registry().Get("query_analysis_metric_range")
	call := aicore.ToolCall{Tool: tool.Spec.Name, Params: map[string]interface{}{"datasource_id": float64(999), "query": "up"}}
	if result := tool.Handler(context.Background(), call); result.ErrorCode != "FORBIDDEN" {
		t.Fatalf("scope bypass: %+v", result)
	}
	db.Model(&models.User{}).Where("id = ?", r.ExecutionUserID).Update("status", 0)
	call.Params["datasource_id"] = float64(a.DatasourceID)
	if result := tool.Handler(context.Background(), call); result.ErrorCode != "FORBIDDEN" {
		t.Fatal("disabled user queried metrics")
	}
	db.Model(&models.User{}).Where("id = ?", r.ExecutionUserID).Update("status", 1)
	until := time.Now().Add(time.Hour)
	db.Model(&a).Update("silenced_until", until)
	if _, _, err := validateAlertAIJob(context.Background(), j); err == nil {
		t.Fatal("silenced alert still eligible")
	}
	for _, s := range runtime.registry().Specs() {
		if s.Risk != aicore.RiskReadonlyLow && s.Risk != aicore.RiskReadonlyExpensive {
			t.Fatal("write tool registered")
		}
	}
}

func TestAlertAIReportRequiresEvidence(t *testing.T) {
	answer := `{"summary":"猜测","assessment":"supported","possible_causes":[],"suggestions":[],"missing_information":[]}`
	report, status, err := validateAIReport(answer, nil)
	if err != nil || status != "insufficient" || report.Assessment != "insufficient" {
		t.Fatal("unqueried answer treated as diagnosis")
	}
	answer = `{"summary":"猜测","assessment":"hypothesis","possible_causes":[{"text":"原因","evidence_ids":["invented"]}]}`
	if _, _, err := validateAIReport(answer, []alertAIEvidence{{ID: "ev-1", Status: "succeeded", HasData: true}}); err == nil {
		t.Fatal("fabricated evidence accepted")
	}
	for _, q := range []string{"up offset 1h", "up @ 0", "rate(foo[31m])", "rate(foo[1y])", "rate(foo[999999999999h])"} {
		if validateAnalysisPromQL(q) == nil {
			t.Fatalf("unbounded query accepted: %s", q)
		}
	}
	if err := validateAnalysisPromQL(`rate(node_cpu_seconds_total{instance="node-1:9100"}[5m])`); err != nil {
		t.Fatal(err)
	}
}

func TestAlertAIRulesAPIAndSecretProjection(t *testing.T) {
	db, a, r := setupAlertAITest(t)
	userID := r.ExecutionUserID
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", userID); c.Next() })
	router.GET("/fault-centers/:id/ai-analysis-rules", ListAlertAIRules)
	router.POST("/fault-centers/:id/ai-analysis-rules", SaveAlertAIRule)
	router.GET("/fault-centers/:id/ai-analysis-options", AlertAIOptions)
	router.GET("/fault-centers/:id/alerts/:alertId/ai-analysis", GetAlertAIAnalysis)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		return w
	}
	root := fmt.Sprintf("/fault-centers/%d", a.FaultCenterID)
	response := request("GET", root+"/ai-analysis-options", "")
	if response.Code != 200 || strings.Contains(response.Body.String(), "base_url") || strings.Contains(response.Body.String(), "test-key") || strings.Contains(response.Body.String(), "unused.invalid") {
		t.Fatal("options leaked model credentials or address")
	}
	body := fmt.Sprintf(`{"name":"第二条","model_id":%d,"priority":20,"status":1,"matchers":[{"namespace":"labels","key":"service","value":"payment"}],"allowed_datasource_ids":[]}`, r.ModelID)
	if response := request("POST", root+"/ai-analysis-rules", body); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	if response := request("POST", root+"/ai-analysis-rules", body); response.Code == 200 {
		t.Fatal("duplicate name accepted")
	}
	j := queuedAIJob(t, a, r)
	response = request("GET", fmt.Sprintf("%s/alerts/%d/ai-analysis", root, a.ID), "")
	if response.Code != 200 || strings.Contains(response.Body.String(), "input_json") || strings.Contains(response.Body.String(), "hidden-secret") {
		t.Fatal("unsafe report projection")
	}
	db.Model(&models.AlertAIAnalysisJob{}).Where("id = ?", j.ID).Update("status", "succeeded")
	if response := request("GET", fmt.Sprintf("/fault-centers/999/alerts/%d/ai-analysis", a.ID), ""); response.Code == 200 {
		t.Fatal("cross center read accepted")
	}
	userID = 999
	if response := request("GET", root+"/ai-analysis-rules", ""); response.Code != 403 {
		t.Fatal("unauthorized rule access")
	}
}

func TestAlertAIRequiresFiringTimestamp(t *testing.T) {
	db, alert, rule := setupAlertAITest(t)
	alert.FiredAt = nil
	if key := alertAIEpisode(alert); key != "" {
		t.Fatalf("alert without firing timestamp has an episode: %q", key)
	}
	if err := enqueueAlertAIAnalysis(alert, rule, time.Now()); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&models.AlertAIAnalysisJob{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("analysis queued without an actual firing episode")
	}
}
