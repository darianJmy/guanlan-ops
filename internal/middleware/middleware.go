package middleware

import (
	authutil "guanlan/internal/auth"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			resp.Error(c, http.StatusUnauthorized, resp.CodeUnauthorized, "未提供认证令牌")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			resp.Error(c, http.StatusUnauthorized, resp.CodeUnauthorized, "认证格式错误")
			c.Abort()
			return
		}

		token := strings.TrimSpace(parts[1])
		if strings.HasPrefix(token, authutil.APITokenPrefix) {
			if authenticateAPIToken(c, token) {
				c.Next()
			}
			return
		}

		claims, err := authutil.ParseToken(token)
		if err != nil {
			resp.Error(c, http.StatusUnauthorized, resp.CodeUnauthorized, "认证令牌无效或已过期")
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Next()
	}
}

func authenticateAPIToken(c *gin.Context, token string) bool {
	var item models.APIAccessToken
	if err := database.DB.Preload("User").Where("token_hash = ?", authutil.HashAPIToken(token)).First(&item).Error; err != nil || item.Status != 1 || item.User.Status != 1 || !item.ExpiresAt.After(time.Now()) {
		resp.Error(c, http.StatusUnauthorized, resp.CodeUnauthorized, "API Token 无效、已禁用或已过期")
		c.Abort()
		return false
	}
	c.Set("user_id", item.UserID)
	c.Set("username", item.User.Username)
	c.Set("auth_type", "api_token")
	c.Set("api_token_id", item.ID)
	now := time.Now()
	if item.LastUsedAt == nil || item.LastUsedAt.Before(now.Add(-time.Minute)) {
		_ = database.DB.Model(&models.APIAccessToken{}).Where("id = ?", item.ID).Updates(map[string]interface{}{"last_used_at": now, "last_used_ip": c.ClientIP()}).Error
	}
	return true
}

func RequirePermission(codes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var count int64
		database.DB.Table("permissions AS p").
			Joins("JOIN role_permissions AS rp ON rp.permission_id = p.id").
			Joins("JOIN roles AS r ON r.id = rp.role_id AND r.status = 1").
			Joins("JOIN user_roles AS ur ON ur.role_id = r.id").
			Where("ur.user_id = ? AND p.status = 1 AND p.code IN ?", c.GetUint("user_id"), codes).
			Count(&count)
		if count == 0 {
			resp.Error(c, http.StatusForbidden, resp.CodeForbidden, "没有操作权限")
			c.Abort()
			return
		}
		c.Next()
	}
}
