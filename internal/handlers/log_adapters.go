package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"guanlan/internal/models"
)

// Stored alongside custom_headers. Only the fields below describe log mapping;
// credentials stay in the existing datasource authentication configuration.
type logSourceConfig struct {
	Index          string `json:"index"`
	Database       string `json:"database"`
	Table          string `json:"table"`
	TimestampField string `json:"timestamp_field"`
	MessageField   string `json:"message_field"`
	ServiceField   string `json:"service_field"`
	LevelField     string `json:"level_field"`
	TraceIDField   string `json:"trace_id_field"`
}

var logIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var logESField = regexp.MustCompile(`^[A-Za-z_@][A-Za-z0-9_@.\-]*$`)
var logESIndex = regexp.MustCompile(`^[a-z0-9_.*\-,]+$`)

func readLogSourceConfig(ds models.Datasource) (logSourceConfig, error) {
	var config struct {
		Logs logSourceConfig `json:"log_query"`
	}
	if ds.Config != "" && json.Unmarshal([]byte(ds.Config), &config) != nil {
		return config.Logs, fmt.Errorf("日志字段配置格式不正确")
	}
	cfg := config.Logs
	if ds.Type != "elasticsearch" && ds.Type != "clickhouse" {
		return cfg, nil
	}
	fields := []string{cfg.TimestampField, cfg.MessageField, cfg.ServiceField, cfg.LevelField, cfg.TraceIDField}
	if cfg.TimestampField == "" || cfg.MessageField == "" {
		return cfg, fmt.Errorf("请配置日志时间字段和正文字段")
	}
	for _, field := range fields {
		if field == "" {
			continue
		}
		valid := logESField.MatchString(field)
		if ds.Type == "clickhouse" {
			valid = logIdentifier.MatchString(field) && field != "__guanlan_timestamp"
		}
		if !valid || len(field) > 128 {
			return cfg, fmt.Errorf("日志字段名称不合法（ClickHouse 使用字母、数字和下划线；Elasticsearch 可使用点分隔路径）")
		}
	}
	if ds.Type == "elasticsearch" {
		if cfg.Index == "" || len(cfg.Index) > 512 || !logESIndex.MatchString(cfg.Index) {
			return cfg, fmt.Errorf("请输入有效的 Elasticsearch 索引或索引模式")
		}
		for _, index := range strings.Split(cfg.Index, ",") {
			if index == "" || index == "." || index == ".." || strings.HasPrefix(index, "-") {
				return cfg, fmt.Errorf("索引模式不合法")
			}
		}
	} else {
		if !logIdentifier.MatchString(cfg.Database) || !logIdentifier.MatchString(cfg.Table) || len(cfg.Database) > 128 || len(cfg.Table) > 128 {
			return cfg, fmt.Errorf("请填写 ClickHouse 数据库和表名，仅支持字母、数字和下划线")
		}
	}
	return cfg, nil
}

func logDatasourceTypes() []string {
	result := make([]string, 0)
	for _, definition := range datasourceDefinitions {
		for _, capability := range definition.Capabilities {
			if capability == "logs" {
				result = append(result, definition.Type)
				break
			}
		}
	}
	return result
}

// All adapters share cancellation, a response cap, and credential-safe errors.
func doLogRequest(ctx context.Context, ds models.Datasource, method, endpoint string, payload []byte, contentType string) ([]byte, error) {
	client, cleanup, err := datasourceHTTPClient(ds, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("无法连接日志数据源")
	}
	defer cleanup()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("日志数据源地址无效")
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("日志查询失败或超时，请检查数据源连接")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("日志数据源返回 HTTP %d，请检查查询、字段映射、认证和读取权限", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
	if err != nil || len(body) > 16<<20 {
		return nil, fmt.Errorf("日志响应过大或读取失败，请缩小范围或返回条数")
	}
	return body, nil
}

func decodeLogJSON(body []byte, dest any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(dest); err != nil {
		return fmt.Errorf("日志数据源返回无效 JSON 响应")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("日志响应包含不完整结果或查询异常")
	}
	return nil
}

func logField(fields map[string]any, path string) any {
	if value, ok := fields[path]; ok {
		return value
	}
	var value any = fields
	for _, part := range strings.Split(path, ".") {
		object, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = object[part]
	}
	return value
}
func logText(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	b, _ := json.Marshal(value)
	return string(b)
}
func mappedLogLabels(fields map[string]any, cfg logSourceConfig) map[string]string {
	labels := map[string]string{}
	for label, field := range map[string]string{"service": cfg.ServiceField, "level": cfg.LevelField, "trace_id": cfg.TraceIDField} {
		if field != "" {
			if value := logField(fields, field); value != nil {
				labels[label] = logText(value)
			}
		}
	}
	return labels
}

func queryElasticsearch(ctx context.Context, ds models.Datasource, query LogQueryRequest) ([]logEntry, error) {
	cfg, err := readLogSourceConfig(ds)
	if err != nil {
		return nil, err
	}
	search := map[string]any{"match_all": map[string]any{}}
	if strings.TrimSpace(query.Query) != "*" {
		search = map[string]any{"query_string": map[string]any{"query": query.Query, "default_field": cfg.MessageField, "default_operator": "AND"}}
	}
	payload := map[string]any{
		"size": query.Limit, "track_total_hits": false, "timeout": "25s",
		"query":           map[string]any{"bool": map[string]any{"must": []any{search}, "filter": []any{map[string]any{"range": map[string]any{cfg.TimestampField: map[string]any{"gte": query.Start.Format(time.RFC3339Nano), "lt": query.End.Format(time.RFC3339Nano)}}}}}},
		"sort":            []any{map[string]any{cfg.TimestampField: map[string]any{"order": "desc"}}},
		"docvalue_fields": []any{map[string]any{"field": cfg.TimestampField, "format": "strict_date_optional_time_nanos"}},
	}
	body, _ := json.Marshal(payload)
	endpoint := strings.TrimRight(ds.URL, "/") + "/" + url.PathEscape(cfg.Index) + "/_search?allow_partial_search_results=false&ignore_unavailable=false&allow_no_indices=false"
	raw, err := doLogRequest(ctx, ds, http.MethodPost, endpoint, body, "application/json")
	if err != nil {
		return nil, err
	}
	var result struct {
		TimedOut bool            `json:"timed_out"`
		Error    json.RawMessage `json:"error"`
		Shards   struct {
			Failed int `json:"failed"`
		} `json:"_shards"`
		Hits *struct {
			Hits []struct {
				Source map[string]any   `json:"_source"`
				Fields map[string][]any `json:"fields"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := decodeLogJSON(raw, &result); err != nil {
		return nil, err
	}
	if result.Hits == nil || result.TimedOut || result.Shards.Failed > 0 || (len(result.Error) > 0 && string(result.Error) != "null") {
		return nil, fmt.Errorf("Elasticsearch 查询未完整成功，请缩小范围或检查索引状态")
	}
	entries := make([]logEntry, 0, len(result.Hits.Hits))
	for _, hit := range result.Hits.Hits {
		values := hit.Fields[cfg.TimestampField]
		if len(values) != 1 || hit.Source == nil {
			return nil, fmt.Errorf("日志缺少唯一时间字段或 _source，请检查日期字段映射")
		}
		timestamp, err := time.Parse(time.RFC3339Nano, logText(values[0]))
		if err != nil || timestamp.Year() < 1970 || timestamp.Year() > 2261 {
			return nil, fmt.Errorf("Elasticsearch 日志时间格式无效")
		}
		message := logField(hit.Source, cfg.MessageField)
		if message == nil {
			return nil, fmt.Errorf("日志缺少正文字段，请检查字段映射")
		}
		entries = append(entries, logEntry{Timestamp: strconv.FormatInt(timestamp.UnixNano(), 10), Line: logText(message), Labels: mappedLogLabels(hit.Source, cfg), Fields: hit.Source})
	}
	if len(entries) > query.Limit {
		entries = entries[:query.Limit]
	}
	return entries, nil
}

func clickHouseLogSQL(cfg logSourceConfig) string {
	// Identifiers have been validated; values are always bound parameters.
	column := func(name string) string { return "`" + name + "`" }
	selected := []string{cfg.TimestampField, cfg.MessageField}
	for _, field := range []string{cfg.ServiceField, cfg.LevelField, cfg.TraceIDField} {
		if field != "" {
			selected = append(selected, field)
		}
	}
	projections := []string{"toString(toUnixTimestamp64Nano(toDateTime64(" + column(cfg.TimestampField) + ", 9, 'UTC'))) AS __guanlan_timestamp"}
	seen := map[string]bool{}
	for _, field := range selected {
		if !seen[field] {
			projections = append(projections, column(field))
			seen[field] = true
		}
	}
	return "SELECT " + strings.Join(projections, ", ") + " FROM " + column(cfg.Database) + "." + column(cfg.Table) +
		" WHERE " + column(cfg.TimestampField) + " >= fromUnixTimestamp64Nano({start:Int64}) AND " + column(cfg.TimestampField) + " < fromUnixTimestamp64Nano({end:Int64})" +
		" AND ({all:UInt8} = 1 OR positionCaseInsensitiveUTF8(toString(" + column(cfg.MessageField) + "), base64Decode({keyword:String})) > 0)" +
		" ORDER BY " + column(cfg.TimestampField) + " DESC LIMIT {limit:UInt32} FORMAT JSON"
}
func queryClickHouse(ctx context.Context, ds models.Datasource, query LogQueryRequest) ([]logEntry, error) {
	cfg, err := readLogSourceConfig(ds)
	if err != nil {
		return nil, err
	}
	params := url.Values{"readonly": {"1"}, "max_execution_time": {"25"}, "max_result_rows": {strconv.Itoa(query.Limit)}, "max_result_bytes": {"15000000"}, "result_overflow_mode": {"throw"}, "wait_end_of_query": {"1"}, "date_time_output_format": {"iso"}, "param_start": {strconv.FormatInt(query.Start.UnixNano(), 10)}, "param_end": {strconv.FormatInt(query.End.UnixNano(), 10)}, "param_limit": {strconv.Itoa(query.Limit)}, "param_keyword": {base64.StdEncoding.EncodeToString([]byte(query.Query))}, "param_all": {"0"}}
	if strings.TrimSpace(query.Query) == "*" {
		params.Set("param_all", "1")
	}
	raw, err := doLogRequest(ctx, ds, http.MethodPost, strings.TrimRight(ds.URL, "/")+"/?"+params.Encode(), []byte(clickHouseLogSQL(cfg)), "text/plain; charset=utf-8")
	if err != nil {
		return nil, err
	}
	var result struct {
		Data []map[string]any `json:"data"`
		Meta []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"meta"`
		Exception string `json:"exception"`
	}
	if err := decodeLogJSON(raw, &result); err != nil {
		return nil, err
	}
	if result.Data == nil || len(result.Meta) == 0 || result.Exception != "" {
		return nil, fmt.Errorf("ClickHouse 查询未完整成功，请检查表和字段映射")
	}
	validTimeType := false
	for _, field := range result.Meta {
		if field.Name == cfg.TimestampField && (field.Type == "DateTime" || strings.HasPrefix(field.Type, "DateTime(") || strings.HasPrefix(field.Type, "DateTime64(")) {
			validTimeType = true
		}
	}
	if !validTimeType {
		return nil, fmt.Errorf("ClickHouse 时间字段必须为 DateTime 或 DateTime64 类型")
	}
	entries := make([]logEntry, 0, len(result.Data))
	for _, fields := range result.Data {
		timestamp := logText(fields["__guanlan_timestamp"])
		ns, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil || ns < 0 {
			return nil, fmt.Errorf("ClickHouse 返回无效日志时间")
		}
		message, ok := fields[cfg.MessageField]
		if !ok || message == nil {
			return nil, fmt.Errorf("ClickHouse 日志正文字段缺失")
		}
		delete(fields, "__guanlan_timestamp")
		entries = append(entries, logEntry{Timestamp: timestamp, Line: logText(message), Labels: mappedLogLabels(fields, cfg), Fields: fields})
	}
	if len(entries) > query.Limit {
		entries = entries[:query.Limit]
	}
	return entries, nil
}

func testLogSource(ds models.Datasource) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	query := LogQueryRequest{Query: "*", Start: now.Add(-time.Hour), End: now, Limit: 1}
	var err error
	switch ds.Type {
	case "elasticsearch":
		_, err = queryElasticsearch(ctx, ds, query)
	case "clickhouse":
		_, err = queryClickHouse(ctx, ds, query)
	default:
		return false, "不支持的日志数据源"
	}
	if err != nil {
		return false, err.Error()
	}
	return true, "连接成功，日志查询接口可用"
}
