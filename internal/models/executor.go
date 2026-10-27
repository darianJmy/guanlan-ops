package models

import "time"

type CertificateExecutor struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"size:128;not null;index" json:"name"`
	Type         string    `gorm:"size:30;not null;default:ssh" json:"type"` // ssh
	Host         string    `gorm:"size:255;not null" json:"host"`
	Port         int       `gorm:"not null;default:22" json:"port"`
	Username     string    `gorm:"size:100;not null" json:"username"`
	AuthType     string    `gorm:"size:30;not null;default:password" json:"auth_type"` // password, private_key
	SecretCipher string    `gorm:"type:longtext" json:"-"`
	Status       int       `gorm:"not null;default:1" json:"status"`
	Description  string    `gorm:"size:500" json:"description"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
