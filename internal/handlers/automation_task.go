package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"guanlan/pkg/shellutil"
	"guanlan/pkg/syncbuffer"
	"guanlan/pkg/textutil"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type saveAutomationTaskReq struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	ScriptID       uint   `json:"script_id"`
	SourceType     string `json:"source_type"`
	RepositoryID   uint   `json:"repository_id"`
	GitRef         string `json:"git_ref"`
	Command        string `json:"command"`
	NodeType       string `json:"node_type"`
	NodeID         uint   `json:"node_id"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	Status         int    `json:"status"`
}

func ListAutomationTasks(c *gin.Context) {
	var rows []models.AutomationTask
	database.DB.Order("id DESC").Find(&rows)
	resp.Success(c, http.StatusOK, "成功", rows)
}

// Task consumers only need an identity and availability, not execution configuration.
func AutomationTaskOptions(c *gin.Context) {
	var rows []struct {
		ID     uint   `json:"id"`
		Name   string `json:"name"`
		Status int    `json:"status"`
	}
	database.DB.Model(&models.AutomationTask{}).Select("id, name, status").Order("id DESC").Scan(&rows)
	resp.Success(c, http.StatusOK, "成功", rows)
}

func CreateAutomationTask(c *gin.Context) {
	var req saveAutomationTaskReq
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "自动化任务格式不正确")
		return
	}
	item, err := buildAutomationTask(req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if err := database.DB.Create(&item).Error; err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "任务名称已存在")
		return
	}
	resp.Success(c, http.StatusOK, "自动化任务已创建", item)
}

func UpdateAutomationTask(c *gin.Context) {
	var existing models.AutomationTask
	if database.DB.First(&existing, c.Param("id")).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "自动化任务不存在")
		return
	}
	var req saveAutomationTaskReq
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "自动化任务格式不正确")
		return
	}
	item, err := buildAutomationTask(req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if err := database.DB.Model(&existing).Updates(map[string]interface{}{
		"name": item.Name, "description": item.Description, "script_id": item.ScriptID,
		"source_type": item.SourceType, "repository_id": item.RepositoryID, "git_ref": item.GitRef,
		"working_directory": item.WorkingDirectory, "command": item.Command,
		"node_type": item.NodeType, "node_id": item.NodeID,
		"timeout_seconds": item.TimeoutSeconds, "max_retries": item.MaxRetries, "status": item.Status,
	}).Error; err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "任务名称已存在")
		return
	}
	database.DB.First(&existing, existing.ID)
	resp.Success(c, http.StatusOK, "自动化任务已更新", existing)
}

func DeleteAutomationTask(c *gin.Context) {
	var item models.AutomationTask
	if database.DB.First(&item, c.Param("id")).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "自动化任务不存在")
		return
	}
	var bindings int64
	database.DB.Model(&models.AlertActionRule{}).Where("action_type = ? AND target_id = ?", "automation_task", item.ID).Count(&bindings)
	if bindings > 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "任务已被故障自动化处置引用，请先解除绑定")
		return
	}
	var probes []models.InspectionItem
	if err := database.DB.Where("type = ?", "script").Find(&probes).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeFailure, "检查任务引用失败")
		return
	}
	for _, probe := range probes {
		var ref struct {
			AutomationTaskID uint `json:"automation_task_id"`
		}
		if json.Unmarshal([]byte(probe.Config), &ref) == nil && ref.AutomationTaskID == item.ID {
			resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "任务已被拨测任务引用，请先解除绑定")
			return
		}
	}
	database.DB.Delete(&item)
	resp.Success(c, http.StatusOK, "自动化任务已删除")
}

func RunAutomationTask(c *gin.Context) {
	var task models.AutomationTask
	if database.DB.First(&task, c.Param("id")).Error != nil || task.Status != 1 {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "自动化任务不存在或已停用")
		return
	}
	input := map[string]interface{}{}
	if c.Request.ContentLength != 0 && c.ShouldBindJSON(&input) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "运行输入必须是 JSON 对象")
		return
	}
	record := executeAutomationTask(task, "manual", 0, input)
	resp.Success(c, http.StatusOK, automationExecutionMessage(record), record)
}

func ListAutomationExecutions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	query := database.DB.Model(&models.AutomationExecution{})
	if taskID, _ := strconv.Atoi(c.Query("task_id")); taskID > 0 {
		query = query.Where("task_id = ?", taskID)
	}
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		taskIDs := database.DB.Model(&models.AutomationTask{}).Select("id").Where("name LIKE ?", "%"+keyword+"%")
		query = query.Where("task_id IN (?)", taskIDs)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if triggerType := strings.TrimSpace(c.Query("trigger_type")); triggerType != "" {
		query = query.Where("trigger_type = ?", triggerType)
	}
	startTime, startSet, err := parseAutomationExecutionTime(c.Query("start_time"))
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "开始时间格式不正确")
		return
	}
	endTime, endSet, err := parseAutomationExecutionTime(c.Query("end_time"))
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "结束时间格式不正确")
		return
	}
	if startSet && endSet && startTime.After(endTime) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "开始时间不能晚于结束时间")
		return
	}
	if startSet {
		query = query.Where("created_at >= ?", startTime)
	}
	if endSet {
		query = query.Where("created_at <= ?", endTime)
	}
	var total int64
	query.Count(&total)
	var rows []models.AutomationExecution
	query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows)
	taskIDs := make([]uint, 0, len(rows))
	for _, row := range rows {
		taskIDs = append(taskIDs, row.TaskID)
	}
	var tasks []models.AutomationTask
	if len(taskIDs) > 0 {
		database.DB.Where("id IN ?", taskIDs).Find(&tasks)
	}
	taskNames := make(map[uint]string, len(tasks))
	for _, task := range tasks {
		taskNames[task.ID] = task.Name
	}
	result := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		name := taskNames[row.TaskID]
		if name == "" {
			name = fmt.Sprintf("已删除任务 #%d", row.TaskID)
		}
		result = append(result, gin.H{
			"id": row.ID, "task_id": row.TaskID, "task_name": name, "trigger_type": row.TriggerType, "trigger_id": row.TriggerID,
			"status": row.Status, "exit_code": row.ExitCode, "duration_ms": row.DurationMs, "output": row.Output, "error": row.Error,
			"revision": row.Revision, "artifact_sha256": row.ArtifactSHA256, "created_at": row.CreatedAt,
		})
	}
	resp.Success(c, http.StatusOK, "成功", gin.H{"list": result, "total": total, "page": page, "page_size": pageSize})
}

func parseAutomationExecutionTime(value string) (time.Time, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, time.Local)
	return parsed, err == nil, err
}

func buildAutomationTask(req saveAutomationTaskReq) (models.AutomationTask, error) {
	item := models.AutomationTask{
		Name: strings.TrimSpace(req.Name), Description: strings.TrimSpace(req.Description), ScriptID: req.ScriptID,
		SourceType: strings.TrimSpace(req.SourceType), RepositoryID: req.RepositoryID, GitRef: strings.TrimSpace(req.GitRef),
		WorkingDirectory: "", Command: strings.TrimSpace(req.Command),
		NodeType: strings.TrimSpace(req.NodeType), NodeID: req.NodeID,
		TimeoutSeconds: req.TimeoutSeconds, MaxRetries: 0, Status: req.Status,
	}
	if item.SourceType == "" {
		item.SourceType = "script"
	}
	if item.NodeType == "" {
		item.NodeType = "direct"
	}
	if item.TimeoutSeconds == 0 {
		item.TimeoutSeconds = 300
	}
	if item.Name == "" || len(item.Name) > 128 {
		return item, errors.New("请输入不超过 128 个字符的任务名称")
	}
	if item.Status != 0 && item.Status != 1 {
		return item, errors.New("启用状态不正确")
	}
	if item.TimeoutSeconds < 1 || item.TimeoutSeconds > 3600 {
		return item, errors.New("超时时间超出允许范围")
	}
	if item.NodeType != "direct" && item.NodeType != "ssh" && item.NodeType != "agent" {
		return item, errors.New("执行节点类型不正确")
	}
	if item.NodeType != "direct" && item.NodeID == 0 {
		return item, errors.New("请选择执行节点")
	}
	if item.SourceType == "repository" {
		var repository models.GitRepository
		if database.DB.First(&repository, item.RepositoryID).Error != nil || repository.Status != 1 {
			return item, errors.New("Git 仓库不存在或已停用")
		}
		if repository.SyncedRevision == "" || repository.SyncedSHA256 == "" {
			return item, errors.New("Git 仓库尚未同步，请先在脚本库中同步仓库")
		}
		if item.Command == "" {
			return item, errors.New("请填写执行命令")
		}
		item.GitRef = ""
		item.ScriptID = 0
	} else if item.SourceType == "script" {
		var script models.Script
		if database.DB.First(&script, item.ScriptID).Error != nil || script.Status != 1 {
			return item, errors.New("脚本不存在或已停用")
		}
	} else {
		return item, errors.New("代码来源不正确")
	}
	return item, nil
}

func executeAutomationTask(task models.AutomationTask, triggerType string, triggerID uint, input map[string]interface{}) models.AutomationExecution {
	if task.SourceType == "repository" {
		task.WorkingDirectory = ""
	}
	encoded, _ := json.Marshal(input)
	record := models.AutomationExecution{TaskID: task.ID, TriggerType: triggerType, TriggerID: triggerID, Status: "running", ExitCode: -1, Input: string(encoded)}
	startedAt := time.Now()
	var result automationRunResult
	var artifact *repositoryArtifact
	var err error
	if task.SourceType == "repository" {
		var repository models.GitRepository
		if database.DB.First(&repository, task.RepositoryID).Error != nil || repository.Status != 1 {
			err = errors.New("Git 仓库不存在或已停用")
		} else {
			synced, syncErr := readSyncedRepositoryArtifact(repository)
			if syncErr != nil {
				err = syncErr
			} else {
				artifact = &synced
				record.Revision, record.ArtifactSHA256 = synced.Revision, synced.SHA256
			}
		}
	}
	if err == nil {
		result, err = executeAutomationTaskOnce(task, encoded, artifact)
	}
	record.DurationMs = time.Since(startedAt).Milliseconds()
	record.Output = textutil.Ellipsis(strings.TrimSpace(result.Output), 16000)
	if err == nil {
		record.Status, record.ExitCode = "success", 0
	} else {
		record.Status, record.ExitCode = "failed", result.ExitCode
		if record.ExitCode == 0 {
			record.ExitCode = 1
		}
		record.Error = textutil.Ellipsis(err.Error(), 2000)
	}
	database.DB.Create(&record)
	return record
}

type automationRunResult struct {
	Output   string
	ExitCode int
}

func executeAutomationTaskOnce(task models.AutomationTask, encodedInput []byte, artifact *repositoryArtifact) (automationRunResult, error) {
	if task.SourceType == "repository" {
		if artifact == nil {
			return automationRunResult{}, errors.New("Git 制品不存在")
		}
		return executeRepositoryArtifact(task, *artifact, encodedInput)
	}
	var command string
	{
		var script models.Script
		if database.DB.First(&script, task.ScriptID).Error != nil || script.Status != 1 {
			return automationRunResult{}, errors.New("脚本不存在或已停用")
		}
		if script.SourceType == "git" {
			command = buildAutomationGitCommand(script, encodedInput)
		} else {
			command = buildAutomationScriptCommand(script.Type, script.Content, encodedInput)
		}
	}
	timeout := time.Duration(task.TimeoutSeconds) * time.Second
	if task.NodeType == "direct" {
		return runLocalAutomationCommand(command, timeout)
	}
	return runRemoteAutomationCommand(task.NodeType, task.NodeID, command, timeout)
}

func runLocalAutomationCommand(command string, timeout time.Duration) (automationRunResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout+2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "timeout", "--signal=TERM", "--kill-after=1s", fmt.Sprintf("%.3fs", timeout.Seconds()), "bash", "-c", command)
	cmd.WaitDelay = time.Second
	var output syncbuffer.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Run()
	result := automationRunResult{Output: output.String(), ExitCode: processExitCode(err)}
	if ctx.Err() == context.DeadlineExceeded || result.ExitCode == 124 || result.ExitCode == 137 {
		if ctx.Err() != nil {
			result.ExitCode = -1
		}
		return result, errors.New("任务执行超时")
	}
	return result, err
}

func runRemoteAutomationCommand(nodeType string, nodeID uint, command string, timeout time.Duration) (automationRunResult, error) {
	switch nodeType {
	case "ssh":
		var node models.CertificateExecutor
		if nodeID == 0 || database.DB.First(&node, nodeID).Error != nil {
			return automationRunResult{}, errors.New("SSH 节点不存在")
		}
		if node.Status != 1 {
			return automationRunResult{}, errors.New("SSH 节点已停用")
		}
		output, exitCode, err := runSSHCommandOutput(node, command)
		return automationRunResult{Output: output, ExitCode: exitCode}, err
	case "agent":
		var node models.AgentConnection
		if nodeID == 0 || database.DB.First(&node, nodeID).Error != nil {
			return automationRunResult{}, errors.New("Agent 节点不存在")
		}
		if !agentVersionAtLeast(node.Version, minimumAgentExecVersion) {
			return automationRunResult{}, fmt.Errorf("Agent 版本过旧，请重新部署（最低版本 %s）", minimumAgentExecVersion)
		}
		remote, err := runAgentCommand(node.ID, command, timeout)
		output := remote.Output
		if output == "" {
			output = strings.TrimSpace(remote.Stdout + "\n" + remote.Stderr)
		}
		result := automationRunResult{Output: output, ExitCode: remote.ExitCode}
		if err == nil && remote.ExitCode != 0 {
			err = errors.New(certificateDefaultString(strings.TrimSpace(remote.Error), fmt.Sprintf("命令退出码 %d", remote.ExitCode)))
		}
		return result, err
	default:
		return automationRunResult{}, errors.New("不支持的节点类型")
	}
}

func processExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

func buildAutomationGitCommand(script models.Script, input []byte) string {
	runner := "bash"
	if script.Type == "python" {
		runner = "python3"
	}
	cloneArgs := "--depth 1"
	if script.GitRef != "" {
		cloneArgs += " --branch " + shellutil.Quote(script.GitRef) + " --single-branch"
	}
	return strings.Join([]string{
		"set -e", "set -o pipefail",
		"AIOPS_TASK_DIR=\"$(mktemp -d /tmp/aiops-task-XXXXXX)\"",
		"trap 'rm -rf \"$AIOPS_TASK_DIR\"' EXIT",
		"AIOPS_INPUT_FILE=\"$AIOPS_TASK_DIR/input.json\"",
		"printf '%s' " + shellutil.Quote(base64.StdEncoding.EncodeToString(input)) + " | base64 -d > \"$AIOPS_INPUT_FILE\"",
		"export AIOPS_INPUT_FILE",
		"git clone " + cloneArgs + " -- " + shellutil.Quote(script.RepositoryURL) + " \"$AIOPS_TASK_DIR/repository\"",
		"cd \"$AIOPS_TASK_DIR/repository\"", "test -f " + shellutil.Quote(script.Entrypoint), runner + " " + shellutil.Quote(script.Entrypoint),
	}, "\n")
}

func buildAutomationScriptCommand(scriptType, scriptContent string, input []byte) string {
	ext := "sh"
	runner := "bash"
	if scriptType == "python" {
		ext, runner = "py", "python3"
	}
	return strings.Join([]string{
		"set -e", "set -o pipefail",
		"AIOPS_TASK_DIR=\"$(mktemp -d /tmp/aiops-task-XXXXXX)\"",
		"trap 'rm -rf \"$AIOPS_TASK_DIR\"' EXIT",
		"AIOPS_INPUT_FILE=\"$AIOPS_TASK_DIR/input.json\"",
		"AIOPS_SCRIPT_FILE=\"$AIOPS_TASK_DIR/task." + ext + "\"",
		"printf '%s' " + shellutil.Quote(base64.StdEncoding.EncodeToString(input)) + " | base64 -d > \"$AIOPS_INPUT_FILE\"",
		"printf '%s' " + shellutil.Quote(base64.StdEncoding.EncodeToString([]byte(shellutil.NormalizeLineEndings(scriptContent)))) + " | base64 -d > \"$AIOPS_SCRIPT_FILE\"",
		"chmod 700 \"$AIOPS_SCRIPT_FILE\"", "export AIOPS_INPUT_FILE", runner + " \"$AIOPS_SCRIPT_FILE\"",
	}, "\n")
}

func automationExecutionMessage(record models.AutomationExecution) string {
	if record.Status == "success" {
		return "自动化任务执行成功"
	}
	return "自动化任务执行失败：" + record.Error
}
