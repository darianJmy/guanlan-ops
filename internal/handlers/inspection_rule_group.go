package handlers

import (
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type saveInspectionRuleGroupReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func ListInspectionRuleGroups(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	query := database.DB.Model(&models.InspectionRuleGroup{})
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("name LIKE ?", like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "查询规则组失败")
		return
	}
	var groups []models.InspectionRuleGroup
	if err := query.Order("updated_at DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&groups).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "查询规则组失败")
		return
	}
	list := make([]gin.H, 0, len(groups))
	for _, group := range groups {
		list = append(list, inspectionRuleGroupResponse(group))
	}
	resp.Success(c, http.StatusOK, "成功", gin.H{
		"list": list, "total": total, "page": page, "page_size": pageSize,
	})
}

func GetInspectionRuleGroup(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var group models.InspectionRuleGroup
	if id < 1 || database.DB.First(&group, id).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "规则组不存在")
		return
	}
	resp.Success(c, http.StatusOK, "成功", inspectionRuleGroupResponse(group))
}

func CreateInspectionRuleGroup(c *gin.Context) {
	var req saveInspectionRuleGroupReq
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "规则组格式不正确")
		return
	}
	name, description, err := normalizeInspectionRuleGroupInput(req.Name, req.Description)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if inspectionRuleGroupNameExists(name, 0) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "规则组名称已存在")
		return
	}
	group := models.InspectionRuleGroup{Name: name, Description: description}
	if database.DB.Create(&group).Error != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "创建规则组失败")
		return
	}
	resp.Success(c, http.StatusOK, "规则组已创建", inspectionRuleGroupResponse(group))
}

func UpdateInspectionRuleGroup(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var group models.InspectionRuleGroup
	if id < 1 || database.DB.First(&group, id).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "规则组不存在")
		return
	}
	var req saveInspectionRuleGroupReq
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "规则组格式不正确")
		return
	}
	name, description, err := normalizeInspectionRuleGroupInput(req.Name, req.Description)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if inspectionRuleGroupNameExists(name, group.ID) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "规则组名称已存在")
		return
	}
	if err := database.DB.Model(&group).Updates(map[string]interface{}{"name": name, "description": description}).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "更新规则组失败")
		return
	}
	database.DB.First(&group, group.ID)
	resp.Success(c, http.StatusOK, "规则组已更新", inspectionRuleGroupResponse(group))
}

func DeleteInspectionRuleGroup(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var group models.InspectionRuleGroup
	if id < 1 || database.DB.First(&group, id).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "规则组不存在")
		return
	}
	var ruleCount int64
	if err := database.DB.Model(&models.InspectionItem{}).Where("group_id = ?", group.ID).Count(&ruleCount).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "检查组内任务失败")
		return
	}
	if ruleCount > 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请先删除规则组内的拨测任务")
		return
	}
	if database.DB.Delete(&group).Error != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "删除规则组失败")
		return
	}
	resp.Success(c, http.StatusOK, "规则组已删除")
}

func normalizeInspectionRuleGroupInput(rawName, rawDescription string) (string, string, error) {
	name, description := strings.TrimSpace(rawName), strings.TrimSpace(rawDescription)
	if name == "" || len([]rune(name)) > 128 {
		return "", "", fmt.Errorf("规则组名称不能为空且不能超过 128 个字符")
	}
	if len([]rune(description)) > 500 {
		return "", "", fmt.Errorf("规则组描述不能超过 500 个字符")
	}
	return name, description, nil
}

func inspectionRuleGroupNameExists(name string, excludeID uint) bool {
	query := database.DB.Model(&models.InspectionRuleGroup{}).Where("name = ?", name)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	query.Count(&count)
	return count > 0
}

func inspectionRuleGroupResponse(group models.InspectionRuleGroup) gin.H {
	var ruleCount int64
	database.DB.Model(&models.InspectionItem{}).Where("group_id = ?", group.ID).Count(&ruleCount)
	return gin.H{"id": group.ID, "name": group.Name, "description": group.Description, "task_count": ruleCount, "created_at": group.CreatedAt, "updated_at": group.UpdatedAt}
}

func validateInspectionRuleGroup(id uint) error {
	var count int64
	if err := database.DB.Model(&models.InspectionRuleGroup{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return fmt.Errorf("查询拨测规则组失败")
	}
	if count == 0 {
		return fmt.Errorf("拨测规则组不存在")
	}
	return nil
}

func inspectionGroupQuery(c *gin.Context, query *gorm.DB) (*gorm.DB, bool) {
	raw, ok := c.GetQuery("group_id")
	if !ok {
		return query, true
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "规则组参数不正确")
		return query, false
	}
	return query.Where("group_id = ?", id), true
}
