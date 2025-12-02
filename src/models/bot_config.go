package models

import (
	"encoding/json"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// BotConfig Bot配置表
type BotConfig struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	CreatorID  uint   `gorm:"not null;index" json:"creator_id"`
	BotHash    string `gorm:"uniqueIndex;not null" json:"bot_hash"`
	Visibility string `gorm:"type:varchar(20);default:'private';index" json:"visibility"` // private/public

	// Model关联
	ModelID uint `gorm:"not null;index" json:"model_id"`

	// Bot类型和能力
	BotType         string `gorm:"type:varchar(20);default:'text';index" json:"bot_type"` // text/image/tts/asr
	RequiresNetwork bool   `gorm:"default:false" json:"requires_network"`                 // 是否需要联网

	// LLM配置（Bot级别的默认配置）
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float32 `json:"temperature,omitempty"`

	// Function Call配置
	FunctionName string         `gorm:"not null;index" json:"function_name"` // Bot名称
	Description  string         `gorm:"type:text" json:"description,omitempty"`
	Parameters   datatypes.JSON `json:"parameters,omitempty"`
	MCPServerURL string         `json:"mcp_server_url,omitempty"`

	// 元数据
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName 指定BotConfig表名
func (BotConfig) TableName() string {
	return "bot_configs"
}

// BotConfigResponse Bot配置响应结构
type BotConfigResponse struct {
	ID              uint                   `json:"id"`
	CreatorID       uint                   `json:"creator_id"`
	BotHash         string                 `json:"bot_hash"`
	Visibility      string                 `json:"visibility"`
	ModelID         uint                   `json:"model_id"`
	BotType         string                 `json:"bot_type"`
	RequiresNetwork bool                   `json:"requires_network"`
	MaxTokens       int                    `json:"max_tokens,omitempty"`
	Temperature     float32                `json:"temperature,omitempty"`
	FunctionName    string                 `json:"function_name"` // Bot名称
	Description     string                 `json:"description,omitempty"`
	Parameters      map[string]interface{} `json:"parameters,omitempty"`
	MCPServerURL    string                 `json:"mcp_server_url,omitempty"`
	IsAdded         bool                   `json:"is_added,omitempty"` // 用户是否已添加
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

// ToResponse 将BotConfig转换为响应结构
func (c *BotConfig) ToResponse() *BotConfigResponse {
	resp := &BotConfigResponse{
		ID:              c.ID,
		CreatorID:       c.CreatorID,
		BotHash:         c.BotHash,
		Visibility:      c.Visibility,
		ModelID:         c.ModelID,
		BotType:         c.BotType,
		RequiresNetwork: c.RequiresNetwork,
		MaxTokens:       c.MaxTokens,
		Temperature:     c.Temperature,
		FunctionName:    c.FunctionName,
		Description:     c.Description,
		MCPServerURL:    c.MCPServerURL,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
	}

	// 解析Parameters JSON
	if c.Parameters != nil {
		var params map[string]interface{}
		if err := json.Unmarshal(c.Parameters, &params); err == nil {
			resp.Parameters = params
		}
	}

	return resp
}

// CreateBotConfigRequest 创建Bot配置请求结构
type CreateBotConfigRequest struct {
	Visibility      string                 `json:"visibility,omitempty"` // private/public
	ModelID         uint                   `json:"model_id" binding:"required"`
	BotType         string                 `json:"bot_type,omitempty"`         // llm/image/tts/asr
	RequiresNetwork bool                   `json:"requires_network,omitempty"` // 是否需要联网
	MaxTokens       int                    `json:"max_tokens,omitempty"`
	Temperature     float32                `json:"temperature,omitempty"`
	FunctionName    string                 `json:"function_name" binding:"required,min=1"` // Bot名称
	Description     string                 `json:"description,omitempty"`
	Parameters      map[string]interface{} `json:"parameters,omitempty"`
	MCPServerURL    string                 `json:"mcp_server_url,omitempty"`
}

// UpdateBotConfigRequest 更新Bot配置请求结构
type UpdateBotConfigRequest struct {
	Visibility      *string                `json:"visibility,omitempty"`
	BotType         *string                `json:"bot_type,omitempty"`
	RequiresNetwork *bool                  `json:"requires_network,omitempty"`
	MaxTokens       *int                   `json:"max_tokens,omitempty"`
	Temperature     *float32               `json:"temperature,omitempty"`
	FunctionName    *string                `json:"function_name,omitempty"` // Bot名称
	Description     *string                `json:"description,omitempty"`
	Parameters      map[string]interface{} `json:"parameters,omitempty"`
	MCPServerURL    *string                `json:"mcp_server_url,omitempty"`
}
