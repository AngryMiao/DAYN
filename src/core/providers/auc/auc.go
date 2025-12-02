package auc

import (
	"angrymiao-ai-server/src/core/providers"
	"angrymiao-ai-server/src/core/utils"
	"context"
	"fmt"
)

// Config AUC配置结构
type Config struct {
	Name string `yaml:"name"` // AUC提供者名称
	Type string
	Data map[string]interface{}
}

// Provider AUC提供者接口
type Provider interface {
	providers.Provider
	// 提交音频识别任务
	SubmitTask(ctx context.Context, audioURL string, userID string) (taskID string, err error)
	// 查询任务状态（如果是callback模式不需要去查auc提供方任务状态，等待callback即可）
	QueryTask(ctx context.Context, taskID string) (*QueryResponse, error)
}

// BaseProvider AUC基础实现
type BaseProvider struct {
	config *Config
	logger *utils.Logger
}

// Config 获取配置
func (p *BaseProvider) Config() *Config {
	return p.config
}

// NewBaseProvider 创建AUC基础提供者
func NewBaseProvider(config *Config, logger *utils.Logger) *BaseProvider {
	return &BaseProvider{
		config: config,
		logger: logger,
	}
}

// Initialize 初始化提供者
func (p *BaseProvider) Initialize() error {
	return nil
}

// Cleanup 清理资源
func (p *BaseProvider) Cleanup() error {
	return nil
}

// Factory AUC工厂函数类型
type Factory func(config *Config, logger *utils.Logger) (Provider, error)

var factories = make(map[string]Factory)

// Register 注册AUC提供者工厂
func Register(name string, factory Factory) {
	factories[name] = factory
}

// Create 创建AUC提供者实例
func Create(name string, config *Config, logger *utils.Logger) (Provider, error) {
	factory, ok := factories[name]
	if !ok {
		return nil, fmt.Errorf("未知的AUC提供者: %s", name)
	}

	provider, err := factory(config, logger)
	if err != nil {
		return nil, fmt.Errorf("创建AUC提供者失败: %v", err)
	}

	return provider, nil
}

// QueryResponse 查询任务响应结构
type QueryResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
	Result  struct {
		Text string `json:"text,omitempty"`
	} `json:"result,omitempty"`
}
