package handlers

import (
	"strings"
	"testing"
	"time"
)

func TestNotificationVarsHandlesNilLifecycleTimes(t *testing.T) {
	now := time.Date(2026, 8, 31, 11, 3, 0, 0, time.Local)
	var recoveredAt *time.Time

	vars := notificationVars(map[string]interface{}{
		"fired_at":     &now,
		"recovered_at": recoveredAt,
	})

	if vars["fired_at"] != "2026-08-31 11:03:00" {
		t.Fatalf("unexpected firing time: %q", vars["fired_at"])
	}
	if vars["recovered_at"] != "" || vars["recover_time_format"] != "" {
		t.Fatalf("nil recovery time must render as empty text: %#v", vars)
	}
}

func TestSystemNotificationEnvelopeFixesTitleColorAndFooter(t *testing.T) {
	firing := map[string]interface{}{"title": "CPU 使用率过高", "title_color": "green", "content": "正文", "status": "firing", "severity": "P0"}
	applySystemNotificationEnvelope(firing)
	if firing["title"] != "【告警中】- 观澜运维平台 🚨" || firing["title_color"] != "red" {
		t.Fatalf("unexpected firing envelope: %#v", firing)
	}
	if !strings.HasSuffix(firing["content"].(string), "🧑‍💻 观澜运维平台 · 运维团队") {
		t.Fatalf("fixed footer is missing: %#v", firing)
	}

	recovered := map[string]interface{}{"title": "CPU 使用率过高", "content": "恢复正文", "status": "resolved", "severity": "P0"}
	applySystemNotificationEnvelope(recovered)
	if recovered["title"] != "【已恢复】- 观澜运维平台 ✅" || recovered["title_color"] != "green" {
		t.Fatalf("unexpected recovery envelope: %#v", recovered)
	}

	applySystemNotificationEnvelope(recovered)
	if strings.Count(recovered["content"].(string), "🧑‍💻 观澜运维平台 · 运维团队") != 1 {
		t.Fatalf("fixed footer must not be duplicated: %#v", recovered)
	}
}

func TestNotificationTemplateRejectsDefinedSections(t *testing.T) {
	content := `{{- define "Event" -}}legacy{{- end -}}`
	if err := validateNotificationTemplateContent(content); err == nil {
		t.Fatal("templates with legacy defined sections must be rejected")
	}
}

func TestSystemNotificationEnvelopeIdentifiesTestMessage(t *testing.T) {
	payload := map[string]interface{}{
		"status": "test", "severity": "P2", "alertname": "通知发送测试",
		"annotations": map[string]string{"description": "这是一条手动发送的测试通知。"},
	}
	content, err := renderNotificationMarkdown("告警名称：{{ .alertname }}\n{{ .annotations.description }}{{ if .is_firing }}告警中{{ end }}", payload)
	if err != nil {
		t.Fatal(err)
	}
	payload["content"] = content
	applySystemNotificationEnvelope(payload)
	if payload["title"] != "【测试通知】- 观澜运维平台 🔔" || payload["title_color"] != "blue" {
		t.Fatalf("unexpected test envelope: %#v", payload)
	}
	if !strings.Contains(content, "通知发送测试") || !strings.Contains(content, "手动发送的测试通知") || strings.Contains(content, "告警中") {
		t.Fatalf("unexpected test content: %s", content)
	}
}

func TestNotificationChannelSeverityColors(t *testing.T) {
	for _, tc := range []struct{ status, severity, color, label, wecom string }{
		{"firing", "P0", "#d54941", "P0", "warning"},
		{"firing", "P1", "#d97904", "P1", "warning"},
		{"firing", "P2", "#3370ff", "P2", "comment"},
		{"resolved", "P0", "#23855a", "已恢复", "info"},
		{"recovered", "P1", "#23855a", "已恢复", "info"},
		{"recovery", "P2", "#23855a", "已恢复", "info"},
		{"test", "P0", "#3370ff", "测试通知", "comment"},
	} {
		t.Run(tc.status+tc.severity, func(t *testing.T) {
			data := map[string]interface{}{"status": tc.status, "severity": tc.severity, "title": "<script>alert(1)</script>", "content": "实例 <node> & 服务\n第二行"}
			for kind, color := range map[string]string{"dingding": tc.color, "qywechat": tc.wecom} {
				heading := notificationMarkdownHeading(kind, data["title"].(string), data)
				if !strings.Contains(heading, `color="`+color+`"`) || !strings.Contains(heading, tc.label) || strings.Contains(heading, "<script>") {
					t.Fatalf("unexpected %s heading: %s", kind, heading)
				}
			}
			body := notificationEmailHTML(data)
			if !strings.Contains(body, "background:"+tc.color) || !strings.Contains(body, tc.label) || !strings.Contains(body, "&lt;node&gt; &amp; 服务<br>第二行") || strings.Contains(body, "<script>") {
				t.Fatalf("unsafe or incorrect HTML: %s", body)
			}
		})
	}
}
