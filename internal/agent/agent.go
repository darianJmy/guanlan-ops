package agent

import (
	"guanlan/pkg/archiveutil"
	"guanlan/pkg/syncbuffer"

	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"

	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const version = "0.2.2"

type envelope struct {
	Type      string      `json:"type"`
	RequestID string      `json:"request_id,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}

type helloPayload struct {
	Hostname     string   `json:"hostname"`
	Region       string   `json:"region"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
}

type execPayload struct {
	Command        string `json:"command"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type artifactExecPayload struct {
	ArtifactBase64   string `json:"artifact_base64"`
	ArtifactSHA256   string `json:"artifact_sha256"`
	Revision         string `json:"revision"`
	WorkingDirectory string `json:"working_directory"`
	Command          string `json:"command"`
	InputJSON        string `json:"input_json"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
}

type execResult struct {
	Output     string `json:"output"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	Error      string `json:"error"`
	DurationMs int64  `json:"duration_ms"`
}

func Run() {
	server := flag.String("server", getenv("AIOPS_SERVER", "ws://127.0.0.1:8080/api/agent/connect"), "platform websocket url")
	token := flag.String("token", getenv("AIOPS_AGENT_TOKEN", ""), "agent token")
	region := flag.String("region", getenv("AIOPS_AGENT_REGION", ""), "agent region label")
	insecure := flag.Bool("insecure", getenv("AIOPS_AGENT_INSECURE", "") == "true", "skip tls certificate verification")
	flag.Parse()

	if strings.TrimSpace(*token) == "" {
		log.Fatal("AIOPS_AGENT_TOKEN is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	backoff := 2 * time.Second
	for ctx.Err() == nil {
		err := connectAndRun(ctx, normalizeWebSocketURL(*server), *token, *region, *insecure)
		if ctx.Err() != nil {
			break
		}
		log.Printf("agent disconnected: %v; reconnecting in %s", err, backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func connectAndRun(ctx context.Context, endpoint, token, region string, insecure bool) error {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)
	dialer := websocket.Dialer{
		HandshakeTimeout:  10 * time.Second,
		EnableCompression: true,
	}
	if insecure {
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	conn, _, err := dialer.DialContext(ctx, endpoint, headers)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetReadLimit(96 << 20)

	log.Printf("connected to %s", endpoint)
	if err := sendHello(conn, region); err != nil {
		return err
	}

	writeMu := &sync.Mutex{}
	errCh := make(chan error, 1)
	go func() {
		for {
			var msg envelope
			if err := conn.ReadJSON(&msg); err != nil {
				errCh <- err
				return
			}
			switch msg.Type {
			case "exec":
				var payload execPayload
				if err := json.Unmarshal([]byte(mustMarshalString(msg.Data)), &payload); err != nil {
					_ = writeAgentEnvelope(writeMu, conn, envelope{
						Type:      "exec_result",
						RequestID: msg.RequestID,
						Data:      execResult{ExitCode: -1, Error: "任务参数解析失败"},
					})
					continue
				}
				result := runExecPayload(payload)
				_ = writeAgentEnvelope(writeMu, conn, envelope{
					Type:      "exec_result",
					RequestID: msg.RequestID,
					Data:      result,
				})
			case "artifact_exec":
				var payload artifactExecPayload
				if err := json.Unmarshal([]byte(mustMarshalString(msg.Data)), &payload); err != nil {
					_ = writeAgentEnvelope(writeMu, conn, envelope{Type: "exec_result", RequestID: msg.RequestID, Data: execResult{ExitCode: -1, Error: "Git 制品任务参数解析失败"}})
					continue
				}
				result := runArtifactExecPayload(payload)
				_ = writeAgentEnvelope(writeMu, conn, envelope{Type: "exec_result", RequestID: msg.RequestID, Data: result})
			default:
				log.Printf("server message: %s", mustMarshalString(msg))
			}
		}
	}()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shutdown"), time.Now().Add(3*time.Second))
			return ctx.Err()
		case err := <-errCh:
			return err
		case <-ticker.C:
			if err := writeAgentEnvelope(writeMu, conn, envelope{Type: "heartbeat"}); err != nil {
				return err
			}
		}
	}
}

func writeAgentEnvelope(writeMu *sync.Mutex, conn *websocket.Conn, msg envelope) error {
	writeMu.Lock()
	defer writeMu.Unlock()
	return conn.WriteJSON(msg)
}

func runExecPayload(payload execPayload) execResult {
	start := time.Now()
	timeout := time.Duration(payload.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", payload.Command)
	var output syncbuffer.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()

	result := execResult{
		Output:     output.String(),
		DurationMs: time.Since(start).Milliseconds(),
	}
	if err != nil {
		result.ExitCode = 1
		result.Error = err.Error()
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			result.ExitCode = -1
			result.Error = "执行超时"
		}
	}
	return result
}

func runArtifactExecPayload(payload artifactExecPayload) execResult {
	start := time.Now()
	result := execResult{ExitCode: -1}
	artifact, err := base64.StdEncoding.DecodeString(payload.ArtifactBase64)
	if err != nil {
		result.Error = "Git 制品编码不正确"
		return result
	}
	digest := sha256.Sum256(artifact)
	if hex.EncodeToString(digest[:]) != payload.ArtifactSHA256 {
		result.Error = "Git 制品校验失败"
		return result
	}
	root, err := os.MkdirTemp("", "aiops-task-artifact-")
	if err != nil {
		result.Error = "创建任务临时目录失败"
		return result
	}
	defer os.RemoveAll(root)
	repositoryDir := filepath.Join(root, "repository")
	if err := os.MkdirAll(repositoryDir, 0700); err != nil {
		result.Error = err.Error()
		return result
	}
	if err := archiveutil.ExtractTarGzip(artifact, repositoryDir); err != nil {
		result.Error = err.Error()
		return result
	}
	inputFile := filepath.Join(root, "input.json")
	if err := os.WriteFile(inputFile, []byte(payload.InputJSON), 0600); err != nil {
		result.Error = "写入任务输入失败"
		return result
	}
	workingDir := filepath.Join(repositoryDir, filepath.FromSlash(payload.WorkingDirectory))
	if info, err := os.Stat(workingDir); err != nil || !info.IsDir() {
		result.Error = "Git 工作目录不存在"
		return result
	}
	timeout := time.Duration(payload.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, "/bin/bash", "-c", payload.Command)
	command.Dir = workingDir
	command.Env = append(os.Environ(), "AIOPS_INPUT_FILE="+inputFile, "AIOPS_GIT_COMMIT="+payload.Revision, "AIOPS_ARTIFACT_SHA256="+payload.ArtifactSHA256)
	var output syncbuffer.Buffer
	command.Stdout, command.Stderr = &output, &output
	err = command.Run()
	result.Output, result.DurationMs = output.String(), time.Since(start).Milliseconds()
	if err == nil {
		result.ExitCode = 0
		return result
	}
	result.ExitCode, result.Error = 1, err.Error()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	} else if ctx.Err() == context.DeadlineExceeded {
		result.ExitCode, result.Error = -1, "执行超时"
	}
	return result
}

func mustMarshalString(value interface{}) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func sendHello(conn *websocket.Conn, region string) error {
	hostname, _ := os.Hostname()
	runtimeMode := strings.ToLower(strings.TrimSpace(getenv("AIOPS_AGENT_RUNTIME", "host")))
	if runtimeMode != "docker" {
		runtimeMode = "host"
	}
	capabilities := []string{
		"heartbeat",
		"probe",
		"shell",
		"artifact_exec_v1",
		runtime.GOOS + "/" + runtime.GOARCH,
		"runtime:" + runtimeMode,
	}
	if runtimeMode == "docker" {
		if image := strings.TrimSpace(os.Getenv("AIOPS_AGENT_IMAGE")); image != "" {
			capabilities = append(capabilities, "docker_image:"+image)
		}
	}
	payload := helloPayload{
		Hostname:     hostname,
		Region:       region,
		Version:      version,
		Capabilities: capabilities,
	}
	return conn.WriteJSON(envelope{Type: "hello", Data: payload})
}

func normalizeWebSocketURL(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return trimmed
	}
	switch parsed.Scheme {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	}
	return parsed.String()
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
