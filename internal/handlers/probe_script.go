package handlers

import (
	"errors"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"strings"
	"time"
)

// Callers reference a task; execution settings always come from automation management.
func loadProbeAutomationTask(req ProbeReq) (models.AutomationTask, error) {
	var task models.AutomationTask
	if req.AutomationTaskID == 0 {
		return task, errors.New("请选择自动化任务；旧脚本拨测需重新绑定自动化任务")
	}
	if req.ScriptID != 0 || req.NodeType != "" || req.NodeID != 0 || req.TimeoutMS != 0 || len(req.ExecutionNodes) != 0 {
		return task, errors.New("脚本、执行节点和超时由自动化任务统一管理，拨测只能引用任务")
	}
	if database.DB.First(&task, req.AutomationTaskID).Error != nil {
		return task, errors.New("自动化任务不存在")
	}
	if task.Status != 1 {
		return task, errors.New("自动化任务已停用")
	}
	return task, nil
}

type scriptProbeResult struct {
	ProbeType          string `json:"probe_type"`
	AutomationTaskID   uint   `json:"automation_task_id"`
	AutomationTaskName string `json:"automation_task_name"`
	ExecutionID        uint   `json:"execution_id,omitempty"`
	NodeType           string `json:"node_type"`
	NodeID             uint   `json:"node_id"`
	ExitCode           int    `json:"exit_code"`
	Output             string `json:"output"`
	Error              string `json:"error,omitempty"`
	Truncated          bool   `json:"truncated,omitempty"`
}

func executeScriptProbe(req ProbeReq) (string, string, int64) {
	return executeAutomationProbe(req, 0)
}

func executeAutomationProbe(req ProbeReq, itemID uint) (string, string, int64) {
	start := time.Now()
	payload := scriptProbeResult{ProbeType: "script", AutomationTaskID: req.AutomationTaskID, ExitCode: -1}
	task, err := loadProbeAutomationTask(req)
	if err != nil {
		payload.Error = err.Error()
		return "fail", mustJSON(payload), time.Since(start).Milliseconds()
	}
	payload.AutomationTaskName, payload.NodeType, payload.NodeID = task.Name, certificateDefaultString(task.NodeType, "direct"), task.NodeID
	record := executeAutomationTask(task, "probe", itemID, map[string]interface{}{"probe_item_id": itemID, "probe_type": "script"})
	payload.ExecutionID, payload.Output, payload.ExitCode, payload.Error = record.ID, record.Output, record.ExitCode, record.Error
	payload.Truncated = len([]rune(record.Output)) > 16000 && strings.HasSuffix(record.Output, "...")
	status := "success"
	if record.Status != "success" {
		status = "fail"
	}
	return status, mustJSON(payload), record.DurationMs
}
