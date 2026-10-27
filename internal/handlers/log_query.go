package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"guanlan/pkg/logger"
	"guanlan/pkg/resp"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"guanlan/internal/database"
	"guanlan/internal/models"
)

type LogQueryRequest struct {
	DatasourceID uint      `json:"datasource_id" binding:"required"`
	Query        string    `json:"query" binding:"required"`
	Start        time.Time `json:"start" binding:"required"`
	End          time.Time `json:"end" binding:"required"`
	Limit        int       `json:"limit"`
}

type logEntry struct {
	Timestamp string            `json:"timestamp"`
	Line      string            `json:"line"`
	Labels    map[string]string `json:"labels"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Fields    map[string]any    `json:"fields,omitempty"`
}

func LogDatasources(c *gin.Context) {
	var sources []models.Datasource
	if err := database.DB.Select("id", "name", "type", "labels").Where("type IN ? AND status = 1", logDatasourceTypes()).Order("name, id").Find(&sources).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "读取日志数据源失败")
		return
	}
	// Explicit projection prevents credentials and connection configuration from leaking.
	result := make([]gin.H, 0, len(sources))
	for _, ds := range sources {
		result = append(result, gin.H{"id": ds.ID, "name": ds.Name, "type": ds.Type, "labels": ds.Labels})
	}
	resp.SuccessData(c, http.StatusOK, result)
}

func QueryLogs(c *gin.Context) {
	var req LogQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请提供数据源、查询语句和有效时间范围")
		return
	}
	if req.Limit == 0 {
		req.Limit = 500
	}
	if strings.TrimSpace(req.Query) == "" || len(req.Query) > 65536 || !req.End.After(req.Start) || req.End.Sub(req.Start) > 31*24*time.Hour || req.Start.Year() < 1970 || req.End.Year() > 2261 || req.Limit < 1 || req.Limit > 5000 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "时间范围须在 31 天内，返回条数须为 1–5000，查询语句不能为空或超过 64 KB")
		return
	}
	var ds models.Datasource
	if err := database.DB.Where("id = ? AND type IN ? AND status = 1", req.DatasourceID, logDatasourceTypes()).First(&ds).Error; err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "日志数据源不存在、已禁用或暂不支持查询")
		return
	}
	if ds.Type == "loki" || ds.Type == "elasticsearch" {
		if err := recordPromQueryHistory(c, ds, req.Query, "logs"); err != nil {
			logger.Warn("record log query history failed", "error", err)
		}
	}
	var entries []logEntry
	var err error
	switch ds.Type {
	case "loki":
		entries, err = queryLoki(c.Request.Context(), ds, req)
	case "elasticsearch":
		entries, err = queryElasticsearch(c.Request.Context(), ds, req)
	case "clickhouse":
		entries, err = queryClickHouse(c.Request.Context(), ds, req)
	default:
		err = fmt.Errorf("暂不支持此日志数据源")
	}
	if err != nil {
		resp.Error(c, http.StatusBadGateway, resp.CodeBadGateway, err.Error())
		return
	}
	resp.SuccessData(c, http.StatusOK, gin.H{"entries": entries, "limit": req.Limit, "limit_reached": len(entries) >= req.Limit})
}

func queryLoki(ctx context.Context, ds models.Datasource, query LogQueryRequest) ([]logEntry, error) {
	params := url.Values{"query": {query.Query}, "start": {query.Start.Format(time.RFC3339Nano)}, "end": {query.End.Format(time.RFC3339Nano)}, "limit": {strconv.Itoa(query.Limit)}, "direction": {"backward"}}
	client, cleanup, err := datasourceHTTPClient(ds, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("无法连接日志数据源")
	}
	defer cleanup()
	// Never forward stored credentials through an upstream redirect.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(ds.URL, "/")+"/loki/api/v1/query_range?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("日志数据源地址无效")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("日志查询失败或超时，请检查数据源连接")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Loki 返回 HTTP %d，请检查查询语法、权限和时间范围", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
	if err != nil || len(body) > 16<<20 {
		return nil, fmt.Errorf("日志响应过大或读取失败，请缩小时间范围或返回条数")
	}
	var result struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Stream map[string]string   `json:"stream"`
				Values [][]json.RawMessage `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &result) != nil || result.Status != "success" {
		return nil, fmt.Errorf("Loki 返回无效响应")
	}
	if result.Data.ResultType != "streams" {
		return nil, fmt.Errorf("请输入返回日志行的 LogQL 查询，暂不支持日志聚合指标")
	}
	entries := make([]logEntry, 0)
	for _, stream := range result.Data.Result {
		for _, value := range stream.Values {
			var entry logEntry
			if len(value) < 2 || json.Unmarshal(value[0], &entry.Timestamp) != nil || json.Unmarshal(value[1], &entry.Line) != nil {
				return nil, fmt.Errorf("Loki 日志格式无效")
			}
			if _, err := strconv.ParseInt(entry.Timestamp, 10, 64); err != nil {
				return nil, fmt.Errorf("Loki 时间戳无效")
			}
			entry.Labels = stream.Stream
			if len(value) > 2 {
				if json.Unmarshal(value[2], &entry.Metadata) != nil {
					return nil, fmt.Errorf("Loki 日志元数据无效")
				}
			}
			entries = append(entries, entry)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		a, _ := strconv.ParseInt(entries[i].Timestamp, 10, 64)
		b, _ := strconv.ParseInt(entries[j].Timestamp, 10, 64)
		return a > b
	})
	if len(entries) > query.Limit {
		entries = entries[:query.Limit]
	}
	return entries, nil
}
