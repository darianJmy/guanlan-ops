package handlers

import (
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type overviewCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type overviewPoint struct {
	Time                 string `json:"time"`
	Alerts               int64  `json:"alerts"`
	NotificationsSuccess int64  `json:"notifications_success"`
	NotificationsFailed  int64  `json:"notifications_failed"`
}

type overviewSummary struct {
	DatasourcesEnabled        int64 `json:"datasources_enabled"`
	Prometheus                int64 `json:"prometheus"`
	PrometheusEnabled         int64 `json:"prometheus_enabled"`
	ProbeAgents               int64 `json:"probe_agents"`
	ProbeAgentsOnline         int64 `json:"probe_agents_online"`
	ProbeTasks                int64 `json:"probe_tasks"`
	ProbeTasksEnabled         int64 `json:"probe_tasks_enabled"`
	ProbeTasksUnhealthy       int64 `json:"probe_tasks_unhealthy"`
	TodayProbeRuns            int64 `json:"today_probe_runs"`
	TodayProbeFailures        int64 `json:"today_probe_failures"`
	DataCenters               int64 `json:"data_centers"`
	Datasources               int64 `json:"datasources"`
	FaultCenters              int64 `json:"fault_centers"`
	ActiveAlerts              int64 `json:"active_alerts"`
	TodayAlerts               int64 `json:"today_alerts"`
	TodayNotificationsSuccess int64 `json:"today_notifications_success"`
	TodayNotificationsFailed  int64 `json:"today_notifications_failed"`
}

type dashboardOverview struct {
	Summary         overviewSummary `json:"summary"`
	Points          []overviewPoint `json:"points"`
	Severity        []overviewCount `json:"severity"`
	DatasourceTypes []overviewCount `json:"datasource_types"`
	Range           string          `json:"range"`
	Timezone        string          `json:"timezone"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

func GetDashboardOverview(c *gin.Context) {
	rangeKey := c.DefaultQuery("range", "today")
	if rangeKey != "today" && rangeKey != "7d" && rangeKey != "30d" {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "统计范围仅支持 today、7d、30d")
		return
	}
	var result dashboardOverview
	err := database.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = loadDashboardOverview(tx, time.Now(), rangeKey)
		return err
	})
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "读取运维概览失败，请稍后重试")
		return
	}
	resp.Success(c, http.StatusOK, "成功", result)
}

func loadDashboardOverview(db *gorm.DB, now time.Time, rangeKey string) (dashboardOverview, error) {
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	now = now.In(zone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone)
	start := today
	format, layout := "%Y-%m-%d %H:00", "2006-01-02 15:04"
	days := 0
	switch rangeKey {
	case "today":
	case "7d":
		days = 6
	case "30d":
		days = 29
	default:
		return dashboardOverview{}, fmt.Errorf("invalid range")
	}
	if rangeKey != "today" {
		start = today.AddDate(0, 0, -days)
		format, layout = "%Y-%m-%d", "2006-01-02"
	}
	out := dashboardOverview{Range: rangeKey, Timezone: "Asia/Shanghai", UpdatedAt: now, Points: []overviewPoint{}, Severity: []overviewCount{}, DatasourceTypes: []overviewCount{}}
	slots := map[string]int{}
	for t := start; !t.After(now); {
		key := t.Format(layout)
		slots[key] = len(out.Points)
		out.Points = append(out.Points, overviewPoint{Time: key})
		if rangeKey == "today" {
			t = t.Add(time.Hour)
		} else {
			t = t.AddDate(0, 0, 1)
		}
	}
	for _, item := range []struct {
		model interface{}
		count *int64
	}{
		{&models.DatasourceGroup{}, &out.Summary.DataCenters},
		{&models.FaultCenter{}, &out.Summary.FaultCenters},
	} {
		if err := db.Model(item.model).Count(item.count).Error; err != nil {
			return out, err
		}
	}
	// Count transitions, excluding repeated evaluations, notification events and silences.
	firing := func() *gorm.DB {
		return db.Model(&models.AlertEvent{}).Where("to_status = ? AND COALESCE(from_status, '') <> ?", "firing", "firing")
	}
	bounded := func(q *gorm.DB, from time.Time) *gorm.DB {
		return q.Where("julianday(created_at) >= julianday(?) AND julianday(created_at) <= julianday(?)", from, now)
	}
	probeTypes := []string{"http", "icmp", "tcp", "certificate_expiry", "script"}
	for _, item := range []struct {
		query *gorm.DB
		count *int64
	}{
		{db.Model(&models.Datasource{}).Where("type = ?", "prometheus"), &out.Summary.Prometheus},
		{db.Model(&models.Datasource{}).Where("type IN ?", supportedDatasourceTypes()), &out.Summary.Datasources},
		{db.Model(&models.Datasource{}).Where("type IN ? AND status = 1", supportedDatasourceTypes()), &out.Summary.DatasourcesEnabled},
		{db.Model(&models.Datasource{}).Where("type = ? AND status = 1", "prometheus"), &out.Summary.PrometheusEnabled},
		{db.Model(&models.AgentConnection{}).Where("status <> ?", "deleted"), &out.Summary.ProbeAgents},
		{db.Model(&models.AgentConnection{}).Where("status = ? AND julianday(last_seen_at) >= julianday(?) AND julianday(last_seen_at) <= julianday(?)", "online", now.Add(-2*time.Minute), now), &out.Summary.ProbeAgentsOnline},
		{db.Model(&models.InspectionItem{}).Where("type IN ?", probeTypes), &out.Summary.ProbeTasks},
		{db.Model(&models.InspectionItem{}).Where("type IN ? AND status = 1", probeTypes), &out.Summary.ProbeTasksEnabled},
		{db.Model(&models.InspectionItem{}).Where("type IN ? AND status = 1 AND last_status IN ?", probeTypes, []string{"fail", "error"}), &out.Summary.ProbeTasksUnhealthy},
		{bounded(db.Model(&models.InspectionRecord{}), today), &out.Summary.TodayProbeRuns},
		{bounded(db.Model(&models.InspectionRecord{}), today).Where("status IN ?", []string{"fail", "error"}), &out.Summary.TodayProbeFailures},
	} {
		if err := item.query.Count(item.count).Error; err != nil {
			return out, err
		}
	}
	if err := bounded(firing(), today).Count(&out.Summary.TodayAlerts).Error; err != nil {
		return out, err
	}
	if err := bounded(db.Model(&models.NotificationRecord{}), today).Where("status = ?", "success").Count(&out.Summary.TodayNotificationsSuccess).Error; err != nil {
		return out, err
	}
	if err := bounded(db.Model(&models.NotificationRecord{}), today).Where("status = ?", "failed").Count(&out.Summary.TodayNotificationsFailed).Error; err != nil {
		return out, err
	}
	if err := db.Model(&models.AlertInstance{}).Select("severity AS name, COUNT(*) AS count").Where("status IN ?", []string{"firing", "recovering"}).Group("severity").Order("severity").Scan(&out.Severity).Error; err != nil {
		return out, err
	}
	for _, row := range out.Severity {
		out.Summary.ActiveAlerts += row.Count
	}
	if err := db.Model(&models.Datasource{}).Where("type IN ?", supportedDatasourceTypes()).Select("type AS name, COUNT(*) AS count").Group("type").Order("count DESC, type ASC").Scan(&out.DatasourceTypes).Error; err != nil {
		return out, err
	}
	bucket := "strftime('" + format + "', created_at, '+8 hours')"
	var rows []struct {
		Bucket string
		Count  int64
	}
	if err := bounded(firing(), start).Select(bucket + " AS bucket, COUNT(*) AS count").Group("bucket").Scan(&rows).Error; err != nil {
		return out, err
	}
	for _, row := range rows {
		if index, ok := slots[row.Bucket]; ok {
			out.Points[index].Alerts = row.Count
		}
	}
	var notifications []struct {
		Bucket string
		Status string
		Count  int64
	}
	if err := bounded(db.Model(&models.NotificationRecord{}), start).Select(bucket+" AS bucket, status, COUNT(*) AS count").Where("status IN ?", []string{"success", "failed"}).Group("bucket, status").Scan(&notifications).Error; err != nil {
		return out, err
	}
	for _, row := range notifications {
		if index, ok := slots[row.Bucket]; ok {
			if row.Status == "success" {
				out.Points[index].NotificationsSuccess = row.Count
			} else {
				out.Points[index].NotificationsFailed = row.Count
			}
		}
	}
	return out, nil
}
