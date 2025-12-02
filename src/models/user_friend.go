package models

import (
	"time"

	"gorm.io/gorm"
)

// UserFriend 用户好友表
type UserFriend struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	UserID     uint   `gorm:"not null;index:idx_user_friend" json:"user_id"`
	FriendType string `gorm:"type:varchar(20);not null;index" json:"friend_type"` // user/bot

	// 当friend_type=user时使用
	FriendUserID *uint `gorm:"index" json:"friend_user_id,omitempty"`

	// 当friend_type=bot时使用
	BotConfigID *uint  `gorm:"index:idx_user_friend" json:"bot_config_id,omitempty"`
	AppKey      string `json:"app_key,omitempty"` // 用户的LLM API密钥

	// 关联（不使用数据库外键，在代码中手动加载）
	BotConfig *BotConfig `gorm:"-" json:"bot_config,omitempty"` // 关联的Bot配置

	// 元数据
	Alias     string         `json:"alias,omitempty"`               // 好友别名
	Priority  int            `gorm:"default:0" json:"priority"`     // 优先级
	IsActive  bool           `gorm:"default:true" json:"is_active"` // 是否启用
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName 指定UserFriend表名
func (UserFriend) TableName() string {
	return "user_friends"
}

// UserBotFriendResponse 用户Bot好友响应结构（包含Bot配置信息）
type UserBotFriendResponse struct {
	ID          uint               `json:"id"`
	UserID      uint               `json:"user_id"`
	BotConfigID uint               `json:"bot_config_id"`
	AppKey      string             `json:"app_key,omitempty"` // 可能需要脱敏
	Alias       string             `json:"alias,omitempty"`
	Priority    int                `json:"priority"`
	IsActive    bool               `json:"is_active"`
	BotConfig   *BotConfigResponse `json:"bot_config,omitempty"` // 关联的Bot配置
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

// AddBotFriendRequest 添加Bot好友请求结构
type AddBotFriendRequest struct {
	BotConfigID uint   `json:"bot_config_id" binding:"required"`
	AppKey      string `json:"app_key,omitempty"` // AppKey改为可选
	Alias       string `json:"alias,omitempty"`
	Priority    int    `json:"priority,omitempty"`
}

// UpdateBotFriendAppKeyRequest 更新Bot好友AppKey请求结构
type UpdateBotFriendAppKeyRequest struct {
	AppKey string `json:"app_key" binding:"required"`
}

// UpdateBotFriendPriorityRequest 更新Bot好友优先级请求结构
type UpdateBotFriendPriorityRequest struct {
	Priority int `json:"priority" binding:"required"`
}

// ToggleBotFriendStatusRequest 切换Bot好友状态请求结构
type ToggleBotFriendStatusRequest struct {
	IsActive bool `json:"is_active" binding:"required"`
}
