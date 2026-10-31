package models

import "time"

type User struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"size:50;uniqueIndex;not null" json:"username"`
	Password  string    `gorm:"size:255;not null" json:"-"`
	Nickname  string    `gorm:"size:50" json:"nickname"`
	Email     string    `gorm:"size:100" json:"email"`
	Phone     string    `gorm:"size:20" json:"phone"`
	Status    int       `json:"status"` // 1=启用 0=禁用
	Roles     []Role    `gorm:"many2many:user_roles" json:"roles"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Role struct {
	ID          uint         `gorm:"primaryKey" json:"id"`
	Name        string       `gorm:"size:50;not null" json:"name"`
	Code        string       `gorm:"size:50;uniqueIndex;not null" json:"code"`
	Description string       `gorm:"size:200" json:"description"`
	Status      int          `gorm:"default:1" json:"status"`
	Permissions []Permission `gorm:"many2many:role_permissions" json:"permissions,omitempty"`
	Menus       []Menu       `gorm:"many2many:role_menus" json:"menus,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type Permission struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:50;not null" json:"name"`
	Code        string    `gorm:"size:50;uniqueIndex;not null" json:"code"`
	Description string    `gorm:"size:200" json:"description"`
	Status      int       `gorm:"default:1" json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Menu struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ParentID   uint      `gorm:"default:0" json:"parent_id"`
	Name       string    `gorm:"size:50;not null" json:"name"`
	Path       string    `gorm:"size:200" json:"path"`
	Component  string    `gorm:"size:200" json:"component"`
	Icon       string    `gorm:"size:50" json:"icon"`
	Sort       int       `gorm:"default:0" json:"sort"`
	Type       int       `gorm:"default:2" json:"type"` // 1=目录 2=菜单 3=按钮
	Permission string    `gorm:"size:50" json:"permission"`
	Status     int       `gorm:"default:1" json:"status"`
	Children   []Menu    `gorm:"-" json:"children,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
