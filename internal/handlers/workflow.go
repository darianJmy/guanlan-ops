package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"guanlan/pkg/shellutil"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type WorkflowStep struct {
	Key               string            `json:"key,omitempty"`
	Type              string            `json:"type"`
	Name              string            `json:"name,omitempty"`
	ID                uint              `json:"id,omitempty"`
	NodeType          string            `json:"node_type,omitempty"`
	NodeID            uint              `json:"node_id,omitempty"`
	TimeoutMS         int               `json:"timeout_ms,omitempty"`
	Input             string            `json:"input,omitempty"`
	ConditionPath     string            `json:"condition_path,omitempty"`
	ConditionOperator string            `json:"condition_operator,omitempty"`
	ConditionValue    string            `json:"condition_value,omitempty"`
	ResultStatus      string            `json:"result_status,omitempty"`
	TrueNext          string            `json:"true_next,omitempty"`
	FalseNext         string            `json:"false_next,omitempty"`
	Next              []string          `json:"next,omitempty"`
	Start             bool              `json:"start,omitempty"`
	GraphVersion      int               `json:"graph_version,omitempty"`
	Position          *WorkflowPosition `json:"position,omitempty"`
}

type WorkflowPosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type CreateWorkflowReq struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Steps       string `json:"steps"`
	Status      int    `json:"status"`
}

type stepResult struct {
	Step       int    `json:"step"`
	Key        string `json:"key,omitempty"`
	Type       string `json:"type"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Result     string `json:"result"`
	DurationMs int64  `json:"duration_ms"`
}

var workflowStepKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var workflowTemplatePattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)
var workflowConditionOperators = map[string]bool{
	"equals": true, "not_equals": true, "contains": true,
	"exists": true, "not_exists": true, "truthy": true,
	"greater_than": true, "less_than": true,
}

func ListWorkflows(c *gin.Context) {
	var workflows []models.Workflow
	database.DB.Order("id ASC").Find(&workflows)
	resp.Success(c, http.StatusOK, "成功", workflows)
}

func CreateWorkflow(c *gin.Context) {
	var req CreateWorkflowReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	steps, err := validateWorkflowRequest(req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	encoded, _ := json.Marshal(steps)
	workflow := models.Workflow{Name: strings.TrimSpace(req.Name), Description: strings.TrimSpace(req.Description), Steps: string(encoded), Status: req.Status}
	if err := database.DB.Create(&workflow).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "创建工作流失败")
		return
	}
	resp.Success(c, http.StatusOK, "创建成功", workflow)
}

func UpdateWorkflow(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req CreateWorkflowReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	steps, err := validateWorkflowRequest(req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	encoded, _ := json.Marshal(steps)
	database.DB.Model(&models.Workflow{}).Where("id = ?", id).Updates(map[string]interface{}{
		"name":        strings.TrimSpace(req.Name),
		"description": strings.TrimSpace(req.Description), "steps": string(encoded), "status": req.Status,
	})
	resp.Success(c, http.StatusOK, "更新成功")
}

func validateWorkflowRequest(req CreateWorkflowReq) ([]WorkflowStep, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("请填写工作流名称")
	}
	if req.Status != 0 && req.Status != 1 {
		return nil, errors.New("工作流状态不正确")
	}
	var steps []WorkflowStep
	if err := json.Unmarshal([]byte(req.Steps), &steps); err != nil {
		return nil, errors.New("工作流步骤格式不正确")
	}
	if len(steps) == 0 {
		return nil, errors.New("请至少添加一个工作流步骤")
	}
	if len(steps) > 50 {
		return nil, errors.New("单个工作流最多包含 50 个步骤")
	}
	keys := make(map[string]struct{}, len(steps))
	for index := range steps {
		if strings.TrimSpace(steps[index].Key) == "" {
			return nil, fmt.Errorf("第 %d 步：请填写节点标识", index+1)
		}
		if steps[index].GraphVersion != 2 {
			return nil, fmt.Errorf("第 %d 步：只支持当前工作流图格式", index+1)
		}
		if !workflowStepKeyPattern.MatchString(steps[index].Key) {
			return nil, fmt.Errorf("第 %d 步：输出变量名只能包含字母、数字和下划线，且不能以数字开头", index+1)
		}
		if _, exists := keys[steps[index].Key]; exists {
			return nil, fmt.Errorf("第 %d 步：输出变量名重复", index+1)
		}
		keys[steps[index].Key] = struct{}{}
		if err := normalizeAndValidateWorkflowStep(&steps[index]); err != nil {
			return nil, fmt.Errorf("第 %d 步：%w", index+1, err)
		}
	}
	if err := normalizeAndValidateWorkflowGraph(steps); err != nil {
		return nil, err
	}
	return steps, nil
}

func normalizeAndValidateWorkflowStep(step *WorkflowStep) error {
	step.Key = strings.TrimSpace(step.Key)
	step.Type = strings.ToLower(strings.TrimSpace(step.Type))
	step.Name = strings.TrimSpace(step.Name)
	switch step.Type {
	case "script":
		if step.ID == 0 {
			return errors.New("请选择执行对象")
		}
	case "condition":
		step.ConditionPath = strings.TrimSpace(strings.Trim(step.ConditionPath, "{} "))
		step.ConditionOperator = strings.ToLower(strings.TrimSpace(step.ConditionOperator))
		step.TrueNext = strings.TrimSpace(step.TrueNext)
		step.FalseNext = strings.TrimSpace(step.FalseNext)
		if step.ConditionPath == "" {
			return errors.New("请填写判断变量路径")
		}
		if !workflowConditionOperators[step.ConditionOperator] {
			return errors.New("条件运算符不正确")
		}
	case "end":
		step.ResultStatus = strings.ToLower(strings.TrimSpace(step.ResultStatus))
		if step.ResultStatus != "success" && step.ResultStatus != "fail" {
			return errors.New("结束状态只能是成功或失败")
		}
	default:
		return errors.New("不支持的步骤类型")
	}
	return nil
}

func normalizeAndValidateWorkflowGraph(steps []WorkflowStep) error {
	indexes := make(map[string]int, len(steps))
	for index := range steps {
		indexes[steps[index].Key] = index
	}
	startCount := 0
	adjacency := make([][]int, len(steps))
	indegree := make([]int, len(steps))
	for index := range steps {
		step := &steps[index]
		step.GraphVersion = 2
		if step.Start {
			startCount++
		}
		if step.Type == "condition" {
			step.Next = nil
			if step.TrueNext == "" || step.FalseNext == "" || step.TrueNext == "__end__" || step.FalseNext == "__end__" {
				return fmt.Errorf("第 %d 步：TRUE 和 FALSE 分支都必须连接节点", index+1)
			}
		} else {
			step.TrueNext, step.FalseNext = "", ""
			step.Next = uniqueWorkflowTargets(step.Next)
			if step.Type == "end" && len(step.Next) > 0 {
				return fmt.Errorf("第 %d 步：结束节点不能连接后续节点", index+1)
			}
		}
		for _, target := range workflowOutgoingTargets(*step) {
			targetIndex, exists := indexes[target]
			if !exists {
				return fmt.Errorf("第 %d 步：连接的目标节点不存在", index+1)
			}
			if targetIndex == index {
				return fmt.Errorf("第 %d 步：节点不能连接自己", index+1)
			}
			adjacency[index] = append(adjacency[index], targetIndex)
			indegree[targetIndex]++
		}
	}
	if startCount == 0 {
		return errors.New("请从开始节点至少连接一个执行节点")
	}
	for index := range steps {
		if steps[index].Start && indegree[index] > 0 {
			return fmt.Errorf("第 %d 步：开始分支节点不能同时接收其他节点连线", index+1)
		}
	}
	state := make([]uint8, len(steps))
	var visit func(int) error
	visit = func(index int) error {
		if state[index] == 1 {
			return errors.New("工作流不能包含循环连线")
		}
		if state[index] == 2 {
			return nil
		}
		state[index] = 1
		for _, target := range adjacency[index] {
			if err := visit(target); err != nil {
				return err
			}
		}
		state[index] = 2
		return nil
	}
	for index := range steps {
		if err := visit(index); err != nil {
			return err
		}
	}
	reachable := make([]bool, len(steps))
	var markReachable func(int)
	markReachable = func(index int) {
		if reachable[index] {
			return
		}
		reachable[index] = true
		for _, target := range adjacency[index] {
			markReachable(target)
		}
	}
	for index := range steps {
		if steps[index].Start {
			markReachable(index)
		}
	}
	for index := range reachable {
		if !reachable[index] {
			return fmt.Errorf("第 %d 步：节点尚未连接到开始节点", index+1)
		}
	}
	hasEnd := false
	for index := range steps {
		if steps[index].Type == "end" {
			hasEnd = true
		}
	}
	if hasEnd {
		for index := range steps {
			if steps[index].Type != "end" && len(workflowOutgoingTargets(steps[index])) == 0 {
				return fmt.Errorf("第 %d 步：流程分支必须连接结束节点", index+1)
			}
		}
	}
	return nil
}

func uniqueWorkflowTargets(targets []string) []string {
	result := make([]string, 0, len(targets))
	seen := map[string]bool{}
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target != "" && !seen[target] {
			seen[target] = true
			result = append(result, target)
		}
	}
	return result
}

func workflowOutgoingTargets(step WorkflowStep) []string {
	if step.Type == "condition" {
		return []string{step.TrueNext, step.FalseNext}
	}
	return step.Next
}

func DeleteWorkflow(c *gin.Context) {
	id := c.Param("id")
	database.DB.Where("workflow_id = ?", id).Delete(&models.WorkflowExecution{})
	database.DB.Delete(&models.Workflow{}, id)
	resp.Success(c, http.StatusOK, "删除成功")
}

func ExecuteWorkflow(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var workflow models.Workflow
	if err := database.DB.First(&workflow, id).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "工作流不存在")
		return
	}
	var req struct {
		Input map[string]interface{} `json:"input"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "工作流输入格式不正确")
			return
		}
	}
	record := runWorkflow(&workflow, req.Input)
	resp.Success(c, http.StatusOK, "工作流执行完成: "+record.Status, record)
}

func runWorkflow(workflow *models.Workflow, inputs ...map[string]interface{}) models.WorkflowExecution {
	start := time.Now()
	var steps []WorkflowStep
	if err := json.Unmarshal([]byte(workflow.Steps), &steps); err != nil {
		return createWorkflowExecution(workflow.ID, "error", mustJSON(gin.H{"error": "工作流步骤解析失败"}), time.Since(start).Milliseconds())
	}
	hasEndStep := false
	for index := range steps {
		hasEndStep = hasEndStep || steps[index].Type == "end"
	}
	results := make([]stepResult, 0, len(steps))
	input := map[string]interface{}{}
	if len(inputs) > 0 && inputs[0] != nil {
		input = inputs[0]
	}
	runtime := map[string]interface{}{"input": input, "steps": map[string]interface{}{}}
	stepIndexes := make(map[string]int, len(steps))
	for index := range steps {
		stepIndexes[steps[index].Key] = index
	}
	type graphEdge struct {
		target int
		branch int // 0=always, 1=true, 2=false
	}
	outgoing := make([][]graphEdge, len(steps))
	incomingCount := make([]int, len(steps))
	for index, step := range steps {
		if step.Type == "condition" {
			for branch, target := range []string{step.TrueNext, step.FalseNext} {
				if targetIndex, exists := stepIndexes[target]; exists {
					outgoing[index] = append(outgoing[index], graphEdge{target: targetIndex, branch: branch + 1})
					incomingCount[targetIndex]++
				}
			}
			continue
		}
		for _, target := range step.Next {
			if targetIndex, exists := stepIndexes[target]; exists {
				outgoing[index] = append(outgoing[index], graphEdge{target: targetIndex})
				incomingCount[targetIndex]++
			}
		}
	}
	type completedStep struct {
		index  int
		result stepResult
	}
	completed := make(chan completedStep, len(steps))
	state := make([]uint8, len(steps)) // 0=pending, 1=running, 2=complete, 3=inactive
	resolvedIncoming := make([]int, len(steps))
	activeIncoming := make([]int, len(steps))
	running := 0
	finalStatus := "success"
	launch := func(index int) {
		state[index] = 1
		running++
		step := steps[index]
		runtimeSnapshot := cloneWorkflowRuntime(runtime)
		go func() {
			stepStart := time.Now()
			result := executeWorkflowStep(step, runtimeSnapshot)
			result.Step = index + 1
			result.Key = step.Key
			result.DurationMs = time.Since(stepStart).Milliseconds()
			completed <- completedStep{index: index, result: result}
		}()
	}
	var resolveEdges func(int, int, bool)
	resolveEdges = func(sourceIndex, selectedBranch int, sourceActive bool) {
		for _, edge := range outgoing[sourceIndex] {
			active := sourceActive && (edge.branch == 0 || edge.branch == selectedBranch)
			resolvedIncoming[edge.target]++
			if active {
				activeIncoming[edge.target]++
			}
			if state[edge.target] != 0 || resolvedIncoming[edge.target] != incomingCount[edge.target] {
				continue
			}
			if activeIncoming[edge.target] > 0 {
				launch(edge.target)
				continue
			}
			state[edge.target] = 3
			skipped := stepResult{Step: edge.target + 1, Key: steps[edge.target].Key, Type: steps[edge.target].Type, Name: certificateDefaultString(steps[edge.target].Name, steps[edge.target].Type), Status: "skipped", Result: "所在分支未被激活"}
			results = append(results, skipped)
			storeWorkflowStepOutput(runtime, steps[edge.target], skipped)
			resolveEdges(edge.target, 0, false)
		}
	}
	for index := range steps {
		if steps[index].Start {
			launch(index)
		}
	}
	for running > 0 {
		done := <-completed
		running--
		state[done.index] = 2
		results = append(results, done.result)
		storeWorkflowStepOutput(runtime, steps[done.index], done.result)
		failed := done.result.Status == "fail" || done.result.Status == "error"
		if failed {
			finalStatus = workflowFinalStatus(hasEndStep, finalStatus, steps[done.index], done.result)
		}
		selectedBranch := 0
		if steps[done.index].Type == "condition" {
			var conditionResult struct {
				Matched bool `json:"matched"`
			}
			_ = json.Unmarshal([]byte(done.result.Result), &conditionResult)
			selectedBranch = 2
			if conditionResult.Matched {
				selectedBranch = 1
			}
		}
		resolveEdges(done.index, selectedBranch, true)
	}
	resultJSON, _ := json.Marshal(results)
	return createWorkflowExecution(workflow.ID, finalStatus, string(resultJSON), time.Since(start).Milliseconds())
}

func workflowFinalStatus(hasEndStep bool, current string, step WorkflowStep, result stepResult) string {
	if hasEndStep && step.Type != "end" {
		return current
	}
	if result.Status == "fail" || result.Status == "error" {
		return result.Status
	}
	return current
}

func cloneWorkflowRuntime(runtime map[string]interface{}) map[string]interface{} {
	encoded, err := json.Marshal(runtime)
	if err != nil {
		return map[string]interface{}{}
	}
	var cloned map[string]interface{}
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return map[string]interface{}{}
	}
	return cloned
}

func createWorkflowExecution(workflowID uint, status, result string, durationMS int64) models.WorkflowExecution {
	record := models.WorkflowExecution{WorkflowID: workflowID, Status: status, Result: result, DurationMs: durationMS}
	database.DB.Create(&record)
	return record
}

func executeWorkflowStep(step WorkflowStep, runtimes ...map[string]interface{}) stepResult {
	runtime := map[string]interface{}{}
	if len(runtimes) > 0 && runtimes[0] != nil {
		runtime = runtimes[0]
	}
	step.Input = renderWorkflowTemplate(step.Input, runtime)
	result := stepResult{Type: step.Type, Name: step.Name}
	switch step.Type {
	case "script":
		var script models.Script
		if database.DB.First(&script, step.ID).Error != nil {
			result.Name, result.Status, result.Result = fmt.Sprintf("脚本 #%d", step.ID), "error", "脚本不存在"
			return result
		}
		if script.Status != 1 {
			result.Name, result.Status, result.Result = script.Name, "error", "脚本已停用"
			return result
		}
		if strings.TrimSpace(step.NodeType) == "" || step.NodeType == "direct" {
			result.Name = script.Name
			result.Status, result.Result, _ = runScriptWithEnvironment(script, time.Duration(normalizeProbeTimeout(step.TimeoutMS))*time.Millisecond, workflowScriptEnvironment(runtime, step.Input))
			return result
		}
		commandName := "sh"
		content := script.Content
		if script.Type == "python" {
			commandName = "python3"
		}
		if script.SourceType == "git" {
			commandName, content = "bash", buildGitScriptCommand(script)
		}
		command := workflowRemoteScriptCommand(commandName, content, workflowScriptEnvironment(runtime, step.Input))
		output, _, _, err := executeRemoteProbeCommand(
			ProbeReq{NodeType: step.NodeType, NodeID: step.NodeID},
			command,
			time.Duration(normalizeProbeTimeout(step.TimeoutMS))*time.Millisecond,
		)
		result.Name = script.Name
		if err != nil {
			result.Status, result.Result = "fail", mustJSON(gin.H{"output": output, "error": err.Error()})
		} else {
			result.Status, result.Result = "success", mustJSON(gin.H{"output": output})
		}
	case "condition":
		matched, actual := evaluateWorkflowCondition(runtime, step.ConditionPath, step.ConditionOperator, renderWorkflowTemplate(step.ConditionValue, runtime))
		result.Name = certificateDefaultString(step.Name, "条件判断")
		result.Status = "success"
		result.Result = mustJSON(gin.H{"matched": matched, "path": step.ConditionPath, "operator": step.ConditionOperator, "expected": step.ConditionValue, "actual": actual})
	case "end":
		result.Name = certificateDefaultString(step.Name, "结束")
		result.Status, result.Result = step.ResultStatus, step.Input
	default:
		result.Status, result.Result = "error", "不支持的步骤类型"
	}
	return result
}

func storeWorkflowStepOutput(runtime map[string]interface{}, step WorkflowStep, result stepResult) {
	steps, _ := runtime["steps"].(map[string]interface{})
	if steps == nil {
		steps = map[string]interface{}{}
		runtime["steps"] = steps
	}
	value := interface{}(result.Result)
	var decoded interface{}
	if json.Unmarshal([]byte(result.Result), &decoded) == nil {
		value = decoded
	}
	entry := map[string]interface{}{"status": result.Status, "name": result.Name, "output": value, "raw": result.Result}
	steps[step.Key] = entry
	runtime["previous"] = entry
	if step.Type == "end" {
		runtime["output"] = value
	}
}

func renderWorkflowTemplate(value string, runtime map[string]interface{}) string {
	return workflowTemplatePattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := workflowTemplatePattern.FindStringSubmatch(match)
		resolved, ok := lookupWorkflowValue(runtime, parts[1])
		if !ok || resolved == nil {
			return ""
		}
		if text, ok := resolved.(string); ok {
			return text
		}
		encoded, _ := json.Marshal(resolved)
		return string(encoded)
	})
}

func lookupWorkflowValue(runtime map[string]interface{}, path string) (interface{}, bool) {
	var current interface{} = runtime
	for _, part := range strings.Split(strings.TrimSpace(path), ".") {
		switch value := current.(type) {
		case map[string]interface{}:
			current, _ = value[part]
		case map[string]string:
			current, _ = value[part]
		case []interface{}:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(value) {
				return nil, false
			}
			current = value[index]
		default:
			return nil, false
		}
		if current == nil {
			return nil, false
		}
	}
	return current, true
}

func workflowScriptEnvironment(runtime map[string]interface{}, stepInput string) map[string]string {
	inputJSON, _ := json.Marshal(runtime["input"])
	contextJSON, _ := json.Marshal(runtime)
	return map[string]string{
		"AIOPS_WORKFLOW_INPUT":   string(inputJSON),
		"AIOPS_WORKFLOW_CONTEXT": string(contextJSON),
		"AIOPS_STEP_INPUT":       stepInput,
	}
}

func workflowRemoteScriptCommand(commandName, content string, environment map[string]string) string {
	return fmt.Sprintf("env AIOPS_WORKFLOW_INPUT=%s AIOPS_WORKFLOW_CONTEXT=%s AIOPS_STEP_INPUT=%s %s",
		shellutil.Quote(environment["AIOPS_WORKFLOW_INPUT"]), shellutil.Quote(environment["AIOPS_WORKFLOW_CONTEXT"]),
		shellutil.Quote(environment["AIOPS_STEP_INPUT"]), shellutil.Command(commandName, []string{"-c", content}))
}

func evaluateWorkflowCondition(runtime map[string]interface{}, path, operator, expected string) (bool, interface{}) {
	actual, exists := lookupWorkflowValue(runtime, path)
	if operator == "exists" {
		return exists, actual
	}
	if operator == "not_exists" {
		return !exists, actual
	}
	if !exists {
		return false, nil
	}
	actualText := strings.TrimSpace(fmt.Sprint(actual))
	switch operator {
	case "equals":
		return actualText == expected, actual
	case "not_equals":
		return actualText != expected, actual
	case "contains":
		return strings.Contains(actualText, expected), actual
	case "truthy":
		return actualText != "" && actualText != "0" && !strings.EqualFold(actualText, "false"), actual
	case "greater_than", "less_than":
		actualNumber, actualErr := strconv.ParseFloat(actualText, 64)
		expectedNumber, expectedErr := strconv.ParseFloat(strings.TrimSpace(expected), 64)
		if actualErr != nil || expectedErr != nil {
			return false, actual
		}
		if operator == "greater_than" {
			return actualNumber > expectedNumber, actual
		}
		return actualNumber < expectedNumber, actual
	default:
		return false, actual
	}
}
