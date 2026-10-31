package handlers

import (
	"strings"
	"testing"
)

func TestParseAlertRuleConfigDocumentSupportsYAML(t *testing.T) {
	content := `apiVersion: aiops/v1
kind: AlertRuleList
rules:
  - name: CPU 使用率过高
    expression: node_cpu_usage > 90
    datasource_group_ids: [1]
    fault_center_id: 1
    for_seconds: 120
    recovery_wait_seconds: 0
    evaluation_interval_seconds: 60
    conditions:
      - severity: P0
        operator: ">"
        threshold: 90
    status: 0`
	document, err := parseAlertRuleConfigDocument(content)
	if err != nil {
		t.Fatalf("parse YAML config: %v", err)
	}
	if len(document.Rules) != 1 || document.Rules[0].Status == nil || *document.Rules[0].Status != 0 {
		t.Fatalf("unexpected document: %#v", document)
	}
}

func TestParseAlertRuleConfigDocumentRejectsJSON(t *testing.T) {
	_, err := parseAlertRuleConfigDocument(`{"apiVersion":"aiops/v1","kind":"AlertRuleList","rules":[]}`)
	if err == nil || !strings.Contains(err.Error(), "仅支持 YAML") {
		t.Fatalf("expected JSON to be rejected, got %v", err)
	}
}

func TestParseAlertRuleConfigDocumentRejectsUnknownFields(t *testing.T) {
	_, err := parseAlertRuleConfigDocument("apiVersion: aiops/v1\nkind: AlertRuleList\nunknown: true\nrules: []")
	if err == nil || !strings.Contains(err.Error(), "field unknown not found") {
		t.Fatalf("expected unknown field to be rejected, got %v", err)
	}
}
