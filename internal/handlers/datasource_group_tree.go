package handlers

import (
	"fmt"
	"gorm.io/gorm"
	"guanlan/internal/models"
)

const maxDatasourceGroupDepth = 5

type datasourceGroupInputError struct{ message string }

func (e *datasourceGroupInputError) Error() string { return e.message }
func groupInputError(message string) error         { return &datasourceGroupInputError{message: message} }

func loadDatasourceGroupTree(tx *gorm.DB) ([]models.DatasourceGroup, error) {
	var groups []models.DatasourceGroup
	err := tx.Order("id ASC").Find(&groups).Error
	return groups, err
}

// Include each selected node and its descendants once, even for overlapping roots.
func datasourceGroupDescendants(groups []models.DatasourceGroup, roots []uint) []uint {
	children := make(map[uint][]uint)
	for _, group := range groups {
		children[group.ParentID] = append(children[group.ParentID], group.ID)
	}
	seen := make(map[uint]bool)
	result := make([]uint, 0)
	var visit func(uint)
	visit = func(id uint) {
		if seen[id] {
			return
		}
		seen[id] = true
		result = append(result, id)
		for _, child := range children[id] {
			visit(child)
		}
	}
	for _, id := range roots {
		visit(id)
	}
	return result
}

func datasourceGroupPath(groups []models.DatasourceGroup, id uint) (string, int) {
	byID := make(map[uint]models.DatasourceGroup)
	for _, group := range groups {
		byID[group.ID] = group
	}
	path, depth := "", 0
	seen := make(map[uint]bool)
	for id != 0 && !seen[id] {
		seen[id] = true
		group, ok := byID[id]
		if !ok {
			break
		}
		if path == "" {
			path = group.Name
		} else {
			path = group.Name + " / " + path
		}
		depth++
		id = group.ParentID
	}
	return path, depth
}

func validateDatasourceGroupTree(tx *gorm.DB, id, parentID uint, name string) error {
	groups, err := loadDatasourceGroupTree(tx)
	if err != nil {
		return err
	}
	byID := make(map[uint]models.DatasourceGroup)
	for _, group := range groups {
		byID[group.ID] = group
		if group.ID != id && group.ParentID == parentID && group.Name == name {
			return groupInputError("同一父分组下已存在同名分组")
		}
	}
	if parentID != 0 {
		if _, ok := byID[parentID]; !ok {
			return groupInputError("父分组不存在")
		}
	}
	if id != 0 {
		for _, descendant := range datasourceGroupDescendants(groups, []uint{id}) {
			if descendant == parentID {
				return groupInputError("不能将分组移动到自身或其子分组下")
			}
		}
	}
	_, parentDepth := datasourceGroupPath(groups, parentID)
	height := 1
	if id != 0 {
		_, rootDepth := datasourceGroupPath(groups, id)
		for _, child := range datasourceGroupDescendants(groups, []uint{id}) {
			_, depth := datasourceGroupPath(groups, child)
			if depth-rootDepth+1 > height {
				height = depth - rootDepth + 1
			}
		}
	}
	if parentDepth+height > maxDatasourceGroupDepth {
		return groupInputError(fmt.Sprintf("实例分组最多支持 %d 级，请调整父分组", maxDatasourceGroupDepth))
	}
	return nil
}

func expandedDatasourceGroupIDs(tx *gorm.DB, roots []uint) ([]uint, error) {
	groups, err := loadDatasourceGroupTree(tx)
	if err != nil {
		return nil, err
	}
	return datasourceGroupDescendants(groups, roots), nil
}

func syncAllDatasourceGroupRuleBindings(tx *gorm.DB) error {
	var selections []models.AlertRuleDatasourceGroup
	if err := tx.Find(&selections).Error; err != nil {
		return err
	}
	seen := make(map[uint]bool)
	for _, selection := range selections {
		if seen[selection.RuleID] {
			continue
		}
		seen[selection.RuleID] = true
		ids, err := effectiveAlertRuleDatasourceIDs(tx, selection.RuleID)
		if err != nil {
			return err
		}
		if err := replaceAlertRuleBindings(tx, selection.RuleID, ids); err != nil {
			return err
		}
	}
	return nil
}

// Reconcile selections after status changes, including legacy direct selections.
func syncDatasourceStatusRuleBindings(tx *gorm.DB, datasourceIDs []uint) error {
	if err := syncAllDatasourceGroupRuleBindings(tx); err != nil {
		return err
	}
	var ruleIDs []uint
	if err := tx.Model(&models.AlertRuleDatasource{}).Distinct("rule_id").Where("datasource_id IN ?", datasourceIDs).Pluck("rule_id", &ruleIDs).Error; err != nil {
		return err
	}
	for _, ruleID := range ruleIDs {
		ids, err := effectiveAlertRuleDatasourceIDs(tx, ruleID)
		if err != nil {
			return err
		}
		if err := replaceAlertRuleBindings(tx, ruleID, ids); err != nil {
			return err
		}
	}
	return nil
}
