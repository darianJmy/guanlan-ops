package handlers

import (
	"context"
	"guanlan/pkg/sshclient"

	"encoding/base64"
	"errors"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"guanlan/pkg/shellutil"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"
)

type saveExecutorReq struct {
	Name        string `json:"name" binding:"required"`
	Type        string `json:"type"`
	Host        string `json:"host" binding:"required"`
	Port        int    `json:"port"`
	Username    string `json:"username" binding:"required"`
	AuthType    string `json:"auth_type"`
	Password    string `json:"password"`
	PrivateKey  string `json:"private_key"`
	Status      int    `json:"status"`
	Description string `json:"description"`
}

func ListCertificateExecutors(c *gin.Context) {
	var items []models.CertificateExecutor
	database.DB.Order("id DESC").Find(&items)
	resp.Success(c, http.StatusOK, "成功", items)
}

func CreateCertificateExecutor(c *gin.Context) {
	var req saveExecutorReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请填写执行器名称、主机和用户名")
		return
	}
	item, err := buildExecutor(req, nil)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if err := database.DB.Create(&item).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "创建执行器失败")
		return
	}
	resp.Success(c, http.StatusOK, "创建成功", item)
}

func UpdateCertificateExecutor(c *gin.Context) {
	var existing models.CertificateExecutor
	if err := database.DB.First(&existing, c.Param("id")).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "执行器不存在")
		return
	}
	var req saveExecutorReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请填写执行器名称、主机和用户名")
		return
	}
	item, err := buildExecutor(req, &existing)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if err := database.DB.Model(&existing).Updates(map[string]interface{}{
		"name":          item.Name,
		"type":          item.Type,
		"host":          item.Host,
		"port":          item.Port,
		"username":      item.Username,
		"auth_type":     item.AuthType,
		"secret_cipher": item.SecretCipher,
		"status":        item.Status,
		"description":   item.Description,
	}).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "更新执行器失败")
		return
	}
	resp.Success(c, http.StatusOK, "更新成功")
}

func DeleteCertificateExecutor(c *gin.Context) {
	id := c.Param("id")
	if names := boundCertificateDomainNames("executor_id", id); len(names) > 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, boundResourceMessage("主机", names))
		return
	}
	if err := database.DB.Delete(&models.CertificateExecutor{}, id).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "删除主机失败")
		return
	}
	resp.Success(c, http.StatusOK, "删除成功")
}

func TestCertificateExecutor(c *gin.Context) {
	var item models.CertificateExecutor
	if err := database.DB.First(&item, c.Param("id")).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "执行器不存在")
		return
	}
	output, err := runSSHCommand(item, "echo executor-ok")
	if err != nil {
		resp.Error(c, http.StatusOK, resp.CodeFailure, err.Error(), output)
		return
	}
	resp.Success(c, http.StatusOK, "连接成功", output)
}

func DeployCertificateDomain(c *gin.Context) {
	start := time.Now()
	var domain models.CertificateDomain
	if err := database.DB.First(&domain, c.Param("id")).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "域名不存在")
		return
	}
	cert, version, privateKey, err := currentCertificateMaterial(domain.CertificateID)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	command, scriptID, targetType, err := buildDeployCommand(domain, cert, version.CertPEM, privateKey)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	output, err := runDomainDeployment(domain, command)
	status := "success"
	if err != nil {
		status = "fail"
		output = strings.TrimSpace(output + "\n" + err.Error())
	}
	record := models.CertificateDeploymentRecord{
		DomainID:       domain.ID,
		CertificateID:  cert.ID,
		ExecutorID:     domain.ExecutorID,
		AgentID:        domain.AgentID,
		DeployScriptID: scriptID,
		TargetType:     targetType,
		Status:         status,
		Output:         output,
		DurationMs:     time.Since(start).Milliseconds(),
	}
	database.DB.Create(&record)
	if err != nil {
		resp.Error(c, http.StatusOK, resp.CodeFailure, "轮转失败", record)
		return
	}
	_ = checkAndSaveDomain(&domain)
	resp.Success(c, http.StatusOK, "轮转完成", record)
}

func runDomainDeployment(domain models.CertificateDomain, command string) (string, error) {
	if domain.AgentID != 0 {
		var agent models.AgentConnection
		if err := database.DB.First(&agent, domain.AgentID).Error; err != nil {
			return "", errors.New("Agent 不存在")
		}
		if strings.TrimSpace(agent.Status) != "online" {
			return "", errors.New("Agent 当前不在线")
		}
		result, err := runAgentCommand(agent.ID, command, 10*time.Minute)
		if err != nil {
			return formatAgentExecOutput(result, err), err
		}
		if result.ExitCode != 0 {
			msg := strings.TrimSpace(result.Error)
			if msg == "" {
				msg = fmt.Sprintf("Agent 命令退出码 %d", result.ExitCode)
			}
			return formatAgentExecOutput(result, nil), errors.New(msg)
		}
		return formatAgentExecOutput(result, nil), nil
	}
	var executor models.CertificateExecutor
	if domain.ExecutorID == 0 {
		return "", errors.New("请先给域名绑定执行器或 Agent")
	}
	if err := database.DB.First(&executor, domain.ExecutorID).Error; err != nil {
		return "", errors.New("执行器不存在")
	}
	return runSSHCommand(executor, command)
}

func formatAgentExecOutput(result agentExecResult, runErr error) string {
	var builder strings.Builder
	if strings.TrimSpace(result.Stdout) != "" {
		builder.WriteString(strings.TrimSpace(result.Stdout))
	}
	if strings.TrimSpace(result.Stderr) != "" {
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(strings.TrimSpace(result.Stderr))
	}
	if runErr != nil {
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(runErr.Error())
	}
	return strings.TrimSpace(builder.String())
}

func ListCertificateDeploymentRecords(c *gin.Context) {
	var records []models.CertificateDeploymentRecord
	tx := database.DB.Order("id DESC")
	if domainID := c.Query("domain_id"); domainID != "" {
		tx = tx.Where("domain_id = ?", domainID)
	}
	if deployScriptID := c.Query("deploy_script_id"); deployScriptID != "" {
		tx = tx.Where("deploy_script_id = ?", deployScriptID)
	}
	tx.Limit(100).Find(&records)
	resp.Success(c, http.StatusOK, "成功", records)
}

func buildDeployCommand(domain models.CertificateDomain, cert models.Certificate, certPEM, privateKey string) (string, uint, string, error) {
	if domain.DeployScriptID == 0 {
		return "", 0, "", errors.New("请选择发布脚本")
	}

	var script models.CertificateDeployScript
	if err := database.DB.First(&script, domain.DeployScriptID).Error; err != nil {
		return "", 0, "", errors.New("发布脚本不存在")
	}
	if script.Status != 1 {
		return "", 0, "", errors.New("发布脚本已停用")
	}
	if script.Type != "shell" && script.Type != "python" {
		return "", 0, "", errors.New("脚本类型只支持 Shell 或 Python")
	}

	params := map[string]string{
		"DOMAIN":    domain.Domain,
		"CERT_NAME": cert.Name,
	}

	return buildDeployScriptCommand(script.Type, script.Content, certPEM, privateKey, params), script.ID, script.Type, nil
}

func buildDeployScriptCommand(scriptType, scriptContent, certPEM, privateKey string, params map[string]string) string {
	var builder strings.Builder
	scriptExt := "sh"
	runner := "bash"
	if scriptType == "python" {
		scriptExt = "py"
		runner = "python3"
	}
	builder.WriteString("set -e\n")
	builder.WriteString("set -o pipefail\n")
	builder.WriteString("CERT_PEM_B64=" + shellutil.Quote(base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(certPEM)+"\n"))) + "\n")
	builder.WriteString("PRIVATE_KEY_PEM_B64=" + shellutil.Quote(base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(privateKey)+"\n"))) + "\n")
	builder.WriteString("SCRIPT_B64=" + shellutil.Quote(base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(shellutil.NormalizeLineEndings(scriptContent))+"\n"))) + "\n")
	builder.WriteString("WORK_DIR=\"$(mktemp -d /tmp/cert-deploy-XXXXXX)\"\n")
	builder.WriteString("CERT_DEPLOY_DIR=\"$WORK_DIR\"\n")
	builder.WriteString("trap 'rm -rf \"$WORK_DIR\"' EXIT\n")
	builder.WriteString("CERT_FILE=\"$WORK_DIR/certificate.pem\"\n")
	builder.WriteString("KEY_FILE=\"$WORK_DIR/private_key.pem\"\n")
	builder.WriteString("SCRIPT_FILE=\"$WORK_DIR/deploy." + scriptExt + "\"\n")
	builder.WriteString("printf '%s' \"$CERT_PEM_B64\" | base64 -d > \"$CERT_FILE\"\n")
	builder.WriteString("printf '%s' \"$PRIVATE_KEY_PEM_B64\" | base64 -d > \"$KEY_FILE\"\n")
	builder.WriteString("printf '%s' \"$SCRIPT_B64\" | base64 -d > \"$SCRIPT_FILE\"\n")
	builder.WriteString("chmod 600 \"$KEY_FILE\"\n")
	builder.WriteString("chmod 700 \"$SCRIPT_FILE\"\n")
	builder.WriteString("export WORK_DIR CERT_DEPLOY_DIR CERT_FILE KEY_FILE\n")
	builder.WriteString("export PRIVATE_KEY_FILE=\"$KEY_FILE\"\n")
	for key, value := range params {
		if !shellutil.IsIdentifier(key) {
			continue
		}
		builder.WriteString("export " + key + "=" + shellutil.Quote(value) + "\n")
	}
	builder.WriteString(runner + " \"$SCRIPT_FILE\"\n")
	return builder.String()
}

func buildExecutor(req saveExecutorReq, existing *models.CertificateExecutor) (models.CertificateExecutor, error) {
	authType := strings.TrimSpace(req.AuthType)
	if authType == "" {
		authType = "password"
	}
	secret := req.Password
	if authType == "private_key" {
		secret = req.PrivateKey
	}
	if strings.TrimSpace(secret) == "" && existing == nil {
		return models.CertificateExecutor{}, errors.New("请填写执行器认证信息")
	}
	secretCipher := ""
	if existing != nil {
		secretCipher = existing.SecretCipher
	}
	if strings.TrimSpace(secret) != "" {
		ciphertext, err := encryptSecret(secret)
		if err != nil {
			return models.CertificateExecutor{}, errors.New("执行器认证信息加密失败")
		}
		secretCipher = ciphertext
	}
	status := req.Status
	if status == 0 {
		status = 1
	}
	item := models.CertificateExecutor{
		Name:         strings.TrimSpace(req.Name),
		Type:         certificateDefaultString(req.Type, "ssh"),
		Host:         strings.TrimSpace(req.Host),
		Port:         defaultSSHPort(req.Port),
		Username:     strings.TrimSpace(req.Username),
		AuthType:     authType,
		SecretCipher: secretCipher,
		Status:       status,
		Description:  strings.TrimSpace(req.Description),
	}
	if item.Type != "ssh" {
		return models.CertificateExecutor{}, errors.New("当前只支持 SSH 执行器")
	}
	if item.AuthType != "password" && item.AuthType != "private_key" {
		return models.CertificateExecutor{}, errors.New("认证方式只支持密码或私钥")
	}
	return item, nil
}

func currentCertificateMaterial(certificateID uint) (models.Certificate, models.CertificateVersion, string, error) {
	if certificateID == 0 {
		return models.Certificate{}, models.CertificateVersion{}, "", errors.New("请先给域名关联证书")
	}
	var cert models.Certificate
	if err := database.DB.First(&cert, certificateID).Error; err != nil {
		return models.Certificate{}, models.CertificateVersion{}, "", errors.New("证书不存在")
	}
	var version models.CertificateVersion
	tx := database.DB.Where("certificate_id = ?", cert.ID)
	if cert.CurrentVersionID > 0 {
		tx = tx.Where("id = ?", cert.CurrentVersionID)
	}
	if err := tx.Order("id DESC").First(&version).Error; err != nil {
		return models.Certificate{}, models.CertificateVersion{}, "", errors.New("证书材料不存在")
	}
	privateKey, err := decryptSecret(version.PrivateKeyCipher)
	if err != nil {
		return models.Certificate{}, models.CertificateVersion{}, "", errors.New("私钥解密失败")
	}
	return cert, version, privateKey, nil
}

func runSSHCommand(executor models.CertificateExecutor, command string) (string, error) {
	output, _, err := runSSHCommandOutput(executor, command)
	return output, err
}

func certificateSSHAuthMethod(authType, secret string) (ssh.AuthMethod, error) {
	if authType == "private_key" {
		method, err := sshclient.PrivateKey([]byte(secret), nil)
		if err != nil {
			return nil, errors.New("解析 SSH 私钥失败")
		}
		return method, nil
	}
	return ssh.Password(secret), nil
}

func buildLinuxDeployCommand(domain models.CertificateDomain, certPEM, privateKey string) string {
	certB64 := base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(certPEM) + "\n"))
	keyB64 := base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(privateKey) + "\n"))
	return fmt.Sprintf(`set -e
CERT_PATH=%s
KEY_PATH=%s
SUDO=""
if [ "$(id -u)" != "0" ]; then
  SUDO="sudo"
fi
$SUDO mkdir -p "$(dirname "$CERT_PATH")" "$(dirname "$KEY_PATH")"
base64 -d <<'__CERT_PEM__' | $SUDO tee "$CERT_PATH" >/dev/null
%s
__CERT_PEM__
base64 -d <<'__PRIVATE_KEY_PEM__' | $SUDO tee "$KEY_PATH" >/dev/null
%s
__PRIVATE_KEY_PEM__
$SUDO chmod 644 "$CERT_PATH"
$SUDO chmod 600 "$KEY_PATH"
%s`, shellutil.Quote(domain.DeployCertPath), shellutil.Quote(domain.DeployKeyPath), certB64, keyB64, domain.DeployReloadCommand)
}

func defaultSSHPort(port int) int {
	if port <= 0 {
		return 22
	}
	return port
}

func certificateDefaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func openExecutorSSHClient(executor models.CertificateExecutor) (*ssh.Client, error) {
	secret, err := decryptSecret(executor.SecretCipher)
	if err != nil {
		return nil, errors.New("执行器认证信息解密失败")
	}
	auth, err := certificateSSHAuthMethod(executor.AuthType, secret)
	if err != nil {
		return nil, err
	}
	return sshclient.Dial(sshclient.Config{Address: net.JoinHostPort(executor.Host, strconv.Itoa(defaultSSHPort(executor.Port))), User: executor.Username, Auth: []ssh.AuthMethod{auth}, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 10 * time.Second})
}
func runSSHCommandOutput(executor models.CertificateExecutor, command string) (string, int, error) {
	client, err := openExecutorSSHClient(executor)
	if err != nil {
		return "", 1, err
	}
	defer client.Close()
	result, err := sshclient.Run(context.Background(), client, command, nil)
	return strings.TrimSpace(result.Output), result.ExitCode, err
}
