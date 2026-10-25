package handlers

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"guanlan/pkg/shellutil"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const probeMetaMarker = "\n__AIOPS_PROBE_META__:"

type ProbeReq struct {
	ScriptID        uint              `json:"script_id"`
	ExpiryAlertDays *int              `json:"expiry_alert_days"`
	Type            string            `json:"type" binding:"required"`
	Target          string            `json:"target"`
	Config          string            `json:"config"`
	Name            string            `json:"name"`
	Host            string            `json:"host"`
	Port            int               `json:"port"`
	Method          string            `json:"method"`
	Path            string            `json:"path"`
	HTTPS           bool              `json:"https"`
	Headers         map[string]string `json:"headers"`
	Params          map[string]string `json:"params"`
	TimeoutMS       int               `json:"timeout_ms"`
	ContentType     string            `json:"content_type"`
	AuthType        string            `json:"auth_type"`
	Username        string            `json:"username"`
	Password        string            `json:"password"`
	SuccessCodes    string            `json:"success_codes"`
	ResponseKeyword string            `json:"response_keyword"`
	Body            string            `json:"body"`
	NodeType        string            `json:"node_type"`
	NodeID          uint              `json:"node_id"`
	Count           int               `json:"count"`
	ServerName      string            `json:"server_name"`
	ClientID        string            `json:"client_id"`
	MQTTTLS         bool              `json:"mqtt_tls"`
	KeepAlive       int               `json:"keepalive_seconds"`
}

type probeExecutionResult struct {
	StatusCode  int    `json:"status_code"`
	ContentType string `json:"content_type,omitempty"`
	Body        string `json:"body"`
	NodeType    string `json:"node_type"`
	NodeName    string `json:"node_name"`
	URL         string `json:"url"`
	Method      string `json:"method"`
	Truncated   bool   `json:"truncated,omitempty"`
}

type networkProbeResult struct {
	CertificateStatus string    `json:"certificate_status,omitempty"`
	ExpiryAlertDays   int       `json:"expiry_alert_days,omitempty"`
	Error             string    `json:"error,omitempty"`
	ProbeType         string    `json:"probe_type"`
	Host              string    `json:"host"`
	Port              int       `json:"port,omitempty"`
	Connected         bool      `json:"connected,omitempty"`
	Output            string    `json:"output,omitempty"`
	NodeType          string    `json:"node_type"`
	NodeName          string    `json:"node_name"`
	Subject           string    `json:"subject,omitempty"`
	Issuer            string    `json:"issuer,omitempty"`
	SerialNumber      string    `json:"serial_number,omitempty"`
	NotBefore         time.Time `json:"not_before,omitempty"`
	NotAfter          time.Time `json:"not_after,omitempty"`
	DaysRemaining     int       `json:"days_remaining,omitempty"`
	FingerprintSHA256 string    `json:"fingerprint_sha256,omitempty"`
}

func InstantProbe(c *gin.Context) {
	var req ProbeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	if req.Type != "http" && req.Type != "icmp" && req.Type != "tcp" && req.Type != "certificate_expiry" && req.Type != "script" {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "不支持的拨测类型")
		return
	}
	if req.Type == "script" {
		if _, err := loadProbeScript(req); err != nil {
			resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
			return
		}
		status, result, duration := executeScriptProbe(req)
		resp.Success(c, http.StatusOK, "拨测完成", gin.H{
			"status": status, "probe_type": "script", "duration_ms": duration, "result": result,
		})
		return
	}
	if req.Type != "http" {
		var result networkProbeResult
		var durationMS int64
		var err error
		switch req.Type {
		case "icmp":
			result, durationMS, err = executeICMPProbe(req)
		case "tcp":
			result, durationMS, err = executeTCPProbe(req)
		case "certificate_expiry":
			result, durationMS, err = executeCertificateExpiryProbe(req)
		}
		if req.Type == "certificate_expiry" {
			status := "success"
			if err != nil {
				status = "fail"
			}
			resp.Success(c, http.StatusOK, "检测完成", gin.H{
				"status": status, "probe_type": req.Type, "duration_ms": durationMS, "result": mustJSON(result),
				"certificate_status": result.CertificateStatus, "not_after": result.NotAfter, "days_remaining": result.DaysRemaining,
				"error": result.Error, "message": result.Output,
			})
			return
		}
		if err != nil {
			resp.Success(c, http.StatusOK, "拨测完成", gin.H{
				"status": "fail", "message": err.Error(), "error": err.Error(), "duration_ms": durationMS,
				"probe_type": req.Type, "result": mustJSON(gin.H{"error": err.Error()}),
			})
			return
		}
		payload := gin.H{
			"status": "success", "message": "拨测成功", "duration_ms": durationMS,
			"probe_type": req.Type, "host": result.Host, "port": result.Port,
			"connected": result.Connected, "output": result.Output,
			"node_type": result.NodeType, "node_name": result.NodeName,
			"result": mustJSON(result),
		}
		if req.Type == "ssl" {
			payload["subject"] = result.Subject
			payload["issuer"] = result.Issuer
			payload["serial_number"] = result.SerialNumber
			payload["not_before"] = result.NotBefore
			payload["not_after"] = result.NotAfter
			payload["days_remaining"] = result.DaysRemaining
			payload["fingerprint_sha256"] = result.FingerprintSHA256
		}
		resp.Success(c, http.StatusOK, "拨测完成", payload)
		return
	}

	result, durationMS, err := executeHTTPProbe(req)
	if err != nil {
		resp.Success(c, http.StatusOK, "拨测完成", gin.H{
			"status": "fail", "result": mustJSON(gin.H{"error": err.Error()}), "error": err.Error(), "duration_ms": durationMS,
		})
		return
	}
	status := "success"
	message := "拨测成功"
	if !statusCodeMatches(result.StatusCode, req.SuccessCodes) {
		status = "fail"
		message = fmt.Sprintf("HTTP 状态码 %d 不符合预期", result.StatusCode)
	} else if keyword := strings.TrimSpace(req.ResponseKeyword); keyword != "" && !strings.Contains(result.Body, keyword) {
		status = "fail"
		message = fmt.Sprintf("响应内容未包含关键字：%s", keyword)
	}
	keyword := strings.TrimSpace(req.ResponseKeyword)
	resp.Success(c, http.StatusOK, "拨测完成", gin.H{
		"status": status, "message": message, "status_code": result.StatusCode, "duration_ms": durationMS,
		"body": result.Body, "content_type": result.ContentType, "node_type": result.NodeType,
		"node_name": result.NodeName, "url": result.URL, "method": result.Method,
		"truncated": result.Truncated, "response_keyword": keyword,
		"keyword_matched": keyword == "" || strings.Contains(result.Body, keyword), "result": mustJSON(result),
	})
}

func executeHTTPProbe(req ProbeReq) (probeExecutionResult, int64, error) {
	start := time.Now()
	requestURL, err := buildProbeURL(req)
	if err != nil {
		return probeExecutionResult{}, 0, err
	}
	args, err := buildProbeCurlArgs(req, requestURL)
	if err != nil {
		return probeExecutionResult{}, 0, err
	}
	timeout := time.Duration(normalizeProbeTimeout(req.TimeoutMS)) * time.Millisecond
	nodeType := certificateDefaultString(strings.TrimSpace(req.NodeType), "direct")
	var output, nodeName string

	switch nodeType {
	case "direct":
		ctx, cancel := context.WithTimeout(context.Background(), timeout+2*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "curl", args...)
		data, cmdErr := cmd.CombinedOutput()
		output = string(data)
		if ctx.Err() == context.DeadlineExceeded {
			return probeExecutionResult{}, time.Since(start).Milliseconds(), errors.New("拨测超时")
		}
		if cmdErr != nil {
			return probeExecutionResult{}, time.Since(start).Milliseconds(), curlExecutionError(output, cmdErr)
		}
		nodeName = "服务端直连"
	case "ssh":
		var node models.CertificateExecutor
		if req.NodeID == 0 || database.DB.First(&node, req.NodeID).Error != nil {
			return probeExecutionResult{}, time.Since(start).Milliseconds(), errors.New("SSH 节点不存在")
		}
		if node.Status != 1 {
			return probeExecutionResult{}, time.Since(start).Milliseconds(), errors.New("SSH 节点已停用")
		}
		output, err = runSSHCommand(node, shellutil.Command("curl", args))
		if err != nil {
			return probeExecutionResult{}, time.Since(start).Milliseconds(), curlExecutionError(output, err)
		}
		nodeName = node.Name
	case "agent":
		var node models.AgentConnection
		if req.NodeID == 0 || database.DB.First(&node, req.NodeID).Error != nil {
			return probeExecutionResult{}, time.Since(start).Milliseconds(), errors.New("Agent 节点不存在")
		}
		if !agentVersionAtLeast(node.Version, minimumAgentExecVersion) {
			return probeExecutionResult{}, time.Since(start).Milliseconds(), fmt.Errorf("Agent 版本过旧，请重新部署（最低版本 %s）", minimumAgentExecVersion)
		}
		agentResult, runErr := runAgentCommand(node.ID, shellutil.Command("curl", args), timeout)
		output = strings.TrimSpace(agentResult.Stdout + "\n" + agentResult.Stderr)
		if runErr != nil {
			return probeExecutionResult{}, time.Since(start).Milliseconds(), runErr
		}
		if agentResult.ExitCode != 0 {
			return probeExecutionResult{}, time.Since(start).Milliseconds(), curlExecutionError(output, fmt.Errorf("命令退出码 %d", agentResult.ExitCode))
		}
		nodeName = node.Name
	default:
		return probeExecutionResult{}, time.Since(start).Milliseconds(), errors.New("不支持的节点类型")
	}

	statusCode, contentType, body, err := parseProbeCurlOutput(output)
	if err != nil {
		return probeExecutionResult{}, time.Since(start).Milliseconds(), err
	}
	truncated := false
	if len(body) > 65536 {
		body = body[:65536]
		truncated = true
	}
	return probeExecutionResult{
		StatusCode: statusCode, ContentType: contentType, Body: body, NodeType: nodeType,
		NodeName: nodeName, URL: requestURL, Method: normalizeProbeMethod(req.Method), Truncated: truncated,
	}, time.Since(start).Milliseconds(), nil
}

func executeTCPProbe(req ProbeReq) (networkProbeResult, int64, error) {
	start := time.Now()
	host, port, timeout, err := normalizeNetworkProbe(req, 80)
	if err != nil {
		return networkProbeResult{}, 0, err
	}
	result := networkProbeResult{ProbeType: "tcp", Host: host, Port: port}
	nodeType := certificateDefaultString(strings.TrimSpace(req.NodeType), "direct")
	if nodeType == "direct" {
		connection, dialErr := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), timeout)
		if dialErr != nil {
			return result, time.Since(start).Milliseconds(), fmt.Errorf("TCP 连接失败: %v", dialErr)
		}
		_ = connection.Close()
		result.Connected = true
		result.NodeType = "direct"
		result.NodeName = "服务端直连"
		result.Output = fmt.Sprintf("TCP %s:%d 连接成功", host, port)
		return result, time.Since(start).Milliseconds(), nil
	}

	seconds := max(1, int(timeout/time.Second))
	inner := fmt.Sprintf("cat < /dev/null > /dev/tcp/%s/%d", host, port)
	command := shellutil.Command("timeout", []string{strconv.Itoa(seconds), "bash", "-c", inner})
	output, resolvedType, nodeName, runErr := executeRemoteProbeCommand(req, command, timeout)
	if runErr != nil {
		return result, time.Since(start).Milliseconds(), fmt.Errorf("TCP 连接失败: %v", runErr)
	}
	result.Connected = true
	result.NodeType = resolvedType
	result.NodeName = nodeName
	result.Output = strings.TrimSpace(output)
	if result.Output == "" {
		result.Output = fmt.Sprintf("TCP %s:%d 连接成功", host, port)
	}
	return result, time.Since(start).Milliseconds(), nil
}

func executeICMPProbe(req ProbeReq) (networkProbeResult, int64, error) {
	start := time.Now()
	host := strings.Trim(strings.TrimSpace(req.Host), "[]")
	if err := validateProbeHost(host); err != nil {
		return networkProbeResult{}, 0, err
	}
	timeout := time.Duration(normalizeProbeTimeout(req.TimeoutMS)) * time.Millisecond
	count := normalizeICMPCount(req.Count)
	waitSeconds := max(1, int(timeout/time.Second))
	args := []string{"-c", strconv.Itoa(count), "-W", strconv.Itoa(waitSeconds), host}
	result := networkProbeResult{ProbeType: "icmp", Host: host}
	nodeType := certificateDefaultString(strings.TrimSpace(req.NodeType), "direct")
	var output, nodeName string
	var err error
	if nodeType == "direct" {
		ctx, cancel := context.WithTimeout(context.Background(), timeout+time.Second)
		defer cancel()
		data, runErr := exec.CommandContext(ctx, "ping", args...).CombinedOutput()
		output = string(data)
		if ctx.Err() == context.DeadlineExceeded {
			err = errors.New("ICMP 拨测超时")
		} else if runErr != nil {
			err = fmt.Errorf("ICMP 拨测失败: %s", commandErrorMessage(output, runErr))
		}
		nodeName = "服务端直连"
	} else {
		command := shellutil.Command("timeout", []string{strconv.Itoa(waitSeconds), "ping"}) + " " + strings.Join(shellQuotedArgs(args), " ")
		output, nodeType, nodeName, err = executeRemoteProbeCommand(req, command, timeout)
		if err != nil {
			err = fmt.Errorf("ICMP 拨测失败: %v", err)
		}
	}
	if err != nil {
		return result, time.Since(start).Milliseconds(), err
	}
	result.Connected = true
	result.Output = strings.TrimSpace(output)
	result.NodeType = nodeType
	result.NodeName = nodeName
	return result, time.Since(start).Milliseconds(), nil
}

func executeSSLProbe(req ProbeReq) (networkProbeResult, int64, error) {
	start := time.Now()
	host, port, timeout, err := normalizeNetworkProbe(req, 443)
	if err != nil {
		return networkProbeResult{}, 0, err
	}
	serverName := strings.TrimSpace(req.ServerName)
	if serverName == "" && net.ParseIP(host) == nil {
		serverName = host
	}
	if serverName != "" {
		if err := validateProbeHost(serverName); err != nil {
			return networkProbeResult{}, 0, errors.New("SSL SNI 格式不正确")
		}
	}
	result := networkProbeResult{ProbeType: "ssl", Host: host, Port: port}
	nodeType := certificateDefaultString(strings.TrimSpace(req.NodeType), "direct")
	if nodeType == "direct" {
		dialer := &net.Dialer{Timeout: timeout}
		connection, dialErr := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(host, strconv.Itoa(port)), &tls.Config{
			ServerName: serverName, InsecureSkipVerify: true, // 拨测需读取自签名或过期证书信息。
		})
		if dialErr != nil {
			return result, time.Since(start).Milliseconds(), fmt.Errorf("SSL 握手失败: %v", dialErr)
		}
		defer connection.Close()
		certificates := connection.ConnectionState().PeerCertificates
		if len(certificates) == 0 {
			return result, time.Since(start).Milliseconds(), errors.New("目标未返回 SSL 证书")
		}
		certificate := certificates[0]
		fingerprint := sha256.Sum256(certificate.Raw)
		result.Connected = true
		result.NodeType = "direct"
		result.NodeName = "服务端直连"
		result.Subject = certificate.Subject.String()
		result.Issuer = certificate.Issuer.String()
		result.SerialNumber = certificate.SerialNumber.Text(16)
		result.NotBefore = certificate.NotBefore
		result.NotAfter = certificate.NotAfter
		result.DaysRemaining = int(time.Until(certificate.NotAfter).Hours() / 24)
		result.FingerprintSHA256 = fmt.Sprintf("%X", fingerprint)
		result.Output = fmt.Sprintf("SSL 握手成功，证书有效期至 %s", certificate.NotAfter.Format(time.RFC3339))
		return result, time.Since(start).Milliseconds(), nil
	}

	connect := net.JoinHostPort(host, strconv.Itoa(port))
	args := []string{"s_client", "-connect", connect, "-showcerts"}
	if serverName != "" {
		args = append(args, "-servername", serverName)
	}
	pipeline := "printf '' | " + shellutil.Command("openssl", args) + " 2>/dev/null | " + shellutil.Command("openssl", []string{"x509", "-noout", "-subject", "-issuer", "-serial", "-dates", "-fingerprint", "-sha256"})
	command := shellutil.Command("timeout", []string{strconv.Itoa(max(1, int(timeout/time.Second))), "sh", "-c", pipeline})
	output, resolvedType, nodeName, runErr := executeRemoteProbeCommand(req, command, timeout)
	if runErr != nil {
		return result, time.Since(start).Milliseconds(), fmt.Errorf("SSL 握手失败: %v", runErr)
	}
	if err := parseOpenSSLCertificate(strings.TrimSpace(output), &result); err != nil {
		return result, time.Since(start).Milliseconds(), err
	}
	result.Connected = true
	result.NodeType = resolvedType
	result.NodeName = nodeName
	result.Output = strings.TrimSpace(output)
	return result, time.Since(start).Milliseconds(), nil
}

func executeMQTTProbe(req ProbeReq) (networkProbeResult, int64, error) {
	start := time.Now()
	defaultPort := 1883
	if req.MQTTTLS {
		defaultPort = 8883
	}
	host, port, timeout, err := normalizeNetworkProbe(req, defaultPort)
	result := networkProbeResult{ProbeType: "mqtt", Host: host, Port: port}
	if err != nil {
		return result, 0, err
	}
	nodeType := certificateDefaultString(strings.TrimSpace(req.NodeType), "direct")
	if nodeType != "direct" {
		return result, time.Since(start).Milliseconds(), errors.New("MQTT 拨测当前仅支持服务端直连")
	}

	dialer := &net.Dialer{Timeout: timeout}
	var connection net.Conn
	address := net.JoinHostPort(host, strconv.Itoa(port))
	if req.MQTTTLS {
		serverName := strings.TrimSpace(req.ServerName)
		if serverName == "" && net.ParseIP(host) == nil {
			serverName = host
		}
		connection, err = tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
			ServerName: serverName, InsecureSkipVerify: true, // 即时拨测需兼容自签名 Broker。
		})
	} else {
		connection, err = dialer.Dial("tcp", address)
	}
	if err != nil {
		return result, time.Since(start).Milliseconds(), fmt.Errorf("MQTT 连接失败: %v", err)
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(timeout))

	packet, clientID, err := buildMQTTConnectPacket(req)
	if err != nil {
		return result, time.Since(start).Milliseconds(), err
	}
	if _, err = connection.Write(packet); err != nil {
		return result, time.Since(start).Milliseconds(), fmt.Errorf("MQTT CONNECT 发送失败: %v", err)
	}
	response := make([]byte, 4)
	if _, err = io.ReadFull(connection, response); err != nil {
		return result, time.Since(start).Milliseconds(), fmt.Errorf("MQTT CONNACK 读取失败: %v", err)
	}
	if response[0] != 0x20 || response[1] != 0x02 {
		return result, time.Since(start).Milliseconds(), errors.New("Broker 返回了无效的 MQTT CONNACK")
	}
	if response[3] != 0 {
		return result, time.Since(start).Milliseconds(), fmt.Errorf("MQTT 连接被拒绝: %s", mqttConnAckMessage(response[3]))
	}
	result.Connected = true
	result.NodeType = "direct"
	result.NodeName = "服务端直连"
	transport := "TCP"
	if req.MQTTTLS {
		transport = "TLS"
	}
	result.Output = fmt.Sprintf("MQTT 3.1.1 建连成功，Client ID: %s，传输协议: %s", clientID, transport)
	return result, time.Since(start).Milliseconds(), nil
}

func buildMQTTConnectPacket(req ProbeReq) ([]byte, string, error) {
	if req.Password != "" && req.Username == "" {
		return nil, "", errors.New("填写 MQTT 密码时必须同时填写用户名")
	}
	clientID := strings.TrimSpace(req.ClientID)
	if clientID == "" {
		clientID = fmt.Sprintf("aiops-probe-%x", time.Now().UnixNano())
	}
	keepAlive := req.KeepAlive
	if keepAlive <= 0 {
		keepAlive = 30
	}
	if keepAlive > 65535 {
		return nil, "", errors.New("MQTT Keep Alive 不能超过 65535 秒")
	}
	flags := byte(0x02) // Clean Session
	if req.Username != "" {
		flags |= 0x80
	}
	if req.Password != "" {
		flags |= 0x40
	}
	variable := []byte{0x00, 0x04, 'M', 'Q', 'T', 'T', 0x04, flags, byte(keepAlive >> 8), byte(keepAlive)}
	payload := make([]byte, 0, len(clientID)+len(req.Username)+len(req.Password)+6)
	var err error
	if payload, err = appendMQTTString(payload, clientID); err != nil {
		return nil, "", errors.New("MQTT Client ID 过长")
	}
	if req.Username != "" {
		if payload, err = appendMQTTString(payload, req.Username); err != nil {
			return nil, "", errors.New("MQTT 用户名过长")
		}
	}
	if req.Password != "" {
		if payload, err = appendMQTTString(payload, req.Password); err != nil {
			return nil, "", errors.New("MQTT 密码过长")
		}
	}
	remaining := len(variable) + len(payload)
	packet := []byte{0x10}
	for {
		encoded := byte(remaining % 128)
		remaining /= 128
		if remaining > 0 {
			encoded |= 0x80
		}
		packet = append(packet, encoded)
		if remaining == 0 {
			break
		}
	}
	packet = append(packet, variable...)
	packet = append(packet, payload...)
	return packet, clientID, nil
}

func appendMQTTString(target []byte, value string) ([]byte, error) {
	if len(value) > 65535 {
		return target, errors.New("MQTT 字符串过长")
	}
	target = append(target, byte(len(value)>>8), byte(len(value)))
	return append(target, []byte(value)...), nil
}

func mqttConnAckMessage(code byte) string {
	messages := map[byte]string{
		1: "协议版本不受支持", 2: "Client ID 被拒绝", 3: "Broker 暂不可用",
		4: "用户名或密码错误", 5: "未授权",
	}
	if message, ok := messages[code]; ok {
		return message
	}
	return fmt.Sprintf("未知返回码 %d", code)
}

func normalizeNetworkProbe(req ProbeReq, defaultPort int) (string, int, time.Duration, error) {
	host := strings.Trim(strings.TrimSpace(req.Host), "[]")
	if err := validateProbeHost(host); err != nil {
		return "", 0, 0, err
	}
	port := req.Port
	if port <= 0 {
		port = defaultPort
	}
	if port > 65535 {
		return "", 0, 0, errors.New("端口范围应为 1-65535")
	}
	return host, port, time.Duration(normalizeProbeTimeout(req.TimeoutMS)) * time.Millisecond, nil
}

func validateProbeHost(host string) error {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" {
		return errors.New("请填写目标 Host")
	}
	if net.ParseIP(host) != nil {
		return nil
	}
	if len(host) > 253 || strings.ContainsAny(host, " /\\;|&$()`'\"<>?#@") {
		return errors.New("目标 Host 只允许填写 IP 或域名")
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return errors.New("目标 Host 格式不正确")
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '-' && character != '_' {
				return errors.New("目标 Host 格式不正确")
			}
		}
	}
	return nil
}

func normalizeICMPCount(count int) int {
	if count <= 0 {
		return 4
	}
	if count > 10 {
		return 10
	}
	return count
}

func executeRemoteProbeCommand(req ProbeReq, command string, timeout time.Duration) (string, string, string, error) {
	nodeType := strings.TrimSpace(req.NodeType)
	switch nodeType {
	case "ssh":
		var node models.CertificateExecutor
		if req.NodeID == 0 || database.DB.First(&node, req.NodeID).Error != nil {
			return "", nodeType, "", errors.New("SSH 节点不存在")
		}
		if node.Status != 1 {
			return "", nodeType, node.Name, errors.New("SSH 节点已停用")
		}
		output, err := runSSHCommand(node, command)
		return output, nodeType, node.Name, err
	case "agent":
		var node models.AgentConnection
		if req.NodeID == 0 || database.DB.First(&node, req.NodeID).Error != nil {
			return "", nodeType, "", errors.New("Agent 节点不存在")
		}
		if !agentVersionAtLeast(node.Version, minimumAgentExecVersion) {
			return "", nodeType, node.Name, fmt.Errorf("Agent 版本过旧，请重新部署（最低版本 %s）", minimumAgentExecVersion)
		}
		agentResult, err := runAgentCommand(node.ID, command, timeout)
		output := strings.TrimSpace(agentResult.Stdout + "\n" + agentResult.Stderr)
		if err == nil && agentResult.ExitCode != 0 {
			err = fmt.Errorf("命令退出码 %d: %s", agentResult.ExitCode, commandErrorMessage(output, nil))
		}
		return output, nodeType, node.Name, err
	default:
		return "", nodeType, "", errors.New("不支持的节点类型")
	}
}

func parseOpenSSLCertificate(output string, result *networkProbeResult) error {
	if output == "" {
		return errors.New("目标未返回 SSL 证书")
	}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "subject":
			result.Subject = strings.TrimSpace(value)
		case "issuer":
			result.Issuer = strings.TrimSpace(value)
		case "serial":
			result.SerialNumber = strings.TrimSpace(value)
		case "notbefore":
			result.NotBefore, _ = time.Parse("Jan 2 15:04:05 2006 MST", strings.TrimSpace(value))
		case "notafter":
			result.NotAfter, _ = time.Parse("Jan 2 15:04:05 2006 MST", strings.TrimSpace(value))
		case "sha256 fingerprint":
			result.FingerprintSHA256 = strings.ReplaceAll(strings.TrimSpace(value), ":", "")
		}
	}
	if result.Subject == "" || result.NotAfter.IsZero() {
		return errors.New("无法解析目标 SSL 证书")
	}
	result.DaysRemaining = int(time.Until(result.NotAfter).Hours() / 24)
	return nil
}

func commandErrorMessage(output string, err error) string {
	output = strings.TrimSpace(output)
	if len(output) > 500 {
		output = output[:500]
	}
	if output != "" {
		return output
	}
	if err != nil {
		return err.Error()
	}
	return "执行失败"
}

func shellQuotedArgs(args []string) []string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellutil.Quote(arg))
	}
	return quoted
}

func buildProbeURL(req ProbeReq) (string, error) {
	if target := strings.TrimSpace(req.Target); target != "" {
		parsed, err := url.ParseRequestURI(target)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return "", errors.New("目标 URL 格式不正确")
		}
		if len(req.Params) > 0 {
			values := parsed.Query()
			for key, value := range req.Params {
				if strings.TrimSpace(key) != "" {
					values.Set(key, value)
				}
			}
			parsed.RawQuery = values.Encode()
		}
		return parsed.String(), nil
	}
	host := strings.TrimSpace(req.Host)
	if host == "" {
		return "", errors.New("请填写目标 Host")
	}
	if strings.Contains(host, "://") || strings.ContainsAny(host, "/?#") {
		return "", errors.New("目标 Host 只填写 IP 或域名")
	}
	scheme := "http"
	defaultPort := 80
	if req.HTTPS {
		scheme = "https"
		defaultPort = 443
	}
	port := req.Port
	if port <= 0 {
		port = defaultPort
	}
	if port > 65535 {
		return "", errors.New("端口范围应为 1-65535")
	}
	hostPort := host
	if port != defaultPort {
		hostPort = net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port))
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	parsed := &url.URL{Scheme: scheme, Host: hostPort, Path: path}
	if len(req.Params) > 0 {
		values := url.Values{}
		for key, value := range req.Params {
			if strings.TrimSpace(key) != "" {
				values.Set(key, value)
			}
		}
		parsed.RawQuery = values.Encode()
	}
	return parsed.String(), nil
}

func buildProbeCurlArgs(req ProbeReq, requestURL string) ([]string, error) {
	method := normalizeProbeMethod(req.Method)
	if method != "GET" && method != "POST" && method != "PUT" && method != "DELETE" && method != "PATCH" && method != "HEAD" {
		return nil, errors.New("不支持的 HTTP 请求方式")
	}
	if !statusCodeExpressionValid(req.SuccessCodes) {
		return nil, errors.New("成功状态码格式不正确")
	}
	timeoutSeconds := float64(normalizeProbeTimeout(req.TimeoutMS)) / 1000
	args := []string{"-sS", "-k", "--max-time", strconv.FormatFloat(timeoutSeconds, 'f', 3, 64), "-X", method}
	keys := sortedKeys(req.Headers)
	for _, key := range keys {
		if strings.TrimSpace(key) != "" {
			args = append(args, "-H", key+": "+req.Headers[key])
		}
	}
	if contentType := strings.TrimSpace(req.ContentType); contentType != "" {
		args = append(args, "-H", "Content-Type: "+contentType)
	}
	switch strings.ToLower(strings.TrimSpace(req.AuthType)) {
	case "", "none":
	case "basic", "digest":
		if strings.TrimSpace(req.Username) == "" {
			return nil, errors.New("认证用户名不能为空")
		}
		args = append(args, "--"+strings.ToLower(req.AuthType), "-u", req.Username+":"+req.Password)
	default:
		return nil, errors.New("不支持的认证方式")
	}
	if req.Body != "" && method != "GET" && method != "HEAD" {
		args = append(args, "--data-raw", req.Body)
	}
	args = append(args, "-w", probeMetaMarker+"%{http_code}|%{content_type}", requestURL)
	return args, nil
}

func parseProbeCurlOutput(output string) (int, string, string, error) {
	index := strings.LastIndex(output, probeMetaMarker)
	if index < 0 {
		return 0, "", "", errors.New("无法解析拨测响应")
	}
	body := strings.TrimSuffix(output[:index], "\n")
	meta := strings.TrimSpace(output[index+len(probeMetaMarker):])
	parts := strings.SplitN(meta, "|", 2)
	code, err := strconv.Atoi(parts[0])
	if err != nil || code <= 0 {
		return 0, "", body, errors.New("目标未返回有效 HTTP 状态码")
	}
	contentType := ""
	if len(parts) == 2 {
		contentType = parts[1]
	}
	return code, contentType, body, nil
}

func normalizeProbeMethod(method string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return "GET"
	}
	return method
}

func normalizeProbeTimeout(timeoutMS int) int {
	if timeoutMS <= 0 {
		return 10000
	}
	if timeoutMS < 100 {
		return 100
	}
	if timeoutMS > 300000 {
		return 300000
	}
	return timeoutMS
}

func statusCodeMatches(code int, expression string) bool {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return code >= 200 && code < 400
	}
	for _, token := range strings.Split(expression, ",") {
		token = strings.TrimSpace(token)
		if strings.Contains(token, "-") {
			parts := strings.SplitN(token, "-", 2)
			start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
			end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err1 == nil && err2 == nil && code >= start && code <= end {
				return true
			}
		} else if expected, err := strconv.Atoi(token); err == nil && code == expected {
			return true
		}
	}
	return false
}

func statusCodeExpressionValid(expression string) bool {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return true
	}
	for _, token := range strings.Split(expression, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			return false
		}
		if strings.Contains(token, "-") {
			parts := strings.SplitN(token, "-", 2)
			start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
			end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err1 != nil || err2 != nil || start < 100 || end > 599 || start > end {
				return false
			}
		} else if code, err := strconv.Atoi(token); err != nil || code < 100 || code > 599 {
			return false
		}
	}
	return true
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func curlExecutionError(output string, err error) error {
	output = strings.TrimSpace(output)
	if output == "" {
		return fmt.Errorf("拨测请求失败: %v", err)
	}
	if len(output) > 500 {
		output = output[:500]
	}
	return fmt.Errorf("拨测请求失败: %s", output)
}

func mustJSON(value interface{}) string {
	data, _ := json.Marshal(value)
	return string(data)
}
