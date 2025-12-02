package models

import (
	"time"
)

// DialogueMessage 按 userID 存储的单条对话消息（去除 ToolCalls 内容）
type DialogueMessage struct {
	ID         uint   `gorm:"primaryKey"`
	UserID     string `gorm:"index;not null"`
	Index      int    `gorm:"not null"` // 在完整对话中的顺序
	Role       string `gorm:"size:32;not null"`
	Content    string `gorm:"type:text;not null"`
	ToolCallID string `gorm:"size:128"`
	BotID      uint   `gorm:"not null;default:0" json:"bot_id"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (DialogueMessage) TableName() string { return "dialogue_messages" }
