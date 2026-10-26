package handlers

import (
	"encoding/json"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func ListTunnels(c *gin.Context) {
	var tunnels []models.Tunnel
	database.DB.Order("id ASC").Find(&tunnels)
	resp.Success(c, http.StatusOK, "成功", tunnels)
}

type CreateTunnelReq struct {
	Name        string `json:"name" binding:"required"`
	Type        string `json:"type" binding:"required"`
	Host        string `json:"host" binding:"required"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	AuthType    string `json:"auth_type"`
	AuthConfig  string `json:"auth_config"`
	Region      string `json:"region"`
	Status      int    `json:"status"`
	Description string `json:"description"`
}

func CreateTunnel(c *gin.Context) {
	var req CreateTunnelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	if req.Port == 0 {
		req.Port = 22
	}
	if req.AuthType == "" {
		req.AuthType = "password"
	}

	t := models.Tunnel{
		Name:        req.Name,
		Type:        req.Type,
		Host:        req.Host,
		Port:        req.Port,
		Username:    req.Username,
		AuthType:    req.AuthType,
		AuthConfig:  req.AuthConfig,
		Region:      req.Region,
		Status:      req.Status,
		Description: req.Description,
	}
	database.DB.Create(&t)
	resp.Success(c, http.StatusOK, "创建成功")
}

func UpdateTunnel(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req CreateTunnelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}

	database.DB.Model(&models.Tunnel{}).Where("id = ?", id).Updates(map[string]interface{}{
		"name":        req.Name,
		"type":        req.Type,
		"host":        req.Host,
		"port":        req.Port,
		"username":    req.Username,
		"auth_type":   req.AuthType,
		"auth_config": req.AuthConfig,
		"region":      req.Region,
		"status":      req.Status,
		"description": req.Description,
	})
	resp.Success(c, http.StatusOK, "更新成功")
}

func DeleteTunnel(c *gin.Context) {
	id := c.Param("id")
	database.DB.Delete(&models.Tunnel{}, id)
	resp.Success(c, http.StatusOK, "删除成功")
}

func TestTunnelConnection(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var tunnel models.Tunnel
	if err := database.DB.First(&tunnel, id).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "隧道节点不存在")
		return
	}

	ok, msg := checkTunnelReachable(tunnel)
	if ok {
		resp.Success(c, http.StatusOK, msg)
	} else {
		resp.Error(c, http.StatusOK, resp.CodeFailure, msg)
	}
}

func checkTunnelReachable(t models.Tunnel) (bool, string) {
	addr := fmt.Sprintf("%s:%d", t.Host, t.Port)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return false, fmt.Sprintf("无法连接 %s: %s", addr, err.Error())
	}
	conn.Close()
	return true, fmt.Sprintf("连接成功: %s (%s:%d)", t.Name, t.Host, t.Port)
}

// GetTunnelOptions 供数据源表单下拉
func GetTunnelOptions(c *gin.Context) {
	var tunnels []models.Tunnel
	database.DB.Select("id, name, region").Where("status = 1").Order("id ASC").Find(&tunnels)
	type opt struct {
		ID     uint   `json:"id"`
		Name   string `json:"name"`
		Region string `json:"region"`
	}
	opts := make([]opt, 0, len(tunnels))
	for _, t := range tunnels {
		opts = append(opts, opt{ID: t.ID, Name: t.Name, Region: t.Region})
	}
	resp.Success(c, http.StatusOK, "成功", opts)
}

// ── Update Datasource handler to fill tunnel_name ──

type DatasourceListQuery struct {
	GroupID  uint   `form:"group_id"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Keyword  string `form:"keyword"`
	Category string `form:"category"`
	Type     string `form:"type"`
	AuthType string `form:"auth_type"`
	Status   string `form:"status"`
	Label    string `form:"label"`
	SortBy   string `form:"sort_by"`
	SortDesc string `form:"sort_desc"`
}

func ListDatasourcesWithTunnel(c *gin.Context) {
	var query DatasourceListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "筛选参数不正确")
		return
	}
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 {
		query.PageSize = 10
	}

	var total int64
	var datasources []models.Datasource
	tx := database.DB.Model(&models.Datasource{}).Where("type IN ?", supportedDatasourceTypes())
	if query.GroupID > 0 {
		groupIDs, err := expandedDatasourceGroupIDs(database.DB, []uint{query.GroupID})
		if err != nil {
			resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "查询实例分组失败")
			return
		}
		tx = tx.Where("id IN (?)", database.DB.Model(&models.DatasourceGroupMember{}).Select("datasource_id").Where("group_id IN ?", groupIDs))
	}
	if query.Category != "" {
		tx = tx.Where("category = ?", query.Category)
	}
	if query.Type != "" {
		tx = tx.Where("type = ?", query.Type)
	}
	if query.AuthType != "" {
		tx = tx.Where("auth_type = ?", query.AuthType)
	}
	if query.Status != "" {
		if status, err := strconv.Atoi(query.Status); err == nil {
			tx = tx.Where("status = ?", status)
		}
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		kw := "%" + keyword + "%"
		tx = tx.Where(
			"name LIKE ? OR category LIKE ? OR type LIKE ? OR url LIKE ? OR labels LIKE ? OR labels LIKE ? OR remark LIKE ?",
			kw, kw, kw, kw, kw, datasourceLabelSearchPattern(keyword), kw,
		)
	}
	if label := strings.TrimSpace(query.Label); label != "" {
		tx = tx.Where("labels LIKE ?", datasourceLabelSearchPattern(label))
	}
	tx.Count(&total)
	tx.Order(datasourceOrderClause(query.SortBy, query.SortDesc)).Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&datasources)

	// fill tunnel names
	tunnelMap := make(map[uint]string)
	var tunnels []models.Tunnel
	database.DB.Select("id, name").Find(&tunnels)
	for _, t := range tunnels {
		tunnelMap[t.ID] = t.Name
	}
	for i := range datasources {
		if datasources[i].TunnelID > 0 {
			datasources[i].TunnelName = tunnelMap[datasources[i].TunnelID]
		}
	}

	resp.Success(c, http.StatusOK, "成功", gin.H{
		"list":      datasources,
		"total":     total,
		"page":      query.Page,
		"page_size": query.PageSize,
	})
}

func datasourceLabelSearchPattern(value string) string {
	value = strings.TrimSpace(value)
	for _, separator := range []string{":", "="} {
		if index := strings.Index(value, separator); index > 0 {
			key := strings.TrimSpace(value[:index])
			labelValue := strings.TrimSpace(value[index+1:])
			if key != "" && labelValue != "" {
				encodedKey, _ := json.Marshal(key)
				encodedValue, _ := json.Marshal(labelValue)
				return "%" + string(encodedKey) + ":" + string(encodedValue) + "%"
			}
		}
	}
	return "%" + value + "%"
}

func datasourceOrderClause(sortBy, sortDesc string) string {
	columns := map[string]string{
		"id":         "id",
		"name":       "name",
		"category":   "category",
		"type":       "type",
		"auth_type":  "auth_type",
		"status":     "status",
		"created_at": "created_at",
		"updated_at": "updated_at",
	}
	column, ok := columns[sortBy]
	if !ok {
		column = "id"
	}
	direction := "ASC"
	if strings.EqualFold(sortDesc, "true") || strings.EqualFold(sortDesc, "desc") || sortDesc == "1" {
		direction = "DESC"
	}
	return column + " " + direction
}

// ── Override Create/Update Datasource to handle tunnel_id ──

type CreateDatasourceWithTunnelReq struct {
	GroupID      uint   `json:"group_id"`
	Name         string `json:"name" binding:"required"`
	Category     string `json:"category"`
	Type         string `json:"type" binding:"required"`
	URL          string `json:"url"`
	AuthType     string `json:"auth_type"`
	AuthConfig   string `json:"auth_config"`
	Config       string `json:"config"`
	Labels       string `json:"labels"`
	Region       string `json:"region"`
	Env          string `json:"env"`
	Domain       string `json:"domain"`
	Capabilities string `json:"capabilities"`
	Priority     int    `json:"priority"`
	TunnelID     uint   `json:"tunnel_id"`
	Status       int    `json:"status"`
	Remark       string `json:"remark"`
}

func CreateDatasourceWithTunnel(c *gin.Context) {
	req := CreateDatasourceWithTunnelReq{Status: 1}
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	buildAndCreateDatasource(req, c)
}

func UpdateDatasourceWithTunnel(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req CreateDatasourceWithTunnelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}
	var existing models.Datasource
	if id < 1 || database.DB.Where("type IN ?", supportedDatasourceTypes()).First(&existing, id).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "数据源实例不存在")
		return
	}
	if err := normalizeDatasourceReq(&req, &existing); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}

	labels, err := normalizeDatasourceLabels(req.Labels)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	req.Labels = labels

	updates := map[string]interface{}{
		"name":         req.Name,
		"category":     categoryOrDefault(req.Category, req.Type),
		"type":         req.Type,
		"url":          req.URL,
		"auth_type":    req.AuthType,
		"config":       req.Config,
		"labels":       req.Labels,
		"region":       req.Region,
		"env":          req.Env,
		"domain":       req.Domain,
		"capabilities": normalizeCapabilities(req.Capabilities, req.Type),
		"priority":     priorityOrDefault(req.Priority),
		"tunnel_id":    req.TunnelID,
		"status":       req.Status,
		"remark":       req.Remark,
		"auth_config":  req.AuthConfig,
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&existing).Updates(updates).Error; err != nil {
			return err
		}
		return syncDatasourceStatusRuleBindings(tx, []uint{existing.ID})
	}); err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "更新实例失败")
		return
	}
	resp.Success(c, http.StatusOK, "更新成功")
}

type BatchDatasourceIDsReq struct {
	IDs []uint `json:"ids"`
}

type BatchDatasourceStatusReq struct {
	IDs    []uint `json:"ids"`
	Status *int   `json:"status"`
}

func BatchUpdateDatasourceStatus(c *gin.Context) {
	var req BatchDatasourceStatusReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 || req.Status == nil || (*req.Status != 0 && *req.Status != 1) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请选择接入并指定正确状态")
		return
	}
	if !requireSupportedDatasourceIDs(c, req.IDs) {
		return
	}

	var updated int64
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.Datasource{}).Where("id IN ?", req.IDs).Update("status", *req.Status)
		if result.Error != nil {
			return result.Error
		}
		updated = result.RowsAffected
		return syncDatasourceStatusRuleBindings(tx, req.IDs)
	})
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "批量更新状态失败")
		return
	}
	resp.Success(c, http.StatusOK, "批量更新成功", gin.H{"updated": updated})
}

func BatchDeleteDatasources(c *gin.Context) {
	var req BatchDatasourceIDsReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请选择要删除的接入")
		return
	}
	if !requireSupportedDatasourceIDs(c, req.IDs) {
		return
	}

	deleted, ok := deleteDatasourceInstances(c, req.IDs)
	if !ok {
		return
	}
	resp.Success(c, http.StatusOK, "批量删除成功", gin.H{"deleted": deleted})
}

func buildAndCreateDatasource(req CreateDatasourceWithTunnelReq, c *gin.Context) {
	ds, err := prepareDatasourceForCreate(req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if req.GroupID > 0 {
		var group models.DatasourceGroup
		if err := database.DB.First(&group, req.GroupID).Error; err != nil {
			resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "实例分组不存在")
			return
		}
	}
	if err := database.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if req.GroupID > 0 {
			var group models.DatasourceGroup
			if err := tx.First(&group, req.GroupID).Error; err != nil {
				return err
			}
		}
		if err := persistNewDatasource(tx, &ds, req.Status); err != nil {
			return err
		}
		if req.GroupID == 0 {
			return nil
		}
		if err := tx.Create(&models.DatasourceGroupMember{GroupID: req.GroupID, DatasourceID: ds.ID}).Error; err != nil {
			return err
		}
		return syncDatasourceGroupRuleBindings(tx, req.GroupID)
	}); err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "创建实例失败")
		return
	}
	resp.Success(c, http.StatusOK, "创建成功")
}

func prepareDatasourceForCreate(req CreateDatasourceWithTunnelReq) (models.Datasource, error) {
	if err := normalizeDatasourceReq(&req, nil); err != nil {
		return models.Datasource{}, err
	}

	labels, err := normalizeDatasourceLabels(req.Labels)
	if err != nil {
		return models.Datasource{}, err
	}
	req.Labels = labels
	if req.AuthType == "" {
		req.AuthType = "none"
	}
	if req.Category == "" {
		req.Category = categoryByType(req.Type)
	}

	ds := models.Datasource{
		Name:         req.Name,
		Category:     req.Category,
		Type:         req.Type,
		URL:          req.URL,
		AuthType:     req.AuthType,
		AuthConfig:   req.AuthConfig,
		Config:       req.Config,
		Labels:       req.Labels,
		Region:       req.Region,
		Env:          req.Env,
		Domain:       req.Domain,
		Capabilities: normalizeCapabilities(req.Capabilities, req.Type),
		Priority:     priorityOrDefault(req.Priority),
		TunnelID:     req.TunnelID,
		Status:       req.Status,
		Remark:       req.Remark,
	}
	return ds, nil
}

func persistNewDatasource(tx *gorm.DB, ds *models.Datasource, status int) error {
	if err := tx.Create(ds).Error; err != nil {
		return err
	}
	return tx.Model(ds).Update("status", status).Error
}

func normalizeDatasourceLabels(raw string) (string, error) {
	labels, err := normalizeInspectionLabels(raw)
	if err != nil {
		return "", fmt.Errorf("%s", strings.Replace(err.Error(), "任务标签", "接入标签", 1))
	}
	return labels, nil
}

// DatasourceAuthConfig exposes auth fields for test connection
type DatasourceAuthConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Token    string `json:"token"`
}

func GetDatasourceAuth(c *gin.Context) {
	id := c.Param("id")
	var ds models.Datasource
	if err := database.DB.Where("type IN ?", supportedDatasourceTypes()).First(&ds, id).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "数据源不存在")
		return
	}
	var auth DatasourceAuthConfig
	if ds.AuthConfig != "" {
		json.Unmarshal([]byte(ds.AuthConfig), &auth)
	}
	resp.Success(c, http.StatusOK, "成功", gin.H{
		"id":          ds.ID,
		"name":        ds.Name,
		"category":    ds.Category,
		"type":        ds.Type,
		"url":         ds.URL,
		"auth_type":   ds.AuthType,
		"auth_config": auth,
		"config":      ds.Config,
		"tunnel_id":   ds.TunnelID,
		"status":      ds.Status,
		"remark":      ds.Remark,
	})
}

func categoryOrDefault(category, dsType string) string {
	if category != "" {
		return category
	}
	return categoryByType(dsType)
}
