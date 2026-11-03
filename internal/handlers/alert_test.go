package handlers

import (
	"encoding/json"
	"guanlan/internal/models"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestApplyAlertTriggerDateFiltersUsesTriggerTime(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatalf("create dry-run gorm database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	query := applyAlertTriggerDateFilters(
		db.Model(&models.AlertInstance{}),
		"2026-08-02",
		"2026-08-02",
	).Find(&[]models.AlertInstance{})
	statement := query.Statement.SQL.String()

	if strings.Contains(statement, "last_event_at") {
		t.Fatalf("date filter must not use last_event_at: %s", statement)
	}
	if count := strings.Count(statement, "COALESCE(fired_at, started_at)"); count != 2 {
		t.Fatalf("expected both date bounds to use trigger time, got %d occurrences in %s", count, statement)
	}
	if len(query.Statement.Vars) != 2 {
		t.Fatalf("expected two date bounds, got %#v", query.Statement.Vars)
	}
	start, ok := query.Statement.Vars[0].(time.Time)
	if !ok || start.Format("2006-01-02 15:04:05") != "2026-08-02 00:00:00" {
		t.Fatalf("unexpected start bound: %#v", query.Statement.Vars[0])
	}
	end, ok := query.Statement.Vars[1].(time.Time)
	if !ok || end.Format("2006-01-02 15:04:05") != "2026-08-03 00:00:00" {
		t.Fatalf("unexpected exclusive end bound: %#v", query.Statement.Vars[1])
	}
}

func TestBuildSilenceRuleRejectsEmptyMatchers(t *testing.T) {
	_, err := buildSilenceRule(1, saveSilenceRuleReq{
		Name:        "全量静默",
		SilenceType: "weekly",
		Weekdays:    []int{1},
		StartTime:   "09:00",
		EndTime:     "18:00",
	})
	if err == nil {
		t.Fatal("silence rule without label matchers must be rejected")
	}
}

func TestBuildSilenceRuleRejectsSeverityMatcher(t *testing.T) {
	_, err := buildSilenceRule(1, saveSilenceRuleReq{
		Name:        "等级静默",
		Matchers:    []models.AlertLabelMatcher{{Namespace: "system_labels", Key: "severity", Value: "P0"}},
		SilenceType: "weekly",
		Weekdays:    []int{1},
		StartTime:   "09:00",
		EndTime:     "18:00",
	})
	if err == nil || !strings.Contains(err.Error(), "不支持按告警等级匹配") {
		t.Fatalf("severity matcher must be rejected, got %v", err)
	}
}

func TestBuildSilenceRulePreservesEnabledStatus(t *testing.T) {
	disabled := 0
	rule, err := buildSilenceRule(1, saveSilenceRuleReq{
		Name: "维护静默", Matchers: []models.AlertLabelMatcher{{Namespace: "labels", Key: "instance", Value: "node-1"}},
		SilenceType: "weekly", Weekdays: []int{1}, StartTime: "09:00", EndTime: "18:00", Status: &disabled,
	})
	if err != nil || rule.Status != 0 {
		t.Fatalf("disabled status must be preserved: status=%d err=%v", rule.Status, err)
	}
	rule, err = buildSilenceRule(1, saveSilenceRuleReq{
		Name: "默认启用静默", Matchers: []models.AlertLabelMatcher{{Namespace: "labels", Key: "instance", Value: "node-1"}},
		SilenceType: "weekly", Weekdays: []int{1}, StartTime: "09:00", EndTime: "18:00",
	})
	if err != nil || rule.Status != 1 {
		t.Fatalf("missing status must default to enabled: status=%d err=%v", rule.Status, err)
	}
}

func TestEnrichAlertResultWithTemplateTimes(t *testing.T) {
	firedAt := time.Date(2026, 8, 31, 14, 3, 6, 0, time.Local)
	recoveredAt := time.Date(2026, 8, 31, 15, 3, 6, 0, time.Local)
	alert := models.AlertInstance{
		LastResult: `{"alertname":"CPU 使用率过高","labels":{"instance":"localhost:9100"}}`,
		FiredAt:    &firedAt,
		ResolvedAt: &recoveredAt,
	}

	enrichAlertResultWithTemplateTimes(&alert)

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(alert.LastResult), &result); err != nil {
		t.Fatalf("enriched result is not valid JSON: %v", err)
	}
	if result["first_trigger_time_format"] != "2026-08-31 14:03:06" {
		t.Fatalf("unexpected trigger time: %#v", result["first_trigger_time_format"])
	}
	if result["recover_time_format"] != "2026-08-31 15:03:06" {
		t.Fatalf("unexpected recovery time: %#v", result["recover_time_format"])
	}
	labels, ok := result["labels"].(map[string]interface{})
	if !ok || labels["instance"] != "localhost:9100" {
		t.Fatalf("PromQL labels must remain unchanged: %#v", result["labels"])
	}
}

func TestEnrichAlertResultWithTemplateTimesKeepsEmptyRecoveryField(t *testing.T) {
	firedAt := time.Date(2026, 8, 31, 14, 3, 6, 0, time.Local)
	alert := models.AlertInstance{LastResult: `{"alertname":"CPU 使用率过高"}`, FiredAt: &firedAt}

	enrichAlertResultWithTemplateTimes(&alert)

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(alert.LastResult), &result); err != nil {
		t.Fatalf("enriched result is not valid JSON: %v", err)
	}
	if value, exists := result["recover_time_format"]; !exists || value != "" {
		t.Fatalf("active alert must expose an empty recovery template field: %#v", result)
	}
}

func TestUnifiedAlertLabelsPreservePrecedenceAndLegacyMatchers(t *testing.T) {
	alert := namespacedAlert(`{"env":"prod","instance":"node-1"}`, `{"env":"staging","region":"east"}`, `{"env":"test","region":"west","team":"ops"}`)
	labels := alertLabelNamespaces(alert)["all_labels"]
	if labels["env"] != "prod" || labels["region"] != "east" || labels["team"] != "ops" {
		t.Fatalf("wrong precedence: %#v", labels)
	}
	for _, matcher := range []models.AlertLabelMatcher{
		{Namespace: "all_labels", Key: "env", Value: "prod"},
		{Namespace: "all_labels", Key: "team", Value: "ops"},
		{Namespace: "external_labels", Key: "env", Value: "test"},
	} {
		if _, err := normalizeAlertLabelMatchers([]models.AlertLabelMatcher{matcher}); err != nil {
			t.Fatal(err)
		}
		if !alertMatchesLabelMatchers(alert, []models.AlertLabelMatcher{matcher}) {
			t.Fatalf("matcher failed: %+v", matcher)
		}
	}
	if alertMatchesLabelMatchers(alert, []models.AlertLabelMatcher{{Namespace: "all_labels", Key: "env", Value: "test"}}) {
		t.Fatal("extra label overrode metric label")
	}
}

func TestUnifiedAlertLabelConflictsAndMissingSystemValues(t *testing.T) {
	alert := models.AlertInstance{Source: alertRuleSource, Title: "SystemName", Severity: "P0", LastResult: `{"datasource":"system-ds","labels":{"alertname":"metric-name","datasource":"metric-ds","env":"prod","empty":""},"datasource_labels":{"alertname":"ds-name","env":"test","region":"east","empty":"fallback"},"external_labels":{"alertname":"extra-name","region":"west","team":"ops"}}`}
	labels := alertLabelNamespaces(alert)["all_labels"]
	for key, want := range map[string]string{"alertname": "SystemName", "datasource": "system-ds", "severity": "P0", "env": "prod", "region": "east", "team": "ops", "empty": ""} {
		if labels[key] != want {
			t.Fatalf("%s: got %q want %q", key, labels[key], want)
		}
	}
	if alertMatchesLabelMatchers(alert, []models.AlertLabelMatcher{{Namespace: "all_labels", Key: "env", Value: "test"}}) {
		t.Fatal("lower priority fallback matched")
	}
	if alertMatchesLabelMatchers(alert, []models.AlertLabelMatcher{{Namespace: "all_labels", Key: "missing", Value: ""}}) {
		t.Fatal("missing label matched empty value")
	}
	alert.LastResult = `{"labels":{"datasource":"metric-ds"},"external_labels":{"datasource":"extra-ds"}}`
	if got := alertLabelNamespaces(alert)["all_labels"]["datasource"]; got != "metric-ds" {
		t.Fatalf("missing system value discarded metric label: %q", got)
	}
	matchers, err := normalizeAlertLabelMatchers([]models.AlertLabelMatcher{{Key: "datasource", Value: "metric-ds"}})
	if err != nil || matchers[0].Namespace != "all_labels" || !alertMatchesLabelMatchers(alert, matchers) {
		t.Fatalf("plain Key/Value failed: %+v %v", matchers, err)
	}
}
