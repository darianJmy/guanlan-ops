package handlers

import (
	"encoding/json"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/logger"
	"guanlan/pkg/resp"
	"guanlan/pkg/textutil"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	inspectionRawRecordLimit  = 12
	inspectionTrendBucketSize = 5 * time.Minute
	inspectionTrendRetention  = 30 * 24 * time.Hour
)

var inspectionHistoryCleanup struct {
	sync.Mutex
	lastRun time.Time
}

// ── InspectionItem CRUD ──

func ListInspectionItems(c *gin.Context) {
	var items []models.InspectionItem
	query, ok := inspectionGroupQuery(c, database.DB.Order("id ASC"))
	if !ok {
		return
	}
	keyword := strings.TrimSpace(c.Query("keyword"))
	if taskType := strings.ToLower(strings.TrimSpace(c.Query("type"))); taskType != "" {
		query = query.Where("type = ?", taskType)
	}
	if status := strings.ToLower(strings.TrimSpace(c.Query("status"))); status != "" {
		switch status {
		case "enabled", "1":
			query = query.Where("status = ?", 1)
		case "disabled", "0":
			query = query.Where("status = ?", 0)
		}
	}
	if err := query.Find(&items).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "查询拨测任务失败")
		return
	}

	labelKeyword := strings.TrimSpace(c.Query("label"))
	labelKey := strings.TrimSpace(c.Query("label_key"))
	labelValue := strings.TrimSpace(c.Query("label_value"))
	if keyword != "" || labelKeyword != "" || labelKey != "" || labelValue != "" {
		filtered := make([]models.InspectionItem, 0, len(items))
		for _, item := range items {
			labels := parseInspectionLabels(item.Labels)
			keywordMatches := keyword == "" || inspectionItemMatchesKeyword(item, keyword)
			labelMatches := (labelKeyword == "" || inspectionLabelsContain(labels, labelKeyword)) && (labelKey == "" && labelValue == "" || inspectionLabelsMatch(labels, labelKey, labelValue))
			if keywordMatches && labelMatches {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	pageValue := strings.TrimSpace(c.Query("page"))
	pageSizeValue := strings.TrimSpace(c.Query("page_size"))
	if pageValue != "" || pageSizeValue != "" {
		page, _ := strconv.Atoi(pageValue)
		pageSize, _ := strconv.Atoi(pageSizeValue)
		if page < 1 {
			page = 1
		}
		if pageSize < 1 {
			pageSize = 20
		}
		if pageSize > 100 {
			pageSize = 100
		}
		total := len(items)
		start := (page - 1) * pageSize
		if start > total {
			start = total
		}
		end := start + pageSize
		if end > total {
			end = total
		}
		resp.Success(c, http.StatusOK, "成功", gin.H{
			"list": inspectionItemListViews(items[start:end]), "total": total, "page": page, "page_size": pageSize,
		})
		return
	}
	resp.Success(c, http.StatusOK, "成功", inspectionItemListViews(items))
}

type inspectionNodeView struct {
	NodeKey              string     `json:"node_key"`
	NodeType             string     `json:"node_type"`
	NodeID               uint       `json:"node_id"`
	NodeName             string     `json:"node_name"`
	State                string     `json:"state"`
	LastStatus           string     `json:"last_status"`
	LastRunAt            *time.Time `json:"last_run_at"`
	ConsecutiveFailures  int        `json:"consecutive_failures"`
	ConsecutiveSuccesses int        `json:"consecutive_successes"`
}

type inspectionItemListView struct {
	models.InspectionItem
	ExecutionNodes []inspectionNodeView `json:"execution_nodes"`
}

func inspectionItemListViews(items []models.InspectionItem) []inspectionItemListView {
	if len(items) == 0 {
		return []inspectionItemListView{}
	}
	itemIDs, sshIDs, agentIDs := make([]uint, 0, len(items)), make([]uint, 0), make([]uint, 0)
	configured := make(map[uint][]inspectionExecutionNode, len(items))
	for _, item := range items {
		itemIDs = append(itemIDs, item.ID)
		nodes := inspectionExecutionNodes(item.Config)
		if len(nodes) == 0 {
			nodes = []inspectionExecutionNode{{NodeType: "direct"}}
		}
		configured[item.ID] = nodes
		for _, node := range nodes {
			switch node.NodeType {
			case "ssh":
				sshIDs = append(sshIDs, node.NodeID)
			case "agent":
				agentIDs = append(agentIDs, node.NodeID)
			}
		}
	}

	names := map[string]string{"direct:0": "服务端直连"}
	if len(sshIDs) > 0 {
		var executors []models.CertificateExecutor
		database.DB.Select("id", "name").Where("id IN ?", sshIDs).Find(&executors)
		for _, executor := range executors {
			names[fmt.Sprintf("ssh:%d", executor.ID)] = executor.Name
		}
	}
	if len(agentIDs) > 0 {
		var agents []models.AgentConnection
		database.DB.Select("id", "name").Where("id IN ?", agentIDs).Find(&agents)
		for _, agent := range agents {
			names[fmt.Sprintf("agent:%d", agent.ID)] = agent.Name
		}
	}

	var nodeStates []models.InspectionNodeState
	database.DB.Where("item_id IN ?", itemIDs).Find(&nodeStates)
	states := make(map[string]models.InspectionNodeState, len(nodeStates))
	for _, state := range nodeStates {
		states[fmt.Sprintf("%d/%s", state.ItemID, state.NodeKey)] = state
	}

	views := make([]inspectionItemListView, 0, len(items))
	for _, item := range items {
		nodeViews := make([]inspectionNodeView, 0, len(configured[item.ID]))
		for _, node := range configured[item.ID] {
			key := inspectionNodeKey(node)
			state := states[fmt.Sprintf("%d/%s", item.ID, key)]
			if state.ID == 0 && len(configured[item.ID]) == 1 {
				state.LastStatus, state.LastRunAt, state.AlertActive = item.LastStatus, item.LastRunAt, item.AlertActive
				state.ConsecutiveFailures, state.ConsecutiveSuccesses = item.ConsecutiveFailures, item.ConsecutiveSuccesses
			}
			name := names[key]
			if name == "" && state.NodeName != "" {
				name = state.NodeName
			}
			if name == "" {
				name = key
			}
			nodeState := "not_run"
			if state.LastStatus == "success" && state.AlertActive {
				nodeState = "recovering"
			} else if state.LastStatus == "success" {
				nodeState = "success"
			} else if state.LastStatus != "" {
				nodeState = "failed"
			}
			nodeViews = append(nodeViews, inspectionNodeView{NodeKey: key, NodeType: node.NodeType, NodeID: node.NodeID, NodeName: name, State: nodeState, LastStatus: state.LastStatus, LastRunAt: state.LastRunAt, ConsecutiveFailures: state.ConsecutiveFailures, ConsecutiveSuccesses: state.ConsecutiveSuccesses})
		}
		views = append(views, inspectionItemListView{InspectionItem: item, ExecutionNodes: nodeViews})
	}
	return views
}

type BatchInspectionItemStatusReq struct {
	IDs    []uint `json:"ids"`
	Status *int   `json:"status"`
}

type BatchInspectionItemIDsReq struct {
	IDs []uint `json:"ids"`
}

func BatchUpdateInspectionItemStatus(c *gin.Context) {
	var req BatchInspectionItemStatusReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 || req.Status == nil || (*req.Status != 0 && *req.Status != 1) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请选择拨测任务并指定正确状态")
		return
	}
	if *req.Status == 1 {
		var retired int64
		if err := database.DB.Model(&models.InspectionItem{}).Where("id IN ? AND type NOT IN ?", req.IDs, []string{"http", "icmp", "tcp", "certificate_expiry", "script"}).Count(&retired).Error; err != nil {
			resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "检查任务类型失败")
			return
		}
		if retired > 0 {
			resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "所选任务包含已移除的拨测类型，请先修改类型")
			return
		}
	}
	result := database.DB.Model(&models.InspectionItem{}).Where("id IN ?", req.IDs).Update("status", *req.Status)
	if result.Error != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "批量更新任务状态失败")
		return
	}
	if *req.Status == 0 {
		now := time.Now()
		for _, id := range req.IDs {
			terminateTaskAlerts("probe-task", id, "task_disabled", now)
		}
		database.DB.Model(&models.InspectionNodeState{}).Where("item_id IN ?", req.IDs).Updates(map[string]interface{}{"consecutive_failures": 0, "consecutive_successes": 0, "alert_active": false})
		database.DB.Model(&models.InspectionItem{}).Where("id IN ?", req.IDs).Updates(map[string]interface{}{"consecutive_failures": 0, "consecutive_successes": 0, "alert_active": false})
	}
	resp.Success(c, http.StatusOK, "批量更新成功", gin.H{"updated": result.RowsAffected})
}

func BatchDeleteInspectionItems(c *gin.Context) {
	var req BatchInspectionItemIDsReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请选择要删除的拨测任务")
		return
	}

	now := time.Now()
	for _, id := range req.IDs {
		terminateTaskAlerts("probe-task", id, "task_deleted", now)
	}

	var deleted int64
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("item_id IN ?", req.IDs).Delete(&models.InspectionRecord{}).Error; err != nil {
			return err
		}
		if err := tx.Where("item_id IN ?", req.IDs).Delete(&models.InspectionNodeState{}).Error; err != nil {
			return err
		}
		if err := tx.Where("item_id IN ?", req.IDs).Delete(&models.InspectionTrendBucket{}).Error; err != nil {
			return err
		}
		result := tx.Where("id IN ?", req.IDs).Delete(&models.InspectionItem{})
		deleted = result.RowsAffected
		return result.Error
	})
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "批量删除拨测任务失败")
		return
	}
	resp.Success(c, http.StatusOK, "批量删除成功", gin.H{"deleted": deleted})
}

type inspectionTrendPoint struct {
	ID             uint      `json:"id"`
	NodeType       string    `json:"node_type"`
	NodeID         uint      `json:"node_id"`
	NodeName       string    `json:"node_name"`
	Status         string    `json:"status"`
	DurationMS     int64     `json:"duration_ms"`
	DurationMaxMS  int64     `json:"duration_max_ms"`
	TotalCount     int       `json:"total_count"`
	SuccessCount   int       `json:"success_count"`
	FailureCount   int       `json:"failure_count"`
	Status2xxCount int       `json:"status_2xx_count"`
	Status3xxCount int       `json:"status_3xx_count"`
	Status4xxCount int       `json:"status_4xx_count"`
	Status5xxCount int       `json:"status_5xx_count"`
	CreatedAt      time.Time `json:"created_at"`
}

func GetInspectionItemOverview(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "拨测任务 ID 不正确")
		return
	}
	var item models.InspectionItem
	if err := database.DB.First(&item, id).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "拨测任务不存在")
		return
	}

	nodeViews := inspectionItemListViews([]models.InspectionItem{item})[0].ExecutionNodes
	selectedNode := inspectionNodeView{}
	requestedType := strings.ToLower(strings.TrimSpace(c.Query("node_type")))
	requestedID, _ := strconv.ParseUint(strings.TrimSpace(c.Query("node_id")), 10, 64)
	for _, node := range nodeViews {
		if requestedType != "" && node.NodeType == requestedType && node.NodeID == uint(requestedID) {
			selectedNode = node
			break
		}
	}
	if selectedNode.NodeKey == "" && len(nodeViews) > 0 {
		selectedNode = nodeViews[0]
	}

	rangeValue, rangeDuration, pointInterval := normalizeInspectionOverviewRange(c.Query("range"))
	rangeStart := time.Now().Add(-rangeDuration)
	var buckets []models.InspectionTrendBucket
	if selectedNode.NodeKey != "" {
		database.DB.Where("item_id = ? AND node_type = ? AND node_id = ? AND bucket_at >= ?", item.ID, selectedNode.NodeType, selectedNode.NodeID, rangeStart).
			Order("bucket_at ASC").Find(&buckets)
	}

	points := aggregateInspectionTrendBuckets(buckets, selectedNode, rangeStart, rangeDuration, pointInterval)
	durations := make([]int64, 0)
	successCount, total := 0, 0
	codeCounts := map[string]int{"2xx": 0, "3xx": 0, "4xx": 0, "5xx": 0, "other": 0}
	var durationTotal int64
	for _, bucket := range buckets {
		averageDuration := int64(0)
		if bucket.TotalCount > 0 {
			averageDuration = bucket.DurationSumMs / int64(bucket.TotalCount)
		}
		total += bucket.TotalCount
		successCount += bucket.SuccessCount
		durationTotal += bucket.DurationSumMs
		for index := 0; index < bucket.TotalCount; index++ {
			durations = append(durations, averageDuration)
		}
		codeCounts["2xx"] += bucket.Status2xxCount
		codeCounts["3xx"] += bucket.Status3xxCount
		codeCounts["4xx"] += bucket.Status4xxCount
		codeCounts["5xx"] += bucket.Status5xxCount
		codeCounts["other"] += bucket.StatusOther
	}

	averageDuration := int64(0)
	successRate := float64(0)
	if total > 0 {
		averageDuration = durationTotal / int64(total)
		successRate = float64(successCount) * 100 / float64(total)
	}
	resp.Success(c, http.StatusOK, "成功", gin.H{
		"item": item, "nodes": nodeViews, "selected_node_key": selectedNode.NodeKey, "range": rangeValue, "points": points,
		"summary": gin.H{
			"total": total, "success": successCount, "failed": total - successCount,
			"success_rate": successRate, "avg_duration_ms": averageDuration,
			"p95_duration_ms": inspectionDurationPercentile(durations, 0.95), "status_code_counts": codeCounts,
		},
	})
}

func normalizeInspectionOverviewRange(value string) (string, time.Duration, time.Duration) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1h":
		return "1h", time.Hour, 5 * time.Minute
	case "6h":
		return "6h", 6 * time.Hour, 10 * time.Minute
	case "7d":
		return "7d", 7 * 24 * time.Hour, 4 * time.Hour
	case "30d":
		return "30d", 30 * 24 * time.Hour, 12 * time.Hour
	default:
		return "24h", 24 * time.Hour, 30 * time.Minute
	}
}

func aggregateInspectionTrendBuckets(buckets []models.InspectionTrendBucket, node inspectionNodeView, start time.Time, duration, interval time.Duration) []inspectionTrendPoint {
	if node.NodeKey == "" || duration <= 0 || interval <= 0 {
		return []inspectionTrendPoint{}
	}
	pointCount := int((duration + interval - 1) / interval)
	points := make([]inspectionTrendPoint, pointCount)
	durationSums := make([]int64, pointCount)
	for index := range points {
		points[index] = inspectionTrendPoint{NodeType: node.NodeType, NodeID: node.NodeID, NodeName: node.NodeName, CreatedAt: start.Add(time.Duration(index) * interval)}
	}
	for _, bucket := range buckets {
		if bucket.BucketAt.Before(start) || !bucket.BucketAt.Before(start.Add(duration)) {
			continue
		}
		index := int(bucket.BucketAt.Sub(start) / interval)
		if index < 0 || index >= pointCount {
			continue
		}
		point := &points[index]
		point.ID = bucket.ID
		point.TotalCount += bucket.TotalCount
		point.SuccessCount += bucket.SuccessCount
		point.FailureCount += bucket.FailureCount
		point.Status2xxCount += bucket.Status2xxCount
		point.Status3xxCount += bucket.Status3xxCount
		point.Status4xxCount += bucket.Status4xxCount
		point.Status5xxCount += bucket.Status5xxCount
		durationSums[index] += bucket.DurationSumMs
		if bucket.DurationMaxMs > point.DurationMaxMS {
			point.DurationMaxMS = bucket.DurationMaxMs
		}
	}
	for index := range points {
		point := &points[index]
		if point.TotalCount == 0 {
			continue
		}
		point.DurationMS = durationSums[index] / int64(point.TotalCount)
		point.Status = "success"
		if point.FailureCount > 0 {
			point.Status = "fail"
		}
	}
	return points
}

func inspectionRecordStatusCode(result string) int {
	var payload struct {
		StatusCode int `json:"status_code"`
		Response   struct {
			StatusCode int `json:"status_code"`
		} `json:"response"`
	}
	if json.Unmarshal([]byte(result), &payload) != nil {
		return 0
	}
	if payload.StatusCode > 0 {
		return payload.StatusCode
	}
	return payload.Response.StatusCode
}

func inspectionStatusCodeGroup(statusCode int) string {
	switch statusCode / 100 {
	case 2:
		return "2xx"
	case 3:
		return "3xx"
	case 4:
		return "4xx"
	case 5:
		return "5xx"
	default:
		return "other"
	}
}

func inspectionDurationPercentile(values []int64, percentile float64) int64 {
	if len(values) == 0 {
		return 0
	}
	ordered := append([]int64(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	index := int(float64(len(ordered)-1) * percentile)
	return ordered[index]
}

type CreateInspectionItemReq struct {
	GroupID           uint   `json:"group_id"`
	Name              string `json:"name" binding:"required"`
	Type              string `json:"type" binding:"required"`
	Target            string `json:"target"`
	Config            string `json:"config"`
	ExternalLabels    string `json:"external_labels"`
	ExpectedResult    string `json:"expected_result"`
	IntervalSeconds   int    `json:"interval_seconds"`
	FailureThreshold  int    `json:"failure_threshold"`
	RecoveryThreshold int    `json:"recovery_threshold"`
	Severity          string `json:"severity"`
	FaultCenterID     uint   `json:"fault_center_id"`
	Status            int    `json:"status"`
}

type inspectionExecutionNode struct {
	NodeType string `json:"node_type"`
	NodeID   uint   `json:"node_id"`
	NodeName string `json:"-"`
}

func CreateInspectionItem(c *gin.Context) {
	var req CreateInspectionItemReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	labels, err := normalizeInspectionLabels(req.ExternalLabels)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	req.ExternalLabels = labels
	req.Severity = normalizeInspectionSeverity(req.Severity)
	if req.GroupID == 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请选择拨测规则组")
		return
	}
	if err := validateInspectionItemReq(req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}

	item := models.InspectionItem{
		GroupID: req.GroupID, Name: strings.TrimSpace(req.Name), Type: strings.ToLower(strings.TrimSpace(req.Type)),
		Target: strings.TrimSpace(req.Target), Config: req.Config, ExpectedResult: strings.TrimSpace(req.ExpectedResult),
		Labels:          req.ExternalLabels,
		IntervalSeconds: req.IntervalSeconds, FailureThreshold: req.FailureThreshold,
		RecoveryThreshold: req.RecoveryThreshold, Severity: req.Severity,
		FaultCenterID: req.FaultCenterID, Status: req.Status,
	}
	if err := database.DB.Create(&item).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "创建拨测任务失败")
		return
	}
	resp.Success(c, http.StatusOK, "创建成功")
}

func UpdateInspectionItem(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req CreateInspectionItemReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	labels, err := normalizeInspectionLabels(req.ExternalLabels)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	req.ExternalLabels = labels
	req.Severity = normalizeInspectionSeverity(req.Severity)
	var existing models.InspectionItem
	if id < 1 || database.DB.First(&existing, id).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "拨测任务不存在")
		return
	}
	if req.GroupID == 0 {
		req.GroupID = existing.GroupID
	}
	if err := validateInspectionItemReq(req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}

	result := database.DB.Model(&models.InspectionItem{}).Where("id = ?", id).Updates(map[string]interface{}{
		"group_id":           req.GroupID,
		"name":               strings.TrimSpace(req.Name),
		"type":               strings.ToLower(strings.TrimSpace(req.Type)),
		"target":             strings.TrimSpace(req.Target),
		"config":             req.Config,
		"labels":             req.ExternalLabels,
		"expected_result":    strings.TrimSpace(req.ExpectedResult),
		"interval_seconds":   req.IntervalSeconds,
		"failure_threshold":  req.FailureThreshold,
		"recovery_threshold": req.RecoveryThreshold,
		"severity":           req.Severity,
		"fault_center_id":    req.FaultCenterID,
		"status":             req.Status,
	})
	if result.Error != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "更新拨测任务失败")
		return
	}
	reconcileInspectionNodeAlerts(models.InspectionItem{ID: uint(id)}, resolveInspectionExecutionNodes(req.Config), time.Now())
	if req.Status == 0 {
		terminateTaskAlerts("probe-task", uint(id), "task_disabled", time.Now())
		database.DB.Model(&models.InspectionNodeState{}).Where("item_id = ?", id).Updates(map[string]interface{}{"consecutive_failures": 0, "consecutive_successes": 0, "alert_active": false})
		database.DB.Model(&models.InspectionItem{}).Where("id = ?", id).Updates(map[string]interface{}{"consecutive_failures": 0, "consecutive_successes": 0, "alert_active": false})
	}
	resp.Success(c, http.StatusOK, "更新成功")
}

var inspectionLabelKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func normalizeInspectionLabels(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "{}", nil
	}
	var labels map[string]string
	if err := json.Unmarshal([]byte(raw), &labels); err != nil || labels == nil {
		return "", fmt.Errorf("任务标签格式不正确")
	}
	if len(labels) > 20 {
		return "", fmt.Errorf("任务标签最多 20 个")
	}
	normalized := make(map[string]string, len(labels))
	for rawKey, rawValue := range labels {
		key := strings.TrimSpace(rawKey)
		value := strings.TrimSpace(rawValue)
		if key == "" || len(key) > 64 || !inspectionLabelKeyPattern.MatchString(key) {
			return "", fmt.Errorf("标签键只能包含字母、数字、点、下划线和短横线，且不超过 64 个字符")
		}
		if len(value) > 200 {
			return "", fmt.Errorf("标签值不能超过 200 个字符")
		}
		normalized[key] = value
	}
	encoded, _ := json.Marshal(normalized)
	return string(encoded), nil
}

func parseInspectionLabels(raw string) map[string]string {
	labels := map[string]string{}
	_ = json.Unmarshal([]byte(raw), &labels)
	return labels
}

func inspectionLabelsMatch(labels map[string]string, key, value string) bool {
	key = strings.TrimSpace(key)
	value = strings.ToLower(strings.TrimSpace(value))
	for labelKey, labelValue := range labels {
		if key != "" && !strings.EqualFold(labelKey, key) {
			continue
		}
		if value == "" || strings.Contains(strings.ToLower(labelValue), value) {
			return true
		}
	}
	return false
}

func inspectionLabelsContain(labels map[string]string, keyword string) bool {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	for key, value := range labels {
		if strings.Contains(strings.ToLower(key), keyword) || strings.Contains(strings.ToLower(value), keyword) {
			return true
		}
	}
	return false
}

func inspectionItemMatchesKeyword(item models.InspectionItem, keyword string) bool {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	return strings.Contains(strings.ToLower(item.Name), keyword)
}

func inspectionLabelSummary(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+labels[key])
	}
	return strings.Join(parts, ", ")
}

func validateInspectionItemReq(req CreateInspectionItemReq) error {
	taskType := strings.ToLower(strings.TrimSpace(req.Type))
	if taskType != "http" && taskType != "icmp" && taskType != "tcp" && taskType != "certificate_expiry" && taskType != "script" {
		return fmt.Errorf("不支持的拨测类型")
	}
	if req.IntervalSeconds < 10 || req.IntervalSeconds > 86400 {
		return fmt.Errorf("拨测周期应为 10 秒至 24 小时")
	}
	if req.FailureThreshold < 1 || req.FailureThreshold > 100 {
		return fmt.Errorf("连续失败阈值应为 1-100 次")
	}
	if req.RecoveryThreshold < 1 || req.RecoveryThreshold > 100 {
		return fmt.Errorf("连续成功阈值应为 1-100 次")
	}
	if req.Severity != "P0" && req.Severity != "P1" && req.Severity != "P2" {
		return fmt.Errorf("告警等级不正确")
	}
	if req.Status != 0 && req.Status != 1 {
		return fmt.Errorf("任务状态不正确")
	}
	if strings.TrimSpace(req.Config) == "" || !json.Valid([]byte(req.Config)) {
		return fmt.Errorf("拨测配置格式不正确")
	}
	if taskType == "script" {
		var config ProbeReq
		if err := json.Unmarshal([]byte(req.Config), &config); err != nil {
			return fmt.Errorf("脚本拨测配置格式不正确")
		}
		if _, err := loadProbeAutomationTask(config); err != nil {
			return err
		}
	}
	if taskType == "certificate_expiry" {
		var config ProbeReq
		if err := json.Unmarshal([]byte(req.Config), &config); err != nil {
			return fmt.Errorf("证书到期配置格式不正确")
		}
		if _, err := certificateExpiryAlertDays(config.ExpiryAlertDays); err != nil {
			return err
		}
		if _, _, _, err := normalizeNetworkProbe(config, 443); err != nil {
			return err
		}
	}
	if taskType != "script" {
		if err := validateInspectionExecutionNodes(req.Config); err != nil {
			return err
		}
	}
	if req.GroupID > 0 {
		if err := validateInspectionRuleGroup(req.GroupID); err != nil {
			return err
		}
	}
	if req.FaultCenterID == 0 {
		return fmt.Errorf("请选择故障中心")
	}
	if req.FaultCenterID > 0 {
		var count int64
		database.DB.Model(&models.FaultCenter{}).Where("id = ? AND status = ?", req.FaultCenterID, 1).Count(&count)
		if count == 0 {
			return fmt.Errorf("故障中心不存在或已停用")
		}
	}
	return nil
}

func normalizeInspectionSeverity(value string) string {
	severity := strings.ToUpper(strings.TrimSpace(value))
	if severity == "" {
		return "P2"
	}
	return severity
}

func inspectionExecutionNodes(config string) []inspectionExecutionNode {
	var payload struct {
		ExecutionNodes   []inspectionExecutionNode `json:"execution_nodes"`
		AutomationTaskID uint                      `json:"automation_task_id"`
	}
	if json.Unmarshal([]byte(config), &payload) != nil {
		return nil
	}
	if payload.AutomationTaskID > 0 {
		var task models.AutomationTask
		if database.DB.First(&task, payload.AutomationTaskID).Error == nil {
			return []inspectionExecutionNode{{NodeType: certificateDefaultString(task.NodeType, "direct"), NodeID: task.NodeID}}
		}
		return nil
	}
	nodes := payload.ExecutionNodes
	result := make([]inspectionExecutionNode, 0, len(nodes))
	seen := map[string]struct{}{}
	for _, node := range nodes {
		node.NodeType = strings.ToLower(strings.TrimSpace(node.NodeType))
		if node.NodeType == "" {
			node.NodeType = "direct"
		}
		if node.NodeType == "direct" {
			node.NodeID = 0
		}
		key := inspectionNodeKey(node)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, node)
	}
	return result
}

func inspectionNodeKey(node inspectionExecutionNode) string {
	return fmt.Sprintf("%s:%d", node.NodeType, node.NodeID)
}

func validateInspectionExecutionNodes(config string) error {
	nodes := inspectionExecutionNodes(config)
	if len(nodes) == 0 {
		return fmt.Errorf("请至少选择一个执行节点")
	}
	if len(nodes) > 50 {
		return fmt.Errorf("执行节点最多选择 50 个")
	}
	for _, node := range nodes {
		switch node.NodeType {
		case "direct":
			continue
		case "ssh":
			var count int64
			database.DB.Model(&models.CertificateExecutor{}).Where("id = ?", node.NodeID).Count(&count)
			if node.NodeID == 0 || count == 0 {
				return fmt.Errorf("SSH 执行节点不存在")
			}
		case "agent":
			var count int64
			database.DB.Model(&models.AgentConnection{}).Where("id = ?", node.NodeID).Count(&count)
			if node.NodeID == 0 || count == 0 {
				return fmt.Errorf("Agent 执行节点不存在")
			}
		default:
			return fmt.Errorf("执行节点类型不正确")
		}
	}
	return nil
}

func resolveInspectionExecutionNodes(config string) []inspectionExecutionNode {
	nodes := inspectionExecutionNodes(config)
	for index := range nodes {
		node := &nodes[index]
		switch node.NodeType {
		case "direct":
			node.NodeName = "服务端直连"
		case "ssh":
			var executor models.CertificateExecutor
			if database.DB.Select("name").First(&executor, node.NodeID).Error == nil {
				node.NodeName = executor.Name
			}
		case "agent":
			var agent models.AgentConnection
			if database.DB.Select("name").First(&agent, node.NodeID).Error == nil {
				node.NodeName = agent.Name
			}
		}
		if node.NodeName == "" {
			node.NodeName = inspectionNodeKey(*node)
		}
	}
	return nodes
}

func DeleteInspectionItem(c *gin.Context) {
	id := c.Param("id")
	parsedID, _ := strconv.ParseUint(id, 10, 64)
	terminateTaskAlerts("probe-task", uint(parsedID), "task_deleted", time.Now())
	database.DB.Where("item_id = ?", id).Delete(&models.InspectionRecord{})
	database.DB.Where("item_id = ?", id).Delete(&models.InspectionNodeState{})
	database.DB.Where("item_id = ?", id).Delete(&models.InspectionTrendBucket{})
	database.DB.Delete(&models.InspectionItem{}, id)
	resp.Success(c, http.StatusOK, "删除成功")
}

// ── Execute Inspection ──

func RunInspectionItem(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var item models.InspectionItem
	if err := database.DB.First(&item, id).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "拨测任务不存在")
		return
	}

	switch item.Type {
	case "http", "icmp", "tcp", "certificate_expiry", "script":
	default:
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "不支持的拨测类型")
		return
	}

	record := executeAndRecordInspection(&item)

	resp.Success(c, http.StatusOK, "拨测完成: "+record.Status, record)
}

func RunAllInspections(c *gin.Context) {
	var items []models.InspectionItem
	database.DB.Where("status = 1 AND type IN ?", []string{"http", "icmp", "tcp", "certificate_expiry", "script"}).Find(&items)
	if len(items) == 0 {
		resp.Success(c, http.StatusOK, "无启用拨测任务")
		return
	}

	successCount, failCount := 0, 0
	for _, item := range items {
		record := executeAndRecordInspection(&item)
		if record.Status == "success" {
			successCount++
		} else {
			failCount++
		}
	}

	resp.Success(c, http.StatusOK, fmt.Sprintf("拨测完成: 成功 %d, 失败 %d/%d", successCount, failCount, len(items)))
}

var runningInspections sync.Map

func executeAndRecordInspection(item *models.InspectionItem) models.InspectionRecord {
	if _, running := runningInspections.LoadOrStore(item.ID, struct{}{}); running {
		return models.InspectionRecord{ItemID: item.ID, Status: "error", Result: `{"error":"任务正在执行，请稍后重试"}`}
	}
	defer runningInspections.Delete(item.ID)
	now := time.Now()
	nodes := resolveInspectionExecutionNodes(item.Config)
	if item.Type == "script" {
		var req ProbeReq
		_ = json.Unmarshal([]byte(item.Config), &req)
		if req.AutomationTaskID == 0 {
			nodes = nil
		}
	}
	if len(nodes) == 0 {
		nodes = []inspectionExecutionNode{{NodeType: "direct", NodeName: "服务端直连"}}
	}
	reconcileInspectionNodeAlerts(*item, nodes, now)
	type nodeExecutionResult struct {
		node       inspectionExecutionNode
		status     string
		result     string
		durationMS int64
	}
	nodeResults := make([]nodeExecutionResult, len(nodes))
	semaphore := make(chan struct{}, min(8, len(nodes)))
	var waitGroup sync.WaitGroup
	for index, node := range nodes {
		waitGroup.Add(1)
		go func(index int, node inspectionExecutionNode) {
			defer waitGroup.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			status, result, durationMS := executeInspectionOnNode(*item, node)
			nodeResults[index] = nodeExecutionResult{node: node, status: status, result: result, durationMS: durationMS}
		}(index, node)
	}
	waitGroup.Wait()

	results := make([]gin.H, 0, len(nodes))
	aggregateStatus, maxFailures, maxSuccesses, totalDuration := "success", 0, 0, int64(0)
	anyAlertActive := false
	for _, execution := range nodeResults {
		node, status, result, durationMS := execution.node, execution.status, execution.result, execution.durationMS
		totalDuration += durationMS
		database.DB.Create(&models.InspectionRecord{ItemID: item.ID, NodeType: node.NodeType, NodeID: node.NodeID, NodeName: node.NodeName, Status: status, Result: result, DurationMs: durationMS})
		recordInspectionTrend(item.ID, node, status, result, durationMS, now)
		retainLatestInspectionRecords(item.ID, node, inspectionRawRecordLimit)
		state := loadInspectionNodeState(item.ID, node)
		updateInspectionNodeLifecycle(*item, node, &state, status, result, now)
		if status != "success" {
			aggregateStatus = status
		}
		maxFailures = max(maxFailures, state.ConsecutiveFailures)
		maxSuccesses = max(maxSuccesses, state.ConsecutiveSuccesses)
		anyAlertActive = anyAlertActive || state.AlertActive
		results = append(results, gin.H{"node_type": node.NodeType, "node_id": node.NodeID, "node_name": node.NodeName, "status": status, "duration_ms": durationMS, "result": inspectionResultValue(result)})
	}
	aggregateResult, _ := json.Marshal(gin.H{"nodes": results})
	updates := map[string]interface{}{
		"last_run_at": now, "last_status": aggregateStatus, "consecutive_failures": maxFailures,
		"consecutive_successes": maxSuccesses, "alert_active": anyAlertActive,
	}
	database.DB.Model(&models.InspectionItem{}).Where("id = ?", item.ID).Updates(updates)
	cleanupExpiredInspectionTrends(now)
	item.LastRunAt = &now
	item.LastStatus = aggregateStatus
	item.ConsecutiveFailures = maxFailures
	item.ConsecutiveSuccesses = maxSuccesses
	item.AlertActive = anyAlertActive
	return models.InspectionRecord{ItemID: item.ID, Status: aggregateStatus, Result: string(aggregateResult), DurationMs: totalDuration, CreatedAt: now}
}

func recordInspectionTrend(itemID uint, node inspectionExecutionNode, status, result string, durationMS int64, now time.Time) {
	bucketAt := now.Truncate(inspectionTrendBucketSize)
	successCount, failureCount := 0, 1
	if status == "success" {
		successCount, failureCount = 1, 0
	}
	statusCounts := map[string]int{"2xx": 0, "3xx": 0, "4xx": 0, "5xx": 0, "other": 0}
	statusCounts[inspectionStatusCodeGroup(inspectionRecordStatusCode(result))] = 1
	if err := database.DB.Exec(`
		INSERT INTO inspection_trend_buckets
		(item_id, node_type, node_id, node_name, bucket_at, total_count, success_count, failure_count, duration_sum_ms, duration_min_ms, duration_max_ms, status2xx_count, status3xx_count, status4xx_count, status5xx_count, status_other, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (item_id, node_type, node_id, bucket_at) DO UPDATE SET
		node_name = excluded.node_name, total_count = total_count + 1,
		success_count = success_count + excluded.success_count, failure_count = failure_count + excluded.failure_count,
		duration_sum_ms = duration_sum_ms + excluded.duration_sum_ms,
		duration_min_ms = MIN(duration_min_ms, excluded.duration_min_ms), duration_max_ms = MAX(duration_max_ms, excluded.duration_max_ms),
		status2xx_count = status2xx_count + excluded.status2xx_count, status3xx_count = status3xx_count + excluded.status3xx_count,
		status4xx_count = status4xx_count + excluded.status4xx_count, status5xx_count = status5xx_count + excluded.status5xx_count,
		status_other = status_other + excluded.status_other, updated_at = excluded.updated_at`,
		itemID, node.NodeType, node.NodeID, node.NodeName, bucketAt, successCount, failureCount, durationMS, durationMS, durationMS,
		statusCounts["2xx"], statusCounts["3xx"], statusCounts["4xx"], statusCounts["5xx"], statusCounts["other"], now, now).Error; err != nil {
		logger.Error("aggregate inspection trend failed", "item_id", itemID, "node", inspectionNodeKey(node), "error", err)
	}
}

func retainLatestInspectionRecords(itemID uint, node inspectionExecutionNode, limit int) {
	var staleIDs []uint
	database.DB.Model(&models.InspectionRecord{}).
		Where("item_id = ? AND node_type = ? AND node_id = ?", itemID, node.NodeType, node.NodeID).
		Order("id DESC").Offset(limit).Limit(100000).Pluck("id", &staleIDs)
	if len(staleIDs) > 0 {
		database.DB.Where("id IN ?", staleIDs).Delete(&models.InspectionRecord{})
	}
}

func cleanupExpiredInspectionTrends(now time.Time) {
	inspectionHistoryCleanup.Lock()
	defer inspectionHistoryCleanup.Unlock()
	if !inspectionHistoryCleanup.lastRun.IsZero() && now.Sub(inspectionHistoryCleanup.lastRun) < 6*time.Hour {
		return
	}
	database.DB.Where("bucket_at < ?", now.Add(-inspectionTrendRetention)).Delete(&models.InspectionTrendBucket{})
	inspectionHistoryCleanup.lastRun = now
}

func inspectionResultValue(result string) interface{} {
	var decoded interface{}
	if json.Unmarshal([]byte(result), &decoded) == nil {
		return decoded
	}
	return result
}

func executeInspectionOnNode(item models.InspectionItem, node inspectionExecutionNode) (string, string, int64) {
	if item.Type == "script" {
		return executeInspection(item)
	}
	var config map[string]interface{}
	if json.Unmarshal([]byte(item.Config), &config) != nil {
		return "error", `{"error":"拨测配置解析失败"}`, 0
	}
	config["node_type"], config["node_id"] = node.NodeType, node.NodeID
	encoded, _ := json.Marshal(config)
	item.Config = string(encoded)
	return executeInspection(item)
}

func loadInspectionNodeState(itemID uint, node inspectionExecutionNode) models.InspectionNodeState {
	state := models.InspectionNodeState{ItemID: itemID, NodeKey: inspectionNodeKey(node), NodeType: node.NodeType, NodeID: node.NodeID, NodeName: node.NodeName}
	database.DB.Where("item_id = ? AND node_key = ?", itemID, state.NodeKey).FirstOrCreate(&state)
	return state
}

func updateInspectionNodeLifecycle(item models.InspectionItem, node inspectionExecutionNode, state *models.InspectionNodeState, status, result string, now time.Time) {
	recoveryDue := false
	if status == "success" {
		state.ConsecutiveFailures = 0
		if state.AlertActive {
			state.ConsecutiveSuccesses++
			recoveryDue = inspectionRecoveryDue(state.ConsecutiveSuccesses, item.RecoveryThreshold)
		} else {
			state.ConsecutiveSuccesses = 0
		}
	} else {
		state.ConsecutiveFailures++
		state.ConsecutiveSuccesses = 0
	}
	previous, next, err := syncInspectionAlert(item, node, status, result, state.ConsecutiveFailures, recoveryDue, now)
	if err != nil {
		logger.Error("update inspection alert failed", "item_id", item.ID, "node", inspectionNodeKey(node), "error", err)
	}
	state.AlertActive = next == alertStatusFiring || next == alertStatusRecovering
	if next == alertStatusResolved {
		sendOrQueueInspectionRecovery(item, node, now)
		state.ConsecutiveSuccesses = 0
	}
	if status != "success" && next == alertStatusFiring {
		wasFiring := previous == alertStatusFiring || previous == alertStatusRecovering
		_, _ = sendInspectionFailureNotification(item, node, status, result, state.ConsecutiveFailures, wasFiring, now)
	}
	state.NodeType, state.NodeID, state.NodeName = node.NodeType, node.NodeID, node.NodeName
	state.LastRunAt, state.LastStatus = &now, status
	database.DB.Save(state)
}

func inspectionFailureNotificationDue(alertActive bool, lastAlertAt *time.Time, repeatIntervalSeconds int, now time.Time) bool {
	if !alertActive || lastAlertAt == nil {
		return true
	}
	if repeatIntervalSeconds < 60 {
		repeatIntervalSeconds = 1800
	}
	return !now.Before(lastAlertAt.Add(time.Duration(repeatIntervalSeconds) * time.Second))
}

func inspectionRecoveryDue(consecutiveSuccesses, recoveryThreshold int) bool {
	return consecutiveSuccesses >= normalizeRecoveryThreshold(recoveryThreshold)
}

func normalizeFailureThreshold(value int) int {
	if value < 1 {
		return 3
	}
	return value
}

func normalizeRecoveryThreshold(value int) int {
	if value < 1 {
		return 3
	}
	return value
}

func renderInspectionAlertDetail(item models.InspectionItem, node inspectionExecutionNode, externalLabels map[string]string) string {
	var config map[string]interface{}
	if json.Unmarshal([]byte(item.Config), &config) != nil {
		return ""
	}
	templateValue, _ := config["alert_detail"].(string)
	if strings.TrimSpace(templateValue) == "" {
		return ""
	}
	labels := map[string]string{
		"task_name": item.Name, "task_type": item.Type, "target": item.Target,
		"probe_node": node.NodeName, "probe_node_type": node.NodeType,
	}
	return strings.TrimSpace(renderPromRuleTemplate(templateValue, labels, map[string]string{}, externalLabels, ""))
}

func sendInspectionFailureNotification(item models.InspectionItem, node inspectionExecutionNode, status, result string, failures int, repeated bool, checkedAt time.Time) (bool, error) {
	var alert models.AlertInstance
	database.DB.Where("fault_center_id = ? AND source = ? AND source_id = ? AND fingerprint = ? AND status = ?", item.FaultCenterID, "probe-task", item.ID, inspectionAlertFingerprint(item.ID, node), alertStatusFiring).Order("id DESC").First(&alert)
	config := loadAlertRuleNotificationConfig(item.FaultCenterID)
	if alert.ID > 0 && queueGroupedFiringAlert(&alert, config, checkedAt) {
		dispatchDueAlertGroups(checkedAt)
		var latest models.AlertInstance
		database.DB.Select("last_notified_at").First(&latest, alert.ID)
		return latest.LastNotifiedAt != nil, nil
	}
	if alert.ID > 0 {
		if suppressionStatus, reason := alertNotificationSuppression(&alert, checkedAt); suppressionStatus != "" {
			updateAlertNotificationState(&alert, suppressionStatus, reason, "", nil)
			return false, nil
		}
	}
	if !inspectionFailureNotificationDue(repeated, alert.LastNotifiedAt, config.RepeatSeconds, checkedAt) {
		return false, nil
	}
	groupKey := ""
	channelIDs := notificationRouteChannelIDs(alert, config.Routes, config.ChannelIDs)
	if len(channelIDs) == 0 {
		if alert.ID > 0 {
			updateAlertNotificationState(&alert, notificationStatusNoReceiver, "没有匹配到可用通知对象", "", nil)
		}
		return false, nil
	}
	notificationKind := "initial"
	title := fmt.Sprintf("拨测任务失败：%s（%s）", item.Name, node.NodeName)
	if alert.LastNotifiedAt != nil {
		notificationKind = "repeat"
		title = fmt.Sprintf("拨测任务持续失败：%s（%s）", item.Name, node.NodeName)
	}
	externalLabels := parseInspectionLabels(item.Labels)
	content := fmt.Sprintf("任务 %s 已连续失败 %d 次（告警阈值 %d 次），当前状态：%s。\n执行节点：%s\n目标：%s\n结果：%s", item.Name, failures, normalizeFailureThreshold(item.FailureThreshold), status, node.NodeName, item.Target, textutil.Ellipsis(result, 800))
	alertDetail := renderInspectionAlertDetail(item, node, externalLabels)
	if alertDetail != "" {
		content += "\n告警详情：" + alertDetail
	}
	labelSummary := inspectionLabelSummary(externalLabels)
	if labelSummary != "" {
		content += "\n标签：" + labelSummary
	}
	payload := map[string]interface{}{
		"title": title, "content": content, "title_color": alertNotificationTitleColor(item.Severity, false), "source": "probe-task",
		"fault_center_id": item.FaultCenterID,
		"task_id":         item.ID, "task_name": item.Name, "probe_type": item.Type, "target": item.Target,
		"node_type": node.NodeType, "node_id": node.NodeID, "node_name": node.NodeName,
		"status": status, "severity": item.Severity, "level": item.Severity, "consecutive_failures": failures, "failure_threshold": normalizeFailureThreshold(item.FailureThreshold),
		"notification_kind": notificationKind, "repeat_interval_seconds": config.RepeatSeconds,
		"result": result, "alert_detail": alertDetail, "checked_at": checkedAt,
	}
	addNotificationLabelNamespace(payload, "system_labels", map[string]string{"alertname": item.Name, "severity": item.Severity})
	addNotificationLabelNamespace(payload, "labels", map[string]string{"task_name": item.Name, "task_type": item.Type, "target": item.Target, "probe_node": node.NodeName, "probe_node_type": node.NodeType})
	addNotificationLabelNamespace(payload, "datasource_labels", map[string]string{})
	addNotificationLabelNamespace(payload, "external_labels", externalLabels)
	eventKey := fmt.Sprintf("probe-task:%d:%s:failure", item.ID, inspectionNodeKey(node))
	sent := false
	var firstErr error
	for _, channelID := range channelIDs {
		var channel models.NotificationChannel
		if err := database.DB.Where("id = ? AND status = ?", channelID, 1).First(&channel).Error; err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := sendNotificationToChannel(channel, "probe.task.failure", eventKey, title, payload); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		} else {
			sent = true
		}
	}
	if sent {
		if alert.ID > 0 {
			updateAlertNotificationState(&alert, notificationStatusSent, "通知已发送", groupKey, nil)
			database.DB.Model(&alert).Update("last_notified_at", checkedAt)
		}
	} else if alert.ID > 0 {
		updateAlertNotificationState(&alert, notificationStatusSendFailed, "通知发送失败", groupKey, nil)
	}
	return sent, firstErr
}

func sendOrQueueInspectionRecovery(item models.InspectionItem, node inspectionExecutionNode, recoveredAt time.Time) {
	var alert models.AlertInstance
	if database.DB.Where("fault_center_id = ? AND source = ? AND source_id = ? AND fingerprint = ? AND status = ?", item.FaultCenterID, "probe-task", item.ID, inspectionAlertFingerprint(item.ID, node), alertStatusResolved).Order("id DESC").First(&alert).Error != nil {
		return
	}
	config := loadAlertRuleNotificationConfig(item.FaultCenterID)
	if !config.RecoveryNotification {
		updateAlertNotificationState(&alert, notificationStatusRecoveryDisabled, "恢复通知未启用", alert.NotificationGroupKey, nil)
		return
	}
	if queueGroupedRecoveryAlert(&alert, config, recoveredAt) {
		dispatchDueAlertGroups(recoveredAt)
		return
	}
	if alert.LastNotifiedAt == nil {
		updateAlertNotificationState(&alert, notificationStatusResolvedUnnotified, "在首次通知前恢复", "", nil)
		return
	}
	sent, err := sendInspectionRecoveryNotification(item, node, alert, config, recoveredAt)
	if !sent && err != nil {
		updateAlertNotificationState(&alert, notificationStatusSendFailed, "恢复通知发送失败", "", nil)
		return
	}
	if !sent {
		updateAlertNotificationState(&alert, notificationStatusNoReceiver, "没有匹配到可用通知对象", "", nil)
		return
	}
	updateAlertNotificationState(&alert, notificationStatusRecoverySent, "恢复通知已发送", "", nil)
}

func sendInspectionRecoveryNotification(item models.InspectionItem, node inspectionExecutionNode, alert models.AlertInstance, config alertRuleNotificationConfig, recoveredAt time.Time) (bool, error) {
	title := fmt.Sprintf("拨测任务已恢复：%s（%s）", item.Name, node.NodeName)
	content := fmt.Sprintf("任务 %s 已恢复正常。\n执行节点：%s\n目标：%s\n恢复判定：连续成功 %d 次", item.Name, node.NodeName, item.Target, normalizeRecoveryThreshold(item.RecoveryThreshold))
	externalLabels := parseInspectionLabels(item.Labels)
	labelSummary := inspectionLabelSummary(externalLabels)
	if labelSummary != "" {
		content += "\n标签：" + labelSummary
	}
	payload := map[string]interface{}{
		"title": title, "content": content, "title_color": alertNotificationTitleColor(item.Severity, true), "source": "probe-task",
		"fault_center_id": item.FaultCenterID,
		"task_id":         item.ID, "task_name": item.Name, "probe_type": item.Type, "target": item.Target,
		"node_type": node.NodeType, "node_id": node.NodeID, "node_name": node.NodeName,
		"status": "recovered", "severity": item.Severity, "level": item.Severity, "recovery_threshold": normalizeRecoveryThreshold(item.RecoveryThreshold),
		"recovered_at": recoveredAt,
	}
	addNotificationLabelNamespace(payload, "system_labels", map[string]string{"alertname": item.Name, "severity": item.Severity})
	addNotificationLabelNamespace(payload, "labels", map[string]string{"task_name": item.Name, "task_type": item.Type, "target": item.Target, "probe_node": node.NodeName, "probe_node_type": node.NodeType})
	addNotificationLabelNamespace(payload, "datasource_labels", map[string]string{})
	addNotificationLabelNamespace(payload, "external_labels", externalLabels)
	eventKey := fmt.Sprintf("probe-task:%d:%s:recovery", item.ID, inspectionNodeKey(node))
	channelIDs := notificationRouteChannelIDs(alert, config.Routes, config.ChannelIDs)
	sent := false
	var firstErr error
	for _, channelID := range channelIDs {
		var channel models.NotificationChannel
		if err := database.DB.Where("id = ? AND status = ?", channelID, 1).First(&channel).Error; err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := sendNotificationToChannel(channel, "probe.task.recovery", eventKey, title, payload); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		} else {
			sent = true
		}
	}
	return sent, firstErr
}

// ── Records ──

func ListInspectionRecords(c *gin.Context) {
	itemID := c.Query("item_id")
	nodeType := strings.ToLower(strings.TrimSpace(c.Query("node_type")))
	nodeID := strings.TrimSpace(c.Query("node_id"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}

	var total int64
	var records []models.InspectionRecord
	tx := database.DB.Model(&models.InspectionRecord{})
	if itemID != "" {
		tx = tx.Where("item_id = ?", itemID)
	}
	if nodeType != "" {
		tx = tx.Where("node_type = ?", nodeType)
	}
	if nodeID != "" {
		tx = tx.Where("node_id = ?", nodeID)
	}
	tx.Count(&total)
	tx.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&records)

	resp.Success(c, http.StatusOK, "成功", gin.H{
		"list":      records,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// ── Execution Engine ──

func executeInspection(item models.InspectionItem) (status, result string, durationMs int64) {
	switch item.Type {
	case "http", "icmp", "tcp", "certificate_expiry", "script":
		return executeProbeTask(item)
	default:
		return "error", `{"error":"不支持的拨测类型: ` + item.Type + `"}`, 0
	}
}

func executeProbeTask(item models.InspectionItem) (string, string, int64) {
	var req ProbeReq
	if err := json.Unmarshal([]byte(item.Config), &req); err != nil {
		return "error", mustJSON(gin.H{"error": "拨测配置解析失败: " + err.Error()}), 0
	}
	req.Type = item.Type
	if req.Type == "script" {
		return executeAutomationProbe(req, item.ID)
	}
	if req.Type == "http" {
		if strings.TrimSpace(req.Target) == "" {
			req.Target = item.Target
		}
		probeResult, durationMS, err := executeHTTPProbe(req)
		if err != nil {
			return "fail", mustJSON(gin.H{"error": err.Error()}), durationMS
		}
		if !statusCodeMatches(probeResult.StatusCode, req.SuccessCodes) {
			return "fail", mustJSON(gin.H{
				"error":       fmt.Sprintf("HTTP 状态码 %d 不符合预期", probeResult.StatusCode),
				"status_code": probeResult.StatusCode, "response": probeResult,
			}), durationMS
		}
		if keyword := strings.TrimSpace(req.ResponseKeyword); keyword != "" && !strings.Contains(probeResult.Body, keyword) {
			return "fail", mustJSON(gin.H{"error": "响应不包含预期关键字", "response_keyword": keyword, "response": probeResult}), durationMS
		}
		return "success", mustJSON(probeResult), durationMS
	}

	var probeResult networkProbeResult
	var durationMS int64
	var err error
	switch req.Type {
	case "icmp":
		probeResult, durationMS, err = executeICMPProbe(req)
	case "tcp":
		probeResult, durationMS, err = executeTCPProbe(req)
	case "certificate_expiry":
		probeResult, durationMS, err = executeCertificateExpiryProbe(req)
		if err != nil {
			return "fail", mustJSON(probeResult), durationMS
		}
	}
	if err != nil {
		return "fail", mustJSON(gin.H{"error": err.Error()}), durationMS
	}
	return "success", mustJSON(probeResult), durationMS
}
