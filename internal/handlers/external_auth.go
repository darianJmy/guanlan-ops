package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	authutil "guanlan/internal/auth"
	"guanlan/internal/database"
	"guanlan/internal/middleware"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"guanlan/pkg/textutil"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"github.com/go-ldap/ldap/v3"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
)

func authenticateLDAP(username, password string) (models.User, error) {
	setting := loadAuthSetting()
	if !setting.LDAPEnabled || strings.TrimSpace(username) == "" || password == "" {
		return models.User{}, errors.New("LDAP 未启用")
	}
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	conn, err := ldap.DialURL(setting.LDAPURL, ldap.DialWithDialer(dialer))
	if err != nil {
		return models.User{}, err
	}
	defer conn.Close()
	if setting.LDAPBindDN != "" {
		bindPassword, err := decryptSecret(setting.LDAPBindPassword)
		if err != nil {
			return models.User{}, err
		}
		if err := conn.Bind(setting.LDAPBindDN, bindPassword); err != nil {
			return models.User{}, err
		}
	}
	filter := strings.ReplaceAll(setting.LDAPUserFilter, "{username}", ldap.EscapeFilter(strings.TrimSpace(username)))
	attributes := uniqueStrings([]string{setting.LDAPUsernameAttr, setting.LDAPDisplayNameAttr, setting.LDAPEmailAttr})
	result, err := conn.Search(ldap.NewSearchRequest(setting.LDAPBaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2, 8, false, filter, attributes, nil))
	if err != nil || len(result.Entries) != 1 {
		return models.User{}, errors.New("LDAP 用户不存在或不唯一")
	}
	entry := result.Entries[0]
	if err := conn.Bind(entry.DN, password); err != nil {
		return models.User{}, err
	}
	externalUsername := strings.TrimSpace(entry.GetAttributeValue(setting.LDAPUsernameAttr))
	if externalUsername == "" {
		externalUsername = strings.TrimSpace(username)
	}
	return syncExternalUser(externalUsername, entry.GetAttributeValue(setting.LDAPDisplayNameAttr), entry.GetAttributeValue(setting.LDAPEmailAttr), setting.DefaultRoleID)
}

func OIDCStart(c *gin.Context) {
	setting := loadAuthSetting()
	if !setting.OIDCEnabled {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "OIDC 登录未启用")
		return
	}
	provider, oauthConfig, err := oidcClient(c.Request.Context(), setting)
	_ = provider
	if err != nil {
		resp.Error(c, http.StatusBadGateway, resp.CodeBadGateway, "OIDC 服务不可用")
		return
	}
	state, err := randomToken(32)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "生成登录状态失败")
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: "aiops_oidc_state", Value: state, Path: "/api/auth/oidc", MaxAge: 300, HttpOnly: true, Secure: requestIsHTTPS(c.Request), SameSite: http.SameSiteLaxMode})
	c.Redirect(http.StatusFound, oauthConfig.AuthCodeURL(state))
}

func OIDCCallback(c *gin.Context) {
	stateCookie, err := c.Request.Cookie("aiops_oidc_state")
	if err != nil || stateCookie.Value == "" || c.Query("state") != stateCookie.Value {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "OIDC 登录状态无效")
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: "aiops_oidc_state", Value: "", Path: "/api/auth/oidc", MaxAge: -1, HttpOnly: true})
	setting := loadAuthSetting()
	provider, oauthConfig, err := oidcClient(c.Request.Context(), setting)
	if err != nil {
		resp.Error(c, http.StatusBadGateway, resp.CodeBadGateway, "OIDC 服务不可用")
		return
	}
	token, err := oauthConfig.Exchange(c.Request.Context(), c.Query("code"))
	if err != nil {
		resp.Error(c, http.StatusUnauthorized, resp.CodeUnauthorized, "OIDC 授权码交换失败")
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		resp.Error(c, http.StatusUnauthorized, resp.CodeUnauthorized, "OIDC 未返回 ID Token")
		return
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: setting.OIDCClientID}).Verify(c.Request.Context(), rawIDToken)
	if err != nil {
		resp.Error(c, http.StatusUnauthorized, resp.CodeUnauthorized, "OIDC ID Token 校验失败")
		return
	}
	claims := map[string]interface{}{}
	if err := idToken.Claims(&claims); err != nil {
		resp.Error(c, http.StatusUnauthorized, resp.CodeUnauthorized, "OIDC 用户信息解析失败")
		return
	}
	username := claimString(claims, setting.OIDCUsernameClaim)
	if username == "" {
		username = claimString(claims, "email")
	}
	if username == "" {
		username = claimString(claims, "sub")
	}
	user, err := syncExternalUser(username, claimString(claims, "name"), claimString(claims, "email"), setting.DefaultRoleID)
	if err != nil || user.Status != 1 {
		resp.Error(c, http.StatusForbidden, resp.CodeForbidden, "OIDC 用户无法登录")
		return
	}
	jwtToken, err := authutil.GenerateToken(user.ID, user.Username)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "生成令牌失败")
		return
	}
	middleware.RecordLogin(c, user.ID, user.Username, "成功", "OIDC 登录成功")
	http.SetCookie(c.Writer, &http.Cookie{Name: "aiops_oidc_login", Value: jwtToken, Path: "/api/auth/oidc", MaxAge: 60, HttpOnly: true, Secure: requestIsHTTPS(c.Request), SameSite: http.SameSiteLaxMode})
	c.Redirect(http.StatusFound, "/login?oidc=complete")
}

func OIDCComplete(c *gin.Context) {
	cookie, err := c.Request.Cookie("aiops_oidc_login")
	if err != nil || cookie.Value == "" {
		resp.Error(c, http.StatusUnauthorized, resp.CodeUnauthorized, "OIDC 登录票据已失效")
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: "aiops_oidc_login", Value: "", Path: "/api/auth/oidc", MaxAge: -1, HttpOnly: true})
	resp.Success(c, http.StatusOK, "登录成功", gin.H{"token": cookie.Value})
}

func oidcClient(ctx context.Context, setting models.AuthSetting) (*oidc.Provider, *oauth2.Config, error) {
	if !setting.OIDCEnabled {
		return nil, nil, errors.New("OIDC 未启用")
	}
	secret, err := decryptSecret(setting.OIDCClientSecret)
	if err != nil {
		return nil, nil, err
	}
	ctx = oidc.ClientContext(ctx, &http.Client{Timeout: 10 * time.Second})
	provider, err := oidc.NewProvider(ctx, setting.OIDCIssuerURL)
	if err != nil {
		return nil, nil, err
	}
	scopes := strings.Fields(setting.OIDCScopes)
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}
	return provider, &oauth2.Config{ClientID: setting.OIDCClientID, ClientSecret: secret, Endpoint: provider.Endpoint(), RedirectURL: setting.OIDCRedirectURL, Scopes: scopes}, nil
}

func syncExternalUser(username, nickname, email string, defaultRoleID uint) (models.User, error) {
	username = strings.TrimSpace(username)
	if username == "" || len(username) > 50 {
		return models.User{}, errors.New("外部用户名无效")
	}
	var user models.User
	if database.DB.Where("username = ?", username).First(&user).Error != nil {
		randomPassword, _ := randomToken(32)
		hash, _ := bcrypt.GenerateFromPassword([]byte(randomPassword), bcrypt.DefaultCost)
		user = models.User{Username: username, Password: string(hash), Nickname: textutil.Ellipsis(strings.TrimSpace(nickname), 50), Email: textutil.Ellipsis(strings.TrimSpace(email), 100), Status: 1}
		if err := database.DB.Create(&user).Error; err != nil {
			return user, err
		}
		if defaultRoleID > 0 {
			var role models.Role
			if database.DB.First(&role, defaultRoleID).Error == nil {
				database.DB.Model(&user).Association("Roles").Append(&role)
			}
		}
	} else {
		updates := map[string]interface{}{}
		if strings.TrimSpace(nickname) != "" {
			updates["nickname"] = textutil.Ellipsis(strings.TrimSpace(nickname), 50)
		}
		if strings.TrimSpace(email) != "" {
			updates["email"] = textutil.Ellipsis(strings.TrimSpace(email), 100)
		}
		if len(updates) > 0 {
			database.DB.Model(&user).Updates(updates)
		}
	}
	return user, nil
}

func randomToken(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
func claimString(claims map[string]interface{}, key string) string {
	value, _ := claims[key].(string)
	return strings.TrimSpace(value)
}
func uniqueStrings(values []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
func requestIsHTTPS(request *http.Request) bool {
	return request.TLS != nil || strings.EqualFold(request.Header.Get("X-Forwarded-Proto"), "https")
}
