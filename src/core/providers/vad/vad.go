package vad

import (
	"angrymiao-ai-server/src/core/utils"
	"fmt"
)

type Config struct {
	Name           string
	Type           string
	Aggressiveness int                    // 0..3
	FrameDuration  int                    // ms
	Params         map[string]interface{} // 保留兼容性，暂不使用
}

// SetUserConfig 设置用户配置（覆盖当前配置）
func (c *Config) SetUserConfig(userConfig *Config) {
	if userConfig == nil {
		return
	}
	*c = *userConfig
}

type Provider interface {
	Initialize() error
	Cleanup() error
	// 如果给定的PCM帧包含语音则返回true
	Process(pcm []byte, sampleRate int, frameMs int) (bool, error)
}

type Factory func(cfg *Config, logger *utils.Logger) (Provider, error)

var factories = map[string]Factory{}

func Register(name string, f Factory) {
	factories[name] = f
}

func Create(name string, cfg *Config, logger *utils.Logger) (Provider, error) {
	f, ok := factories[name]
	if !ok {
		return nil, fmt.Errorf("未知的VAD提供者: %s", name)
	}
	return f(cfg, logger)
}
