package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"guanlan/pkg/resp"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"guanlan/internal/database"
	"guanlan/internal/models"
)

func explorePermission(userID uint, code string) bool {
	var count int64
	database.DB.Table("permissions AS p").Joins("JOIN role_permissions AS rp ON rp.permission_id = p.id").Joins("JOIN roles AS r ON r.id = rp.role_id AND r.status = 1").Joins("JOIN user_roles AS ur ON ur.role_id = r.id").Where("ur.user_id = ? AND p.status = 1 AND p.code = ?", userID, code).Count(&count)
	return count > 0
}
func ExploreDatasources(c *gin.Context) {
	var sources []models.Datasource
	if err := database.DB.Select("id", "name", "type", "config").Where("status = 1 AND type IN ?", supportedDatasourceTypes()).Order("name,id").Find(&sources).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "读取数据源失败")
		return
	}
	permissions := map[string]bool{}
	for _, code := range []string{"datasource:list", "observability:logs", "observability:traces", "query:sql"} {
		permissions[code] = explorePermission(c.GetUint("user_id"), code)
	}
	result := make([]gin.H, 0)
	for _, ds := range sources {
		modes := []string{}
		if isMetricDatasourceType(ds.Type) && permissions["datasource:list"] {
			modes = append(modes, "metrics")
		}
		if ds.Type == "clickhouse" && permissions["query:sql"] {
			modes = append(modes, "sql")
		}
		if ds.Type == "jaeger" && permissions["observability:traces"] {
			modes = append(modes, "traces")
		}
		if permissions["observability:logs"] && (ds.Type == "loki" || ds.Type == "elasticsearch" || ds.Type == "clickhouse") {
			if _, err := readLogSourceConfig(ds); err == nil {
				modes = append(modes, "logs")
			}
		}
		if len(modes) > 0 {
			result = append(result, gin.H{"id": ds.ID, "name": ds.Name, "type": ds.Type, "modes": modes})
		}
	}
	resp.SuccessData(c, http.StatusOK, result)
}

type SQLQueryRequest struct {
	DatasourceID uint      `json:"datasource_id" binding:"required"`
	Query        string    `json:"query" binding:"required"`
	Start        time.Time `json:"start" binding:"required"`
	End          time.Time `json:"end" binding:"required"`
	Limit        int       `json:"limit"`
}
type sqlResult struct {
	Meta      []map[string]string `json:"meta"`
	Data      []map[string]any    `json:"data"`
	Exception string              `json:"exception"`
}

// A small lexer validates the statement envelope, not SQL semantics. ClickHouse
// enforces readonly independently. Comments/strings cannot bypass the envelope.
func validateExploreSQL(sql string) error {
	tokens := []string{}
	for i := 0; i < len(sql); {
		ch := sql[i]
		if ch == '\'' || ch == '"' || ch == '`' {
			quote := ch
			i++
			closed := false
			for i < len(sql) {
				if sql[i] == '\\' {
					i += 2
					continue
				}
				if sql[i] == quote {
					if i+1 < len(sql) && sql[i+1] == quote {
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return fmt.Errorf("SQL 字符串未闭合")
			}
			continue
		}
		if i+1 < len(sql) && sql[i:i+2] == "--" {
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(sql) && sql[i:i+2] == "/*" {
			end := strings.Index(sql[i+2:], "*/")
			if end < 0 {
				return fmt.Errorf("SQL 注释未闭合")
			}
			i += end + 4
			continue
		}
		if ch == ';' {
			return fmt.Errorf("仅支持单条 SELECT / WITH 查询，请移除分号")
		}
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_' {
			start := i
			i++
			for i < len(sql) && (sql[i] >= 'a' && sql[i] <= 'z' || sql[i] >= 'A' && sql[i] <= 'Z' || sql[i] >= '0' && sql[i] <= '9' || sql[i] == '_') {
				i++
			}
			tokens = append(tokens, strings.ToUpper(sql[start:i]))
			continue
		}
		i++
	}
	if len(tokens) == 0 || (tokens[0] != "SELECT" && tokens[0] != "WITH") {
		return fmt.Errorf("仅支持只读 SELECT / WITH 查询")
	}
	for _, token := range tokens {
		switch token {
		case "INSERT", "UPDATE", "DELETE", "ALTER", "DROP", "CREATE", "TRUNCATE", "ATTACH", "DETACH", "RENAME", "GRANT", "REVOKE", "SYSTEM", "KILL", "SETTINGS", "FORMAT", "INTO", "OUTFILE":
			return fmt.Errorf("查询不能包含写操作、SETTINGS、FORMAT 或文件输出")
		}
	}
	return nil
}
func queryClickHouseSQL(ctx context.Context, ds models.Datasource, req SQLQueryRequest) (sqlResult, error) {
	var result sqlResult
	if err := validateExploreSQL(req.Query); err != nil {
		return result, err
	}
	params := url.Values{"readonly": {"1"}, "max_execution_time": {"25"}, "max_result_bytes": {"15000000"}, "result_overflow_mode": {"throw"}, "wait_end_of_query": {"1"}, "param_from": {strconv.FormatInt(req.Start.Unix(), 10)}, "param_to": {strconv.FormatInt(req.End.Unix(), 10)}}
	// Outer LIMIT bounds both ordinary and aggregate SELECTs. A newline ends a trailing SQL comment.
	query := fmt.Sprintf("SELECT * FROM (\n%s\n) AS explore_result LIMIT %d FORMAT JSON", req.Query, req.Limit)
	raw, err := doLogRequest(ctx, ds, http.MethodPost, strings.TrimRight(ds.URL, "/")+"/?"+params.Encode(), []byte(query), "text/plain; charset=utf-8")
	if err != nil {
		return result, err
	}
	if err = decodeLogJSON(raw, &result); err != nil {
		return result, err
	}
	if result.Meta == nil || result.Data == nil || result.Exception != "" {
		return result, fmt.Errorf("ClickHouse 未返回完整表格结果")
	}
	if len(result.Data) > req.Limit {
		result.Data = result.Data[:req.Limit]
	}
	return result, nil
}
func QuerySQL(c *gin.Context) {
	var req SQLQueryRequest
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请提供 SQL、数据源和时间范围")
		return
	}
	if req.Limit == 0 {
		req.Limit = 500
	}
	if req.Limit < 1 || req.Limit > 5000 || len(req.Query) > 65536 || !req.End.After(req.Start) || req.Start.Year() < 1970 || req.End.Year() > 2261 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "查询参数或时间范围无效，最多返回 5000 行")
		return
	}
	if err := validateExploreSQL(req.Query); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	var ds models.Datasource
	if database.DB.Where("id = ? AND type = ? AND status = 1", req.DatasourceID, "clickhouse").First(&ds).Error != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "ClickHouse 数据源不存在或已禁用")
		return
	}
	result, err := queryClickHouseSQL(c.Request.Context(), ds, req)
	if err != nil {
		resp.Error(c, http.StatusBadGateway, resp.CodeBadGateway, err.Error())
		return
	}
	resp.SuccessData(c, http.StatusOK, gin.H{"columns": result.Meta, "rows": result.Data, "limit_reached": len(result.Data) >= req.Limit})
}
func testClickHouseExplore(ds models.Datasource) (bool, string) {
	if _, err := readLogSourceConfig(ds); err == nil {
		return testLogSource(ds)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := queryClickHouseSQL(ctx, ds, SQLQueryRequest{Query: "SELECT 1 AS connected", Start: time.Now().Add(-time.Hour), End: time.Now(), Limit: 1})
	if err != nil {
		return false, err.Error()
	}
	return true, "ClickHouse SQL 查询接口连接成功"
}

type traceSpan struct {
	TraceID    string `json:"traceID"`
	SpanID     string `json:"spanID"`
	Operation  string `json:"operationName"`
	Start      int64  `json:"startTime"`
	Duration   int64  `json:"duration"`
	ProcessID  string `json:"processID"`
	References []struct {
		Type   string `json:"refType"`
		SpanID string `json:"spanID"`
	} `json:"references"`
	Tags []map[string]any `json:"tags"`
}
type traceData struct {
	TraceID   string      `json:"traceID"`
	Spans     []traceSpan `json:"spans"`
	Processes map[string]struct {
		Service string `json:"serviceName"`
	} `json:"processes"`
}

func QueryTraces(c *gin.Context) {
	var req struct {
		DatasourceID uint   `json:"datasource_id" binding:"required"`
		TraceID      string `json:"trace_id" binding:"required"`
	}
	if c.ShouldBindJSON(&req) != nil || !regexp.MustCompile(`^(?:[a-fA-F0-9]{16}|[a-fA-F0-9]{32})$`).MatchString(req.TraceID) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请输入 16 或 32 位十六进制 Trace ID")
		return
	}
	var ds models.Datasource
	if database.DB.Where("id = ? AND type = ? AND status = 1", req.DatasourceID, "jaeger").First(&ds).Error != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "Jaeger 数据源不存在或已禁用")
		return
	}
	raw, err := doLogRequest(c.Request.Context(), ds, http.MethodGet, strings.TrimRight(ds.URL, "/")+"/api/traces/"+req.TraceID, nil, "")
	var result struct {
		Data   []traceData       `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if err == nil {
		err = decodeLogJSON(raw, &result)
	}
	if err != nil || len(result.Errors) > 0 || result.Data == nil {
		resp.Error(c, http.StatusBadGateway, resp.CodeBadGateway, "链路查询失败，请检查 Trace ID、数据源及读取权限")
		return
	}
	for _, trace := range result.Data {
		if !strings.EqualFold(trace.TraceID, req.TraceID) || trace.Spans == nil || len(trace.Spans) > 10000 {
			resp.Error(c, http.StatusBadGateway, resp.CodeBadGateway, "链路结果无效或 Span 数量过多")
			return
		}
		for _, span := range trace.Spans {
			if span.Start < 0 || span.Duration < 0 || span.Start > 9000000000000000 || span.Duration > 9000000000000000-span.Start {
				resp.Error(c, http.StatusBadGateway, resp.CodeBadGateway, "Span 时间无效")
				return
			}
		}
	}
	resp.SuccessData(c, http.StatusOK, result.Data)
}
func testJaegerExplore(ds models.Datasource) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := doLogRequest(ctx, ds, http.MethodGet, strings.TrimRight(ds.URL, "/")+"/api/services", nil, "")
	var result struct {
		Data []string `json:"data"`
	}
	if err != nil || decodeLogJSON(raw, &result) != nil || result.Data == nil {
		return false, "Jaeger 查询接口不可用"
	}
	return true, "Jaeger 查询接口连接成功"
}
