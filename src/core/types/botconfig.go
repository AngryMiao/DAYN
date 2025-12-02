package types

import (
	"time"

	"gorm.io/datatypes"
)

// BotConfig Bot配置结构（用于连接处理，不是数据库模型）
// 从 user_friends + bot_configs + model_configs 组装而成
type BotConfig struct {
	ID     uint   `json:"id"`
	UserID string `json:"user_id"`

	// LLM配置（来自 model_configs）
	LLMType   string `json:"llm_type,omitempty"`   // "qwen", "chatglm", "ollama", "coze", "openai"
	ModelName string `json:"model_name,omitempty"` // 模型名称
	BaseURL   string `json:"base_url,omitempty"`   // API基础URL

	// 用户的 API Key（来自 user_friends.app_key）
	APIKey string `json:"api_key,omitempty"`

	// Bot 级别配置（来自 bot_configs）
	MaxTokens   int     `json:"max_tokens,omitempty"`  // 最大token数
	Temperature float32 `json:"temperature,omitempty"` // 温度参数

	// Function Call配置（来自 bot_configs）
	FunctionName string         `json:"function_name,omitempty"`  // 函数名称
	Description  string         `json:"description,omitempty"`    // 函数描述
	Parameters   datatypes.JSON `json:"parameters,omitempty"`     // JSON格式的参数定义
	MCPServerURL string         `json:"mcp_server_url,omitempty"` // MCP服务器URL

	// 用户好友配置（来自 user_friends）
	IsActive bool `json:"is_active"` // 是否启用
	Priority int  `json:"priority"`  // 优先级，数字越大优先级越高

	// 元数据
	BotHash   string    `json:"bot_hash,omitempty"` // Bot哈希值
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
