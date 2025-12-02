package webrtc

import (
	"angrymiao-ai-server/src/core/providers/vad"
	"angrymiao-ai-server/src/core/utils"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/hackers365/go-webrtcvad"
)

const (
	// DefaultSampleRate WebRTC VAD 支持的采样率 (8000, 16000, 32000, 48000)
	DefaultSampleRate = 16000
	// DefaultMode VAD 敏感度模式 (0: 最不敏感, 3: 最敏感)
	DefaultMode = 2
	// FrameDuration 帧持续时间 (ms)，WebRTC VAD 支持 10ms, 20ms, 30ms
	FrameDuration = 20
)

// Provider 实现了基于 WebRTC VAD 的语音活动检测
// 使用 go-webrtcvad 库提供真正的 WebRTC VAD 功能
// 输入的 pcm 需为 16 位小端单声道 PCM
type Provider struct {
	logger         *utils.Logger
	webrtcVad      *webrtcvad.VAD // WebRTC VAD 实例
	sampleRate     int            // 采样率
	mode           int            // VAD 模式 (0-3)
	frameSize      int            // 每帧采样数
	frameSizeBytes int            // 每帧字节数
	frameDuration  int            // 帧持续时间(ms)
	initialized    bool           // 是否已初始化
	lastUsed       time.Time      // 最后使用时间
	mu             sync.RWMutex   // 读写锁保证线程安全
}

// New 创建新的 WebRTC VAD Provider
func New(logger *utils.Logger, cfg *vad.Config) (*Provider, error) {
	// 设置默认值
	mode := DefaultMode
	if cfg.Aggressiveness >= 0 && cfg.Aggressiveness <= 3 {
		mode = cfg.Aggressiveness
	}

	frameDuration := FrameDuration
	if cfg.FrameDuration > 0 {
		frameDuration = cfg.FrameDuration
	}

	// 采样率改为运行时从客户端握手参数获取，不在配置中固定
	// 这里保持为0，表示由 Process 时的入参决定
	sampleRate := 0 // 采样率为0表示由运行时决定，此时不进行校验；>0时需校验是否受支持
	if sampleRate > 0 && !isValidSampleRate(sampleRate) {
		return nil, fmt.Errorf("unsupported sample rate: %d, supported rates: 8000, 16000, 32000, 48000", sampleRate)
	}

	provider := &Provider{
		logger:        logger,
		sampleRate:    sampleRate,
		mode:          mode,
		frameDuration: frameDuration,
		lastUsed:      time.Now(),
	}

	// 初始化 WebRTC VAD
	if err := provider.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize WebRTC VAD: %w", err)
	}

	return provider, nil
}

// initialize 初始化 WebRTC VAD 实例
func (p *Provider) initialize() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		return nil
	}

	// 计算帧大小: 使用有效采样率(若未指定则采用默认值) * 帧持续时间(ms) / 1000
	effectiveSR := p.sampleRate
	if effectiveSR <= 0 {
		effectiveSR = DefaultSampleRate
	}
	p.frameSize = effectiveSR / 1000 * p.frameDuration
	p.frameSizeBytes = p.frameSize * 2 // 16-bit PCM，每个采样2字节

	// 创建 WebRTC VAD 实例
	var err error
	p.webrtcVad, err = webrtcvad.New()
	if err != nil || p.webrtcVad == nil {
		return fmt.Errorf("failed to create WebRTC VAD instance: %w", err)
	}

	// 设置 VAD 敏感度模式
	err = p.webrtcVad.SetMode(p.mode)
	if err != nil {
		webrtcvad.Free(p.webrtcVad)
		p.webrtcVad = nil
		return fmt.Errorf("failed to set WebRTC VAD mode: %w", err)
	}

	p.initialized = true
	p.lastUsed = time.Now()

	return nil
}

// Initialize 实现 vad.Provider 接口
func (p *Provider) Initialize() error {
	return p.initialize()
}

// Cleanup 清理资源，实现 vad.Provider 接口
func (p *Provider) Cleanup() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized && p.webrtcVad != nil {
		webrtcvad.Free(p.webrtcVad)
		p.webrtcVad = nil
		p.initialized = false
		p.logger.Info("WebRTC VAD资源已释放")
	}
	return nil
}

// Process 处理音频帧并检测语音活动
// 参数 pcm: 16位小端PCM音频数据
// 参数 sampleRate: 采样率(如果<=0则使用配置的采样率)
// 参数 frameMs: 帧持续时间(如果<=0则使用配置的帧持续时间)
// 返回 true 表示检测到语音活动
func (p *Provider) Process(pcm []byte, sampleRate int, frameMs int) (bool, error) {
	// 参数校验
	if len(pcm) == 0 {
		return false, nil
	}

	// 检查VAD实例是否已初始化
	if !p.initialized || p.webrtcVad == nil {
		p.logger.Error("VAD实例未初始化，尝试重新初始化")
		if err := p.initialize(); err != nil {
			return false, fmt.Errorf("VAD初始化失败: %w", err)
		}
	}

	// 使用传入的参数或默认值
	if sampleRate <= 0 {
		// 运行时优先用 Provider 中的值，否则回退到默认值
		if p.sampleRate > 0 {
			sampleRate = p.sampleRate
		} else {
			sampleRate = DefaultSampleRate
		}
	}
	if frameMs <= 0 {
		frameMs = p.frameDuration
	}

	// 验证采样率是否被WebRTC VAD支持
	var frameBytes int
	if !isValidSampleRate(sampleRate) {
		p.logger.Error("不支持的采样率: %d, WebRTC VAD只支持: 8000, 16000, 32000, 48000 Hz", sampleRate)
		// 尝试使用最接近的支持的采样率
		if sampleRate < 12000 {
			sampleRate = 8000
		} else if sampleRate < 24000 {
			sampleRate = 16000
		} else if sampleRate < 40000 {
			sampleRate = 32000
		} else {
			sampleRate = 48000
		}
		p.logger.Error("自动调整为支持的采样率: %d Hz", sampleRate)
		// 重新计算帧大小
		frameBytes = sampleRate * 2 * frameMs / 1000
	}

	// 验证PCM数据必须是16位(2字节对齐)
	if len(pcm)%2 != 0 {
		p.logger.Error("PCM数据格式错误: 长度必须是偶数(16位), 实际: %d字节", len(pcm))
		return false, fmt.Errorf("pcm data must be 16-bit (even number of bytes), got: %d", len(pcm))
	}

	// 更新最后使用时间
	p.lastUsed = time.Now()

	// 计算当前帧大小（如果之前没有重新计算）
	if frameBytes <= 0 {
		frameBytes = sampleRate * 2 * frameMs / 1000 // 2 = 16bit = 2 bytes per sample
		if frameBytes <= 0 {
			frameBytes = p.frameSizeBytes
		}
	}

	// 如果数据不足一帧，返回 false
	if len(pcm) < frameBytes {
		p.logger.Error("VAD数据不足一帧: 收到%d字节, 需要%d字节(采样率:%d, 帧长:%dms)", len(pcm), frameBytes, sampleRate, frameMs)
		return false, nil
	}

	// 处理多帧数据：逐帧检测，统计语音活动帧数
	// 如果超过半数的帧被判定为语音，则认为整体包含语音活动
	activityCount := 0
	totalFrames := 0

	// 参考Demo：WebRTC VAD要求固定的帧大小
	// Demo中frameSize是固定配置值，WebRTC VAD只支持特定帧大小

	// WebRTC VAD支持的帧大小（字节）
	supportedFrameSizes := []int{
		sampleRate * 2 * 10 / 1000, // 10ms
		sampleRate * 2 * 20 / 1000, // 20ms
		sampleRate * 2 * 30 / 1000, // 30ms
	}

	// 选择最适合的帧大小
	actualFrameBytes := supportedFrameSizes[1] // 默认20ms
	for _, size := range supportedFrameSizes {
		if len(pcm)%size == 0 {
			actualFrameBytes = size
			break
		}
	}

	p.logger.Debug("使用帧大小: %d字节 (数据长度: %d字节)", actualFrameBytes, len(pcm))

	for offset := 0; offset+actualFrameBytes <= len(pcm); offset += actualFrameBytes {
		frameData := pcm[offset : offset+actualFrameBytes]
		totalFrames++

		// 调用 WebRTC VAD 检测该帧
		isSpeech, err := p.webrtcVad.Process(sampleRate, frameData)
		if err != nil {
			p.logger.Error("WebRTC VAD处理失败: 采样率=%d, 帧大小=%d字节, 错误=%v", sampleRate, len(frameData), err)
			return false, fmt.Errorf("WebRTC VAD process error: %w", err)
		}

		if isSpeech {
			activityCount++
		}
	}

	// 判断规则：如果超过半数帧为语音，则判定为有语音活动
	isActive := activityCount >= (totalFrames+1)/2 // 向上取整

	return isActive, nil
}

// isValidSampleRate 检查采样率是否被 WebRTC VAD 支持
func isValidSampleRate(sampleRate int) bool {
	// WebRTC VAD 支持的采样率: 8000, 16000, 32000, 48000 Hz
	validRates := []int{8000, 16000, 32000, 48000}
	for _, rate := range validRates {
		if rate == sampleRate {
			return true
		}
	}
	return false
}

// BytesToFloat32 将16位PCM字节数组转换为float32数组
// 用于某些场景下需要float32格式的音频数据
func BytesToFloat32(pcmBytes []byte) []float32 {
	if len(pcmBytes)%2 != 0 {
		return nil
	}

	samples := make([]float32, len(pcmBytes)/2)
	for i := 0; i < len(samples); i++ {
		// 小端序读取16位整数
		intSample := int16(binary.LittleEndian.Uint16(pcmBytes[i*2:]))
		// 转换为float32范围 [-1.0, 1.0]
		samples[i] = float32(intSample) / 32768.0
	}
	return samples
}

// Float32ToBytes 将float32数组转换为16位PCM字节数组
// 这是 demo 项目中的核心转换函数
func Float32ToBytes(samples []float32) []byte {
	pcmBytes := make([]byte, len(samples)*2)

	for i, sample := range samples {
		// 将 float32 (-1.0 到 1.0) 转换为 int16 (-32768 到 32767)
		var intSample int16
		if sample > 1.0 {
			intSample = 32767
		} else if sample < -1.0 {
			intSample = -32768
		} else {
			intSample = int16(sample * 32767)
		}

		// 小端序写入字节数组
		binary.LittleEndian.PutUint16(pcmBytes[i*2:], uint16(intSample))
	}

	return pcmBytes
}

// Reset 重置 Provider 状态
func (p *Provider) Reset() error {
	p.lastUsed = time.Now()
	return nil
}

// IsValid 检查资源是否有效
func (p *Provider) IsValid() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.initialized && p.webrtcVad != nil
}

// GetLastUsed 获取最后使用时间
func (p *Provider) GetLastUsed() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastUsed
}

// 注册到 VAD 工厂表
func init() {
	vad.Register("WebRTC", func(cfg *vad.Config, logger *utils.Logger) (vad.Provider, error) {
		return New(logger, cfg)
	})
}
