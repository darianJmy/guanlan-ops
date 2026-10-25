package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"guanlan/pkg/shellutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	agentTokenPrefix            = "aia_"
	minimumAgentExecVersion     = "0.1.1"
	minimumAgentArtifactVersion = "0.2.0"
)

var (
	agentWSUpgrader = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	agentHub = struct {
		sync.RWMutex
		clients map[uint]*agentClient
		pending map[string]pendingAgentExec
	}{clients: make(map[uint]*agentClient), pending: make(map[string]pendingAgentExec)}
)

type agentClient struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

type pendingAgentExec struct {
	agentID uint
	ch      chan agentExecResult
}

type createAgentReq struct {
	Name           string `json:"name" binding:"required"`
	Region         string `json:"region"`
	Description    string `json:"description"`
	DeploymentType string `json:"deployment_type"`
	DockerImage    string `json:"docker_image"`
}

type updateAgentReq struct {
	Name        string `json:"name" binding:"required"`
	Region      string `json:"region"`
	Description string `json:"description"`
}

type agentHello struct {
	Type         string   `json:"type"`
	Hostname     string   `json:"hostname"`
	Region       string   `json:"region"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
}

type agentEnvelope struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	Data      json.RawMessage `json:"data"`
}

type agentExecReq struct {
	Command        string `json:"command"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type agentArtifactExecReq struct {
	ArtifactBase64   string `json:"artifact_base64"`
	ArtifactSHA256   string `json:"artifact_sha256"`
	Revision         string `json:"revision"`
	WorkingDirectory string `json:"working_directory"`
	Command          string `json:"command"`
	InputJSON        string `json:"input_json"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
}

type agentExecResult struct {
	Output     string `json:"output"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	Error      string `json:"error"`
	DurationMs int64  `json:"duration_ms"`
}

type agentRunCommandReq struct {
	Command        string `json:"command" binding:"required"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

func ListAgents(c *gin.Context) {
	markStaleAgentsOffline()
	var items []models.AgentConnection
	database.DB.Order("id DESC").Find(&items)
	resp.Success(c, http.StatusOK, "成功", items)
}

func CreateAgent(c *gin.Context) {
	var req createAgentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请填写 Agent 名称")
		return
	}
	token, tokenHash, err := newAgentToken()
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "生成 Agent Token 失败")
		return
	}
	deploymentType := normalizeAgentDeploymentType(req.DeploymentType)
	if deploymentType == "docker" && len(strings.TrimSpace(req.DockerImage)) > 255 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "Docker 镜像名称不能超过 255 个字符")
		return
	}
	capabilities, _ := json.Marshal(initialAgentCapabilities(deploymentType, req.DockerImage))
	item := models.AgentConnection{
		Name:         strings.TrimSpace(req.Name),
		TokenHash:    tokenHash,
		Status:       "offline",
		Region:       strings.TrimSpace(req.Region),
		Capabilities: string(capabilities),
		Description:  strings.TrimSpace(req.Description),
	}
	if item.Name == "" {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请填写 Agent 名称")
		return
	}
	if err := database.DB.Create(&item).Error; err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "Agent 名称已存在")
		return
	}
	resp.Success(c, http.StatusOK, "创建成功", gin.H{"agent": item, "token": token})
}

func normalizeAgentDeploymentType(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "docker") {
		return "docker"
	}
	return "agent"
}

func initialAgentCapabilities(deploymentType, dockerImage string) []string {
	if normalizeAgentDeploymentType(deploymentType) != "docker" {
		return []string{"runtime:host"}
	}
	capabilities := []string{"runtime:docker"}
	if image := strings.TrimSpace(dockerImage); image != "" {
		capabilities = append(capabilities, "docker_image:"+image)
	}
	return capabilities
}

func UpdateAgent(c *gin.Context) {
	var req updateAgentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请填写 Agent 名称")
		return
	}
	updates := map[string]interface{}{
		"name":        strings.TrimSpace(req.Name),
		"region":      strings.TrimSpace(req.Region),
		"description": strings.TrimSpace(req.Description),
	}
	if updates["name"] == "" {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请填写 Agent 名称")
		return
	}
	if err := database.DB.Model(&models.AgentConnection{}).Where("id = ?", c.Param("id")).Updates(updates).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "更新 Agent 失败")
		return
	}
	resp.Success(c, http.StatusOK, "更新成功")
}

func RotateAgentToken(c *gin.Context) {
	token, tokenHash, err := newAgentToken()
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "生成 Agent Token 失败")
		return
	}
	if err := database.DB.Model(&models.AgentConnection{}).Where("id = ?", c.Param("id")).Update("token_hash", tokenHash).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "刷新 Agent Token 失败")
		return
	}
	resp.Success(c, http.StatusOK, "刷新成功", gin.H{"token": token})
}

func DeleteAgent(c *gin.Context) {
	id := c.Param("id")
	if names := boundCertificateDomainNames("agent_id", id); len(names) > 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, boundResourceMessage("Agent", names))
		return
	}
	agentHub.Lock()
	if conn := agentHub.clients[mustParseUint(id)]; conn != nil {
		_ = conn.conn.Close()
		delete(agentHub.clients, mustParseUint(id))
	}
	agentHub.Unlock()
	database.DB.Delete(&models.AgentConnection{}, id)
	resp.Success(c, http.StatusOK, "删除成功")
}

func AgentConnect(c *gin.Context) {
	token := agentTokenFromRequest(c.Request)
	agent, ok := findAgentByToken(token)
	if !ok {
		resp.Error(c, http.StatusUnauthorized, resp.CodeUnauthorized, "Agent Token 无效")
		return
	}

	conn, err := agentWSUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	now := time.Now()
	database.DB.Model(&models.AgentConnection{}).Where("id = ?", agent.ID).Updates(map[string]interface{}{
		"status":       "online",
		"remote_addr":  c.ClientIP(),
		"last_seen_at": &now,
		"connected_at": &now,
	})

	agentHub.Lock()
	if old := agentHub.clients[agent.ID]; old != nil {
		_ = old.conn.Close()
	}
	client := &agentClient{conn: conn}
	agentHub.clients[agent.ID] = client
	agentHub.Unlock()

	_ = writeAgentJSON(client, gin.H{"type": "welcome", "agent_id": agent.ID, "server_time": now.Format(time.RFC3339)})
	handleAgentMessages(agent.ID, client)
}

func RunAgentCommand(c *gin.Context) {
	var agent models.AgentConnection
	if err := database.DB.First(&agent, c.Param("id")).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "Agent 不存在")
		return
	}
	if !agentVersionAtLeast(agent.Version, minimumAgentExecVersion) {
		resp.Error(c, http.StatusOK, resp.CodeFailure, fmt.Sprintf("Agent 版本 %s 过旧，请重新部署 Agent（最低版本 %s）", certificateDefaultString(agent.Version, "未知"), minimumAgentExecVersion))
		return
	}
	var req agentRunCommandReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Command) == "" {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请输入执行命令")
		return
	}
	timeout := time.Duration(defaultAgentTimeout(req.TimeoutSeconds)) * time.Second
	result, err := runAgentCommand(agent.ID, req.Command, timeout)
	if err != nil {
		resp.Error(c, http.StatusOK, resp.CodeFailure, err.Error(), result)
		return
	}
	if result.ExitCode != 0 {
		message := strings.TrimSpace(result.Error)
		if message == "" {
			message = fmt.Sprintf("命令退出码 %d", result.ExitCode)
		}
		resp.Error(c, http.StatusOK, resp.CodeFailure, message, result)
		return
	}
	resp.Success(c, http.StatusOK, "执行完成", result)
}

func AgentInstallScript(c *gin.Context) {
	token := agentTokenFromRequest(c.Request)
	agent, ok := findAgentByToken(token)
	if !ok {
		c.String(http.StatusUnauthorized, "Agent Token 无效\n")
		return
	}
	serverURL := agentConnectURL(c)
	region := agent.Region
	if queryRegion := strings.TrimSpace(c.Query("region")); queryRegion != "" {
		region = queryRegion
	}
	c.Header("Content-Type", "text/x-shellscript; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="install-aiops-agent.sh"`)
	c.String(http.StatusOK, buildAgentInstallScript(serverURL, agentBinaryURL(c, token), token, region))
}

func AgentBinary(c *gin.Context) {
	token := agentTokenFromRequest(c.Request)
	if _, ok := findAgentByToken(token); !ok {
		c.String(http.StatusUnauthorized, "Agent Token 无效\n")
		return
	}
	goos := certificateDefaultString(strings.TrimSpace(c.Query("os")), "linux")
	arch := certificateDefaultString(strings.TrimSpace(c.Query("arch")), "amd64")
	if goos != "linux" || (arch != "amd64" && arch != "arm64") {
		c.String(http.StatusBadRequest, "暂只支持 linux/amd64 和 linux/arm64\n")
		return
	}
	binPath := filepath.Join("data", "agents", fmt.Sprintf("aiops-agent-%s-%s", goos, arch))
	if _, err := os.Stat(binPath); err != nil {
		c.String(http.StatusNotFound, "Agent 二进制不存在，请先在平台构建\n")
		return
	}
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", `attachment; filename="aiops-agent"`)
	c.File(binPath)
}

func handleAgentMessages(agentID uint, client *agentClient) {
	conn := client.conn
	conn.SetReadLimit(64 * 1024)
	_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		touchAgent(agentID)
		return nil
	})

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			var msg agentEnvelope
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			switch msg.Type {
			case "hello":
				var hello agentHello
				_ = json.Unmarshal(msg.Data, &hello)
				updateAgentHello(agentID, hello)
			case "heartbeat":
				touchAgent(agentID)
				_ = writeAgentJSON(client, gin.H{"type": "heartbeat_ack", "server_time": time.Now().Format(time.RFC3339)})
			case "exec_result":
				var result agentExecResult
				_ = json.Unmarshal(msg.Data, &result)
				completeAgentExec(agentID, msg.RequestID, result)
			default:
				touchAgent(agentID)
			}
		}
	}()

	for {
		select {
		case <-ticker.C:
			if err := conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second)); err != nil {
				markAgentDisconnected(agentID)
				return
			}
		case <-done:
			markAgentDisconnected(agentID)
			return
		}
	}
}

func runAgentCommand(agentID uint, command string, timeout time.Duration) (agentExecResult, error) {
	requestID, err := newAgentRequestID()
	if err != nil {
		return agentExecResult{}, errors.New("生成 Agent 任务 ID 失败")
	}
	ch := make(chan agentExecResult, 1)
	agentHub.Lock()
	client := agentHub.clients[agentID]
	if client == nil {
		agentHub.Unlock()
		return agentExecResult{}, errors.New("Agent 当前不在线")
	}
	agentHub.pending[requestID] = pendingAgentExec{agentID: agentID, ch: ch}
	agentHub.Unlock()

	payload := gin.H{
		"type":       "exec",
		"request_id": requestID,
		"data": agentExecReq{
			Command:        command,
			TimeoutSeconds: int(timeout.Seconds()),
		},
	}
	if err := writeAgentJSON(client, payload); err != nil {
		removeAgentExec(requestID)
		return agentExecResult{}, errors.New("下发 Agent 任务失败")
	}

	select {
	case result := <-ch:
		return result, nil
	case <-time.After(timeout + 5*time.Second):
		removeAgentExec(requestID)
		return agentExecResult{}, errors.New("Agent 执行超时")
	}
}

func runAgentArtifact(agentID uint, request agentArtifactExecReq, timeout time.Duration) (agentExecResult, error) {
	requestID, err := newAgentRequestID()
	if err != nil {
		return agentExecResult{}, errors.New("生成 Agent 任务 ID 失败")
	}
	ch := make(chan agentExecResult, 1)
	agentHub.Lock()
	client := agentHub.clients[agentID]
	if client == nil {
		agentHub.Unlock()
		return agentExecResult{}, errors.New("Agent 当前不在线")
	}
	agentHub.pending[requestID] = pendingAgentExec{agentID: agentID, ch: ch}
	agentHub.Unlock()
	payload := gin.H{"type": "artifact_exec", "request_id": requestID, "data": request}
	if err := writeAgentJSON(client, payload); err != nil {
		removeAgentExec(requestID)
		return agentExecResult{}, errors.New("下发 Agent Git 制品失败")
	}
	select {
	case result := <-ch:
		return result, nil
	case <-time.After(timeout + 30*time.Second):
		removeAgentExec(requestID)
		return agentExecResult{}, errors.New("Agent Git 制品执行超时")
	}
}

func completeAgentExec(agentID uint, requestID string, result agentExecResult) {
	if strings.TrimSpace(requestID) == "" {
		return
	}
	agentHub.Lock()
	pending, ok := agentHub.pending[requestID]
	if ok && pending.agentID == agentID {
		delete(agentHub.pending, requestID)
	}
	agentHub.Unlock()
	if ok && pending.agentID == agentID {
		select {
		case pending.ch <- result:
		default:
		}
	}
}

func removeAgentExec(requestID string) {
	agentHub.Lock()
	delete(agentHub.pending, requestID)
	agentHub.Unlock()
}

func failAgentExecs(agentID uint, message string) {
	var targets []chan agentExecResult
	agentHub.Lock()
	for requestID, pending := range agentHub.pending {
		if pending.agentID == agentID {
			targets = append(targets, pending.ch)
			delete(agentHub.pending, requestID)
		}
	}
	agentHub.Unlock()
	for _, ch := range targets {
		select {
		case ch <- agentExecResult{ExitCode: -1, Error: message}:
		default:
		}
	}
}

func writeAgentJSON(client *agentClient, payload interface{}) error {
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	return client.conn.WriteJSON(payload)
}

func updateAgentHello(agentID uint, hello agentHello) {
	now := time.Now()
	caps, _ := json.Marshal(hello.Capabilities)
	updates := map[string]interface{}{
		"hostname":     strings.TrimSpace(hello.Hostname),
		"version":      strings.TrimSpace(hello.Version),
		"capabilities": string(caps),
		"last_seen_at": &now,
		"status":       "online",
	}
	if strings.TrimSpace(hello.Region) != "" {
		updates["region"] = strings.TrimSpace(hello.Region)
	}
	database.DB.Model(&models.AgentConnection{}).Where("id = ?", agentID).Updates(updates)
}

func touchAgent(agentID uint) {
	now := time.Now()
	database.DB.Model(&models.AgentConnection{}).Where("id = ?", agentID).Updates(map[string]interface{}{
		"status":       "online",
		"last_seen_at": &now,
	})
}

func markAgentDisconnected(agentID uint) {
	now := time.Now()
	agentHub.Lock()
	delete(agentHub.clients, agentID)
	agentHub.Unlock()
	failAgentExecs(agentID, "Agent 连接已断开")
	database.DB.Model(&models.AgentConnection{}).Where("id = ?", agentID).Updates(map[string]interface{}{
		"status":          "offline",
		"disconnected_at": &now,
	})
}

func markStaleAgentsOffline() {
	cutoff := time.Now().Add(-2 * time.Minute)
	database.DB.Model(&models.AgentConnection{}).
		Where("status = ? AND (last_seen_at IS NULL OR last_seen_at < ?)", "online", cutoff).
		Updates(map[string]interface{}{"status": "offline"})
}

func findAgentByToken(token string) (models.AgentConnection, bool) {
	if strings.TrimSpace(token) == "" {
		return models.AgentConnection{}, false
	}
	tokenHash := hashAgentToken(token)
	var agents []models.AgentConnection
	database.DB.Where("status <> ?", "deleted").Find(&agents)
	for _, item := range agents {
		if subtle.ConstantTimeCompare([]byte(item.TokenHash), []byte(tokenHash)) == 1 {
			return item, true
		}
	}
	return models.AgentConnection{}, false
}

func agentTokenFromRequest(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	if token := r.Header.Get("X-Agent-Token"); token != "" {
		return strings.TrimSpace(token)
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

func agentConnectURL(c *gin.Context) string {
	scheme := "ws"
	if c.GetHeader("X-Forwarded-Proto") == "https" || c.Request.TLS != nil {
		scheme = "wss"
	}
	host := c.Request.Host
	if forwardedHost := strings.TrimSpace(c.GetHeader("X-Forwarded-Host")); forwardedHost != "" {
		host = forwardedHost
	}
	return fmt.Sprintf("%s://%s/api/agent/connect", scheme, host)
}

func agentBinaryURL(c *gin.Context, token string) string {
	scheme := "http"
	if c.GetHeader("X-Forwarded-Proto") == "https" || c.Request.TLS != nil {
		scheme = "https"
	}
	host := c.Request.Host
	if forwardedHost := strings.TrimSpace(c.GetHeader("X-Forwarded-Host")); forwardedHost != "" {
		host = forwardedHost
	}
	return fmt.Sprintf("%s://%s/api/agent/bin?token=%s", scheme, host, token)
}

func buildAgentInstallScript(serverURL, binaryURL, token, region string) string {
	return fmt.Sprintf(`#!/bin/sh
set -eu

AGENT_NAME="aiops-agent"
INSTALL_DIR="${AIOPS_AGENT_INSTALL_DIR:-/usr/local/bin}"
CONFIG_DIR="${AIOPS_AGENT_CONFIG_DIR:-/etc/aiops-agent}"
BIN_PATH="$INSTALL_DIR/aiops-agent"
SERVER=%s
BINARY_URL=%s
TOKEN=%s
REGION=%s

if [ "$(id -u)" != "0" ]; then
  echo "请使用 root 用户执行安装脚本。" >&2
  exit 1
fi

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "不支持的 CPU 架构: $ARCH" >&2; exit 1 ;;
esac

DOWNLOAD_URL="$BINARY_URL&os=$OS&arch=$ARCH"
TMP_FILE="$(mktemp)"
trap 'rm -f "$TMP_FILE"' EXIT

echo "下载 AI Ops Agent: $OS/$ARCH"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$DOWNLOAD_URL" -o "$TMP_FILE"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "$TMP_FILE" "$DOWNLOAD_URL"
else
  echo "需要 curl 或 wget 下载 Agent 二进制。" >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR" "$CONFIG_DIR"
install -m 0755 "$TMP_FILE" "$BIN_PATH"

cat > "$CONFIG_DIR/agent.env" <<EOF_ENV
AIOPS_SERVER=$SERVER
AIOPS_AGENT_TOKEN=$TOKEN
AIOPS_AGENT_REGION=$REGION
EOF_ENV
chmod 600 "$CONFIG_DIR/agent.env"

if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  cat > /etc/systemd/system/aiops-agent.service <<EOF_SERVICE
[Unit]
Description=AI Ops Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=$CONFIG_DIR/agent.env
ExecStart=$BIN_PATH
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF_SERVICE
  systemctl daemon-reload
  systemctl enable aiops-agent
  systemctl restart aiops-agent
  systemctl status aiops-agent --no-pager || true
else
  pkill -f "$BIN_PATH" >/dev/null 2>&1 || true
  nohup env AIOPS_SERVER="$SERVER" AIOPS_AGENT_TOKEN="$TOKEN" AIOPS_AGENT_REGION="$REGION" "$BIN_PATH" >/var/log/aiops-agent.log 2>&1 &
  echo "systemd 不可用，已使用后台进程启动，日志: /var/log/aiops-agent.log"
fi

echo "AI Ops Agent 已安装并启动。"
`, shellutil.Quote(serverURL), shellutil.Quote(binaryURL), shellutil.Quote(token), shellutil.Quote(region))
}

func newAgentToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := agentTokenPrefix + hex.EncodeToString(raw)
	return token, hashAgentToken(token), nil
}

func newAgentRequestID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func defaultAgentTimeout(seconds int) int {
	if seconds <= 0 {
		return 300
	}
	if seconds > 1800 {
		return 1800
	}
	return seconds
}

func agentVersionAtLeast(current, minimum string) bool {
	parse := func(value string) ([3]int, bool) {
		var version [3]int
		value = strings.TrimPrefix(strings.TrimSpace(value), "v")
		if _, err := fmt.Sscanf(value, "%d.%d.%d", &version[0], &version[1], &version[2]); err != nil {
			return version, false
		}
		return version, true
	}
	currentVersion, currentOK := parse(current)
	minimumVersion, minimumOK := parse(minimum)
	if !currentOK || !minimumOK {
		return false
	}
	for i := range currentVersion {
		if currentVersion[i] != minimumVersion[i] {
			return currentVersion[i] > minimumVersion[i]
		}
	}
	return true
}

func hashAgentToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func mustParseUint(value string) uint {
	var id uint
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0
		}
		id = id*10 + uint(ch-'0')
	}
	return id
}
