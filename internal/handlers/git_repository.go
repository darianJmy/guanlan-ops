package handlers

import (
	"errors"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"guanlan/pkg/textutil"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type saveGitRepositoryReq struct {
	Name          string `json:"name"`
	RepositoryURL string `json:"repository_url"`
	DefaultRef    string `json:"default_ref"`
	CredentialID  uint   `json:"credential_id"`
	Description   string `json:"description"`
	Status        int    `json:"status"`
}

var gitRepositorySyncLocks sync.Map

func ListGitRepositories(c *gin.Context) {
	var rows []models.GitRepository
	database.DB.Preload("Credential").Order("id DESC").Find(&rows)
	result := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		result = append(result, gitRepositoryResponse(row))
	}
	resp.Success(c, http.StatusOK, "成功", result)
}

func CreateGitRepository(c *gin.Context) {
	var req saveGitRepositoryReq
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "Git 仓库格式不正确")
		return
	}
	item, err := buildGitRepository(req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if err := database.DB.Create(&item).Error; err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "仓库名称已存在")
		return
	}
	database.DB.Preload("Credential").First(&item, item.ID)
	resp.Success(c, http.StatusOK, "Git 仓库已创建", gitRepositoryResponse(item))
}

func UpdateGitRepository(c *gin.Context) {
	var existing models.GitRepository
	if database.DB.First(&existing, c.Param("id")).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "Git 仓库不存在")
		return
	}
	var req saveGitRepositoryReq
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "Git 仓库格式不正确")
		return
	}
	item, err := buildGitRepository(req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	sourceChanged := existing.RepositoryURL != item.RepositoryURL || existing.DefaultRef != item.DefaultRef || existing.CredentialID != item.CredentialID
	if err := database.DB.Model(&existing).Updates(map[string]interface{}{"name": item.Name, "repository_url": item.RepositoryURL, "default_ref": item.DefaultRef, "credential_id": item.CredentialID, "description": item.Description, "status": item.Status}).Error; err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "仓库名称已存在")
		return
	}
	if sourceChanged {
		database.DB.Model(&existing).Updates(map[string]interface{}{"synced_revision": "", "synced_sha256": "", "synced_size": 0, "synced_at": nil, "last_sync_attempt_at": nil, "last_sync_error": ""})
		_ = removeRepositoryArtifact(existing.ID)
	}
	database.DB.Preload("Credential").First(&existing, existing.ID)
	resp.Success(c, http.StatusOK, "Git 仓库已更新", gitRepositoryResponse(existing))
}

func DeleteGitRepository(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var count int64
	database.DB.Model(&models.AutomationTask{}).Where("source_type = ? AND repository_id = ?", "repository", id).Count(&count)
	if count > 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "仓库已被自动化任务引用，请先解除绑定")
		return
	}
	result := database.DB.Delete(&models.GitRepository{}, id)
	if result.RowsAffected == 0 {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "Git 仓库不存在")
		return
	}
	_ = removeRepositoryArtifact(uint(id))
	resp.Success(c, http.StatusOK, "Git 仓库已删除")
}

func SyncGitRepository(c *gin.Context) {
	repositoryID, err := strconv.Atoi(c.Param("id"))
	if err != nil || repositoryID < 1 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "Git 仓库 ID 不正确")
		return
	}
	lockValue, _ := gitRepositorySyncLocks.LoadOrStore(uint(repositoryID), &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	var item models.GitRepository
	if database.DB.First(&item, repositoryID).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "Git 仓库不存在")
		return
	}
	attemptedAt := time.Now()
	artifact, err := buildRepositoryArtifact(item, item.DefaultRef)
	if err != nil {
		database.DB.Model(&item).Updates(map[string]interface{}{"last_sync_attempt_at": &attemptedAt, "last_sync_error": textutil.Ellipsis(err.Error(), 2000)})
		resp.Error(c, http.StatusOK, resp.CodeFailure, err.Error())
		return
	}
	if err := storeRepositoryArtifact(item.ID, artifact); err != nil {
		database.DB.Model(&item).Updates(map[string]interface{}{"last_sync_attempt_at": &attemptedAt, "last_sync_error": err.Error()})
		resp.Error(c, http.StatusOK, resp.CodeFailure, err.Error())
		return
	}
	syncedAt := time.Now()
	database.DB.Model(&item).Updates(map[string]interface{}{
		"synced_revision": artifact.Revision, "synced_sha256": artifact.SHA256, "synced_size": len(artifact.Data),
		"synced_at": &syncedAt, "last_sync_attempt_at": &attemptedAt, "last_sync_error": "",
	})
	resp.Success(c, http.StatusOK, "仓库同步成功", gin.H{"revision": artifact.Revision, "size": len(artifact.Data), "sha256": artifact.SHA256, "synced_at": syncedAt})
}

func buildGitRepository(req saveGitRepositoryReq) (models.GitRepository, error) {
	item := models.GitRepository{Name: strings.TrimSpace(req.Name), RepositoryURL: strings.TrimSpace(req.RepositoryURL), DefaultRef: strings.TrimSpace(req.DefaultRef), CredentialID: req.CredentialID, Description: strings.TrimSpace(req.Description), Status: req.Status}
	if item.Name == "" || len(item.Name) > 128 {
		return item, errors.New("请输入不超过 128 个字符的仓库名称")
	}
	if err := validateGitRepositoryURL(item.RepositoryURL); err != nil {
		return item, err
	}
	if item.DefaultRef != "" && (strings.HasPrefix(item.DefaultRef, "-") || strings.ContainsAny(item.DefaultRef, "\r\n\t ")) {
		return item, errors.New("默认分支或标签格式不正确")
	}
	if item.Status != 0 && item.Status != 1 {
		return item, errors.New("启用状态不正确")
	}
	var credential models.GitCredential
	if item.CredentialID == 0 || database.DB.First(&credential, item.CredentialID).Error != nil || credential.Status != 1 {
		return item, errors.New("请选择已启用的 Git 认证")
	}
	item.Credential = credential
	return item, nil
}

func gitRepositoryResponse(item models.GitRepository) gin.H {
	return gin.H{
		"id": item.ID, "name": item.Name, "repository_url": item.RepositoryURL, "default_ref": item.DefaultRef,
		"credential_id": item.CredentialID, "credential_name": item.Credential.Name, "description": item.Description, "status": item.Status,
		"synced_revision": item.SyncedRevision, "synced_sha256": item.SyncedSHA256, "synced_size": item.SyncedSize,
		"synced_at": item.SyncedAt, "last_sync_attempt_at": item.LastSyncAttemptAt, "last_sync_error": item.LastSyncError,
		"created_at": item.CreatedAt, "updated_at": item.UpdatedAt,
	}
}
