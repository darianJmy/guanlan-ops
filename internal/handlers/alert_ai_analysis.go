package handlers

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func requireAlertAI(c *gin.Context, edit bool) (uint, bool) {
	permission := "alert:ai-analysis:list"
	if edit {
		permission = "alert:ai-analysis:edit"
	}
	for _, p := range []string{"alert:event:list", permission} {
		if authorizeAIPermission(c.Request.Context(), currentUserID(c), p) != nil {
			resp.Error(c, http.StatusForbidden, resp.CodeForbidden, "没有告警 AI 分析权限")
			return 0, false
		}
	}
	return requireFaultCenter(c)
}
func hydrateAIRule(r *models.AlertAIAnalysisRule) {
	r.Matchers = decodeAlertLabelMatchers(r.MatchersJSON)
	r.AllowedDatasourceIDs = []uint{}
	_ = json.Unmarshal([]byte(r.AllowedDatasourceIDsJSON), &r.AllowedDatasourceIDs)
}
func ListAlertAIRules(c *gin.Context) {
	id, ok := requireAlertAI(c, false)
	if !ok {
		return
	}
	rows := []models.AlertAIAnalysisRule{}
	if database.DB.Where("fault_center_id = ?", id).Order("priority ASC, id ASC").Find(&rows).Error != nil {
		resp.Error(c, 500, resp.CodeInternalServerError, "读取分析规则失败")
		return
	}
	for i := range rows {
		hydrateAIRule(&rows[i])
		database.DB.Model(&models.AIModel{}).Where("id = ?", rows[i].ModelID).Pluck("name", &rows[i].ModelName)
		var latest models.AlertAIAnalysisJob
		if database.DB.Select("created_at").Where("rule_id = ?", rows[i].ID).Order("id DESC").Take(&latest).Error == nil {
			rows[i].LatestAnalysisAt = &latest.CreatedAt
		}
	}
	resp.Success(c, 200, "成功", rows)
}
func SaveAlertAIRule(c *gin.Context) {
	id, ok := requireAlertAI(c, true)
	if !ok {
		return
	}
	var req struct {
		Name                 string                     `json:"name"`
		Matchers             []models.AlertLabelMatcher `json:"matchers"`
		ModelID              uint                       `json:"model_id"`
		Priority             int                        `json:"priority"`
		Status               int                        `json:"status"`
		AllowedDatasourceIDs []uint                     `json:"allowed_datasource_ids"`
	}
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, 400, resp.CodeBadRequest, "规则格式不正确")
		return
	}
	matchers, err := normalizeAlertLabelMatchers(req.Matchers)
	if err != nil || len(matchers) == 0 || strings.TrimSpace(req.Name) == "" || len(req.Name) > 128 || req.Priority < 0 || req.Priority > 10000 || (req.Status != 0 && req.Status != 1) || req.ModelID == 0 || len(req.AllowedDatasourceIDs) > 20 {
		resp.Error(c, 400, resp.CodeBadRequest, "请检查名称、匹配标签、模型和优先级")
		return
	}
	if authorizeAITool(c.Request.Context(), currentUserID(c)) != nil {
		resp.Error(c, 403, resp.CodeForbidden, "配置分析需要数据源查询权限")
		return
	}
	if _, err := resolveAIModel(c.Request.Context(), req.ModelID); err != nil {
		resp.Error(c, 400, resp.CodeBadRequest, "请选择已启用的模型")
		return
	}
	seen := map[uint]bool{}
	ids := []uint{}
	for _, dsID := range req.AllowedDatasourceIDs {
		var ds models.Datasource
		if dsID == 0 || database.DB.Where("id = ? AND status = 1", dsID).First(&ds).Error != nil || !isMetricDatasourceType(ds.Type) {
			resp.Error(c, 400, resp.CodeBadRequest, "请选择已启用的指标数据源")
			return
		}
		if !seen[dsID] {
			seen[dsID] = true
			ids = append(ids, dsID)
		}
	}
	r := models.AlertAIAnalysisRule{FaultCenterID: id, ExecutionUserID: currentUserID(c), Version: 1}
	if c.Param("ruleId") != "" {
		n, e := strconv.ParseUint(c.Param("ruleId"), 10, 64)
		if e != nil || n == 0 || database.DB.Where("id = ? AND fault_center_id = ?", n, id).First(&r).Error != nil {
			resp.Error(c, 404, resp.CodeNotFound, "规则不存在")
			return
		}
		r.Version++
	}
	r.Name = strings.TrimSpace(req.Name)
	r.MatchersJSON = encodeJSON(matchers)
	r.ModelID = req.ModelID
	r.Priority = req.Priority
	r.Status = req.Status
	r.AllowedDatasourceIDsJSON = encodeJSON(ids)
	if database.DB.Save(&r).Error != nil {
		resp.Error(c, 400, resp.CodeBadRequest, "保存失败，请检查规则名称是否重复")
		return
	}
	if r.Status == 0 {
		database.DB.Model(&models.AlertAIAnalysisJob{}).Where("rule_id = ? AND status = 'queued'", r.ID).Updates(map[string]interface{}{"status": "cancelled", "finished_at": time.Now(), "error_message": "分析规则已停用"})
	}
	hydrateAIRule(&r)
	resp.Success(c, 200, "分析规则已保存", r)
}
func DeleteAlertAIRule(c *gin.Context) {
	id, ok := requireAlertAI(c, true)
	if !ok {
		return
	}
	n, err := strconv.ParseUint(c.Param("ruleId"), 10, 64)
	if err != nil || n == 0 {
		resp.Error(c, 400, resp.CodeBadRequest, "规则 ID 无效")
		return
	}
	result := database.DB.Where("id = ? AND fault_center_id = ?", n, id).Delete(&models.AlertAIAnalysisRule{})
	if result.Error != nil {
		resp.Error(c, 500, resp.CodeInternalServerError, "删除失败")
		return
	}
	if result.RowsAffected == 0 {
		resp.Error(c, 404, resp.CodeNotFound, "规则不存在")
		return
	}
	database.DB.Model(&models.AlertAIAnalysisJob{}).Where("rule_id = ? AND status = 'queued'", n).Updates(map[string]interface{}{"status": "cancelled", "finished_at": time.Now(), "error_message": "分析规则已删除"})
	resp.Success(c, 200, "分析规则已删除")
}
func AlertAIOptions(c *gin.Context) {
	if _, ok := requireAlertAI(c, true); !ok {
		return
	}
	if authorizeAITool(c.Request.Context(), currentUserID(c)) != nil {
		resp.Error(c, 403, resp.CodeForbidden, "没有数据源查询权限")
		return
	}
	modelsList := []struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
	}{}
	sources := []struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	}{}
	if database.DB.Model(&models.AIModel{}).Select("id,name").Where("status = 1").Order("id ASC").Scan(&modelsList).Error != nil || database.DB.Model(&models.Datasource{}).Select("id,name,type").Where("status = 1 AND type IN ?", []string{"prometheus", "victoriametrics"}).Order("id ASC").Scan(&sources).Error != nil {
		resp.Error(c, 500, resp.CodeInternalServerError, "读取选项失败")
		return
	}
	resp.Success(c, 200, "成功", gin.H{"models": modelsList, "datasources": sources})
}
func GetAlertAIAnalysis(c *gin.Context) {
	id, ok := requireAlertAI(c, false)
	if !ok {
		return
	}
	alertID, e := strconv.ParseUint(c.Param("alertId"), 10, 64)
	var alert models.AlertInstance
	if e != nil || alertID == 0 || database.DB.Where("id = ? AND fault_center_id = ?", alertID, id).First(&alert).Error != nil {
		resp.Error(c, 404, resp.CodeNotFound, "告警不存在")
		return
	}
	// Evidence is only available to readers with access to the underlying module.
	if authorizeAITool(c.Request.Context(), currentUserID(c)) != nil {
		resp.Error(c, 403, resp.CodeForbidden, "没有分析证据的数据源查询权限")
		return
	}
	rows := []models.AlertAIAnalysisJob{}
	if database.DB.Where("alert_id = ? AND fault_center_id = ?", alertID, id).Order("id DESC").Limit(10).Find(&rows).Error != nil {
		resp.Error(c, 500, resp.CodeInternalServerError, "读取分析失败")
		return
	}
	results := []gin.H{}
	for _, j := range rows {
		var result interface{}
		var evidence interface{}
		var trace interface{}
		_ = json.Unmarshal([]byte(j.ResultJSON), &result)
		_ = json.Unmarshal([]byte(j.EvidenceJSON), &evidence)
		_ = json.Unmarshal([]byte(j.TraceJSON), &trace)
		results = append(results, gin.H{"job": j, "result": result, "evidence": evidence, "trace": trace})
	}
	key := alertAIEpisode(alert)
	resp.Success(c, 200, "成功", gin.H{"episode_key": key, "analyses": results, "notice": "分析结果仅在详情展示；告警通知不等待 AI。", "empty_notice": alertAIEmptyNotice(alert)})
}

func alertAIEmptyNotice(alert models.AlertInstance) string {
	if alert.Status != alertStatusFiring {
		return "本轮尚无分析记录，已恢复或未正式触发的告警不启动分析"
	}
	var center models.FaultCenter
	if database.DB.Where("id = ? AND status = 1", alert.FaultCenterID).First(&center).Error != nil {
		return "故障中心未启用分析"
	}
	if suppressed, err := alertAISuppressed(alert, time.Now()); err != nil {
		return "暂时无法检查分析资格"
	} else if suppressed {
		return "告警已静默或抑制，暂不分析"
	}
	var rules []models.AlertAIAnalysisRule
	if database.DB.Where("fault_center_id = ? AND status = 1", alert.FaultCenterID).Find(&rules).Error != nil {
		return "暂时无法读取分析规则"
	}
	for _, r := range rules {
		if alertMatchesLabelMatchers(alert, decodeAlertLabelMatchers(r.MatchersJSON)) {
			return "已匹配分析规则，等待后台检查并入队"
		}
	}
	return "未匹配已启用的 AI 分析规则"
}

func PreviewAlertAIRule(c *gin.Context) {
	id, ok := requireAlertAI(c, true)
	if !ok {
		return
	}
	var req struct {
		AlertID  uint                       `json:"alert_id"`
		Matchers []models.AlertLabelMatcher `json:"matchers"`
	}
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, 400, resp.CodeBadRequest, "预览参数无效")
		return
	}
	matchers, err := normalizeAlertLabelMatchers(req.Matchers)
	if err != nil || len(matchers) == 0 {
		resp.Error(c, 400, resp.CodeBadRequest, "请完整填写匹配标签")
		return
	}
	var alert models.AlertInstance
	if req.AlertID == 0 || database.DB.Where("id = ? AND fault_center_id = ?", req.AlertID, id).First(&alert).Error != nil {
		resp.Error(c, 404, resp.CodeNotFound, "告警不存在")
		return
	}
	resp.Success(c, 200, "成功", gin.H{"matched": alertMatchesLabelMatchers(alert, matchers)})
}
