package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/cloudwego/eino/schema"
	aicore "guanlan/internal/ai"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type alertAIEvidence struct {
	ID             string      `json:"id"`
	Tool           string      `json:"tool"`
	DatasourceID   uint        `json:"datasource_id,omitempty"`
	DatasourceName string      `json:"datasource_name,omitempty"`
	Query          string      `json:"query,omitempty"`
	RangeStart     time.Time   `json:"range_start"`
	RangeEnd       time.Time   `json:"range_end"`
	Status         string      `json:"status"`
	HasData        bool        `json:"has_data"`
	Summary        string      `json:"summary"`
	Data           interface{} `json:"data,omitempty"`
}
type alertAIToolRuntime struct {
	job          models.AlertAIAnalysisJob
	started      time.Time
	calls, bytes int
	evidence     []alertAIEvidence
}

var aiSensitiveText = regexp.MustCompile(`(?i)(password|passwd|secret|token|api[_-]?key|authorization|credential)\s*[:=]\s*[^\s,;"}]+`)

func aiSafeData(value interface{}) interface{} {
	// Convert typed maps/slices before recursively applying the existing secret/URL projection.
	var decoded interface{}
	raw, _ := json.Marshal(value)
	_ = json.Unmarshal(raw, &decoded)
	var clean func(interface{}) interface{}
	clean = func(v interface{}) interface{} {
		switch x := v.(type) {
		case string:
			return aiSensitiveText.ReplaceAllString(x, "[已隐藏]")
		case map[string]interface{}:
			for k, v := range x {
				x[k] = clean(v)
			}
			return x
		case []interface{}:
			for i, v := range x {
				x[i] = clean(v)
			}
			return x
		}
		return v
	}
	return clean(sanitizeOperationResult(decoded))
}
func alertAIContext(a models.AlertInstance) interface{} {
	var snapshot map[string]interface{}
	_ = json.Unmarshal([]byte(a.LastResult), &snapshot)
	fields := map[string]interface{}{"id": a.ID, "title": a.Title, "source": a.Source, "datasource_id": a.DatasourceID, "severity": a.Severity, "status": a.Status, "started_at": a.StartedAt, "fired_at": a.FiredAt, "labels": alertLabelNamespaces(a)}
	for _, key := range []string{"expression", "condition", "value", "datasource", "probe_type", "status", "duration_ms"} {
		if v, ok := snapshot[key]; ok {
			fields[key] = v
		}
	}
	return aiSafeData(fields)
}
func (r *alertAIToolRuntime) catalog() string {
	var ids []uint
	_ = json.Unmarshal([]byte(r.job.DatasourceIDsJSON), &ids)
	list := []map[string]interface{}{}
	for _, id := range ids {
		var ds models.Datasource
		if database.DB.Where("id = ? AND status = 1", id).First(&ds).Error == nil && isMetricDatasourceType(ds.Type) {
			list = append(list, map[string]interface{}{"id": id, "name": ds.Name, "type": ds.Type})
		}
	}
	return encodeJSON(aiSafeData(list))
}
func (r *alertAIToolRuntime) registry() *aicore.Registry {
	registry := aicore.NewRegistry()
	for _, name := range []string{"query_analysis_metric_range", "query_analysis_metric_instant"} {
		toolName := name
		_ = registry.Register(aicore.ToolSpec{Name: name, Description: "查询授权 Prometheus/VictoriaMetrics 指标。range 固定为分析开始前15分钟，步长15秒；instant 固定为分析开始时。返回 evidence_id 和真实数据。必须用告警标签定位目标，单次最多50条序列。", Risk: aicore.RiskReadonlyExpensive, TimeoutSeconds: 8, ParamsSchema: map[string]*schema.ParameterInfo{"datasource_id": {Type: schema.Integer, Required: true, Desc: "授权数据源 ID"}, "query": {Type: schema.String, Required: true, Desc: "只读 PromQL，禁止 offset/@，回看窗口总计不超过30分钟"}}}, func(ctx context.Context, call aicore.ToolCall) aicore.ToolResult {
			return r.metric(ctx, toolName, call.Params)
		})
	}
	_ = registry.Register(aicore.ToolSpec{Name: "get_analysis_alert_context", Description: "读取本次告警的脱敏快照、标签、表达式和阈值", Risk: aicore.RiskReadonlyLow, TimeoutSeconds: 3, ParamsSchema: map[string]*schema.ParameterInfo{}}, func(ctx context.Context, call aicore.ToolCall) aicore.ToolResult {
		if _, _, err := validateAlertAIJob(ctx, r.job); err != nil {
			return aicore.ToolResult{Tool: call.Tool, ErrorCode: "FORBIDDEN", Message: "分析资格失效"}
		}
		var data interface{}
		_ = json.Unmarshal([]byte(r.job.InputJSON), &data)
		return aicore.ToolResult{Tool: call.Tool, OK: true, Data: data}
	})
	_ = registry.Register(aicore.ToolSpec{Name: "list_analysis_related_alerts", Description: "查询同故障中心近30分钟内，instance/service/target 与本告警相同的关联告警，最多20条", Risk: aicore.RiskReadonlyLow, TimeoutSeconds: 5, ParamsSchema: map[string]*schema.ParameterInfo{}}, func(ctx context.Context, call aicore.ToolCall) aicore.ToolResult { return r.related(ctx, call.Tool) })
	return registry
}

var aiPromLookback = regexp.MustCompile(`\[([^\]]*)\]`)
var aiPromDuration = regexp.MustCompile(`([0-9]+)(ms|s|m|h|d|w|y)`)
var aiPromOffset = regexp.MustCompile(`\boffset\b`)

func validateAnalysisPromQL(query string) error {
	if len(query) > 2048 || strings.TrimSpace(query) == "" || strings.Contains(query, "@") || aiPromOffset.MatchString(query) {
		return fmt.Errorf("查询过长或包含不允许的时间偏移")
	}
	total := time.Duration(0)
	for _, m := range aiPromLookback.FindAllStringSubmatch(query, -1) {
		window := strings.Split(m[1], ":")[0]
		remaining := aiPromDuration.ReplaceAllString(window, "")
		if remaining != "" || window == "" {
			return fmt.Errorf("回看窗口必须使用固定时长")
		}
		for _, part := range aiPromDuration.FindAllStringSubmatch(window, -1) {
			n, err := strconv.ParseUint(part[1], 10, 32)
			if err != nil || n > 1800000 {
				return fmt.Errorf("回看窗口超出上限")
			}
			unit := time.Second
			switch part[2] {
			case "ms":
				unit = time.Millisecond
			case "m":
				unit = time.Minute
			case "h":
				unit = time.Hour
			case "d", "w", "y":
				return fmt.Errorf("回看窗口超出上限")
			}
			total += time.Duration(n) * unit
		}
	}
	if total > 30*time.Minute {
		return fmt.Errorf("回看窗口总计不能超过30分钟")
	}
	return nil
}
func (r *alertAIToolRuntime) metric(ctx context.Context, name string, params map[string]interface{}) aicore.ToolResult {
	fail := func(code, msg string) aicore.ToolResult {
		return aicore.ToolResult{Tool: name, ErrorCode: code, Message: msg}
	}
	r.calls++
	if r.calls > 12 || r.bytes >= 64*1024 {
		return fail("BUDGET_EXCEEDED", "本次查询预算已用尽")
	}
	rule, alert, err := validateAlertAIJob(ctx, r.job)
	if err != nil {
		return fail("FORBIDDEN", "分析资格失效")
	}
	n, ok := params["datasource_id"].(float64)
	if !ok || n <= 0 || n != float64(uint(n)) {
		return fail("INVALID_PARAMS", "数据源 ID 无效")
	}
	id := uint(n)
	var allowed []uint
	_ = json.Unmarshal([]byte(r.job.DatasourceIDsJSON), &allowed)
	if !aiContainsID(allowed, id) || (!aiContainsID(rule.AllowedDatasourceIDs, id) && alert.DatasourceID != id) {
		return fail("FORBIDDEN", "数据源不在本次分析授权范围内")
	}
	query, _ := params["query"].(string)
	if err := validateAnalysisPromQL(query); err != nil {
		return fail("INVALID_PARAMS", err.Error())
	}
	// Sensitive query literals must not be copied into provider context or persisted evidence.
	if aiSensitiveText.MatchString(query) {
		return fail("INVALID_PARAMS", "查询包含敏感字段")
	}
	var ds models.Datasource
	if database.DB.WithContext(ctx).Where("id = ? AND status = 1", id).First(&ds).Error != nil || !isMetricDatasourceType(ds.Type) {
		return fail("DATASOURCE_UNAVAILABLE", "指标数据源不可用")
	}
	start, end := r.started.Add(-15*time.Minute), r.started
	evidence := alertAIEvidence{ID: fmt.Sprintf("ev-%d", len(r.evidence)+1), Tool: name, DatasourceID: id, DatasourceName: ds.Name, Query: query, RangeStart: start, RangeEnd: end, Status: "failed"}
	values := url.Values{"query": {query}, "timeout": {"8s"}, "limit": {"50"}}
	endpoint := "query"
	if name == "query_analysis_metric_range" {
		endpoint = "query_range"
		values.Set("start", strconv.FormatInt(start.Unix(), 10))
		values.Set("end", strconv.FormatInt(end.Unix(), 10))
		values.Set("step", "15")
	} else {
		values.Set("time", strconv.FormatInt(end.Unix(), 10))
		evidence.RangeStart = end
	}
	data, err := analysisPromQuery(ctx, ds, endpoint, values)
	if err != nil {
		evidence.Summary = err.Error()
		r.evidence = append(r.evidence, evidence)
		return fail("QUERY_FAILED", evidence.Summary)
	}
	projected := aiSafeData(data)
	size := len(encodeJSON(projected))
	if size > 16*1024 || r.bytes+size > 64*1024 {
		evidence.Summary = "查询结果过大，请缩小查询范围"
		r.evidence = append(r.evidence, evidence)
		return fail("RESULT_TOO_LARGE", evidence.Summary)
	}
	r.bytes += size
	evidence.Status = "succeeded"
	evidence.HasData = len(data) > 0
	evidence.Summary = fmt.Sprintf("返回 %d 条序列", len(data))
	evidence.Data = projected
	r.evidence = append(r.evidence, evidence)
	return aicore.ToolResult{Tool: name, OK: true, Data: projected, Meta: map[string]interface{}{"evidence_id": evidence.ID, "start": evidence.RangeStart, "end": end}, Summary: evidence.Summary}
}
func analysisPromQuery(ctx context.Context, ds models.Datasource, endpoint string, values url.Values) ([]map[string]interface{}, error) {
	client, cleanup, err := datasourceHTTPClient(ds, 8*time.Second)
	if err != nil {
		return nil, fmt.Errorf("数据源连接配置无效")
	}
	defer cleanup()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(ds.URL, "/")+"/api/v1/"+endpoint+"?"+values.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("查询地址无效")
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("指标查询失败或超时")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("指标服务返回 HTTP %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 128*1024+1))
	if err != nil || len(raw) > 128*1024 {
		return nil, fmt.Errorf("指标响应过大或读取失败")
	}
	data, err := parsePromResponse(raw)
	if err != nil {
		return nil, fmt.Errorf("指标服务返回错误或不支持的查询结果")
	}
	if len(data) > 50 {
		return nil, fmt.Errorf("序列超过50条，请限定节点或聚合")
	}
	for _, row := range data {
		if points, ok := row["values"].([]interface{}); ok && len(points) > 120 {
			return nil, fmt.Errorf("查询点数超出上限")
		}
	}
	return data, nil
}
func (r *alertAIToolRuntime) related(ctx context.Context, name string) aicore.ToolResult {
	r.calls++
	if r.calls > 12 || r.bytes >= 64*1024 {
		return aicore.ToolResult{Tool: name, ErrorCode: "BUDGET_EXCEEDED", Message: "查询预算已用尽"}
	}
	_, a, err := validateAlertAIJob(ctx, r.job)
	if err != nil {
		return aicore.ToolResult{Tool: name, ErrorCode: "FORBIDDEN"}
	}
	var candidates []models.AlertInstance
	if database.DB.WithContext(ctx).Where("fault_center_id = ? AND id <> ? AND last_event_at >= ?", a.FaultCenterID, a.ID, r.started.Add(-30*time.Minute)).Order("id DESC").Limit(100).Find(&candidates).Error != nil {
		return aicore.ToolResult{Tool: name, ErrorCode: "QUERY_FAILED"}
	}
	labels := alertLabelNamespaces(a)["labels"]
	data := []interface{}{}
	for _, candidate := range candidates {
		other := alertLabelNamespaces(candidate)["labels"]
		match := false
		for _, key := range []string{"instance", "service", "target"} {
			if labels[key] != "" && labels[key] == other[key] {
				match = true
			}
		}
		if match {
			data = append(data, alertAIContext(candidate))
		}
		if len(data) >= 20 {
			break
		}
	}
	size := len(encodeJSON(data))
	if size > 16*1024 || r.bytes+size > 64*1024 {
		return aicore.ToolResult{Tool: name, ErrorCode: "RESULT_TOO_LARGE"}
	}
	r.bytes += size
	e := alertAIEvidence{ID: fmt.Sprintf("ev-%d", len(r.evidence)+1), Tool: name, RangeStart: r.started.Add(-30 * time.Minute), RangeEnd: r.started, Status: "succeeded", HasData: len(data) > 0, Summary: fmt.Sprintf("返回 %d 条关联告警（最近100条中筛选）", len(data)), Data: data}
	r.evidence = append(r.evidence, e)
	return aicore.ToolResult{Tool: name, OK: true, Data: data, Summary: e.Summary, Meta: map[string]interface{}{"evidence_id": e.ID}}
}
