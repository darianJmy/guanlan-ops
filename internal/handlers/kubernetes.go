package handlers

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"guanlan/pkg/textutil"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type K8sResourceReq struct {
	DatasourceID  uint   `json:"datasource_id"`
	Resource      string `json:"resource"`
	Namespace     string `json:"namespace"`
	Name          string `json:"name"`
	LabelSelector string `json:"label_selector"`
	FieldSelector string `json:"field_selector"`
	Limit         int    `json:"limit"`
}

type K8sResourceItem struct {
	Name      string                 `json:"name"`
	Namespace string                 `json:"namespace,omitempty"`
	Kind      string                 `json:"kind,omitempty"`
	Status    string                 `json:"status,omitempty"`
	Age       string                 `json:"age,omitempty"`
	Labels    map[string]interface{} `json:"labels,omitempty"`
	Raw       map[string]interface{} `json:"raw,omitempty"`
}

type K8sResourceResult struct {
	Datasource string                 `json:"datasource"`
	Resource   string                 `json:"resource"`
	Namespace  string                 `json:"namespace,omitempty"`
	Items      []K8sResourceItem      `json:"items"`
	Raw        map[string]interface{} `json:"raw,omitempty"`
}

type K8sNamespaceResourceSummary struct {
	Datasource           string            `json:"datasource"`
	Namespace            string            `json:"namespace"`
	PodCount             int               `json:"pod_count"`
	RequestCPU           float64           `json:"request_cpu_cores"`
	LimitCPU             float64           `json:"limit_cpu_cores"`
	RequestMemoryBytes   int64             `json:"request_memory_bytes"`
	LimitMemoryBytes     int64             `json:"limit_memory_bytes"`
	QuotaRequestCPU      float64           `json:"quota_request_cpu_cores,omitempty"`
	QuotaLimitCPU        float64           `json:"quota_limit_cpu_cores,omitempty"`
	QuotaRequestMemory   int64             `json:"quota_request_memory_bytes,omitempty"`
	QuotaLimitMemory     int64             `json:"quota_limit_memory_bytes,omitempty"`
	QuotaHardLimitCPU    float64           `json:"quota_hard_limit_cpu_cores,omitempty"`
	QuotaHardLimitMemory int64             `json:"quota_hard_limit_memory_bytes,omitempty"`
	Quotas               []K8sQuotaSummary `json:"quotas,omitempty"`
}

type K8sQuotaSummary struct {
	Name string            `json:"name"`
	Used map[string]string `json:"used"`
	Hard map[string]string `json:"hard"`
}

func ListK8sResources(c *gin.Context) {
	var req K8sResourceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	result, err := listK8sResources(req)
	if err != nil {
		resp.Error(c, http.StatusOK, resp.CodeFailure, err.Error())
		return
	}
	resp.Success(c, http.StatusOK, "成功", result)
}

func GetK8sResource(c *gin.Context) {
	var req K8sResourceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		resp.Error(c, http.StatusOK, resp.CodeFailure, "请指定资源名称")
		return
	}
	result, err := getK8sResource(req)
	if err != nil {
		resp.Error(c, http.StatusOK, resp.CodeFailure, err.Error())
		return
	}
	resp.Success(c, http.StatusOK, "成功", result)
}

func listK8sResources(req K8sResourceReq) (K8sResourceResult, error) {
	ds, err := selectK8sDatasource(req.DatasourceID, "")
	if err != nil {
		return K8sResourceResult{}, err
	}
	path, namespaced, canonical, err := k8sResourcePath(req.Resource, req.Namespace, "")
	if err != nil {
		return K8sResourceResult{}, err
	}
	query := url.Values{}
	if strings.TrimSpace(req.LabelSelector) != "" {
		query.Set("labelSelector", req.LabelSelector)
	}
	if strings.TrimSpace(req.FieldSelector) != "" {
		query.Set("fieldSelector", req.FieldSelector)
	}
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}

	raw, err := executeK8sJSON(ds, path)
	if err != nil {
		return K8sResourceResult{}, err
	}
	items := parseK8sResourceItems(raw, canonical, req.Limit)
	if !namespaced {
		req.Namespace = ""
	}
	return K8sResourceResult{Datasource: ds.Name, Resource: canonical, Namespace: req.Namespace, Items: items}, nil
}

func getK8sResource(req K8sResourceReq) (K8sResourceResult, error) {
	ds, err := selectK8sDatasource(req.DatasourceID, "")
	if err != nil {
		return K8sResourceResult{}, err
	}
	path, namespaced, canonical, err := k8sResourcePath(req.Resource, req.Namespace, req.Name)
	if err != nil {
		return K8sResourceResult{}, err
	}
	raw, err := executeK8sJSON(ds, path)
	if err != nil {
		return K8sResourceResult{}, err
	}
	if !namespaced {
		req.Namespace = ""
	}
	return K8sResourceResult{Datasource: ds.Name, Resource: canonical, Namespace: req.Namespace, Raw: raw}, nil
}

func executeK8sAPIGet(ds models.Datasource, path, view string) (map[string]interface{}, error) {
	return executeK8sAPIGetContext(context.Background(), ds, path, view)
}

func executeK8sAPIGetContext(ctx context.Context, ds models.Datasource, path, view string) (map[string]interface{}, error) {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" || !strings.HasPrefix(cleanPath, "/") {
		return nil, fmt.Errorf("Kubernetes API path 必须以 / 开头")
	}
	if strings.Contains(cleanPath, "://") || strings.Contains(cleanPath, "..") {
		return nil, fmt.Errorf("Kubernetes API path 不合法")
	}
	if !isAllowedK8sAPIPath(cleanPath) {
		return nil, fmt.Errorf("不允许访问该 Kubernetes API path: %s", cleanPath)
	}
	raw, err := executeK8sJSONContext(ctx, ds, cleanPath)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(view, "compact") {
		return compactK8sAPIResult(cleanPath, raw), nil
	}
	return sanitizeK8sAPIResult(cleanPath, raw), nil
}

func isAllowedK8sAPIPath(path string) bool {
	allowedPrefixes := []string{
		"/api/v1",
		"/apis/apps/v1",
		"/apis/batch/v1",
		"/apis/networking.k8s.io/v1",
		"/apis/autoscaling/v1",
		"/apis/autoscaling/v2",
		"/apis/monitoring.coreos.com/v1",
	}
	for _, prefix := range allowedPrefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") || strings.HasPrefix(path, prefix+"?") {
			return true
		}
	}
	return false
}

func sanitizeK8sAPIResult(path string, raw map[string]interface{}) map[string]interface{} {
	if strings.Contains(path, "/secrets") {
		redactSecretData(raw)
	}
	body, err := json.Marshal(raw)
	if err != nil || len(body) <= 20000 {
		return raw
	}
	return map[string]interface{}{
		"truncated": true,
		"message":   "Kubernetes 响应过大，已截断；请缩小 namespace、labelSelector 或资源范围。",
		"preview":   textutil.Ellipsis(string(body), 20000),
	}
}

func compactK8sAPIResult(path string, raw map[string]interface{}) map[string]interface{} {
	switch {
	case isK8sListPath(path, "pods"):
		return compactPodList(raw)
	case isK8sListPath(path, "ingresses"):
		return compactIngressList(raw)
	case isK8sListPath(path, "deployments"), isK8sListPath(path, "statefulsets"), isK8sListPath(path, "daemonsets"), isK8sListPath(path, "replicasets"):
		return compactWorkloadList(raw)
	case isK8sListPath(path, "services"):
		return compactServiceList(raw)
	case isK8sListPath(path, "nodes"):
		return compactNodeList(raw)
	default:
		return sanitizeK8sAPIResult(path, raw)
	}
}

func isK8sListPath(path, resource string) bool {
	cleanPath := strings.Split(path, "?")[0]
	return strings.HasSuffix(cleanPath, "/"+resource)
}

func compactPodList(raw map[string]interface{}) map[string]interface{} {
	rawItems, _ := raw["items"].([]interface{})
	items := make([]map[string]interface{}, 0, len(rawItems))
	abnormal := 0
	for _, rawItem := range rawItems {
		item, _ := rawItem.(map[string]interface{})
		meta, _ := item["metadata"].(map[string]interface{})
		status, _ := item["status"].(map[string]interface{})
		spec, _ := item["spec"].(map[string]interface{})
		ready, total, restarts, reasons, messages := summarizePodContainers(status)
		phase := stringFromMap(status, "phase")
		isAbnormal := podIsAbnormal(phase, ready, total, restarts, reasons, meta)
		if isAbnormal {
			abnormal++
		}
		items = append(items, map[string]interface{}{
			"namespace": stringFromMap(meta, "namespace"),
			"name":      stringFromMap(meta, "name"),
			"phase":     phase,
			"ready":     fmt.Sprintf("%d/%d", ready, total),
			"restarts":  restarts,
			"reasons":   reasons,
			"messages":  messages,
			"node":      stringFromMap(spec, "nodeName"),
			"age":       ageText(stringFromMap(meta, "creationTimestamp")),
			"abnormal":  isAbnormal,
		})
	}
	return map[string]interface{}{
		"kind":           "PodList",
		"total":          len(items),
		"abnormal_total": abnormal,
		"items":          items,
	}
}

func summarizePodContainers(status map[string]interface{}) (int, int, int, []string, []string) {
	var ready, total, restarts int
	reasons := make([]string, 0)
	messages := make([]string, 0)
	for _, field := range []string{"initContainerStatuses", "containerStatuses"} {
		statuses, _ := status[field].([]interface{})
		for _, rawStatus := range statuses {
			total++
			containerStatus, _ := rawStatus.(map[string]interface{})
			if value, ok := containerStatus["ready"].(bool); ok && value {
				ready++
			}
			restarts += intFromMap(containerStatus, "restartCount")
			state, _ := containerStatus["state"].(map[string]interface{})
			for _, stateName := range []string{"waiting", "terminated"} {
				stateValue, _ := state[stateName].(map[string]interface{})
				reason := stringFromMap(stateValue, "reason")
				message := stringFromMap(stateValue, "message")
				if reason != "" && reason != "Completed" {
					reasons = appendUnique(reasons, reason)
				}
				if message != "" {
					messages = appendUnique(messages, textutil.Ellipsis(message, 220))
				}
			}
		}
	}
	return ready, total, restarts, reasons, messages
}

func podIsAbnormal(phase string, ready, total, restarts int, reasons []string, meta map[string]interface{}) bool {
	if stringFromMap(meta, "deletionTimestamp") != "" {
		return true
	}
	if phase != "" && phase != "Running" && phase != "Succeeded" {
		return true
	}
	if total > 0 && ready < total && phase != "Succeeded" {
		return true
	}
	if restarts > 3 {
		return true
	}
	return len(reasons) > 0
}

func compactIngressList(raw map[string]interface{}) map[string]interface{} {
	rawItems, _ := raw["items"].([]interface{})
	items := make([]map[string]interface{}, 0, len(rawItems))
	for _, rawItem := range rawItems {
		item, _ := rawItem.(map[string]interface{})
		meta, _ := item["metadata"].(map[string]interface{})
		spec, _ := item["spec"].(map[string]interface{})
		items = append(items, map[string]interface{}{
			"namespace": stringFromMap(meta, "namespace"),
			"name":      stringFromMap(meta, "name"),
			"class":     stringFromMap(spec, "ingressClassName"),
			"hosts":     ingressHosts(spec),
			"age":       ageText(stringFromMap(meta, "creationTimestamp")),
		})
	}
	return map[string]interface{}{"kind": "IngressList", "total": len(items), "items": items}
}

func compactWorkloadList(raw map[string]interface{}) map[string]interface{} {
	kind := defaultString(stringFromMap(raw, "kind"), "WorkloadList")
	rawItems, _ := raw["items"].([]interface{})
	items := make([]map[string]interface{}, 0, len(rawItems))
	for _, rawItem := range rawItems {
		item, _ := rawItem.(map[string]interface{})
		meta, _ := item["metadata"].(map[string]interface{})
		status, _ := item["status"].(map[string]interface{})
		items = append(items, map[string]interface{}{
			"namespace": stringFromMap(meta, "namespace"),
			"name":      stringFromMap(meta, "name"),
			"replicas":  intFromMap(status, "replicas"),
			"ready":     intFromMap(status, "readyReplicas"),
			"available": intFromMap(status, "availableReplicas"),
			"updated":   intFromMap(status, "updatedReplicas"),
			"age":       ageText(stringFromMap(meta, "creationTimestamp")),
			"abnormal":  intFromMap(status, "replicas") != intFromMap(status, "readyReplicas"),
		})
	}
	return map[string]interface{}{"kind": kind, "total": len(items), "items": items}
}

func compactServiceList(raw map[string]interface{}) map[string]interface{} {
	rawItems, _ := raw["items"].([]interface{})
	items := make([]map[string]interface{}, 0, len(rawItems))
	for _, rawItem := range rawItems {
		item, _ := rawItem.(map[string]interface{})
		meta, _ := item["metadata"].(map[string]interface{})
		spec, _ := item["spec"].(map[string]interface{})
		items = append(items, map[string]interface{}{
			"namespace":  stringFromMap(meta, "namespace"),
			"name":       stringFromMap(meta, "name"),
			"type":       stringFromMap(spec, "type"),
			"cluster_ip": stringFromMap(spec, "clusterIP"),
			"age":        ageText(stringFromMap(meta, "creationTimestamp")),
		})
	}
	return map[string]interface{}{"kind": "ServiceList", "total": len(items), "items": items}
}

func compactNodeList(raw map[string]interface{}) map[string]interface{} {
	rawItems, _ := raw["items"].([]interface{})
	items := make([]map[string]interface{}, 0, len(rawItems))
	for _, rawItem := range rawItems {
		item, _ := rawItem.(map[string]interface{})
		meta, _ := item["metadata"].(map[string]interface{})
		status, _ := item["status"].(map[string]interface{})
		items = append(items, map[string]interface{}{
			"name":   stringFromMap(meta, "name"),
			"status": k8sResourceStatus("nodes", status),
			"age":    ageText(stringFromMap(meta, "creationTimestamp")),
		})
	}
	return map[string]interface{}{"kind": "NodeList", "total": len(items), "items": items}
}

func ingressHosts(spec map[string]interface{}) []string {
	rules, _ := spec["rules"].([]interface{})
	hosts := make([]string, 0, len(rules))
	for _, rawRule := range rules {
		rule, _ := rawRule.(map[string]interface{})
		if host := stringFromMap(rule, "host"); host != "" {
			hosts = append(hosts, host)
		}
	}
	return hosts
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func redactSecretData(value interface{}) {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, item := range typed {
			if key == "data" || key == "stringData" {
				typed[key] = map[string]interface{}{"redacted": true}
				continue
			}
			redactSecretData(item)
		}
	case []interface{}:
		for _, item := range typed {
			redactSecretData(item)
		}
	}
}

func summarizeK8sNamespaceResources(ds models.Datasource, namespace string) (K8sNamespaceResourceSummary, error) {
	ns := strings.TrimSpace(namespace)
	if ns == "" {
		return K8sNamespaceResourceSummary{}, fmt.Errorf("请指定 Kubernetes namespace")
	}
	summary := K8sNamespaceResourceSummary{Datasource: ds.Name, Namespace: ns}

	pods, err := executeK8sJSON(ds, "/api/v1/namespaces/"+url.PathEscape(ns)+"/pods")
	if err != nil {
		return summary, err
	}
	podItems, _ := pods["items"].([]interface{})
	summary.PodCount = len(podItems)
	for _, rawPod := range podItems {
		pod, _ := rawPod.(map[string]interface{})
		spec, _ := pod["spec"].(map[string]interface{})
		containers, _ := spec["containers"].([]interface{})
		for _, rawContainer := range containers {
			addContainerResources(&summary, rawContainer)
		}
		initContainers, _ := spec["initContainers"].([]interface{})
		for _, rawContainer := range initContainers {
			addContainerResources(&summary, rawContainer)
		}
	}

	quotas, err := executeK8sJSON(ds, "/api/v1/namespaces/"+url.PathEscape(ns)+"/resourcequotas")
	if err == nil {
		parseResourceQuotas(&summary, quotas)
	}
	return summary, nil
}

func addContainerResources(summary *K8sNamespaceResourceSummary, rawContainer interface{}) {
	container, _ := rawContainer.(map[string]interface{})
	resources, _ := container["resources"].(map[string]interface{})
	requests, _ := resources["requests"].(map[string]interface{})
	limits, _ := resources["limits"].(map[string]interface{})
	summary.RequestCPU += parseCPUQuantity(valueString(requests["cpu"]))
	summary.LimitCPU += parseCPUQuantity(valueString(limits["cpu"]))
	summary.RequestMemoryBytes += parseMemoryQuantity(valueString(requests["memory"]))
	summary.LimitMemoryBytes += parseMemoryQuantity(valueString(limits["memory"]))
}

func parseResourceQuotas(summary *K8sNamespaceResourceSummary, raw map[string]interface{}) {
	items, _ := raw["items"].([]interface{})
	for _, rawItem := range items {
		item, _ := rawItem.(map[string]interface{})
		meta, _ := item["metadata"].(map[string]interface{})
		status, _ := item["status"].(map[string]interface{})
		used := stringMap(status["used"])
		hard := stringMap(status["hard"])
		summary.Quotas = append(summary.Quotas, K8sQuotaSummary{
			Name: stringFromMap(meta, "name"),
			Used: used,
			Hard: hard,
		})
		summary.QuotaRequestCPU += parseCPUQuantity(used["requests.cpu"])
		summary.QuotaLimitCPU += parseCPUQuantity(used["limits.cpu"])
		summary.QuotaRequestMemory += parseMemoryQuantity(used["requests.memory"])
		summary.QuotaLimitMemory += parseMemoryQuantity(used["limits.memory"])
		summary.QuotaHardLimitCPU += parseCPUQuantity(hard["limits.cpu"])
		summary.QuotaHardLimitMemory += parseMemoryQuantity(hard["limits.memory"])
	}
}

func selectK8sDatasource(datasourceID uint, question string) (models.Datasource, error) {
	var ds models.Datasource
	tx := database.DB.Where("status = 1 AND type = ?", "k8s")
	if datasourceID > 0 {
		tx = tx.Where("id = ?", datasourceID)
	}
	var list []models.Datasource
	if err := tx.Order("id ASC").Find(&list).Error; err != nil {
		return ds, err
	}
	if len(list) == 0 {
		return ds, fmt.Errorf("当前没有可用的 Kubernetes 数据源，请先在「接入中心」添加并启用")
	}
	if datasourceID > 0 {
		return list[0], nil
	}
	matched := selectDatasourcesByQuestion(strings.ToLower(question), list)
	if len(matched) > 0 {
		return matched[0], nil
	}
	return list[0], nil
}

func k8sResourcePath(resource, namespace, name string) (string, bool, string, error) {
	res := normalizeK8sResource(resource)
	ns := strings.TrimSpace(namespace)
	itemName := strings.TrimSpace(name)
	if res == "" {
		return "", false, "", fmt.Errorf("请指定 Kubernetes 资源类型")
	}

	build := func(base string, namespaced bool) string {
		if itemName == "" {
			return base
		}
		return strings.TrimRight(base, "/") + "/" + url.PathEscape(itemName)
	}

	switch res {
	case "namespaces":
		return build("/api/v1/namespaces", false), false, res, nil
	case "nodes":
		return build("/api/v1/nodes", false), false, res, nil
	case "pods", "services", "configmaps", "secrets", "endpoints", "serviceaccounts", "events", "resourcequotas":
		if ns == "" {
			return build("/api/v1/"+res, true), true, res, nil
		}
		return build("/api/v1/namespaces/"+url.PathEscape(ns)+"/"+res, true), true, res, nil
	case "servicemonitors", "podmonitors", "prometheusrules":
		if ns == "" {
			return build("/apis/monitoring.coreos.com/v1/"+res, true), true, res, nil
		}
		return build("/apis/monitoring.coreos.com/v1/namespaces/"+url.PathEscape(ns)+"/"+res, true), true, res, nil
	case "deployments", "statefulsets", "daemonsets", "replicasets":
		if ns == "" {
			return build("/apis/apps/v1/"+res, true), true, res, nil
		}
		return build("/apis/apps/v1/namespaces/"+url.PathEscape(ns)+"/"+res, true), true, res, nil
	case "jobs", "cronjobs":
		if ns == "" {
			return build("/apis/batch/v1/"+res, true), true, res, nil
		}
		return build("/apis/batch/v1/namespaces/"+url.PathEscape(ns)+"/"+res, true), true, res, nil
	case "ingresses":
		if ns == "" {
			return build("/apis/networking.k8s.io/v1/"+res, true), true, res, nil
		}
		return build("/apis/networking.k8s.io/v1/namespaces/"+url.PathEscape(ns)+"/"+res, true), true, res, nil
	default:
		return "", false, "", fmt.Errorf("暂不支持的 Kubernetes 资源类型：%s", resource)
	}
}

func normalizeK8sResource(resource string) string {
	value := strings.ToLower(strings.TrimSpace(resource))
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	switch value {
	case "ns", "namespace", "namespaces", "命名空间":
		return "namespaces"
	case "node", "nodes", "节点":
		return "nodes"
	case "pod", "pods", "容器组":
		return "pods"
	case "svc", "service", "services", "服务":
		return "services"
	case "event", "events", "事件":
		return "events"
	case "deploy", "deployment", "deployments":
		return "deployments"
	case "sts", "statefulset", "statefulsets":
		return "statefulsets"
	case "ds", "daemonset", "daemonsets":
		return "daemonsets"
	case "rs", "replicaset", "replicasets":
		return "replicasets"
	case "cm", "configmap", "configmaps":
		return "configmaps"
	case "secret", "secrets":
		return "secrets"
	case "ep", "endpoint", "endpoints":
		return "endpoints"
	case "sa", "serviceaccount", "serviceaccounts":
		return "serviceaccounts"
	case "job", "jobs":
		return "jobs"
	case "cronjob", "cronjobs":
		return "cronjobs"
	case "ing", "ingress", "ingresses":
		return "ingresses"
	case "servicemonitor", "servicemonitors", "sm":
		return "servicemonitors"
	case "podmonitor", "podmonitors", "pm":
		return "podmonitors"
	case "prometheusrule", "prometheusrules":
		return "prometheusrules"
	case "resourcequota", "resourcequotas", "quota", "quotas":
		return "resourcequotas"
	default:
		return value
	}
}

func executeK8sJSON(ds models.Datasource, path string) (map[string]interface{}, error) {
	return executeK8sJSONContext(context.Background(), ds, path)
}

func executeK8sJSONContext(ctx context.Context, ds models.Datasource, path string) (map[string]interface{}, error) {
	endpoint := firstNonEmpty(strings.TrimSpace(ds.URL), k8sEndpointFromAuthConfig(ds.AuthConfig))
	token := k8sTokenFromAuthConfig(ds.AuthConfig)
	if endpoint == "" {
		return nil, fmt.Errorf("Kubernetes 接入地址为空")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(endpoint, "/")+path, nil)
	if err != nil {
		return nil, fmt.Errorf("Kubernetes 请求构造失败: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Kubernetes 请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Kubernetes HTTP %d: %s", resp.StatusCode, textutil.Ellipsis(string(body), 500))
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("解析 Kubernetes 响应失败: %w", err)
	}
	return parsed, nil
}

func parseK8sResourceItems(raw map[string]interface{}, resource string, limit int) []K8sResourceItem {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rawItems, _ := raw["items"].([]interface{})
	items := make([]K8sResourceItem, 0, min(limit, len(rawItems)))
	for _, rawItem := range rawItems {
		itemMap, ok := rawItem.(map[string]interface{})
		if !ok {
			continue
		}
		meta, _ := itemMap["metadata"].(map[string]interface{})
		status, _ := itemMap["status"].(map[string]interface{})
		createdAt := stringFromMap(meta, "creationTimestamp")
		item := K8sResourceItem{
			Name:      stringFromMap(meta, "name"),
			Namespace: stringFromMap(meta, "namespace"),
			Kind:      strings.TrimSuffix(resource, "s"),
			Status:    k8sResourceStatus(resource, status),
			Age:       ageText(createdAt),
		}
		if labels, ok := meta["labels"].(map[string]interface{}); ok {
			item.Labels = labels
		}
		items = append(items, item)
		if len(items) >= limit {
			break
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Namespace == items[j].Namespace {
			return items[i].Name < items[j].Name
		}
		return items[i].Namespace < items[j].Namespace
	})
	return items
}

func k8sResourceStatus(resource string, status map[string]interface{}) string {
	switch resource {
	case "pods":
		return stringFromMap(status, "phase")
	case "namespaces":
		return stringFromMap(status, "phase")
	case "nodes":
		if conditions, ok := status["conditions"].([]interface{}); ok {
			for _, condition := range conditions {
				cond, _ := condition.(map[string]interface{})
				if stringFromMap(cond, "type") == "Ready" {
					if stringFromMap(cond, "status") == "True" {
						return "Ready"
					}
					return "NotReady"
				}
			}
		}
	case "deployments", "statefulsets", "daemonsets", "replicasets":
		ready := intFromMap(status, "readyReplicas")
		replicas := intFromMap(status, "replicas")
		if replicas > 0 {
			return fmt.Sprintf("%d/%d ready", ready, replicas)
		}
	}
	return ""
}

func stringFromMap(m map[string]interface{}, key string) string {
	if value, ok := m[key].(string); ok {
		return value
	}
	return ""
}

func intFromMap(m map[string]interface{}, key string) int {
	switch value := m[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func ageText(createdAt string) string {
	if createdAt == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return ""
	}
	d := time.Since(t)
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

func valueString(value interface{}) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func stringMap(value interface{}) map[string]string {
	result := map[string]string{}
	raw, ok := value.(map[string]interface{})
	if !ok {
		return result
	}
	for key, item := range raw {
		result[key] = valueString(item)
	}
	return result
}

func parseCPUQuantity(value string) float64 {
	v := strings.TrimSpace(value)
	if v == "" {
		return 0
	}
	if strings.HasSuffix(v, "m") {
		parsed, err := strconv.ParseFloat(strings.TrimSuffix(v, "m"), 64)
		if err != nil {
			return 0
		}
		return parsed / 1000
	}
	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func parseMemoryQuantity(value string) int64 {
	v := strings.TrimSpace(value)
	if v == "" {
		return 0
	}
	units := []struct {
		suffix string
		size   float64
	}{
		{"Ki", 1024},
		{"Mi", 1024 * 1024},
		{"Gi", 1024 * 1024 * 1024},
		{"Ti", 1024 * 1024 * 1024 * 1024},
		{"K", 1000},
		{"M", 1000 * 1000},
		{"G", 1000 * 1000 * 1000},
		{"T", 1000 * 1000 * 1000 * 1000},
	}
	for _, unit := range units {
		if strings.HasSuffix(v, unit.suffix) {
			parsed, err := strconv.ParseFloat(strings.TrimSuffix(v, unit.suffix), 64)
			if err != nil {
				return 0
			}
			return int64(parsed * unit.size)
		}
	}
	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return int64(parsed)
}

func formatCPUCores(value float64) string {
	return fmt.Sprintf("%.3f C", value)
}

func formatGiB(bytes int64) string {
	return fmt.Sprintf("%.2f Gi", float64(bytes)/(1024*1024*1024))
}
