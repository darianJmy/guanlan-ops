package handlers

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"guanlan/pkg/sshclient"
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
