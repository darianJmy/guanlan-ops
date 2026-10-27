package handlers

import (
	"encoding/json"
	"fmt"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Definitions are the server-owned source of supported types and capabilities.
// Adding logs/traces requires a real adapter before exposing a new definition.
// Protocol is independent of product type: VictoriaMetrics shares PromQL APIs.
type datasourceDefinition struct {
	Type           string   `json:"type"`
	Name           string   `json:"name"`
	Category       string   `json:"category"`
	Capabilities   []string `json:"capabilities"`
	Protocol       string   `json:"protocol"`
	URLPlaceholder string   `json:"url_placeholder"`
	URLHelp        string   `json:"url_help"`
	testConnection func(models.Datasource) (bool, string)
}

var datasourceDefinitions = []datasourceDefinition{
	{Type: "prometheus", Name: "Prometheus", Category: "observability", Capabilities: []string{"metrics"}, Protocol: "promql", URLPlaceholder: "http://prometheus.example.com:9090", URLHelp: "填写 Prometheus 服务地址，不包含 /api/v1/query。", testConnection: testPrometheus},
	{Type: "victoriametrics", Name: "VictoriaMetrics", Category: "observability", Capabilities: []string{"metrics"}, Protocol: "promql", URLPlaceholder: "http://victoriametrics.example.com:8428", URLHelp: "单机版：http://host:8428；集群版：http://vmselect:8481/select/0/prometheus（0 为租户 ID）。填写查询地址，不要填写 vminsert 写入地址。", testConnection: testVictoriaMetrics},
	{Type: "loki", Name: "Loki", Category: "observability", Capabilities: []string{"logs"}, Protocol: "logql", URLPlaceholder: "http://loki.example.com:3100", URLHelp: "填写 Loki 查询服务地址，不包含 /loki/api/v1。多租户可在高级配置添加 X-Scope-OrgID。", testConnection: testLoki},
	{Type: "elasticsearch", Name: "Elasticsearch", Category: "observability", Capabilities: []string{"logs"}, Protocol: "elasticsearch", URLPlaceholder: "http://elasticsearch.example.com:9200", URLHelp: "填写 Elasticsearch HTTP/HTTPS 地址，索引在日志配置中填写。支持自定义 HTTP 头配置 ApiKey 认证。", testConnection: testLogSource},
	{Type: "clickhouse", Name: "ClickHouse", Category: "database", Capabilities: []string{"sql", "logs"}, Protocol: "clickhouse", URLPlaceholder: "http://clickhouse.example.com:8123", URLHelp: "填写 ClickHouse HTTP 地址（常用端口 8123；HTTPS 8443），使用具备目标日志表读取权限的账号。", testConnection: testClickHouseExplore},
	{Type: "jaeger", Name: "Jaeger", Category: "observability", Capabilities: []string{"traces"}, Protocol: "jaeger", URLPlaceholder: "http://jaeger.example.com:16686", URLHelp: "填写 Jaeger Query HTTP 服务地址，支持按 Trace ID 查询。", testConnection: testJaegerExplore},
}

func datasourceDefinitionFor(kind string) (datasourceDefinition, bool) {
	for _, definition := range datasourceDefinitions {
		if definition.Type == kind {
			return definition, true
		}
	}
	return datasourceDefinition{}, false
}

func supportedDatasourceTypes() []string {
	types := make([]string, 0, len(datasourceDefinitions))
	for _, definition := range datasourceDefinitions {
		types = append(types, definition.Type)
	}
	return types
}

func isSupportedDatasourceType(kind string) bool {
	_, ok := datasourceDefinitionFor(kind)
	return ok
}

func metricDatasourceTypes() []string {
	types := make([]string, 0)
	for _, definition := range datasourceDefinitions {
		if isMetricDatasourceType(definition.Type) {
			types = append(types, definition.Type)
		}
	}
	return types
}

func isMetricDatasourceType(kind string) bool {
	definition, ok := datasourceDefinitionFor(kind)
	if !ok || definition.Protocol != "promql" {
		return false
	}
	for _, capability := range definition.Capabilities {
		if capability == "metrics" {
			return true
		}
	}
	return false
}

func ListDatasourceTypes(c *gin.Context) {
	resp.Success(c, http.StatusOK, "成功", datasourceDefinitions)
}

// A constant query validates the actual read endpoint, including tenant path and auth.
// VictoriaMetrics does not need to implement Prometheus buildinfo to be usable.
func testVictoriaMetrics(ds models.Datasource) (bool, string) {
	client, cleanup, err := datasourceHTTPClient(ds, 5*time.Second)
	if err != nil {
		return false, err.Error()
	}
	defer cleanup()
	response, err := client.Get(strings.TrimRight(ds.URL, "/") + "/api/v1/query?query=vector%281%29")
	if err != nil {
		return false, "连接失败: " + err.Error()
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false, fmt.Sprintf("VictoriaMetrics 返回 HTTP %d", response.StatusCode)
	}
	var result struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Value []json.RawMessage `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return false, "不是有效的 VictoriaMetrics 查询 API 响应"
	}
	if result.Status != "success" || result.Data.ResultType != "vector" || len(result.Data.Result) != 1 || len(result.Data.Result[0].Value) != 2 || string(result.Data.Result[0].Value[1]) != `"1"` {
		return false, "未返回有效的指标查询结果，请检查查询地址和租户路径"
	}
	return true, "连接成功 (VictoriaMetrics 指标查询可用)"
}
