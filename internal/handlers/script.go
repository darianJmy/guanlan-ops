package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"guanlan/pkg/shellutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ── Script CRUD ──

func ListScripts(c *gin.Context) {
	var scripts []models.Script
	query := database.DB.Order("id ASC")
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		pattern := "%" + keyword + "%"
		query = query.Where("name LIKE ? OR description LIKE ?", pattern, pattern)
	}
	query.Find(&scripts)
	resp.Success(c, http.StatusOK, "成功", scripts)
}

func GetScript(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "脚本 ID 不正确")
		return
	}
	var script models.Script
	if err := database.DB.First(&script, id).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "脚本不存在")
		return
	}
	resp.Success(c, http.StatusOK, "成功", script)
}

type CreateScriptReq struct {
	Name          string `json:"name" binding:"required"`
	Type          string `json:"type" binding:"required"`
	Content       string `json:"content"`
	SourceType    string `json:"source_type"`
	RepositoryURL string `json:"repository_url"`
	GitRef        string `json:"git_ref"`
	Entrypoint    string `json:"entrypoint"`
	Description   string `json:"description"`
	Status        int    `json:"status"`
}

func CreateScript(c *gin.Context) {
	var req CreateScriptReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	if req.Status == 0 {
		req.Status = 1
	}

	s, err := buildScript(req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if scriptNameExists(s.Name, 0) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "脚本名称已存在")
		return
	}
	if err := database.DB.Create(&s).Error; err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "脚本名称已存在")
		return
	}
	resp.Success(c, http.StatusOK, "创建成功")
}

func UpdateScript(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "脚本 ID 不正确")
		return
	}
	var existing models.Script
	if err := database.DB.First(&existing, id).Error; err != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "脚本不存在")
		return
	}
	var req CreateScriptReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	script, err := buildScript(req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	script.Status = req.Status
	if scriptNameExists(script.Name, uint(id)) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "脚本名称已存在")
		return
	}

	if err := database.DB.Model(&existing).Updates(map[string]interface{}{
		"name":           script.Name,
		"type":           script.Type,
		"content":        script.Content,
		"source_type":    script.SourceType,
		"repository_url": script.RepositoryURL,
		"git_ref":        script.GitRef,
		"entrypoint":     script.Entrypoint,
		"description":    script.Description,
		"status":         script.Status,
	}).Error; err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "脚本名称已存在")
		return
	}
	resp.Success(c, http.StatusOK, "更新成功")
}

func scriptNameExists(name string, excludeID uint) bool {
	query := database.DB.Model(&models.Script{}).Where("name = ?", name)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	return query.Count(&count).Error == nil && count > 0
}

func buildScript(req CreateScriptReq) (models.Script, error) {
	scriptType := strings.ToLower(strings.TrimSpace(req.Type))
	if scriptType != "shell" && scriptType != "python" {
		return models.Script{}, errors.New("脚本运行方式只支持 Shell 或 Python")
	}
	status := req.Status
	if status == 0 {
		status = 1
	}
	script := models.Script{
		Name: strings.TrimSpace(req.Name), Type: scriptType,
		Content: strings.TrimSpace(req.Content), SourceType: strings.ToLower(strings.TrimSpace(req.SourceType)),
		RepositoryURL: strings.TrimSpace(req.RepositoryURL), GitRef: strings.TrimSpace(req.GitRef), Entrypoint: strings.TrimSpace(req.Entrypoint),
		Description: strings.TrimSpace(req.Description), Status: status,
	}
	if script.SourceType == "" {
		script.SourceType = "inline"
	}
	if script.Name == "" {
		return models.Script{}, errors.New("请填写脚本名称")
	}
	if script.SourceType == "inline" && script.Content == "" {
		return models.Script{}, errors.New("请填写脚本内容")
	}
	if script.SourceType == "git" {
		if err := validateGitScript(script); err != nil {
			return models.Script{}, err
		}
		script.Content = ""
	} else if script.SourceType != "inline" {
		return models.Script{}, errors.New("脚本来源不正确")
	}
	return script, nil
}

func validateGitScript(script models.Script) error {
	if err := validateGitRepositoryURL(script.RepositoryURL); err != nil {
		return err
	}
	if script.Entrypoint == "" {
		return errors.New("请填写入口文件")
	}
	cleaned := path.Clean(strings.ReplaceAll(script.Entrypoint, "\\", "/"))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return errors.New("入口文件必须是仓库内的相对路径")
	}
	if script.GitRef != "" && (strings.HasPrefix(script.GitRef, "-") || strings.ContainsAny(script.GitRef, "\r\n\t ")) {
		return errors.New("Git 分支或标签格式不正确")
	}
	return nil
}

func validateGitRepositoryURL(repositoryURL string) error {
	if repositoryURL == "" {
		return errors.New("请填写 Git 仓库地址")
	}
	if strings.ContainsAny(repositoryURL, "\r\n\t ") {
		return errors.New("Git 仓库地址不能包含空白字符")
	}
	validRepository := strings.HasPrefix(repositoryURL, "git@") && strings.Contains(repositoryURL, ":")
	if parsed, err := url.Parse(repositoryURL); err == nil && (parsed.Scheme == "https" || parsed.Scheme == "ssh") && parsed.Host != "" {
		if parsed.User == nil || func() bool { _, hasPassword := parsed.User.Password(); return !hasPassword }() {
			validRepository = true
		}
	}
	if !validRepository {
		return errors.New("Git 仓库仅支持 HTTPS、SSH 或 git@host:path 地址，且不能在 URL 中携带密码")
	}
	return nil
}

func DeleteScript(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "脚本 ID 不正确")
		return
	}
	var taskCount int64
	database.DB.Model(&models.AutomationTask{}).Where("source_type = ? AND script_id = ?", "script", id).Count(&taskCount)
	if taskCount > 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "脚本已被自动化任务引用，请先解除绑定")
		return
	}
	var probes []models.InspectionItem
	if err := database.DB.Where("type = ?", "script").Find(&probes).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "检查脚本引用失败")
		return
	}
	for _, probe := range probes {
		var config ProbeReq
		if json.Unmarshal([]byte(probe.Config), &config) == nil && uint64(config.ScriptID) == id {
			resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "脚本已被拨测任务引用，请先解除绑定")
			return
		}
	}
	database.DB.Where("script_id = ?", id).Delete(&models.ScriptExecution{})
	database.DB.Delete(&models.Script{}, id)
	resp.Success(c, http.StatusOK, "删除成功")
}

func runScript(script models.Script) (status, output string, durationMs int64) {
	return runScriptWithTimeout(script, 30*time.Second)
}

func runScriptWithTimeout(script models.Script, timeout time.Duration) (status, output string, durationMs int64) {
	return runScriptWithEnvironment(script, timeout, nil)
}

func runScriptWithEnvironment(script models.Script, timeout time.Duration, environment map[string]string) (status, output string, durationMs int64) {
	start := time.Now()
	defer func() { durationMs = time.Since(start).Milliseconds() }()
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var cmd *exec.Cmd
	if script.SourceType == "git" {
		cmd = exec.CommandContext(ctx, "bash", "-c", buildGitScriptCommand(script))
	} else if script.Type == "shell" {
		cmd = exec.CommandContext(ctx, "sh", "-c", script.Content)
	} else if script.Type == "python" {
		cmd = exec.CommandContext(ctx, "python3", "-c", script.Content)
	} else {
		return "error", "不支持的脚本类型: " + script.Type, 0
	}
	cmd.Env = os.Environ()
	keys := make([]string, 0, len(environment))
	for key := range environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		cmd.Env = append(cmd.Env, key+"="+environment[key])
	}

	out, err := cmd.CombinedOutput()
	output = strings.TrimSpace(string(out))

	if ctx.Err() == context.DeadlineExceeded {
		return "fail", output + fmt.Sprintf("\n[超时: %s]", timeout), 0
	}
	if err != nil {
		return "fail", output + "\n" + err.Error(), 0
	}
	return "success", output, 0
}

func buildGitScriptCommand(script models.Script) string {
	runner := "bash"
	if script.Type == "python" {
		runner = "python3"
	}
	cloneArgs := "--depth 1"
	if script.GitRef != "" {
		cloneArgs += " --branch " + shellutil.Quote(script.GitRef) + " --single-branch"
	}
	return strings.Join([]string{
		"set -e", "set -o pipefail", "AIOPS_REPO_DIR=\"$(mktemp -d /tmp/aiops-repo-XXXXXX)\"",
		"trap 'rm -rf \"$AIOPS_REPO_DIR\"' EXIT",
		"git clone " + cloneArgs + " -- " + shellutil.Quote(script.RepositoryURL) + " \"$AIOPS_REPO_DIR/repository\"",
		"cd \"$AIOPS_REPO_DIR/repository\"", "test -f " + shellutil.Quote(script.Entrypoint), runner + " " + shellutil.Quote(script.Entrypoint),
	}, "\n")
}

// ── Script Executions ──

func ListScriptExecutions(c *gin.Context) {
	scriptID := c.Query("script_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}

	var total int64
	var records []models.ScriptExecution
	tx := database.DB.Model(&models.ScriptExecution{})
	if scriptID != "" {
		tx = tx.Where("script_id = ?", scriptID)
	}
	tx.Count(&total)
	tx.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&records)

	resp.Success(c, http.StatusOK, "成功", gin.H{"list": records, "total": total, "page": page, "page_size": pageSize})
}
