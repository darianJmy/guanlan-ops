package handlers

import (
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type UserListQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Keyword  string `form:"keyword"`
	Status   *int   `form:"status"`
}

func ListUsers(c *gin.Context) {
	var query UserListQuery
	c.ShouldBindQuery(&query)
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 {
		query.PageSize = 10
	}

	var total int64
	var users []models.User
	tx := database.DB.Model(&models.User{})
	if query.Keyword != "" {
		kw := "%" + query.Keyword + "%"
		tx = tx.Where("username LIKE ?", kw)
	}
	if query.Status != nil {
		tx = tx.Where("status = ?", *query.Status)
	}
	tx.Count(&total)
	tx.Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Preload("Roles").Find(&users)

	resp.Success(c, http.StatusOK, "成功", gin.H{
		"list":      users,
		"total":     total,
		"page":      query.Page,
		"page_size": query.PageSize,
	})
}

type CreateUserReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Nickname string `json:"nickname"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Status   int    `json:"status"`
}

func CreateUser(c *gin.Context) {
	var req CreateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}

	if req.Status == 0 {
		req.Status = 1
	}

	hashed, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	user := models.User{
		Username: req.Username,
		Password: string(hashed),
		Nickname: req.Nickname,
		Email:    req.Email,
		Phone:    req.Phone,
		Status:   req.Status,
	}

	if err := database.DB.Create(&user).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeBadRequest, "用户名已存在")
		return
	}

	resp.Success(c, http.StatusOK, "创建成功")
}

type UpdateUserReq struct {
	Nickname string `json:"nickname"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Status   int    `json:"status"`
}

func UpdateUser(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req UpdateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}

	database.DB.Model(&models.User{}).Where("id = ?", id).Updates(map[string]interface{}{
		"nickname": req.Nickname,
		"email":    req.Email,
		"phone":    req.Phone,
		"status":   req.Status,
	})

	resp.Success(c, http.StatusOK, "更新成功")
}

func DeleteUser(c *gin.Context) {
	id := c.Param("id")
	var user models.User
	if err := database.DB.First(&user, id).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "用户不存在")
		return
	}
	database.DB.Select("Roles").Delete(&user)
	resp.Success(c, http.StatusOK, "删除成功")
}

type AssignUserRolesReq struct {
	RoleIDs []uint `json:"role_ids"`
}

func AssignUserRoles(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req AssignUserRolesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}

	var user models.User
	if err := database.DB.First(&user, id).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "用户不存在")
		return
	}

	var roles []models.Role
	if len(req.RoleIDs) > 0 {
		database.DB.Find(&roles, req.RoleIDs)
	}
	database.DB.Model(&user).Association("Roles").Replace(roles)

	resp.Success(c, http.StatusOK, "分配成功")
}

func ResetPassword(c *gin.Context) {
	id := c.Param("id")
	hashed, _ := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	database.DB.Model(&models.User{}).Where("id = ?", id).Update("password", string(hashed))
	resp.Success(c, http.StatusOK, "密码已重置为 123456")
}
