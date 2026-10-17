package model

import "time"

type RagSession struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID        string `gorm:"primaryKey;size:64"`
	UserID    string `gorm:"index;size:64"`
	Platform  string `gorm:"index;size:20"`
	KBID      string `gorm:"size:64"`
	Status    string `gorm:"index;size:20;default:active"`
	Config    string `gorm:"type:text;default:'{}'"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type RagMessage struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID        uint      `gorm:"primaryKey;autoIncrement"`
	SessionID string    `gorm:"size:120;index" json:"session_id"`
	MessageID string    `gorm:"size:64"`
	Role      string    `gorm:"size:20"`
	Content   string    `gorm:"type:text"`
	Timestamp time.Time `gorm:"index"`
}
