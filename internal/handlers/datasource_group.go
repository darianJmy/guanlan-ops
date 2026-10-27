package handlers

import (
	"errors"
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

type saveDatasourceGroupReq struct {
	ParentID      *uint  `json:"parent_id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	DatasourceIDs []uint `json:"datasource_ids"`
}

func ListDatasourceGroups(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	query := database.DB.Model(&models.DatasourceGroup{})
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("name LIKE ? OR description LIKE ?", like, like)
	}
	var groups []models.DatasourceGroup
	if err := query.Order("created_at ASC, id ASC").Find(&groups).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "查询数据源分组失败")
		return
	}
	list := make([]gin.H, 0, len(groups))
	for _, group := range groups {
		list = append(list, datasourceGroupResponse(group))
	}
	resp.Success(c, http.StatusOK, "成功", list)
}

func CreateDatasourceGroup(c *gin.Context) {
	var req saveDatasourceGroupReq
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "数据源分组格式不正确")
		return
	}
	name, description, datasourceIDs, err := normalizeDatasourceGroupInput(req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	parentID := uint(0)
	if req.ParentID != nil {
		parentID = *req.ParentID
	}
	group := models.DatasourceGroup{Name: name, Description: description, ParentID: parentID}
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := validateDatasourceGroupTree(tx, 0, parentID, name); err != nil {
			return err
		}
		if err := tx.Create(&group).Error; err != nil {
			return err
		}
		if err := replaceDatasourceGroupMembers(tx, group.ID, datasourceIDs); err != nil {
			return err
		}
		return syncDatasourceGroupRuleBindings(tx, group.ID)
	}); err != nil {
		respondDatasourceGroupError(c, err, "创建数据源分组失败")
		return
	}
	resp.Success(c, http.StatusOK, "数据源分组已创建", datasourceGroupResponse(group))
}

func UpdateDatasourceGroup(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var group models.DatasourceGroup
	if id < 1 || database.DB.First(&group, id).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "数据源分组不存在")
		return
	}
	var req saveDatasourceGroupReq
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "数据源分组格式不正确")
		return
	}
	name, description, datasourceIDs, err := normalizeDatasourceGroupInput(req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	parentID := group.ParentID
	if req.ParentID != nil {
		parentID = *req.ParentID
	}
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := validateDatasourceGroupTree(tx, group.ID, parentID, name); err != nil {
			return err
		}
		if err := tx.Model(&group).Updates(map[string]interface{}{"name": name, "description": description, "parent_id": parentID}).Error; err != nil {
			return err
		}
		if err := replaceDatasourceGroupMembers(tx, group.ID, datasourceIDs); err != nil {
			return err
		}
		return syncAllDatasourceGroupRuleBindings(tx)
	}); err != nil {
		respondDatasourceGroupError(c, err, "更新数据源分组失败")
		return
	}
	database.DB.First(&group, group.ID)
	resp.Success(c, http.StatusOK, "数据源分组已更新", datasourceGroupResponse(group))
}

func DeleteDatasourceGroup(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var group models.DatasourceGroup
	if id < 1 || database.DB.First(&group, id).Error != nil {
		resp.Error(c, http.StatusNotFound, resp.CodeNotFound, "数据源分组不存在")
		return
	}
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		var memberCount int64
		if err := tx.Model(&models.DatasourceGroupMember{}).Where("group_id = ?", group.ID).Count(&memberCount).Error; err != nil {
			return err
		}
		if memberCount > 0 {
			return groupInputError("分组内存在数据源，不能删除")
		}
		var usageCount int64
		if err := tx.Model(&models.AlertRuleDatasourceGroup{}).Where("datasource_group_id = ?", group.ID).Count(&usageCount).Error; err != nil {
			return err
		}
		if usageCount > 0 {
			return groupInputError("该数据源分组仍被告警规则使用，请先解除绑定")
		}
		var childCount int64
		if err := tx.Model(&models.DatasourceGroup{}).Where("parent_id = ?", group.ID).Count(&childCount).Error; err != nil {
			return err
		}
		if childCount > 0 {
			return groupInputError("请先移动或删除子分组")
		}
		if err := tx.Delete(&group).Error; err != nil {
			return err
		}
		return syncAllDatasourceGroupRuleBindings(tx)
	}); err != nil {
		respondDatasourceGroupError(c, err, "删除数据源分组失败")
		return
	}
	resp.Success(c, http.StatusOK, "数据源分组已删除")
}

func normalizeDatasourceGroupInput(req saveDatasourceGroupReq) (string, string, []uint, error) {
	name, description := strings.TrimSpace(req.Name), strings.TrimSpace(req.Description)
	if name == "" || len([]rune(name)) > 128 {
		return "", "", nil, fmt.Errorf("数据源分组名称不能为空且不能超过 128 个字符")
	}
	if len([]rune(description)) > 500 {
		return "", "", nil, fmt.Errorf("数据源分组描述不能超过 500 个字符")
	}
	datasourceIDs := normalizeChannelIDs(req.DatasourceIDs)
	var count int64
	database.DB.Model(&models.Datasource{}).Where("id IN ? AND type IN ?", datasourceIDs, supportedDatasourceTypes()).Count(&count)
	if count != int64(len(datasourceIDs)) {
		return "", "", nil, fmt.Errorf("数据源不存在或类型不受支持")
	}
	return name, description, datasourceIDs, nil
}

func respondDatasourceGroupError(c *gin.Context, err error, message string) {
	var inputErr *datasourceGroupInputError
	if errors.As(err, &inputErr) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, inputErr.Error())
		return
	}
	resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, message)
}

func replaceDatasourceGroupMembers(tx *gorm.DB, groupID uint, datasourceIDs []uint) error {
	if err := tx.Where("group_id = ?", groupID).Delete(&models.DatasourceGroupMember{}).Error; err != nil {
		return err
	}
	for _, datasourceID := range datasourceIDs {
		if err := tx.Create(&models.DatasourceGroupMember{GroupID: groupID, DatasourceID: datasourceID}).Error; err != nil {
			return err
		}
	}
	return nil
}

func datasourceGroupMemberIDs(tx *gorm.DB, groupID uint) ([]uint, error) {
	var members []models.DatasourceGroupMember
	if err := tx.Where("group_id = ?", groupID).Order("id ASC").Find(&members).Error; err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.DatasourceID)
	}
	return ids, nil
}

func syncDatasourceGroupRuleBindings(tx *gorm.DB, datasourceGroupID uint) error {
	groups, err := loadDatasourceGroupTree(tx)
	if err != nil {
		return err
	}
	byID := make(map[uint]models.DatasourceGroup)
	for _, group := range groups {
		byID[group.ID] = group
	}
	ids := []uint{}
	seen := make(map[uint]bool)
	for id := datasourceGroupID; id != 0 && !seen[id]; id = byID[id].ParentID {
		seen[id] = true
		ids = append(ids, id)
	}
	var selections []models.AlertRuleDatasourceGroup
	if err := tx.Where("datasource_group_id IN ?", ids).Find(&selections).Error; err != nil {
		return err
	}
	rules := make(map[uint]bool)
	for _, selection := range selections {
		if rules[selection.RuleID] {
			continue
		}
		rules[selection.RuleID] = true
		datasourceIDs, err := effectiveAlertRuleDatasourceIDs(tx, selection.RuleID)
		if err != nil {
			return err
		}
		if err := replaceAlertRuleBindings(tx, selection.RuleID, datasourceIDs); err != nil {
			return err
		}
	}
	return nil
}

func datasourceGroupResponse(group models.DatasourceGroup) gin.H {
	var members []models.DatasourceGroupMember
	database.DB.Where("group_id = ?", group.ID).Order("id ASC").Find(&members)
	datasourceIDs := make([]uint, 0, len(members))
	datasourceNames := make([]string, 0, len(members))
	for _, member := range members {
		var datasource models.Datasource
		if database.DB.Where("type IN ?", supportedDatasourceTypes()).Select("id", "name").First(&datasource, member.DatasourceID).Error != nil {
			continue
		}
		datasourceIDs = append(datasourceIDs, member.DatasourceID)
		datasourceNames = append(datasourceNames, datasource.Name)
	}
	var usageCount, ruleCount int64
	database.DB.Table("alert_rules").Joins("JOIN alert_rule_datasource_groups ON alert_rule_datasource_groups.rule_id = alert_rules.id").Where("alert_rule_datasource_groups.datasource_group_id = ?", group.ID).Distinct("alert_rules.group_id").Count(&usageCount)
	database.DB.Model(&models.AlertRuleDatasourceGroup{}).Where("datasource_group_id = ?", group.ID).Count(&ruleCount)
	groups, _ := loadDatasourceGroupTree(database.DB)
	path, depth := datasourceGroupPath(groups, group.ID)
	descendantIDs := datasourceGroupDescendants(groups, []uint{group.ID})
	var effectiveIDs []uint
	database.DB.Model(&models.DatasourceGroupMember{}).Distinct("datasource_id").Where("group_id IN ?", descendantIDs).Where("datasource_id IN (?)", database.DB.Model(&models.Datasource{}).Select("id").Where("type IN ?", supportedDatasourceTypes())).Order("datasource_id").Pluck("datasource_id", &effectiveIDs)
	if effectiveIDs == nil {
		effectiveIDs = []uint{}
	}
	return gin.H{"parent_id": group.ParentID, "path": path, "depth": depth, "effective_datasource_ids": effectiveIDs, "scope_member_count": len(effectiveIDs), "id": group.ID, "name": group.Name, "description": group.Description, "datasource_ids": datasourceIDs, "datasource_names": datasourceNames, "member_count": len(datasourceIDs), "usage_count": usageCount, "rule_count": ruleCount, "created_at": group.CreatedAt, "updated_at": group.UpdatedAt}
}
