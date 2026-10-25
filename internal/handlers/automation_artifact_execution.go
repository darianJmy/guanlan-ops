package handlers

import (
	"guanlan/pkg/archiveutil"
	"guanlan/pkg/shellutil"
	"guanlan/pkg/sshclient"
	"guanlan/pkg/syncbuffer"

	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"

	"os"
	"os/exec"
	"path/filepath"

	"strings"
	"time"
)

func executeRepositoryArtifact(task models.AutomationTask, artifact repositoryArtifact, input []byte) (automationRunResult, error) {
	timeout := time.Duration(task.TimeoutSeconds) * time.Second
	switch task.NodeType {
	case "direct":
		return executeLocalRepositoryArtifact(task, artifact, input, timeout)
	case "ssh":
		return executeSSHRepositoryArtifact(task, artifact, input, timeout)
	case "agent":
		return executeAgentRepositoryArtifact(task, artifact, input, timeout)
	default:
		return automationRunResult{}, errors.New("不支持的节点类型")
	}
}

func executeLocalRepositoryArtifact(task models.AutomationTask, artifact repositoryArtifact, input []byte, timeout time.Duration) (automationRunResult, error) {
	root, err := os.MkdirTemp("", "aiops-task-artifact-")
	if err != nil {
		return automationRunResult{}, errors.New("创建任务临时目录失败")
	}
	defer os.RemoveAll(root)
	repositoryDir := filepath.Join(root, "repository")
	if err := os.MkdirAll(repositoryDir, 0700); err != nil {
		return automationRunResult{}, err
	}
	if err := archiveutil.ExtractTarGzip(artifact.Data, repositoryDir); err != nil {
		return automationRunResult{}, err
	}
	inputFile := filepath.Join(root, "input.json")
	if err := os.WriteFile(inputFile, input, 0600); err != nil {
		return automationRunResult{}, errors.New("写入任务输入失败")
	}
	workingDir := filepath.Join(repositoryDir, filepath.FromSlash(task.WorkingDirectory))
	if info, err := os.Stat(workingDir); err != nil || !info.IsDir() {
		return automationRunResult{}, errors.New("Git 工作目录不存在")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, "bash", "-c", task.Command)
	command.Dir = workingDir
	command.Env = append(os.Environ(), "AIOPS_INPUT_FILE="+inputFile, "AIOPS_GIT_COMMIT="+artifact.Revision, "AIOPS_ARTIFACT_SHA256="+artifact.SHA256)
	var output syncbuffer.Buffer
	command.Stdout, command.Stderr = &output, &output
	err = command.Run()
	result := automationRunResult{Output: output.String(), ExitCode: processExitCode(err)}
	if ctx.Err() == context.DeadlineExceeded {
		result.ExitCode = -1
		return result, errors.New("任务执行超时")
	}
	return result, err
}

func executeSSHRepositoryArtifact(task models.AutomationTask, artifact repositoryArtifact, input []byte, timeout time.Duration) (automationRunResult, error) {
	var node models.CertificateExecutor
	if task.NodeID == 0 || database.DB.First(&node, task.NodeID).Error != nil {
		return automationRunResult{}, errors.New("SSH 节点不存在")
	}
	if node.Status != 1 {
		return automationRunResult{}, errors.New("SSH 节点已停用")
	}
	command := buildRemoteArtifactCommand(task, artifact, input)
	return runSSHCommandWithInput(node, command, artifact.Data, timeout)
}

func executeAgentRepositoryArtifact(task models.AutomationTask, artifact repositoryArtifact, input []byte, timeout time.Duration) (automationRunResult, error) {
	var node models.AgentConnection
	if task.NodeID == 0 || database.DB.First(&node, task.NodeID).Error != nil {
		return automationRunResult{}, errors.New("Agent 节点不存在")
	}
	if !agentVersionAtLeast(node.Version, minimumAgentArtifactVersion) {
		return automationRunResult{}, fmt.Errorf("Agent 版本过旧，请重新部署（Git 制品最低版本 %s）", minimumAgentArtifactVersion)
	}
	result, err := runAgentArtifact(node.ID, agentArtifactExecReq{ArtifactBase64: base64.StdEncoding.EncodeToString(artifact.Data), ArtifactSHA256: artifact.SHA256, Revision: artifact.Revision, WorkingDirectory: task.WorkingDirectory, Command: task.Command, InputJSON: string(input), TimeoutSeconds: task.TimeoutSeconds}, timeout)
	if err == nil && result.ExitCode != 0 {
		err = fmt.Errorf("命令退出码 %d", result.ExitCode)
	}
	output := result.Output
	if output == "" {
		output = strings.TrimSpace(result.Stdout + "\n" + result.Stderr)
	}
	return automationRunResult{Output: output, ExitCode: result.ExitCode}, err
}

func buildRemoteArtifactCommand(task models.AutomationTask, artifact repositoryArtifact, input []byte) string {
	workDir := "$AIOPS_TASK_DIR/repository"
	if task.WorkingDirectory != "" {
		workDir += "/" + task.WorkingDirectory
	}
	return strings.Join([]string{
		"set -e", "set -o pipefail", "AIOPS_TASK_DIR=\"$(mktemp -d /tmp/aiops-task-XXXXXX)\"", "trap 'rm -rf \"$AIOPS_TASK_DIR\"' EXIT",
		"cat > \"$AIOPS_TASK_DIR/artifact.tar.gz\"",
		"test \"$(sha256sum \"$AIOPS_TASK_DIR/artifact.tar.gz\" | awk '{print $1}')\" = " + shellutil.Quote(artifact.SHA256),
		"mkdir -p \"$AIOPS_TASK_DIR/repository\"", "tar -xzf \"$AIOPS_TASK_DIR/artifact.tar.gz\" -C \"$AIOPS_TASK_DIR/repository\"",
		"printf '%s' " + shellutil.Quote(base64.StdEncoding.EncodeToString(input)) + " | base64 -d > \"$AIOPS_TASK_DIR/input.json\"",
		"export AIOPS_INPUT_FILE=\"$AIOPS_TASK_DIR/input.json\"", "export AIOPS_GIT_COMMIT=" + shellutil.Quote(artifact.Revision), "export AIOPS_ARTIFACT_SHA256=" + shellutil.Quote(artifact.SHA256),
		"cd \"" + workDir + "\"", "bash -c " + shellutil.Quote(task.Command),
	}, "\n")
}

func runSSHCommandWithInput(executor models.CertificateExecutor, command string, input []byte, timeout time.Duration) (automationRunResult, error) {
	client, err := openExecutorSSHClient(executor)
	if err != nil {
		return automationRunResult{}, err
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	result, err := sshclient.Run(ctx, client, command, input)
	if errors.Is(err, context.DeadlineExceeded) {
		err = errors.New("任务执行超时")
	}
	return automationRunResult{Output: strings.TrimSpace(result.Output), ExitCode: result.ExitCode}, err
}
