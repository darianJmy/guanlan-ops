package handlers

import (
	"context"
	"encoding/json"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/internal/serviceportal/store"
	"guanlan/internal/serviceportal/targetconfig"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Definitions and ACLs belong to 观澜. Runtime availability must not hide
// direct links; its response contributes only live status and port limits.
func listServicePortalLocal(c *gin.Context, user serviceAccessUser, path string) {
	var resources []models.ServicePortalResource
	var grants []models.ServicePortalGrant
	if err := database.DB.Order("created_at, id").Find(&resources).Error; err != nil {
		c.JSON(503, gin.H{"code": 503, "message": "读取服务配置失败"})
		return
	}
	if err := database.DB.Where("user_id = ?", user.ID).Find(&grants).Error; err != nil {
		c.JSON(503, gin.H{"code": 503, "message": "读取服务权限失败"})
		return
	}
	state := store.State{Grants: map[string][]string{}}
	for _, r := range resources {
		state.Resources = append(state.Resources, store.Resource{ID: r.ID, FolderID: r.FolderID})
	}
	for _, grant := range grants {
		state.Grants[user.ID] = append(state.Grants[user.ID], grant.ResourceID)
	}
	identity := store.User{ID: user.ID, Admin: user.Admin, Enabled: user.Enabled, FolderAccess: user.FolderAccess}
	// A short bounded request keeps direct-only installations responsive even
	// when a configured runtime is unreachable.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	var runtimeData json.RawMessage
	if target, err := servicePortalTarget(path); err == nil {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
		if err == nil {
			req.Header.Set("Authorization", c.GetHeader("Authorization"))
			if resp, err := servicePortalHTTP.Do(req); err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					_ = json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&runtimeData)
				}
			}
		}
	}
	if path == "/me" {
		info := struct {
			MinPort int `json:"minPort"`
			MaxPort int `json:"maxPort"`
		}{MinPort: 18000, MaxPort: 18999}
		_ = json.Unmarshal(runtimeData, &info)
		c.JSON(200, gin.H{"code": 0, "data": gin.H{"user": user, "minPort": info.MinPort, "maxPort": info.MaxPort}})
		return
	}
	var live []struct {
		Resource struct {
			ID string `json:"id"`
		} `json:"resource"`
		Status      string `json:"status"`
		Error       string `json:"error"`
		ClusterName string `json:"clusterName"`
	}
	_ = json.Unmarshal(runtimeData, &live)
	result := make([]gin.H, 0, len(resources))
	for _, r := range resources {
		access := state.AccessLevel(identity, r.ID)
		if store.AccessRank(access) < store.AccessRank("view") {
			continue
		}
		item := gin.H{"resource": toServicePortalResource(r), "access_level": access, "status": "unknown", "error": "服务访问后端状态暂不可用"}
		if r.Type == targetconfig.HTTPDirect {
			item["status"], item["error"] = "direct", ""
		} else {
			for _, row := range live {
				if row.Resource.ID == r.ID {
					item["status"], item["error"], item["clusterName"] = row.Status, row.Error, row.ClusterName
					break
				}
			}
		}
		result = append(result, item)
	}
	c.JSON(200, gin.H{"code": 0, "data": result})
}
