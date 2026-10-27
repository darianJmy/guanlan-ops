package handlers

import (
	"context"
	"errors"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/shellutil"
	"guanlan/pkg/syncbuffer"
	"os/exec"
	"strings"
	"time"
)

// Script probes always reference the library; request bodies cannot inject script content.
func loadProbeScript(req ProbeReq) (models.Script, error) {
	var script models.Script
	if req.ScriptID == 0 {
		return script, errors.New("请选择脚本库中的脚本")
	}
	if req.TimeoutMS < 100 || req.TimeoutMS > 300000 {
		return script, errors.New("脚本超时时间应为 100 至 300000 毫秒")
	}
	if err := database.DB.First(&script, req.ScriptID).Error; err != nil {
		return script, errors.New("脚本不存在")
	}
	if script.Status != 1 {
		return script, errors.New("脚本已停用")
	}
	if script.Type != "shell" && script.Type != "python" {
		return script, errors.New("脚本只支持 Shell 或 Python")
	}
	if script.SourceType == "git" {
		if err := validateGitScript(script); err != nil {
			return script, err
		}
	} else if (script.SourceType != "" && script.SourceType != "inline") || strings.TrimSpace(script.Content) == "" {
		return script, errors.New("脚本来源或内容不正确")
	}
	return script, nil
}

type scriptProbeResult struct {
	ProbeType  string `json:"probe_type"`
	ScriptID   uint   `json:"script_id"`
	ScriptName string `json:"script_name"`
	NodeType   string `json:"node_type"`
	NodeID     uint   `json:"node_id"`
	ExitCode   int    `json:"exit_code"`
	Output     string `json:"output"`
	Error      string `json:"error,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`
}

func executeScriptProbe(req ProbeReq) (status, result string, durationMS int64) {
	start := time.Now()
	payload := scriptProbeResult{ProbeType: "script", ScriptID: req.ScriptID, NodeType: certificateDefaultString(req.NodeType, "direct"), NodeID: req.NodeID, ExitCode: -1}
	finish := func(err error) (string, string, int64) {
		status := "success"
		if err != nil {
			status = "fail"
			payload.Error = err.Error()
		}
		payload.Truncated = len(payload.Output) > 65536
		if payload.Truncated {
			payload.Output = strings.ToValidUTF8(payload.Output[:65536], "")
		}
		return status, mustJSON(payload), time.Since(start).Milliseconds()
	}
	script, err := loadProbeScript(req)
	if err != nil {
		return finish(err)
	}
	payload.ScriptName = script.Name
	command := buildAutomationScriptCommand(script.Type, script.Content, []byte("{}"))
	if script.SourceType == "git" {
		command = buildAutomationGitCommand(script, []byte("{}"))
	}
	timeout := time.Duration(req.TimeoutMS) * time.Millisecond
	// GNU timeout terminates the process group, including children of the script.
	command = shellutil.Command("timeout", []string{"--signal=TERM", "--kill-after=1s", fmt.Sprintf("%.3fs", timeout.Seconds()), "bash", "-c", command})
	var execution automationRunResult
	if payload.NodeType == "direct" {
		ctx, cancel := context.WithTimeout(context.Background(), timeout+2*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "bash", "-c", command)
		cmd.WaitDelay = time.Second
		output := syncbuffer.Buffer{Limit: 65537}
		cmd.Stdout, cmd.Stderr = &output, &output
		err = cmd.Run()
		execution = automationRunResult{Output: output.String(), ExitCode: processExitCode(err)}
		if ctx.Err() != nil {
			err = errors.New("脚本执行超时")
			execution.ExitCode = -1
		}
	} else if payload.NodeType == "ssh" {
		var node models.CertificateExecutor
		if req.NodeID == 0 || database.DB.First(&node, req.NodeID).Error != nil {
			return finish(errors.New("SSH 节点不存在"))
		}
		if node.Status != 1 {
			return finish(errors.New("SSH 节点已停用"))
		}
		execution, err = runSSHCommandWithInput(node, command, nil, timeout+2*time.Second)
	} else {
		execution, err = runRemoteAutomationCommand(payload.NodeType, req.NodeID, command, timeout+2*time.Second)
	}
	payload.Output, payload.ExitCode = execution.Output, execution.ExitCode
	if err != nil && payload.ExitCode == 0 {
		payload.ExitCode = -1
	}
	if payload.ExitCode == 124 || payload.ExitCode == 137 {
		err = errors.New("脚本执行超时")
	}
	if err == nil && payload.ExitCode != 0 {
		err = fmt.Errorf("脚本退出码 %d", payload.ExitCode)
	}
	return finish(err)
}
