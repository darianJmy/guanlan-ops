package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/logger"
	"guanlan/pkg/resp"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type PromQueryReq struct {
	DatasourceID uint   `json:"datasource_id"`
	Query        string `json:"query" binding:"required"`
	Time         string `json:"time"` // RFC3339 or unix timestamp
}

func PromInstantQuery(c *gin.Context) {
	var req PromQueryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误: "+err.Error())
		return
	}

	ds, err := getPromDatasource(req.DatasourceID)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if err := recordPromQueryHistory(c, ds, req.Query, "table"); err != nil {
		logger.Warn("record Prometheus query history failed", "error", err)
	}

	params := url.Values{}
	params.Set("query", req.Query)
	if req.Time != "" {
		params.Set("time", req.Time)
	}
	apiURL := fmt.Sprintf("%s/api/v1/query?%s", strings.TrimRight(ds.URL, "/"), params.Encode())

	body, err := doPromHTTPContext(c.Request.Context(), ds, apiURL)
	if err != nil {
		resp.Error(c, http.StatusOK, resp.CodeFailure, err.Error())
		return
	}
	writePromResponse(c, body)
}

type PromRangeReq struct {
	DatasourceID uint   `json:"datasource_id"`
	Query        string `json:"query" binding:"required"`
	Start        string `json:"start" binding:"required"`
	End          string `json:"end" binding:"required"`
	Step         string `json:"step" binding:"required"`
}

func PromRangeQuery(c *gin.Context) {
	var req PromRangeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误: "+err.Error())
		return
	}

	ds, err := getPromDatasource(req.DatasourceID)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if err := recordPromQueryHistory(c, ds, req.Query, "graph"); err != nil {
		logger.Warn("record Prometheus range query history failed", "error", err)
	}

	params := url.Values{}
	params.Set("query", req.Query)
	params.Set("start", req.Start)
	params.Set("end", req.End)
	params.Set("step", req.Step)
	apiURL := fmt.Sprintf("%s/api/v1/query_range?%s", strings.TrimRight(ds.URL, "/"), params.Encode())

	body, err := doPromHTTPContext(c.Request.Context(), ds, apiURL)
	if err != nil {
		resp.Error(c, http.StatusOK, resp.CodeFailure, err.Error())
		return
	}
	writePromResponse(c, body)
}

// PromMetadata returns available metrics, label names, or label values
func PromMetadata(c *gin.Context) {
	datasourceID, _ := strconv.Atoi(c.Query("datasource_id"))
	target := c.Query("target") // "metrics" | "labels" | "label_values"
	match := c.Query("match")   // label selector

	ds, err := getPromDatasource(uint(datasourceID))
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}

	var apiURL string
	switch target {
	case "metrics":
		apiURL = fmt.Sprintf("%s/api/v1/label/__name__/values", strings.TrimRight(ds.URL, "/"))
	case "labels":
		apiURL = fmt.Sprintf("%s/api/v1/labels", strings.TrimRight(ds.URL, "/"))
		if match != "" {
			apiURL += "?match[]=" + url.QueryEscape(match)
		}
	case "label_values":
		label := c.Query("label")
		apiURL = fmt.Sprintf("%s/api/v1/label/%s/values", strings.TrimRight(ds.URL, "/"), label)
		if match != "" {
			apiURL += "?match[]=" + url.QueryEscape(match)
		}
	default:
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "unknown target")
		return
	}

	body, err := doPromHTTPContext(c.Request.Context(), ds, apiURL)
	if err != nil {
		resp.Error(c, http.StatusOK, resp.CodeFailure, err.Error())
		return
	}

	writePromResponse(c, body)
}

// writePromResponse writes a Prometheus API response in the standard format.
func writePromResponse(c *gin.Context, body []byte) {
	var promResp struct {
		Status string      `json:"status"`
		Data   interface{} `json:"data"`
		Error  string      `json:"error"`
	}
	json.Unmarshal(body, &promResp)
	if promResp.Status == "success" {
		resp.Success(c, http.StatusOK, "成功", promResp.Data)
		return
	}
	msg := promResp.Error
	if msg == "" {
		msg = string(body)
	}
	resp.Error(c, http.StatusOK, resp.CodeFailure, msg)
}

// PromDatasources returns available Prometheus datasources
func PromDatasources(c *gin.Context) {
	var ds []models.Datasource
	database.DB.Where("status = 1 AND type IN ?", metricDatasourceTypes()).Find(&ds)
	// only return id, name
	type item struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
		URL  string `json:"url"`
		Type string `json:"type"`
	}
	result := make([]item, len(ds))
	for i := range ds {
		result[i] = item{ID: ds[i].ID, Name: ds[i].Name, URL: ds[i].URL, Type: ds[i].Type}
	}
	resp.Success(c, http.StatusOK, "成功", result)
}

func getPromDatasource(id uint) (models.Datasource, error) {
	var ds models.Datasource
	if id > 0 {
		if err := database.DB.First(&ds, id).Error; err != nil {
			return ds, fmt.Errorf("数据源不存在")
		}
	} else {
		// pick first enabled prometheus
		if err := database.DB.Where("status = 1 AND type IN ?", metricDatasourceTypes()).First(&ds).Error; err != nil {
			return ds, fmt.Errorf("没有可用的指标数据源")
		}
	}
	if ds.Status != 1 {
		return ds, fmt.Errorf("数据源实例已禁用")
	}
	if !isMetricDatasourceType(ds.Type) {
		return ds, fmt.Errorf("该数据源不支持 PromQL 指标查询")
	}
	return ds, nil
}

func doPromHTTP(ds models.Datasource, apiURL string) ([]byte, error) {
	return doPromHTTPContext(context.Background(), ds, apiURL)
}

func doPromHTTPContext(ctx context.Context, ds models.Datasource, apiURL string) ([]byte, error) {
	client, cleanup, err := datasourceHTTPClient(ds, 30*time.Second)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 Prometheus 失败: %s", err.Error())
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// On error, wrap as JSON so the frontend can parse it
	if resp.StatusCode != http.StatusOK {
		return []byte(fmt.Sprintf(`{"status":"error","error":"Prometheus returned %d: %s"}`, resp.StatusCode, string(body))), nil
	}
	return body, nil
}

// Decode a Prometheus response (used by chat handler)
func parsePromResponse(body []byte) ([]map[string]interface{}, error) {
	var promResp struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string                   `json:"resultType"`
			Result     []map[string]interface{} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &promResp); err != nil {
		return nil, err
	}
	if promResp.Status != "success" {
		return nil, fmt.Errorf("prometheus error response")
	}
	return promResp.Data.Result, nil
}
