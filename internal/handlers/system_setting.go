package handlers

import (
	"context"
	"crypto/tls"
	"errors"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"github.com/go-ldap/ldap/v3"
	"gopkg.in/yaml.v3"
)

type saveSystemSettingReq struct {
	SystemName     string `json:"system_name"`
	SystemSubtitle string `json:"system_subtitle"`
	LogoData       string `json:"logo_data"`
	DefaultTheme   string `json:"default_theme"`
}

type saveAuthSettingReq struct {
	LocalEnabled        bool   `json:"local_enabled"`
	LDAPEnabled         bool   `json:"ldap_enabled"`
	LDAPURL             string `json:"ldap_url"`
	LDAPBaseDN          string `json:"ldap_base_dn"`
	LDAPBindDN          string `json:"ldap_bind_dn"`
	LDAPBindPassword    string `json:"ldap_bind_password"`
	LDAPUserFilter      string `json:"ldap_user_filter"`
	LDAPUsernameAttr    string `json:"ldap_username_attr"`
	LDAPDisplayNameAttr string `json:"ldap_display_name_attr"`
	LDAPEmailAttr       string `json:"ldap_email_attr"`
	LDAPConfig          string `json:"ldap_config"`
	OIDCEnabled         bool   `json:"oidc_enabled"`
	OIDCIssuerURL       string `json:"oidc_issuer_url"`
	OIDCClientID        string `json:"oidc_client_id"`
	OIDCClientSecret    string `json:"oidc_client_secret"`
	OIDCRedirectURL     string `json:"oidc_redirect_url"`
	OIDCScopes          string `json:"oidc_scopes"`
	OIDCUsernameClaim   string `json:"oidc_username_claim"`
	OIDCConfig          string `json:"oidc_config"`
	DefaultRoleID       uint   `json:"default_role_id"`
}

type ldapYAMLConfig struct {
	Connection ldapConnectionYAML `yaml:"connection"`
	UserSearch ldapUserSearchYAML `yaml:"user_search"`
	Attributes ldapAttributesYAML `yaml:"attributes"`
}

type ldapConnectionYAML struct {
	URL          string `yaml:"url"`
	BindDN       string `yaml:"bind_dn"`
	BindPassword string `yaml:"bind_password"`
}

type ldapUserSearchYAML struct {
	BaseDN string `yaml:"base_dn"`
	Filter string `yaml:"filter"`
}

type ldapAttributesYAML struct {
	Username    string `yaml:"username"`
	DisplayName string `yaml:"display_name"`
	Email       string `yaml:"email"`
}

type oidcYAMLConfig struct {
	Connection    oidcConnectionYAML    `yaml:"connection"`
	Authorization oidcAuthorizationYAML `yaml:"authorization"`
	Claims        oidcClaimsYAML        `yaml:"claims"`
}

type oidcConnectionYAML struct {
	IssuerURL    string `yaml:"issuer_url"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	RedirectURL  string `yaml:"redirect_url"`
}

type oidcAuthorizationYAML struct {
	Scopes []string `yaml:"scopes"`
}

type oidcClaimsYAML struct {
	Username string `yaml:"username"`
}

func PublicSystemSettings(c *gin.Context) {
	system := loadSystemSetting()
	auth := loadAuthSetting()
	resp.Success(c, http.StatusOK, "成功", gin.H{
		"system_name": system.SystemName, "system_subtitle": system.SystemSubtitle, "logo_data": system.LogoData,
		"default_theme": system.DefaultTheme, "local_enabled": true, "ldap_enabled": auth.LDAPEnabled, "oidc_enabled": auth.OIDCEnabled,
	})
}

func GetSystemSettings(c *gin.Context) {
	resp.Success(c, http.StatusOK, "成功", loadSystemSetting())
}

func UpdateSystemSettings(c *gin.Context) {
	var req saveSystemSettingReq
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "基础设置格式不正确")
		return
	}
	item, err := buildSystemSetting(req)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	existing := loadSystemSetting()
	if err := database.DB.Model(&existing).Updates(map[string]interface{}{
		"system_name": item.SystemName, "system_subtitle": item.SystemSubtitle, "logo_data": item.LogoData, "default_theme": item.DefaultTheme,
	}).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "保存基础设置失败")
		return
	}
	resp.Success(c, http.StatusOK, "基础设置已保存", item)
}

func GetAuthSettings(c *gin.Context) {
	resp.Success(c, http.StatusOK, "成功", authSettingResponse(loadAuthSetting()))
}

func UpdateAuthSettings(c *gin.Context) {
	var req saveAuthSettingReq
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "登录认证配置格式不正确")
		return
	}
	existing := loadAuthSetting()
	item, err := buildAuthSetting(req, &existing)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, err.Error())
		return
	}
	if err := database.DB.Model(&existing).Updates(authSettingUpdates(item)).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "保存登录认证配置失败")
		return
	}
	resp.Success(c, http.StatusOK, "登录认证配置已保存", authSettingResponse(item))
}

func TestLDAPSettings(c *gin.Context) {
	var req saveAuthSettingReq
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "LDAP 配置格式不正确")
		return
	}
	item, err := buildAuthSettingForTest(req, loadAuthSetting())
	if err == nil {
		err = testLDAPConnection(item)
	}
	if err != nil {
		resp.Error(c, http.StatusOK, resp.CodeFailure, err.Error())
		return
	}
	resp.Success(c, http.StatusOK, "LDAP 连接成功")
}

func TestOIDCSettings(c *gin.Context) {
	var req saveAuthSettingReq
	if c.ShouldBindJSON(&req) != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "OIDC 配置格式不正确")
		return
	}
	item, err := buildAuthSettingForTest(req, loadAuthSetting())
	if err == nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		_, err = oidc.NewProvider(ctx, item.OIDCIssuerURL)
	}
	if err != nil {
		resp.Error(c, http.StatusOK, resp.CodeFailure, "OIDC Discovery 失败："+err.Error())
		return
	}
	resp.Success(c, http.StatusOK, "OIDC Discovery 成功")
}

func loadSystemSetting() models.SystemSetting {
	item := models.SystemSetting{ID: 1, SystemName: "观澜运维平台", SystemSubtitle: "智能运维控制台", DefaultTheme: "system"}
	database.DB.FirstOrCreate(&item, models.SystemSetting{ID: 1})
	return item
}

func loadAuthSetting() models.AuthSetting {
	item := models.AuthSetting{ID: 1, LocalEnabled: true, LDAPUserFilter: "(uid={username})", LDAPUsernameAttr: "uid", LDAPDisplayNameAttr: "cn", LDAPEmailAttr: "mail", OIDCScopes: "openid profile email", OIDCUsernameClaim: "preferred_username"}
	database.DB.FirstOrCreate(&item, models.AuthSetting{ID: 1})
	return item
}

func buildSystemSetting(req saveSystemSettingReq) (models.SystemSetting, error) {
	item := models.SystemSetting{ID: 1, SystemName: strings.TrimSpace(req.SystemName), SystemSubtitle: strings.TrimSpace(req.SystemSubtitle), LogoData: strings.TrimSpace(req.LogoData), DefaultTheme: strings.TrimSpace(req.DefaultTheme)}
	if item.SystemName == "" || len([]rune(item.SystemName)) > 100 {
		return item, errors.New("请输入不超过 100 个字符的系统名称")
	}
	if len([]rune(item.SystemSubtitle)) > 200 {
		return item, errors.New("系统副标题不能超过 200 个字符")
	}
	if item.DefaultTheme != "light" && item.DefaultTheme != "dark" && item.DefaultTheme != "system" {
		return item, errors.New("默认主题不正确")
	}
	if item.LogoData != "" && (!strings.HasPrefix(item.LogoData, "data:image/png;base64,") && !strings.HasPrefix(item.LogoData, "data:image/jpeg;base64,") && !strings.HasPrefix(item.LogoData, "data:image/webp;base64,")) {
		return item, errors.New("Logo 仅支持 PNG、JPEG 或 WebP 图片")
	}
	if len(item.LogoData) > 2<<20 {
		return item, errors.New("Logo 图片不能超过 1.5 MB")
	}
	return item, nil
}

func buildAuthSetting(req saveAuthSettingReq, existing *models.AuthSetting) (models.AuthSetting, error) {
	item, err := buildAuthSettingForTest(req, *existing)
	if err != nil {
		return item, err
	}
	item.LocalEnabled = true
	return item, nil
}

func buildAuthSettingForTest(req saveAuthSettingReq, existing models.AuthSetting) (models.AuthSetting, error) {
	var err error
	req, err = applyLDAPYAMLConfig(req)
	if err != nil {
		return models.AuthSetting{}, err
	}
	req, err = applyOIDCYAMLConfig(req)
	if err != nil {
		return models.AuthSetting{}, err
	}
	item := models.AuthSetting{ID: 1, LocalEnabled: true, LDAPEnabled: req.LDAPEnabled, LDAPURL: strings.TrimSpace(req.LDAPURL), LDAPBaseDN: strings.TrimSpace(req.LDAPBaseDN), LDAPBindDN: strings.TrimSpace(req.LDAPBindDN), LDAPBindPassword: existing.LDAPBindPassword, LDAPUserFilter: strings.TrimSpace(req.LDAPUserFilter), LDAPUsernameAttr: strings.TrimSpace(req.LDAPUsernameAttr), LDAPDisplayNameAttr: strings.TrimSpace(req.LDAPDisplayNameAttr), LDAPEmailAttr: strings.TrimSpace(req.LDAPEmailAttr), OIDCEnabled: req.OIDCEnabled, OIDCIssuerURL: strings.TrimRight(strings.TrimSpace(req.OIDCIssuerURL), "/"), OIDCClientID: strings.TrimSpace(req.OIDCClientID), OIDCClientSecret: existing.OIDCClientSecret, OIDCRedirectURL: strings.TrimSpace(req.OIDCRedirectURL), OIDCScopes: strings.TrimSpace(req.OIDCScopes), OIDCUsernameClaim: strings.TrimSpace(req.OIDCUsernameClaim), DefaultRoleID: req.DefaultRoleID}
	if item.LDAPEnabled && item.OIDCEnabled {
		return item, errors.New("LDAP 和 OIDC 只能启用一种认证方式")
	}
	if item.LDAPUserFilter == "" {
		item.LDAPUserFilter = "(uid={username})"
	}
	if item.LDAPUsernameAttr == "" {
		item.LDAPUsernameAttr = "uid"
	}
	if item.LDAPDisplayNameAttr == "" {
		item.LDAPDisplayNameAttr = "cn"
	}
	if item.LDAPEmailAttr == "" {
		item.LDAPEmailAttr = "mail"
	}
	if item.OIDCScopes == "" {
		item.OIDCScopes = "openid profile email"
	}
	if item.OIDCUsernameClaim == "" {
		item.OIDCUsernameClaim = "preferred_username"
	}
	if req.LDAPBindPassword != "" {
		ciphertext, err := encryptSecret(req.LDAPBindPassword)
		if err != nil {
			return item, errors.New("LDAP 密码加密失败")
		}
		item.LDAPBindPassword = ciphertext
	}
	if req.OIDCClientSecret != "" {
		ciphertext, err := encryptSecret(req.OIDCClientSecret)
		if err != nil {
			return item, errors.New("OIDC Secret 加密失败")
		}
		item.OIDCClientSecret = ciphertext
	}
	if item.LDAPEnabled {
		parsed, err := url.Parse(item.LDAPURL)
		if err != nil || (parsed.Scheme != "ldap" && parsed.Scheme != "ldaps") || parsed.Host == "" {
			return item, errors.New("LDAP 地址必须使用 ldap:// 或 ldaps://")
		}
		if item.LDAPBaseDN == "" || !strings.Contains(item.LDAPUserFilter, "{username}") {
			return item, errors.New("请填写 Base DN，用户过滤器必须包含 {username}")
		}
		if item.LDAPBindDN != "" && item.LDAPBindPassword == "" {
			return item, errors.New("请填写 LDAP 服务账号密码")
		}
	}
	if item.OIDCEnabled {
		for _, value := range []string{item.OIDCIssuerURL, item.OIDCClientID, item.OIDCRedirectURL} {
			if value == "" {
				return item, errors.New("请完整填写 OIDC Issuer、Client ID 和回调地址")
			}
		}
		if item.OIDCClientSecret == "" {
			return item, errors.New("请填写 OIDC Client Secret")
		}
		parsed, err := url.Parse(item.OIDCIssuerURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return item, errors.New("OIDC Issuer 必须是 HTTPS 地址")
		}
	}
	if item.DefaultRoleID > 0 {
		var role models.Role
		if database.DB.First(&role, item.DefaultRoleID).Error != nil || role.Status != 1 {
			return item, errors.New("默认角色不存在或已停用")
		}
	}
	return item, nil
}

func authSettingUpdates(item models.AuthSetting) map[string]interface{} {
	return map[string]interface{}{"local_enabled": true, "ldap_enabled": item.LDAPEnabled, "ldap_url": item.LDAPURL, "ldap_base_dn": item.LDAPBaseDN, "ldap_bind_dn": item.LDAPBindDN, "ldap_bind_password": item.LDAPBindPassword, "ldap_user_filter": item.LDAPUserFilter, "ldap_username_attr": item.LDAPUsernameAttr, "ldap_display_name_attr": item.LDAPDisplayNameAttr, "ldap_email_attr": item.LDAPEmailAttr, "oidc_enabled": item.OIDCEnabled, "oidc_issuer_url": item.OIDCIssuerURL, "oidc_client_id": item.OIDCClientID, "oidc_client_secret": item.OIDCClientSecret, "oidc_redirect_url": item.OIDCRedirectURL, "oidc_scopes": item.OIDCScopes, "oidc_username_claim": item.OIDCUsernameClaim, "default_role_id": item.DefaultRoleID}
}

func authSettingResponse(item models.AuthSetting) gin.H {
	return gin.H{"id": item.ID, "local_enabled": true, "ldap_enabled": item.LDAPEnabled, "ldap_url": item.LDAPURL, "ldap_base_dn": item.LDAPBaseDN, "ldap_bind_dn": item.LDAPBindDN, "ldap_bind_password_configured": item.LDAPBindPassword != "", "ldap_user_filter": item.LDAPUserFilter, "ldap_username_attr": item.LDAPUsernameAttr, "ldap_display_name_attr": item.LDAPDisplayNameAttr, "ldap_email_attr": item.LDAPEmailAttr, "ldap_config": ldapConfigYAML(item), "oidc_enabled": item.OIDCEnabled, "oidc_issuer_url": item.OIDCIssuerURL, "oidc_client_id": item.OIDCClientID, "oidc_client_secret_configured": item.OIDCClientSecret != "", "oidc_redirect_url": item.OIDCRedirectURL, "oidc_scopes": item.OIDCScopes, "oidc_username_claim": item.OIDCUsernameClaim, "oidc_config": oidcConfigYAML(item), "default_role_id": item.DefaultRoleID, "updated_at": item.UpdatedAt}
}

func applyLDAPYAMLConfig(req saveAuthSettingReq) (saveAuthSettingReq, error) {
	if strings.TrimSpace(req.LDAPConfig) == "" {
		return req, nil
	}
	var config ldapYAMLConfig
	decoder := yaml.NewDecoder(strings.NewReader(req.LDAPConfig))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return req, errors.New("LDAP YAML 配置格式不正确：" + err.Error())
	}
	req.LDAPURL = config.Connection.URL
	req.LDAPBaseDN = config.UserSearch.BaseDN
	req.LDAPBindDN = config.Connection.BindDN
	req.LDAPBindPassword = config.Connection.BindPassword
	req.LDAPUserFilter = config.UserSearch.Filter
	req.LDAPUsernameAttr = config.Attributes.Username
	req.LDAPDisplayNameAttr = config.Attributes.DisplayName
	req.LDAPEmailAttr = config.Attributes.Email
	return req, nil
}

func ldapConfigYAML(item models.AuthSetting) string {
	data, err := yaml.Marshal(ldapYAMLConfig{
		Connection: ldapConnectionYAML{URL: item.LDAPURL, BindDN: item.LDAPBindDN},
		UserSearch: ldapUserSearchYAML{BaseDN: item.LDAPBaseDN, Filter: item.LDAPUserFilter},
		Attributes: ldapAttributesYAML{Username: item.LDAPUsernameAttr, DisplayName: item.LDAPDisplayNameAttr, Email: item.LDAPEmailAttr},
	})
	if err != nil {
		return ""
	}
	return string(data)
}

func applyOIDCYAMLConfig(req saveAuthSettingReq) (saveAuthSettingReq, error) {
	if strings.TrimSpace(req.OIDCConfig) == "" {
		return req, nil
	}
	var config oidcYAMLConfig
	decoder := yaml.NewDecoder(strings.NewReader(req.OIDCConfig))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return req, errors.New("OIDC YAML 配置格式不正确：" + err.Error())
	}
	req.OIDCIssuerURL = config.Connection.IssuerURL
	req.OIDCClientID = config.Connection.ClientID
	req.OIDCClientSecret = config.Connection.ClientSecret
	req.OIDCRedirectURL = config.Connection.RedirectURL
	req.OIDCScopes = strings.Join(config.Authorization.Scopes, " ")
	req.OIDCUsernameClaim = config.Claims.Username
	return req, nil
}

func oidcConfigYAML(item models.AuthSetting) string {
	data, err := yaml.Marshal(oidcYAMLConfig{
		Connection: oidcConnectionYAML{
			IssuerURL:   item.OIDCIssuerURL,
			ClientID:    item.OIDCClientID,
			RedirectURL: item.OIDCRedirectURL,
		},
		Authorization: oidcAuthorizationYAML{Scopes: strings.Fields(item.OIDCScopes)},
		Claims:        oidcClaimsYAML{Username: item.OIDCUsernameClaim},
	})
	if err != nil {
		return ""
	}
	return string(data)
}

func testLDAPConnection(item models.AuthSetting) error {
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	conn, err := ldap.DialURL(item.LDAPURL, ldap.DialWithDialer(dialer), ldap.DialWithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12}))
	if err != nil {
		return errors.New("LDAP 连接失败：" + err.Error())
	}
	defer conn.Close()
	if item.LDAPBindDN != "" {
		password, err := decryptSecret(item.LDAPBindPassword)
		if err != nil {
			return errors.New("LDAP 密码解密失败")
		}
		if err := conn.Bind(item.LDAPBindDN, password); err != nil {
			return errors.New("LDAP 服务账号认证失败：" + err.Error())
		}
	}
	return nil
}
