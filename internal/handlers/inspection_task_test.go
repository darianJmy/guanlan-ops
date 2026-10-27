package handlers

import (
	"encoding/json"
	"guanlan/internal/models"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestInspectionTaskAllowsTenSecondInterval(t *testing.T) {
	req := CreateInspectionItemReq{
		Type: "http", IntervalSeconds: 10, FailureThreshold: 1, RecoveryThreshold: 1,
		Severity: "P2", Status: 1, Config: `{"execution_nodes":[{"node_type":"direct","node_id":0}]}`,
	}
	if err := validateInspectionItemReq(req); err != nil && err.Error() == "拨测周期应为 10 秒至 24 小时" {
		t.Fatalf("expected 10-second interval to pass interval validation, got %v", err)
	}
	req.IntervalSeconds = 9
	if err := validateInspectionItemReq(req); err == nil || err.Error() != "拨测周期应为 10 秒至 24 小时" {
		t.Fatalf("expected 9-second interval to be rejected, got %v", err)
	}
}

func TestInspectionSeverityDefaultsAndNormalizes(t *testing.T) {
	if got := normalizeInspectionSeverity(""); got != "P2" {
		t.Fatalf("expected empty severity to default to P2, got %q", got)
	}
	if got := normalizeInspectionSeverity(" p0 "); got != "P0" {
		t.Fatalf("expected severity to normalize to P0, got %q", got)
	}
}

func TestInspectionExecutionNodesRequireCurrentArray(t *testing.T) {
	nodes := inspectionExecutionNodes(`{"execution_nodes":[{"node_type":"ssh","node_id":2},{"node_type":"agent","node_id":3},{"node_type":"ssh","node_id":2}]}`)
	if len(nodes) != 2 || inspectionNodeKey(nodes[0]) != "ssh:2" || inspectionNodeKey(nodes[1]) != "agent:3" {
		t.Fatalf("unexpected execution nodes: %#v", nodes)
	}
	if legacy := inspectionExecutionNodes(`{"node_type":"ssh","node_id":8}`); len(legacy) != 0 {
		t.Fatalf("legacy execution node config must be rejected: %#v", legacy)
	}
}

func TestInspectionExecutionRejectsPrometheusProbe(t *testing.T) {
	status, result, _ := executeInspection(models.InspectionItem{Type: "prometheus", Config: `{}`})
	if status != "error" || !strings.Contains(result, "不支持的拨测类型") {
		t.Fatalf("Prometheus probe compatibility must be removed: status=%q result=%q", status, result)
	}
}

func TestInspectionTaskDueUsesConfiguredInterval(t *testing.T) {
	now := time.Now()
	lastRun := now.Add(-59 * time.Second)
	item := models.InspectionItem{Status: 1, IntervalSeconds: 60, LastRunAt: &lastRun}
	if inspectionTaskDue(item, now) {
		t.Fatal("task should not be due before its interval")
	}
	lastRun = now.Add(-60 * time.Second)
	item.LastRunAt = &lastRun
	if !inspectionTaskDue(item, now) {
		t.Fatal("task should be due when its interval elapsed")
	}
}

func TestInspectionLabelsAreNormalizedAndMatchFilters(t *testing.T) {
	raw, err := normalizeInspectionLabels(`{" env ":" production ","service":"gateway"}`)
	if err != nil {
		t.Fatalf("normalize labels: %v", err)
	}
	labels := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &labels); err != nil {
		t.Fatalf("decode normalized labels: %v", err)
	}
	if labels["env"] != "production" || labels["service"] != "gateway" {
		t.Fatalf("unexpected labels: %#v", labels)
	}
	if !inspectionLabelsMatch(labels, "ENV", "prod") {
		t.Fatal("expected case-insensitive key and partial value match")
	}
	if inspectionLabelsMatch(labels, "region", "") {
		t.Fatal("unexpected match for missing key")
	}
	if !inspectionLabelsContain(labels, "ENV") || !inspectionLabelsContain(labels, "gate") {
		t.Fatal("expected label search to match both keys and partial values")
	}
	if inspectionLabelsContain(labels, "staging") {
		t.Fatal("unexpected fuzzy label match")
	}
	if got := inspectionLabelSummary(labels); got != "env=production, service=gateway" {
		t.Fatalf("unexpected label summary: %q", got)
	}
}

func TestInspectionTaskKeywordMatchesNameOnly(t *testing.T) {
	item := models.InspectionItem{Name: "生产API接口", Target: "https://api.example.com", Labels: `{"env":"production","service":"gateway"}`}
	for _, keyword := range []string{"生产", " api ", "接口"} {
		if !inspectionItemMatchesKeyword(item, keyword) {
			t.Fatalf("expected keyword %q to match task", keyword)
		}
	}
	for _, keyword := range []string{"example.com", "ENV", "gate", "staging"} {
		if inspectionItemMatchesKeyword(item, keyword) {
			t.Fatalf("unexpected task keyword match for %q", keyword)
		}
	}
}

func TestTaskAlertSnapshotKeepsLabelNamespacesSeparate(t *testing.T) {
	runtimeLabels, snapshot := buildTaskAlertSnapshot("scheduled-task", "每日巡检", "script", "check.sh", "fail", "exit 1", "检查核心服务", `{"env":"prod","service":"order"}`)
	alert := models.AlertInstance{Source: "scheduled-task", Severity: "P2", Labels: runtimeLabels, LastResult: snapshot}
	namespaces := alertLabelNamespaces(alert)
	if namespaces["system_labels"]["alertname"] != "每日巡检" || namespaces["system_labels"]["severity"] != "P2" {
		t.Fatalf("unexpected system labels: %#v", namespaces["system_labels"])
	}
	if namespaces["labels"]["alertname"] != "" || namespaces["labels"]["task_name"] != "每日巡检" || namespaces["labels"]["target"] != "check.sh" {
		t.Fatalf("unexpected runtime labels: %#v", namespaces["labels"])
	}
	if namespaces["external_labels"]["env"] != "prod" || namespaces["external_labels"]["service"] != "order" {
		t.Fatalf("unexpected external labels: %#v", namespaces["external_labels"])
	}
	if len(namespaces["datasource_labels"]) != 0 {
		t.Fatalf("scheduled tasks must not invent datasource labels: %#v", namespaces["datasource_labels"])
	}
}

func TestInhibitionEqualLabelsRequireMatchingValues(t *testing.T) {
	source := namespacedAlert(`{"instance":"node-1"}`, `{"env":"prod"}`, `{"service":"gateway"}`)
	target := namespacedAlert(`{"instance":"node-1"}`, `{"env":"prod"}`, `{"service":"checkout"}`)

	if !inhibitionEqualLabelsMatch(source, target, []models.AlertLabelSelector{{Namespace: "labels", Key: "instance"}, {Namespace: "datasource_labels", Key: "env"}}) {
		t.Fatal("alerts with the same configured label value should match")
	}
	if inhibitionEqualLabelsMatch(source, target, []models.AlertLabelSelector{{Namespace: "external_labels", Key: "service"}}) {
		t.Fatal("all configured equal labels must have the same value")
	}
	if inhibitionEqualLabelsMatch(source, target, []models.AlertLabelSelector{{Namespace: "datasource_labels", Key: "region"}}) {
		t.Fatal("a missing equal label must not match")
	}
	if !inhibitionEqualLabelsMatch(source, target, nil) {
		t.Fatal("an empty equal-label configuration should not restrict matching")
	}
}

func TestAlertMatchersKeepNamespacesIndependent(t *testing.T) {
	alert := namespacedAlert(`{"env":"prom"}`, `{"env":"test"}`, `{"env":"production"}`)
	matchers := []models.AlertLabelMatcher{
		{Namespace: "labels", Key: "env", Value: "prom"},
		{Namespace: "datasource_labels", Key: "env", Value: "test"},
		{Namespace: "external_labels", Key: "env", Value: "production"},
	}
	if !alertMatchesLabelMatchers(alert, matchers) {
		t.Fatal("same-named labels from different namespaces must remain independently matchable")
	}
	matchers[1].Value = "production"
	if alertMatchesLabelMatchers(alert, matchers) {
		t.Fatal("a different label value must not match")
	}
}

func TestNotificationRouteChannelsPreferFirstMatchingRoute(t *testing.T) {
	routes := []models.AlertNotificationRoute{
		{LabelNamespace: "datasource_labels", LabelKey: "env", LabelValue: "test", ChannelIDs: "8,9", Status: 1},
		{LabelNamespace: "external_labels", LabelKey: "env", LabelValue: "production", ChannelIDs: "10", Status: 1},
	}
	got := notificationRouteChannelIDs(namespacedAlert(`{"env":"prom"}`, `{"env":"test"}`, `{"env":"production"}`), routes, []uint{3})
	if len(got) != 2 || got[0] != 8 || got[1] != 9 {
		t.Fatalf("expected the first matching route, got %#v", got)
	}
}

func TestNotificationRouteChannelsFallBackToDefaults(t *testing.T) {
	routes := []models.AlertNotificationRoute{{LabelNamespace: "labels", LabelKey: "namespace", LabelValue: "store", ChannelIDs: "8", Status: 1}}
	got := notificationRouteChannelIDs(namespacedAlert(`{"namespace":"iot"}`, `{}`, `{}`), routes, []uint{3, 4})
	if len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Fatalf("expected default channels, got %#v", got)
	}
}

func TestInspectionLabelsRejectInvalidKey(t *testing.T) {
	if _, err := normalizeInspectionLabels(`{"invalid key":"value"}`); err == nil {
		t.Fatal("expected invalid label key to be rejected")
	}
}

func TestInspectionLabelVariablesRenderInNotificationTemplate(t *testing.T) {
	payload := map[string]interface{}{
		"labels": map[string]string{"env": "production", "service": "gateway"},
	}
	content, err := renderNotificationMarkdown("环境：${labels.env}\n标签：${labels}", payload)
	if err != nil || content != "环境：production\n标签：env=production, service=gateway" {
		t.Fatalf("unexpected rendered notification: %q", content)
	}
}

func TestInspectionAlertDetailRendersTaskAndExternalLabels(t *testing.T) {
	item := models.InspectionItem{
		Name:   "登录接口",
		Type:   "http",
		Target: "https://api.example.com/health",
		Config: `{"alert_detail":"${external_labels.env} 环境的 ${labels.task_name} 在 ${labels.probe_node} 检测到 ${labels.target} 异常"}`,
	}
	node := inspectionExecutionNode{NodeType: "direct", NodeName: "服务端直连"}
	got := renderInspectionAlertDetail(item, node, map[string]string{"env": "生产"})
	want := "生产 环境的 登录接口 在 服务端直连 检测到 https://api.example.com/health 异常"
	if got != want {
		t.Fatalf("unexpected alert detail: %q", got)
	}
}

func TestInspectionFailureNotificationDueHonorsRepeatInterval(t *testing.T) {
	now := time.Now()
	lastAlert := now.Add(-29 * time.Minute)
	if inspectionFailureNotificationDue(true, &lastAlert, 1800, now) {
		t.Fatal("repeat notification should wait for its configured interval")
	}
	lastAlert = now.Add(-30 * time.Minute)
	if !inspectionFailureNotificationDue(true, &lastAlert, 1800, now) {
		t.Fatal("repeat notification should be due after its interval")
	}
	if !inspectionFailureNotificationDue(false, nil, 1800, now) {
		t.Fatal("initial notification should be due immediately")
	}
}

func TestInspectionRecoveryDueRequiresConsecutiveSuccesses(t *testing.T) {
	if inspectionRecoveryDue(2, 3) {
		t.Fatal("recovery should not be due before the success threshold")
	}
	if !inspectionRecoveryDue(3, 3) {
		t.Fatal("recovery should be due at the success threshold")
	}
	if inspectionRecoveryDue(0, 3) {
		t.Fatal("recovery cannot be due without a successful probe")
	}
}

func TestInspectionRecordStatusCodeSupportsSuccessAndFailurePayloads(t *testing.T) {
	if got := inspectionRecordStatusCode(`{"status_code":204}`); got != 204 {
		t.Fatalf("unexpected top-level status code: %d", got)
	}
	if got := inspectionRecordStatusCode(`{"error":"unexpected status","response":{"status_code":503}}`); got != 503 {
		t.Fatalf("unexpected nested status code: %d", got)
	}
	if got := inspectionRecordStatusCode(`{"error":"timeout"}`); got != 0 {
		t.Fatalf("expected missing status code to be zero, got %d", got)
	}
}

func TestInspectionDurationPercentile(t *testing.T) {
	values := []int64{100, 500, 200, 300, 400}
	if got := inspectionDurationPercentile(values, 0.95); got != 400 {
		t.Fatalf("unexpected p95 duration: %d", got)
	}
	if got := inspectionDurationPercentile(nil, 0.95); got != 0 {
		t.Fatalf("empty percentile should be zero, got %d", got)
	}
}

func TestInspectionStatusCodeGroup(t *testing.T) {
	cases := map[int]string{200: "2xx", 302: "3xx", 404: "4xx", 503: "5xx", 0: "other"}
	for code, expected := range cases {
		if got := inspectionStatusCodeGroup(code); got != expected {
			t.Fatalf("status %d: expected %s, got %s", code, expected, got)
		}
	}
}

func TestInspectionAlertStateStartsPendingOnFirstFailure(t *testing.T) {
	state := nextInspectionAlertState("", false, 1, 3, false)
	if state != "pending" {
		t.Fatalf("expected pending after first failure, got %q", state)
	}
}

func TestInspectionAlertStateFiresAtConfiguredThreshold(t *testing.T) {
	state := nextInspectionAlertState("pending", false, 3, 3, false)
	if state != "firing" {
		t.Fatalf("expected firing at threshold, got %q", state)
	}
}

func TestInspectionAlertStateResolvesPendingImmediately(t *testing.T) {
	state := nextInspectionAlertState("pending", true, 0, 3, false)
	if state != "resolved" {
		t.Fatalf("expected a pending alert to resolve on success, got %q", state)
	}
}

func TestInspectionAlertStateEntersRecoveringDuringRecoveryWait(t *testing.T) {
	if state := nextInspectionAlertState("firing", true, 0, 3, false); state != "recovering" {
		t.Fatalf("expected recovering during recovery wait, got %q", state)
	}
	if state := nextInspectionAlertState("recovering", true, 0, 3, true); state != "resolved" {
		t.Fatalf("expected resolved after recovery wait, got %q", state)
	}
}

func TestInspectionAlertStateReturnsToFiringWhenRecoveryFails(t *testing.T) {
	if state := nextInspectionAlertState("recovering", false, 1, 3, false); state != "firing" {
		t.Fatalf("expected firing when recovery fails, got %q", state)
	}
}

func TestGroupNotificationDueWaitsBeforeFirstNotification(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.Local)
	state := models.AlertGroupState{FirstSeenAt: now.Add(-29 * time.Second)}
	if groupNotificationDue(state, 30, 300, now) {
		t.Fatal("first grouped notification should wait for the configured group wait")
	}
	state.FirstSeenAt = now.Add(-30 * time.Second)
	if !groupNotificationDue(state, 30, 300, now) {
		t.Fatal("first grouped notification should be due after the configured group wait")
	}
}

func TestGroupNotificationDueUsesIntervalForGroupUpdates(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.Local)
	lastNotifiedAt := now.Add(-4 * time.Minute)
	state := models.AlertGroupState{LastNotifiedAt: &lastNotifiedAt}
	if groupNotificationDue(state, 30, 300, now) {
		t.Fatal("a group update should wait for the configured group interval")
	}
	lastNotifiedAt = now.Add(-5 * time.Minute)
	state.LastNotifiedAt = &lastNotifiedAt
	if !groupNotificationDue(state, 30, 300, now) {
		t.Fatal("a group update should be due after the configured group interval")
	}
}

func TestSilenceRuleMatchesEveryConfiguredLabel(t *testing.T) {
	alert := namespacedAlert(`{"instance":"node-1"}`, `{"env":"prod"}`, `{"service":"gateway"}`)
	if silenceRuleMatchesAlert(models.AlertSilenceRule{}, alert) {
		t.Fatal("silence rule without matchers must not match an alert")
	}
	rule := models.AlertSilenceRule{Matchers: []models.AlertLabelMatcher{{Namespace: "datasource_labels", Key: "env", Value: "prod"}, {Namespace: "external_labels", Key: "service", Value: "gateway"}}}
	if !silenceRuleMatchesAlert(rule, alert) {
		t.Fatal("silence rule should match when every configured label matches")
	}
	rule.Matchers = []models.AlertLabelMatcher{{Namespace: "datasource_labels", Key: "env", Value: "test"}}
	if silenceRuleMatchesAlert(rule, alert) {
		t.Fatal("silence rule should not match a different label value")
	}
}

func namespacedAlert(labels, datasourceLabels, externalLabels string) models.AlertInstance {
	return models.AlertInstance{LastResult: `{"labels":` + labels + `,"datasource_labels":` + datasourceLabels + `,"external_labels":` + externalLabels + `}`}
}

func TestSilenceRuleActiveAtSupportsOnceAndWeeklyWindows(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	start := time.Date(2026, 8, 20, 9, 0, 0, 0, location)
	end := time.Date(2026, 8, 22, 18, 0, 0, 0, location)
	once := models.AlertSilenceRule{SilenceType: "once", StartAt: &start, EndAt: &end, Status: 1}
	if !silenceRuleActiveAt(once, time.Date(2026, 8, 21, 12, 0, 0, 0, location)) {
		t.Fatal("one-time rule should be active inside its datetime range")
	}
	if silenceRuleActiveAt(once, time.Date(2026, 8, 23, 12, 0, 0, 0, location)) {
		t.Fatal("one-time rule should be inactive after its datetime range")
	}
	weekly := models.AlertSilenceRule{SilenceType: "weekly", Weekdays: []int{1, 2, 3, 4, 5}, StartTime: "09:00", EndTime: "18:00", Status: 1}
	if silenceRuleActiveAt(weekly, time.Date(2026, 8, 21, 8, 59, 0, 0, location)) {
		t.Fatal("weekly rule should be inactive before its daily start time")
	}
	if !silenceRuleActiveAt(weekly, time.Date(2026, 8, 21, 10, 0, 0, 0, location)) {
		t.Fatal("weekly rule should be active on a selected weekday inside its daily time window")
	}
	overnight := models.AlertSilenceRule{SilenceType: "weekly", Weekdays: []int{5}, StartTime: "22:00", EndTime: "06:00", Status: 1}
	if !silenceRuleActiveAt(overnight, time.Date(2026, 8, 21, 23, 0, 0, 0, location)) {
		t.Fatal("overnight daily rule should be active after its start time")
	}
	if !silenceRuleActiveAt(overnight, time.Date(2026, 8, 22, 5, 0, 0, 0, location)) {
		t.Fatal("the after-midnight portion should belong to the selected previous weekday")
	}
	if silenceRuleActiveAt(overnight, time.Date(2026, 8, 22, 22, 0, 0, 0, location)) {
		t.Fatal("weekly rule should be inactive on an unselected weekday")
	}
	disabled := once
	disabled.Status = 0
	if silenceRuleActiveAt(disabled, time.Date(2026, 8, 21, 12, 0, 0, 0, location)) {
		t.Fatal("disabled silence rule must never become active")
	}
}

func TestNormalizeSilenceSchedule(t *testing.T) {
	weekdays, encoded, start, end, err := normalizeSilenceSchedule("weekly", []int{5, 1, 5}, "09:00", "18:00")
	if err != nil || !reflect.DeepEqual(weekdays, []int{1, 5}) || encoded != "[1,5]" || start != "09:00" || end != "18:00" {
		t.Fatalf("unexpected normalized weekly schedule: weekdays=%v encoded=%q start=%q end=%q err=%v", weekdays, encoded, start, end, err)
	}
	if _, _, _, _, err := normalizeSilenceSchedule("weekly", nil, "09:00", "18:00"); err == nil {
		t.Fatal("weekly schedule should require at least one weekday")
	}
	if _, _, _, _, err := normalizeSilenceSchedule("weekly", []int{1}, "09:00", "09:00"); err == nil {
		t.Fatal("weekly schedule should reject identical start and end times")
	}
}
