package handlers

import (
	"encoding/json"
	"guanlan/internal/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestQueryPromRuleDatasourceUsesDatasourceCurrentTime(t *testing.T) {
	const expression = `rate(node_cpu_seconds_total{mode="idle"}[5m])`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" || r.URL.Query().Get("query") != expression {
			t.Errorf("unexpected query request: %s", r.URL)
		}
		if r.URL.Query().Has("time") {
			t.Errorf("current alert query must not supply a local evaluation time: %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"instance":"node-1"},"value":[1792770623,"100"]}]}}`))
	}))
	defer server.Close()
	series, err := queryPromRuleDatasource(models.Datasource{URL: server.URL}, expression)
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 || series[0].Value != "100" || series[0].Labels["instance"] != "node-1" {
		t.Fatalf("expected current datasource sample despite its different clock: %#v", series)
	}
}

func TestRenderPromRuleTemplateSupportsLabelVariables(t *testing.T) {
	labels := map[string]string{"job": "api", "instance": "10.0.0.8:9090"}
	datasourceLabels := map[string]string{"env": "production"}
	externalLabels := map[string]string{"business": "order"}
	got := renderPromRuleTemplate("任务 ${labels.job}，实例 ${labels.instance}，环境 ${datasource_labels.env}，业务 ${external_labels.business}", labels, datasourceLabels, externalLabels, "1")
	want := "任务 api，实例 10.0.0.8:9090，环境 production，业务 order"
	if got != want {
		t.Fatalf("renderPromRuleTemplate() = %q, want %q", got, want)
	}
}

func TestBuildPromAlertResultSnapshotContainsRelevantEvaluationContext(t *testing.T) {
	rule := models.AlertRule{
		ID: 12, GroupID: 3, Name: "HighLatency", Description: "接口延迟告警", Expression: "latency_seconds > 0.5",
		ForSeconds: 600, RecoveryWaitSeconds: 60, NoDataPolicy: "resolve", EvaluationIntervalSeconds: 30,
		Severity: "P1",
		Labels:   `{"team":"platform","severity":"P1"}`, Annotations: `{"description":"api-1 is slow"}`,
		FaultCenterID: 8, Status: 1,
	}
	datasource := models.Datasource{ID: 5, Name: "prod-prometheus", Type: "prometheus", Category: "observability", URL: "http://prometheus:9090", Labels: `{"cluster":"prod-a"}`, Env: "prod", Region: "cn-east"}
	labels := map[string]string{"instance": "api-1", "job": "api"}
	datasourceLabels := promDatasourceLabels(datasource)
	externalLabels := map[string]string{"team": "platform"}
	annotations := map[string]string{"description": "api-1 is slow"}
	item := promVectorSeries{Value: "0.8", ConditionID: 7, Condition: "> 0.5", Severity: "P1", ForSeconds: 600}

	snapshot := buildPromAlertResultSnapshot(rule, datasource, "fingerprint-123", item, labels, datasourceLabels, externalLabels, annotations)
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("snapshot must be JSON serializable: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("snapshot must decode as JSON: %v", err)
	}
	if len(decoded) != 12 {
		t.Fatalf("snapshot should only contain directly relevant evaluation fields: %#v", decoded)
	}
	if decoded["alertname"] != "HighLatency" || decoded["datasource"] != "prod-prometheus" || decoded["fingerprint"] != "fingerprint-123" || decoded["datasource_id"] != nil || decoded["severity"] != "P1" || decoded["description"] != "接口延迟告警" || decoded["expression"] != "latency_seconds > 0.5" || decoded["condition"] != "> 0.5" || decoded["value"] != "0.8" {
		t.Fatalf("missing evaluation fields: %#v", decoded)
	}
	gotLabels := decoded["labels"].(map[string]interface{})
	if len(gotLabels) != 2 || gotLabels["instance"] != "api-1" || gotLabels["job"] != "api" || gotLabels["alertname"] != nil || gotLabels["team"] != nil || gotLabels["severity"] != nil {
		t.Fatalf("labels must contain only labels returned by PromQL: %#v", gotLabels)
	}
	gotExternalLabels := decoded["external_labels"].(map[string]interface{})
	if len(gotExternalLabels) != 1 || gotExternalLabels["team"] != "platform" || gotExternalLabels["severity"] != nil || gotExternalLabels["cluster"] != nil || gotExternalLabels["instance"] != nil {
		t.Fatalf("external labels must be separate from PromQL labels: %#v", gotExternalLabels)
	}
	gotDatasourceLabels := decoded["datasource_labels"].(map[string]interface{})
	if len(gotDatasourceLabels) != 1 || gotDatasourceLabels["cluster"] != "prod-a" || gotDatasourceLabels["env"] != nil || gotDatasourceLabels["region"] != nil || gotDatasourceLabels["domain"] != nil || gotDatasourceLabels["datasource"] != nil {
		t.Fatalf("datasource labels must contain only explicitly configured labels: %#v", gotDatasourceLabels)
	}
	gotAnnotations := decoded["annotations"].(map[string]interface{})
	if gotAnnotations["summary"] != nil || gotAnnotations["description"] != "api-1 is slow" {
		t.Fatalf("snapshot must exclude the unsupported summary annotation: %#v", gotAnnotations)
	}
}

func TestPrometheusAlertNamespacesNeverInjectSystemLabels(t *testing.T) {
	alert := models.AlertInstance{
		Source:     alertRuleSource,
		Severity:   "P1",
		Labels:     `{"instance":"legacy","alertname":"legacy-name","severity":"P0"}`,
		LastResult: `{"alertname":"HighLatency","datasource":"prod-prometheus","severity":"P1","labels":{"instance":"api-1"},"datasource_labels":{"env":"prod"},"external_labels":{"team":"platform"}}`,
	}

	namespaces := alertLabelNamespaces(alert)
	if len(namespaces["system_labels"]) != 3 || namespaces["system_labels"]["alertname"] != "HighLatency" || namespaces["system_labels"]["severity"] != "P1" || namespaces["system_labels"]["datasource"] != "prod-prometheus" {
		t.Fatalf("unexpected system labels: %#v", namespaces["system_labels"])
	}
	if len(namespaces["labels"]) != 1 || namespaces["labels"]["instance"] != "api-1" {
		t.Fatalf("Prometheus labels namespace must remain raw: %#v", namespaces["labels"])
	}
	if namespaces["labels"]["alertname"] != "" || namespaces["labels"]["severity"] != "" || namespaces["labels"]["team"] != "" {
		t.Fatalf("system and custom labels must not be injected into Prometheus labels: %#v", namespaces["labels"])
	}
}

func TestSystemLabelMatchersOnlyAllowSupportedKeys(t *testing.T) {
	matchers, err := normalizeAlertLabelMatchers([]models.AlertLabelMatcher{{Namespace: "system_labels", Key: "alertname", Value: "HighLatency"}, {Namespace: "system_labels", Key: "severity", Value: "P1"}, {Namespace: "system_labels", Key: "datasource", Value: "prod-prometheus"}})
	if err != nil || len(matchers) != 3 {
		t.Fatalf("expected supported system labels, got %#v, %v", matchers, err)
	}
	alert := models.AlertInstance{Severity: "P1", LastResult: `{"alertname":"HighLatency","datasource":"prod-prometheus"}`}
	if !alertMatchesLabelMatchers(alert, matchers) {
		t.Fatalf("system label matchers must match top-level alert fields: %#v", alertLabelNamespaces(alert))
	}
	if _, err := normalizeAlertLabelMatchers([]models.AlertLabelMatcher{{Namespace: "system_labels", Key: "fingerprint", Value: "abc"}}); err == nil {
		t.Fatal("fingerprint must not be accepted as a system label matcher")
	}
}

func TestAlertRuleExternalLabelsExcludeSystemSeverity(t *testing.T) {
	labels := decodeAlertRuleExternalLabels(`{"team":"platform","severity":"P0"}`)
	if len(labels) != 1 || labels["team"] != "platform" || labels["severity"] != "" {
		t.Fatalf("rule-defined labels must remain external and exclude system severity: %#v", labels)
	}
}

func TestPromAlertPersistenceIgnoresChangingSampleValue(t *testing.T) {
	alert := models.AlertInstance{
		FaultCenterID:   8,
		Fingerprint:     "fingerprint",
		Title:           "HighLatency",
		Target:          "prod-prometheus · instance=api-1",
		Severity:        "P1",
		Status:          alertStatusFiring,
		Labels:          `{"instance":"api-1"}`,
		LastProbeStatus: "success",
		LastResult:      `{"fingerprint":"fingerprint","datasource_labels":{"cluster":"prod-a"},"value":"0.8"}`,
	}
	metadata := promAlertPersistenceMetadata{
		FaultCenterID:    8,
		Fingerprint:      "fingerprint",
		Title:            "HighLatency",
		Target:           "prod-prometheus · instance=api-1",
		Severity:         "P1",
		Status:           alertStatusFiring,
		Labels:           `{"instance":"api-1"}`,
		DatasourceLabels: `{"cluster":"prod-a"}`,
		ProbeStatus:      "success",
	}
	if promAlertNeedsPersistence(alert, metadata) {
		t.Fatal("a new sample value alone must not rewrite the active alert row")
	}
	alert.LastResult = `{"fingerprint":"fingerprint","datasource_labels":{"cluster":"prod-a","env":"prod","region":"cn-east","domain":"payments","datasource":"prod-prometheus"},"value":"0.8"}`
	if !promAlertNeedsPersistence(alert, metadata) {
		t.Fatal("changed datasource labels must be persisted")
	}
	alert.LastResult = `{"fingerprint":"fingerprint","datasource_labels":{"cluster":"prod-a"},"value":"0.8"}`
	metadata.Status = alertStatusResolved
	if !promAlertNeedsPersistence(alert, metadata) {
		t.Fatal("a lifecycle transition must be persisted")
	}
}

func TestPromAlertNotificationTemplateSupportsAllLabelNamespaces(t *testing.T) {
	payload := map[string]interface{}{
		"labels":            map[string]string{"instance": "api-1"},
		"datasource_labels": map[string]string{"env": "prod"},
		"external_labels":   map[string]string{"business": "order"},
	}
	content, err := renderNotificationMarkdown(
		"实例 ${labels.instance}，环境 ${datasource_labels.env}，业务 ${external_labels.business}",
		payload,
	)
	if err != nil || content != "实例 api-1，环境 prod，业务 order" {
		t.Fatalf("unexpected rendered notification content: %q", content)
	}
}

func TestNotificationMarkdownOnlyControlsContent(t *testing.T) {
	content, err := renderNotificationMarkdown(
		"**等级：** ${severity}\n\n${content}",
		map[string]interface{}{"severity": "P0", "content": "CPU 使用率过高"},
	)
	if err != nil || content != "**等级：** P0\n\nCPU 使用率过高" {
		t.Fatalf("unexpected markdown content: %q", content)
	}
}

func TestNotificationMarkdownUsesStructuredAlertData(t *testing.T) {
	payload := map[string]interface{}{
		"alertname": "CPU 使用率过高",
		"severity":  "P0",
		"value":     "100",
		"labels":    map[string]string{"instance": "localhost:9100"},
		"annotations": map[string]string{
			"description": "CPU 已超过阈值",
		},
	}
	content, err := renderNotificationMarkdown(`**告警：** {{ .alertname }}
{{- with .labels.instance }}
**实例：** {{ . }}
{{- end }}
{{- with .labels.pod }}
**Pod：** {{ . }}
{{- end }}
{{- if eq .severity "P0" }}
**紧急告警**
{{- end }}`, payload)
	if err != nil {
		t.Fatalf("render structured markdown: %v", err)
	}
	if strings.Contains(content, "Pod") || !strings.Contains(content, "**告警：** CPU 使用率过高") || !strings.Contains(content, "**实例：** localhost:9100") || !strings.Contains(content, "**紧急告警**") {
		t.Fatalf("unexpected structured markdown: %q", content)
	}
}

func TestNotificationMarkdownCanRangeGroupedAlerts(t *testing.T) {
	payload := map[string]interface{}{
		"alerts": []map[string]interface{}{
			{"alertname": "CPU 使用率过高", "severity": "P0", "labels": map[string]string{"instance": "node-1"}},
			{"alertname": "内存使用率过高", "severity": "P1", "labels": map[string]string{}},
		},
	}
	content, err := renderNotificationMarkdown(`{{- range .alerts }}
- [{{ .severity }}] {{ .alertname }}{{ with .labels.instance }} · {{ . }}{{ end }}
{{- end }}`, payload)
	if err != nil {
		t.Fatalf("render grouped markdown: %v", err)
	}
	if !strings.Contains(content, "- [P0] CPU 使用率过高 · node-1") || !strings.Contains(content, "- [P1] 内存使用率过高") {
		t.Fatalf("unexpected grouped markdown: %q", content)
	}
}

func TestNormalizeAlertRuleAnnotationsOnlyAcceptsDescription(t *testing.T) {
	annotations, err := normalizeAlertRuleAnnotations(map[string]string{"description": "markdown body"})
	if err != nil || annotations["description"] != "markdown body" {
		t.Fatalf("description annotation must be accepted: %#v, %v", annotations, err)
	}
	if _, err := normalizeAlertRuleAnnotations(map[string]string{"summary": "unsupported"}); err == nil {
		t.Fatal("unsupported annotations must be rejected")
	}
}

func TestNormalizeAlertRuleGroupInput(t *testing.T) {
	name, description, err := normalizeAlertRuleGroupInput("  Kubernetes 核心规则  ", "  集群基础资源告警  ")
	if err != nil || name != "Kubernetes 核心规则" || description != "集群基础资源告警" {
		t.Fatalf("unexpected normalized group: %q, %q, %v", name, description, err)
	}
	if _, _, err := normalizeAlertRuleGroupInput("   ", "empty"); err == nil {
		t.Fatal("expected an empty group name to be rejected")
	}
	if _, _, err := normalizeAlertRuleGroupInput(strings.Repeat("规", 129), "too long"); err == nil {
		t.Fatal("expected an overlong group name to be rejected")
	}
}

func TestNextPromRuleAlertStateHonorsForAndRecoveryWait(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		current    string
		active     bool
		stateSince time.Time
		forSeconds int
		recoverSec int
		want       string
	}{
		{name: "new condition enters pending", active: true, stateSince: now, forSeconds: 60, want: alertStatusPending},
		{name: "pending becomes firing after for", current: alertStatusPending, active: true, stateSince: now.Add(-60 * time.Second), forSeconds: 60, want: alertStatusFiring},
		{name: "pending stays pending before for", current: alertStatusPending, active: true, stateSince: now.Add(-30 * time.Second), forSeconds: 60, want: alertStatusPending},
		{name: "firing enters recovering", current: alertStatusFiring, active: false, stateSince: now, recoverSec: 30, want: alertStatusRecovering},
		{name: "recovering resolves after wait", current: alertStatusRecovering, active: false, stateSince: now.Add(-30 * time.Second), recoverSec: 30, want: alertStatusResolved},
		{name: "condition returns while recovering", current: alertStatusRecovering, active: true, stateSince: now, recoverSec: 30, want: alertStatusFiring},
		{name: "pending disappears without firing", current: alertStatusPending, active: false, stateSince: now, forSeconds: 60, want: alertStatusResolved},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := nextPromRuleAlertState(test.current, test.active, test.stateSince, test.forSeconds, test.recoverSec, now)
			if got != test.want {
				t.Fatalf("expected %q, got %q", test.want, got)
			}
		})
	}
}

func TestValidateAlertRuleTimingRejectsWindowsShorterThanEvaluationInterval(t *testing.T) {
	tests := []struct {
		name       string
		interval   int
		forSeconds int
		recovery   int
		wantError  bool
	}{
		{name: "immediate trigger and recovery remain valid", interval: 120, forSeconds: 0, recovery: 0},
		{name: "windows equal to interval are valid", interval: 120, forSeconds: 120, recovery: 120},
		{name: "windows longer than interval are valid", interval: 120, forSeconds: 180, recovery: 240},
		{name: "for shorter than interval is rejected", interval: 120, forSeconds: 30, recovery: 120, wantError: true},
		{name: "recovery shorter than interval is rejected", interval: 120, forSeconds: 120, recovery: 60, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateAlertRuleTiming(test.interval, test.forSeconds, test.recovery)
			if (err != nil) != test.wantError {
				t.Fatalf("validateAlertRuleTiming() error = %v, wantError %v", err, test.wantError)
			}
		})
	}
}

func TestApplyAlertStateTransitionMaintainsLifecycleTimestamps(t *testing.T) {
	now := time.Date(2026, 8, 21, 9, 30, 0, 0, time.UTC)

	firing := map[string]interface{}{}
	applyAlertStateTransition(firing, alertStatusPending, alertStatusFiring, now)
	if firing["state_changed_at"] != now || firing["fired_at"] != now {
		t.Fatalf("pending to firing must record transition and firing timestamps: %#v", firing)
	}
	if _, exists := firing["resolved_at"]; exists {
		t.Fatalf("firing transition must not set resolved_at: %#v", firing)
	}

	retriggered := map[string]interface{}{}
	applyAlertStateTransition(retriggered, alertStatusRecovering, alertStatusFiring, now)
	if retriggered["state_changed_at"] != now {
		t.Fatalf("recovering to firing must record state_changed_at: %#v", retriggered)
	}
	if _, exists := retriggered["fired_at"]; exists {
		t.Fatalf("recovering to firing must preserve the incident's original fired_at: %#v", retriggered)
	}

	resolved := map[string]interface{}{}
	applyAlertStateTransition(resolved, alertStatusRecovering, alertStatusResolved, now)
	if resolved["state_changed_at"] != now || resolved["resolved_at"] != now {
		t.Fatalf("resolved transition must record transition and recovery timestamps: %#v", resolved)
	}

	unchanged := map[string]interface{}{}
	applyAlertStateTransition(unchanged, alertStatusFiring, alertStatusFiring, now)
	if len(unchanged) != 0 {
		t.Fatalf("unchanged state must not rewrite lifecycle timestamps: %#v", unchanged)
	}
}

func TestForcedAlertTerminationDoesNotMasqueradeAsRecovery(t *testing.T) {
	for _, reason := range []string{"rule_disabled", "rule_deleted", "datasource_unbound"} {
		if got := alertEndStatus(reason); got != alertStatusTerminated {
			t.Fatalf("reason %q should terminate the alert, got %q", reason, got)
		}
	}
	if got := alertEndStatus("condition_cleared"); got != alertStatusResolved {
		t.Fatalf("a cleared condition should remain a natural recovery, got %q", got)
	}
}

func TestPromSeriesFingerprintIsStableAcrossLabelOrder(t *testing.T) {
	first := promSeriesFingerprint(7, 12, map[string]string{"instance": "node-1", "job": "node"})
	second := promSeriesFingerprint(7, 12, map[string]string{"job": "node", "instance": "node-1"})
	if first == "" || first != second {
		t.Fatalf("expected stable non-empty fingerprint, got %q and %q", first, second)
	}
	if first == promSeriesFingerprint(8, 12, map[string]string{"instance": "node-1", "job": "node"}) {
		t.Fatal("different datasources must produce different fingerprints")
	}
	if first == promSeriesFingerprint(7, 12, map[string]string{"instance": "node-2", "job": "node"}) {
		t.Fatal("different PromQL labels must produce different fingerprints")
	}
}

func TestPromAlertIdentityUsesPersistedFingerprint(t *testing.T) {
	alert := models.AlertInstance{
		Fingerprint: "persisted-fingerprint",
		LastResult:  `{"alertname":"CPU 使用率过高","labels":{"instance":"node-1"}}`,
	}

	if got := promAlertIdentity(alert); got != alert.Fingerprint {
		t.Fatalf("expected persisted fingerprint %q, got %q", alert.Fingerprint, got)
	}
}

func TestPromAlertIdentityRequiresPersistedFingerprint(t *testing.T) {
	alert := models.AlertInstance{LastResult: `{"alertname":"CPU 使用率过高","labels":{"instance":"node-1"}}`}
	if got := promAlertIdentity(alert); got != "" {
		t.Fatalf("missing persisted fingerprint must not be reconstructed, got %q", got)
	}
}

func TestApplyAlertRuleConditions(t *testing.T) {
	rule := models.AlertRule{Severity: "P1", ForSeconds: 30}
	series := []promVectorSeries{{Labels: map[string]string{"instance": "node-1"}, Value: "85"}}
	conditions := []models.AlertRuleCondition{
		{ID: 1, Severity: "P2", Operator: ">", Threshold: 80},
		{ID: 2, Severity: "P0", Operator: ">", Threshold: 90},
	}
	matched := applyAlertRuleConditions(series, conditions, rule)
	if len(matched) != 1 || matched[0].ConditionID != 1 || matched[0].Severity != "P2" || matched[0].ForSeconds != 30 {
		t.Fatalf("unexpected condition matches: %#v", matched)
	}
	if empty := applyAlertRuleConditions(series, nil, rule); len(empty) != 0 {
		t.Fatalf("rules without current conditions must not produce alerts: %#v", empty)
	}
}

func TestApplyAlertRuleConditionsChoosesHighestSeverityPerSeries(t *testing.T) {
	rule := models.AlertRule{Severity: "P2", ForSeconds: 60}
	series := []promVectorSeries{{Labels: map[string]string{"instance": "node-1"}, Value: "95"}}
	conditions := []models.AlertRuleCondition{
		{ID: 1, Severity: "P2", Operator: ">", Threshold: 70},
		{ID: 2, Severity: "P1", Operator: ">", Threshold: 80},
		{ID: 3, Severity: "P0", Operator: ">", Threshold: 90},
	}
	matched := applyAlertRuleConditions(series, conditions, rule)
	if len(matched) != 1 {
		t.Fatalf("expected one alert candidate per series, got %#v", matched)
	}
	if matched[0].Severity != "P0" || matched[0].ConditionID != 3 || matched[0].ForSeconds != 60 {
		t.Fatalf("expected highest severity condition, got %#v", matched[0])
	}
}

func TestAlertEvaluationActionMatchesPrometheusNoDataSemantics(t *testing.T) {
	tests := []struct {
		status string
		want   alertEvaluationAction
	}{
		{status: "success", want: alertEvaluationSync},
		{status: "no_data", want: alertEvaluationSync},
		{status: "error", want: alertEvaluationKeep},
		{status: "stale", want: alertEvaluationKeep},
	}
	for _, test := range tests {
		if got := alertRuleEvaluationAction(test.status); got != test.want {
			t.Fatalf("status=%s: expected %q, got %q", test.status, test.want, got)
		}
	}
}

func TestNormalizeAlertRuleConditionsLimitsSeverityAndOperators(t *testing.T) {
	threshold := 80.0
	valid, err := normalizeAlertRuleConditions([]saveAlertRuleConditionReq{
		{Severity: "P0", Operator: ">=", Threshold: &threshold},
		{Severity: "P1", Operator: "<", Threshold: &threshold},
		{Severity: "P2", Operator: "!=", Threshold: &threshold},
	}, 45)
	if err != nil || len(valid) != 3 {
		t.Fatalf("expected three valid severity conditions, got %#v, %v", valid, err)
	}

	if _, err := normalizeAlertRuleConditions([]saveAlertRuleConditionReq{
		{Severity: "P1", Operator: ">", Threshold: &threshold},
		{Severity: "P1", Operator: "<", Threshold: &threshold},
	}, 0); err == nil {
		t.Fatal("expected duplicate severities to be rejected")
	}
	if _, err := normalizeAlertRuleConditions([]saveAlertRuleConditionReq{
		{Severity: "P0", Operator: "active", Threshold: &threshold},
	}, 0); err == nil {
		t.Fatal("expected unsupported operators to be rejected")
	}
	tooMany := []saveAlertRuleConditionReq{
		{Severity: "P0", Operator: ">", Threshold: &threshold},
		{Severity: "P1", Operator: ">", Threshold: &threshold},
		{Severity: "P2", Operator: ">", Threshold: &threshold},
		{Severity: "P0", Operator: ">", Threshold: &threshold},
	}
	if _, err := normalizeAlertRuleConditions(tooMany, 0); err == nil {
		t.Fatal("expected more than three conditions to be rejected")
	}
	if _, err := normalizeAlertRuleConditions(nil, 0); err == nil {
		t.Fatal("expected missing current conditions to be rejected")
	}
}

func TestParsePromVectorResponseSeparatesSuccessNoDataAndError(t *testing.T) {
	series, err := parsePromVectorResponse([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"instance":"node-1"},"value":[1724140800,"1"]}]}}`))
	if err != nil || len(series) != 1 || series[0].Labels["instance"] != "node-1" || series[0].Value != "1" {
		t.Fatalf("unexpected vector result: %#v, %v", series, err)
	}
	empty, err := parsePromVectorResponse([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	if err != nil || len(empty) != 0 {
		t.Fatalf("expected successful no-data result, got %#v, %v", empty, err)
	}
	if _, err := parsePromVectorResponse([]byte(`{"status":"error","error":"query timeout"}`)); err == nil {
		t.Fatal("expected Prometheus error response to remain an evaluation error")
	}
}
