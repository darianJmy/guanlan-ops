package handlers

import (
	"guanlan/internal/database"
	"guanlan/internal/models"
	"sync"
	"time"
)

var schedulerOnce sync.Once
var inspectionWorkers = make(chan struct{}, 8)

func StartScheduler() {
	schedulerOnce.Do(func() {
		go schedulerLoop()
		go alertAIAnalysisLoop()
	})
}

func schedulerLoop() {
	historyTicker := time.NewTicker(time.Hour)
	defer historyTicker.Stop()
	cleanupExpiredInspectionRecords(time.Now())
	taskTicker := time.NewTicker(time.Second)
	alertRuleTicker := time.NewTicker(5 * time.Second)
	notificationTicker := time.NewTicker(time.Second)
	defer taskTicker.Stop()
	defer alertRuleTicker.Stop()
	defer notificationTicker.Stop()
	runDueAlertRuleBindings(time.Now())
	for {
		select {
		case now := <-historyTicker.C:
			cleanupExpiredInspectionRecords(now)
		case now := <-notificationTicker.C:
			dispatchDueAlertGroups(now)
			dispatchDueAlertActions(now)
		case <-taskTicker.C:
			runPendingTasks()
		case now := <-alertRuleTicker.C:
			runDueAlertRuleBindings(now)
		}
	}
}

func runPendingTasks() {
	runDueInspectionItems(time.Now())
}

func runDueInspectionItems(now time.Time) {
	var items []models.InspectionItem
	database.DB.Where("status = ? AND type IN ?", 1, []string{"http", "icmp", "tcp", "certificate_expiry", "script"}).Find(&items)
	for index := range items {
		item := &items[index]
		if _, running := runningInspections.Load(item.ID); running {
			continue
		}
		if !inspectionTaskDue(*item, now) {
			continue
		}
		select {
		case inspectionWorkers <- struct{}{}:
			go func(item models.InspectionItem) {
				defer func() { <-inspectionWorkers }()
				executeAndRecordInspection(&item)
			}(*item)
		default:
			return
		}
	}
}

func inspectionTaskDue(item models.InspectionItem, now time.Time) bool {
	if item.Status != 1 {
		return false
	}
	interval := item.IntervalSeconds
	if interval < 10 {
		interval = 300
	}
	return item.LastRunAt == nil || now.Sub(*item.LastRunAt) >= time.Duration(interval)*time.Second
}
