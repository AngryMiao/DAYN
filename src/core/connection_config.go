package core

import (
	"fmt"

	"angrymiao-ai-server/src/core/providers/asr"
	"angrymiao-ai-server/src/core/providers/llm"
	"angrymiao-ai-server/src/core/providers/tts"
	providersvad "angrymiao-ai-server/src/core/providers/vad"
)

// ConfigurableASRProvider ASR 可配置接口
type ConfigurableASRProvider interface {
	UpdateConfig(userConfig *asr.Config) error
}

// ConfigurableLLMProvider LLM 可配置接口
type ConfigurableLLMProvider interface {
	UpdateConfig(userConfig *llm.Config) error
}

// ConfigurableTTSProvider TTS 可配置接口
type ConfigurableTTSProvider interface {
	UpdateConfig(userConfig *tts.Config) error
}

// ApplyUserASRConfig 应用用户级 ASR 配置
func (h *ConnectionHandler) ApplyUserASRConfig(userConfig *asr.Config) error {
	if userConfig == nil {
		return nil
	}
	if h.providers.asr == nil {
		return fmt.Errorf("ASR provider 未初始化")
	}

	// 类型断言检查是否支持配置更新
	if configurable, ok := h.providers.asr.(ConfigurableASRProvider); ok {
		if err := configurable.UpdateConfig(userConfig); err != nil {
			h.logger.Error(fmt.Sprintf("应用用户ASR配置失败: %v", err))
			return fmt.Errorf("应用用户ASR配置失败: %v", err)
		}
		h.logger.Info("成功应用用户ASR配置")
		return nil
	}

	h.logger.Warn("ASR provider 不支持动态配置更新")
	return nil
}

// ApplyUserLLMConfig 应用用户级 LLM 配置
func (h *ConnectionHandler) ApplyUserLLMConfig(userConfig *llm.Config) error {
	if userConfig == nil {
		return nil
	}
	if h.providers.llm == nil {
		return fmt.Errorf("LLM provider 未初始化")
	}

	// 类型断言检查是否支持配置更新
	if configurable, ok := h.providers.llm.(ConfigurableLLMProvider); ok {
		if err := configurable.UpdateConfig(userConfig); err != nil {
			h.logger.Error(fmt.Sprintf("应用用户LLM配置失败: %v", err))
			return fmt.Errorf("应用用户LLM配置失败: %v", err)
		}
		h.logger.Info("成功应用用户LLM配置")
		return nil
	}

	h.logger.Warn("LLM provider 不支持动态配置更新")
	return nil
}

// ApplyUserTTSConfig 应用用户级 TTS 配置
func (h *ConnectionHandler) ApplyUserTTSConfig(userConfig *tts.Config) error {
	if userConfig == nil {
		return nil
	}
	if h.providers.tts == nil {
		return fmt.Errorf("TTS provider 未初始化")
	}

	// 类型断言检查是否支持配置更新
	if configurable, ok := h.providers.tts.(ConfigurableTTSProvider); ok {
		if err := configurable.UpdateConfig(userConfig); err != nil {
			h.logger.Error(fmt.Sprintf("应用用户TTS配置失败: %v", err))
			return fmt.Errorf("应用用户TTS配置失败: %v", err)
		}
		h.logger.Info("成功应用用户TTS配置")
		return nil
	}

	h.logger.Warn("TTS provider 不支持动态配置更新")
	return nil
}

// ApplyUserVADConfig 应用用户级 VAD 配置
func (h *ConnectionHandler) ApplyUserVADConfig(userConfig *providersvad.Config) error {
	if userConfig == nil {
		return nil
	}
	if h.providers.vad == nil {
		return fmt.Errorf("VAD provider 未初始化")
	}

	// VAD 暂时不支持动态配置更新
	h.logger.Warn("VAD provider 暂不支持动态配置更新")
	return nil
}
