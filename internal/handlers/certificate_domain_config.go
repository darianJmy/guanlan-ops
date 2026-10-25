package handlers

import (
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

const (
	certificateDomainConfigAPIVersion = "aiops/v1"
	certificateDomainConfigKind       = "CertificateDomainList"
	certificateDomainConfigMaxItems   = 1000
)

type certificateDomainConfigDocument struct {
	APIVersion string                         `yaml:"apiVersion"`
	Kind       string                         `yaml:"kind"`
	Domains    []certificateDomainConfigEntry `yaml:"domains"`
}

type certificateDomainConfigEntry struct {
	Domain               string `yaml:"domain"`
	Port                 int    `yaml:"port"`
	FaultCenterID        uint   `yaml:"fault_center_id"`
	MonitorEnabled       *bool  `yaml:"monitor_enabled,omitempty"`
	CheckIntervalMinutes int    `yaml:"check_interval_minutes"`
	ExpiryAlertDays      int    `yaml:"expiry_alert_days"`
	Severity             string `yaml:"severity"`
	Description          string `yaml:"description,omitempty"`
}

type importCertificateDomainConfigReq struct {
	Content   string `json:"content" binding:"required"`
	Overwrite bool   `json:"overwrite"`
	DryRun    bool   `json:"dry_run"`
}

type certificateDomainImportResult struct {
	Total   int `json:"total"`
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

type preparedCertificateDomainImport struct {
	Request  saveDomainReq
	Existing *models.CertificateDomain
}

func ExportCertificateDomains(c *gin.Context) {
	var domains []models.CertificateDomain
	if err := database.DB.Order("domain ASC, port ASC, id ASC").Find(&domains).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "读取域名配置失败")
		return
	}
	document := certificateDomainConfigDocument{APIVersion: certificateDomainConfigAPIVersion, Kind: certificateDomainConfigKind, Domains: make([]certificateDomainConfigEntry, 0, len(domains))}
	for _, item := range domains {
		enabled := item.MonitorEnabled
		document.Domains = append(document.Domains, certificateDomainConfigEntry{
			Domain: item.Domain, Port: defaultPort(item.Port), FaultCenterID: item.FaultCenterID, MonitorEnabled: &enabled,
			CheckIntervalMinutes: defaultInterval(item.CheckIntervalMinutes), ExpiryAlertDays: item.ExpiryAlertDays,
			Severity: normalizeCertificateDomainSeverity(item.Severity), Description: item.Description,
		})
	}
	sort.SliceStable(document.Domains, func(i, j int) bool {
		if document.Domains[i].Domain == document.Domains[j].Domain {
			return document.Domains[i].Port < document.Domains[j].Port
		}
		return document.Domains[i].Domain < document.Domains[j].Domain
	})
	content, err := yaml.Marshal(document)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "生成导出文件失败")
		return
	}
	filename := fmt.Sprintf("certificate-domains-%s.yaml", time.Now().Format("20060102-150405"))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(http.StatusOK, "application/yaml; charset=utf-8", content)
}

func ImportCertificateDomains(c *gin.Context) {
	var payload importCertificateDomainConfigReq
	if err := c.ShouldBindJSON(&payload); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请粘贴 YAML 配置")
		return
	}
	document, err := parseCertificateDomainConfigDocument(payload.Content)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	prepared, result, err := prepareCertificateDomainImport(document, payload.Overwrite)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if payload.DryRun {
		resp.Success(c, http.StatusOK, "配置校验通过", result)
		return
	}

	err = database.DB.Transaction(func(tx *gorm.DB) error {
		for _, item := range prepared {
			if item.Existing != nil && !payload.Overwrite {
				continue
			}
			if item.Existing == nil {
				if err := createImportedCertificateDomain(tx, item.Request); err != nil {
					return err
				}
				continue
			}
			if err := updateImportedCertificateDomain(tx, item.Existing.ID, item.Request); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "导入域名配置失败，未写入任何数据")
		return
	}
	resp.Success(c, http.StatusOK, "导入完成", result)
}

func parseCertificateDomainConfigDocument(content string) (certificateDomainConfigDocument, error) {
	document := certificateDomainConfigDocument{}
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return document, fmt.Errorf("配置内容不能为空")
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return document, fmt.Errorf("仅支持 YAML 配置，请将 JSON 转换为 YAML 后再导入")
	}
	decoder := yaml.NewDecoder(strings.NewReader(trimmed))
	decoder.KnownFields(true)
	if err := decoder.Decode(&document); err != nil {
		return document, fmt.Errorf("YAML 配置解析失败：%v", err)
	}
	if document.APIVersion != certificateDomainConfigAPIVersion || document.Kind != certificateDomainConfigKind {
		return document, fmt.Errorf("配置类型不正确，应为 apiVersion: %s、kind: %s", certificateDomainConfigAPIVersion, certificateDomainConfigKind)
	}
	if len(document.Domains) == 0 {
		return document, fmt.Errorf("配置中至少需要一个域名")
	}
	if len(document.Domains) > certificateDomainConfigMaxItems {
		return document, fmt.Errorf("单次最多导入 %d 个域名", certificateDomainConfigMaxItems)
	}
	return document, nil
}

func prepareCertificateDomainImport(document certificateDomainConfigDocument, overwrite bool) ([]preparedCertificateDomainImport, certificateDomainImportResult, error) {
	domainNames := make([]string, 0, len(document.Domains))
	requests := make([]saveDomainReq, 0, len(document.Domains))
	seen := make(map[string]struct{}, len(document.Domains))
	for index, entry := range document.Domains {
		request, err := certificateDomainConfigRequest(entry)
		if err != nil {
			return nil, certificateDomainImportResult{}, fmt.Errorf("第 %d 个域名配置错误：%v", index+1, err)
		}
		key := certificateDomainConfigKey(request.Domain, request.Port)
		if _, exists := seen[key]; exists {
			return nil, certificateDomainImportResult{}, fmt.Errorf("配置中存在重复域名：%s", key)
		}
		seen[key] = struct{}{}
		domainNames = append(domainNames, request.Domain)
		requests = append(requests, request)
	}

	var existingDomains []models.CertificateDomain
	if err := database.DB.Where("domain IN ?", domainNames).Order("id ASC").Find(&existingDomains).Error; err != nil {
		return nil, certificateDomainImportResult{}, fmt.Errorf("检查重复域名失败")
	}
	existingByKey := make(map[string]*models.CertificateDomain, len(existingDomains))
	for index := range existingDomains {
		item := &existingDomains[index]
		key := certificateDomainConfigKey(item.Domain, item.Port)
		if _, exists := existingByKey[key]; exists {
			return nil, certificateDomainImportResult{}, fmt.Errorf("系统中存在重复域名：%s，请先处理重复数据", key)
		}
		existingByKey[key] = item
	}

	result := certificateDomainImportResult{Total: len(requests)}
	prepared := make([]preparedCertificateDomainImport, 0, len(requests))
	for _, request := range requests {
		existing := existingByKey[certificateDomainConfigKey(request.Domain, request.Port)]
		prepared = append(prepared, preparedCertificateDomainImport{Request: request, Existing: existing})
		if existing == nil {
			result.Created++
		} else if overwrite {
			result.Updated++
		} else {
			result.Skipped++
		}
	}
	return prepared, result, nil
}

func certificateDomainConfigRequest(entry certificateDomainConfigEntry) (saveDomainReq, error) {
	domain := normalizeTLSName(entry.Domain)
	port := defaultPort(entry.Port)
	if domain == "" || len([]rune(domain)) > 255 {
		return saveDomainReq{}, fmt.Errorf("域名不能为空且不能超过 255 个字符")
	}
	if port < 1 || port > 65535 {
		return saveDomainReq{}, fmt.Errorf("端口必须在 1-65535 之间")
	}
	if err := validateCertificateDomainFaultCenter(entry.FaultCenterID); err != nil {
		return saveDomainReq{}, err
	}
	severity, err := validateCertificateDomainAlertPolicy(entry.ExpiryAlertDays, entry.Severity)
	if err != nil {
		return saveDomainReq{}, err
	}
	if len([]rune(strings.TrimSpace(entry.Description))) > 500 {
		return saveDomainReq{}, fmt.Errorf("备注不能超过 500 个字符")
	}
	enabled := true
	if entry.MonitorEnabled != nil {
		enabled = *entry.MonitorEnabled
	}
	return saveDomainReq{
		Domain: domain, Port: port, FaultCenterID: entry.FaultCenterID, MonitorEnabled: &enabled,
		CheckIntervalMinutes: defaultInterval(entry.CheckIntervalMinutes), ExpiryAlertDays: entry.ExpiryAlertDays,
		Severity: severity, Description: strings.TrimSpace(entry.Description),
	}, nil
}

func createImportedCertificateDomain(tx *gorm.DB, request saveDomainReq) error {
	enabled := request.MonitorEnabled == nil || *request.MonitorEnabled
	item := models.CertificateDomain{
		Domain: request.Domain, Port: request.Port, FaultCenterID: request.FaultCenterID, MonitorEnabled: enabled,
		CheckIntervalMinutes: request.CheckIntervalMinutes, ExpiryAlertDays: request.ExpiryAlertDays, Severity: request.Severity,
		AlertBeforeDays: defaultAlertDays(""), Status: "unknown", Description: request.Description,
	}
	if err := tx.Create(&item).Error; err != nil {
		return err
	}
	if !enabled {
		return tx.Model(&item).UpdateColumn("monitor_enabled", false).Error
	}
	return nil
}

func updateImportedCertificateDomain(tx *gorm.DB, id uint, request saveDomainReq) error {
	enabled := request.MonitorEnabled == nil || *request.MonitorEnabled
	return tx.Model(&models.CertificateDomain{}).Where("id = ?", id).Updates(map[string]interface{}{
		"domain": request.Domain, "port": request.Port, "fault_center_id": request.FaultCenterID,
		"certificate_id": 0, "executor_id": 0, "agent_id": 0, "deploy_script_id": 0, "notification_channel_id": 0,
		"deploy_params": "", "deploy_cert_path": "", "deploy_key_path": "", "deploy_reload_command": "",
		"monitor_enabled": enabled, "check_interval_minutes": request.CheckIntervalMinutes,
		"expiry_alert_days": request.ExpiryAlertDays, "severity": request.Severity, "description": request.Description,
	}).Error
}

func certificateDomainConfigKey(domain string, port int) string {
	return strings.ToLower(normalizeTLSName(domain)) + ":" + fmt.Sprint(defaultPort(port))
}
