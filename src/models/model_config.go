package models

import (
	"time"

	"gorm.io/gorm"
)

// ModelConfig 模型配置表
type ModelConfig struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	LLMType     string         `gorm:"not null;index:idx_type_name" json:"llm_type"` // qwen/chatglm/ollama/coze/openai
	ModelName   string         `gorm:"not null;index:idx_type_name" json:"model_name"`
	LLMProtocol string         `gorm:"default:openai" json:"llm_protocol"` // llm 的协议类型【openai,ollama】
	BaseURL     string         `json:"base_url,omitempty"`
	Description string         `gorm:"type:text" json:"description,omitempty"`
	IsPublic    bool           `gorm:"default:false" json:"is_public"` // 是否为公共模型
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"` // 软删除
}

// TableName 指定ModelConfig表名
func (ModelConfig) TableName() string {
	return "model_configs"
}
