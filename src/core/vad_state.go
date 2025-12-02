package core

import (
	"sync"
	"sync/atomic"
)

// VADState VAD状态管理
// 负责跟踪语音活动状态、空闲时间、音频缓冲等
type VADState struct {
	mu sync.RWMutex

	// 语音活动状态
	haveVoice         bool  // 是否检测到语音
	haveVoiceLastTime int64 // 最后检测到语音的时间(ms)
	voiceStop         bool  // 语音是否已停止

	// 空闲时间管理（原子操作）
	idleDuration int64 // 累计空闲时间(ms)

	// 音频缓冲管理
	audioBuffer      []byte // 音频数据缓冲区
	frameSize        int    // 每帧字节数
	maxBufferFrames  int    // 最大缓冲帧数
	vadCheckFrames   int    // VAD检测需要的最小帧数

	// 静音检测配置
	silenceThreshold int64 // 静音阈值(ms)，超过此时间判定为语音结束
}

// NewVADState 创建新的VAD状态管理器
func NewVADState(frameSize int, silenceThreshold int64) *VADState {
	return &VADState{
		frameSize:         frameSize,
		maxBufferFrames:   10,                      // 默认保留10帧
		vadCheckFrames:    3,                       // 默认累积3帧（60ms @ 20ms/frame）才进行VAD
		silenceThreshold:  silenceThreshold,        // 静音阈值
		audioBuffer:       make([]byte, 0, frameSize*10),
		haveVoice:         false,
		haveVoiceLastTime: 0,
		voiceStop:         false,
		idleDuration:      0,
	}
}

// === 音频缓冲管理 ===

// AddAudioData 添加音频数据到缓冲区
func (v *VADState) AddAudioData(data []byte) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.audioBuffer = append(v.audioBuffer, data...)
}

// GetBufferedFrameCount 获取缓冲区中的帧数
func (v *VADState) GetBufferedFrameCount() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.frameSize <= 0 {
		return 0
	}
	return len(v.audioBuffer) / v.frameSize
}

// GetBufferedData 获取指定帧数的数据（不删除）
func (v *VADState) GetBufferedData(frameCount int) []byte {
	v.mu.RLock()
	defer v.mu.RUnlock()
	
	byteCount := frameCount * v.frameSize
	if byteCount > len(v.audioBuffer) {
		byteCount = len(v.audioBuffer)
	}
	
	data := make([]byte, byteCount)
	copy(data, v.audioBuffer[:byteCount])
	return data
}

// GetAndClearAllData 获取所有缓冲数据并清空缓冲区
func (v *VADState) GetAndClearAllData() []byte {
	v.mu.Lock()
	defer v.mu.Unlock()
	
	data := make([]byte, len(v.audioBuffer))
	copy(data, v.audioBuffer)
	v.audioBuffer = v.audioBuffer[:0]
	return data
}

// RemoveOldFrames 删除旧的音频帧，保留最近的帧
func (v *VADState) RemoveOldFrames(keepFrames int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	
	currentFrames := len(v.audioBuffer) / v.frameSize
	if currentFrames > keepFrames {
		removeBytes := (currentFrames - keepFrames) * v.frameSize
		v.audioBuffer = v.audioBuffer[removeBytes:]
	}
}

// ClearBuffer 清空音频缓冲区
func (v *VADState) ClearBuffer() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.audioBuffer = v.audioBuffer[:0]
}

// HasEnoughDataForVAD 检查是否有足够的数据进行VAD检测
func (v *VADState) HasEnoughDataForVAD() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return len(v.audioBuffer) >= v.vadCheckFrames*v.frameSize
}

// === 空闲时间管理（原子操作） ===

// AddIdleDuration 累加空闲时间
func (v *VADState) AddIdleDuration(duration int64) int64 {
	return atomic.AddInt64(&v.idleDuration, duration)
}

// GetIdleDuration 获取当前空闲时间
func (v *VADState) GetIdleDuration() int64 {
	return atomic.LoadInt64(&v.idleDuration)
}

// ResetIdleDuration 重置空闲时间
func (v *VADState) ResetIdleDuration() {
	atomic.StoreInt64(&v.idleDuration, 0)
}

// === 语音活动状态管理 ===

// SetHaveVoice 设置语音活动状态
func (v *VADState) SetHaveVoice(haveVoice bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.haveVoice = haveVoice
}

// GetHaveVoice 获取语音活动状态
func (v *VADState) GetHaveVoice() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.haveVoice
}

// SetHaveVoiceLastTime 设置最后检测到语音的时间
func (v *VADState) SetHaveVoiceLastTime(timeMs int64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.haveVoiceLastTime = timeMs
}

// GetHaveVoiceLastTime 获取最后检测到语音的时间
func (v *VADState) GetHaveVoiceLastTime() int64 {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.haveVoiceLastTime
}

// SetVoiceStop 设置语音停止标志
func (v *VADState) SetVoiceStop(stop bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.voiceStop = stop
}

// GetVoiceStop 获取语音停止标志
func (v *VADState) GetVoiceStop() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.voiceStop
}

// === 静音检测 ===

// IsSilence 判断是否进入静音状态
// 参数：idleDuration 当前空闲持续时间(ms)
func (v *VADState) IsSilence(idleDuration int64) bool {
	return idleDuration > v.silenceThreshold
}

// GetSilenceThreshold 获取静音阈值
func (v *VADState) GetSilenceThreshold() int64 {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.silenceThreshold
}

// SetSilenceThreshold 设置静音阈值
func (v *VADState) SetSilenceThreshold(threshold int64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.silenceThreshold = threshold
}

// === 配置管理 ===

// SetVADCheckFrames 设置VAD检测需要的最小帧数
func (v *VADState) SetVADCheckFrames(frames int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.vadCheckFrames = frames
}

// GetVADCheckFrames 获取VAD检测需要的最小帧数
func (v *VADState) GetVADCheckFrames() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.vadCheckFrames
}

// SetMaxBufferFrames 设置最大缓冲帧数
func (v *VADState) SetMaxBufferFrames(frames int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.maxBufferFrames = frames
}

// GetMaxBufferFrames 获取最大缓冲帧数
func (v *VADState) GetMaxBufferFrames() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.maxBufferFrames
}

// === 状态重置 ===

// Reset 重置所有状态
func (v *VADState) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()
	
	v.haveVoice = false
	v.haveVoiceLastTime = 0
	v.voiceStop = false
	atomic.StoreInt64(&v.idleDuration, 0)
	v.audioBuffer = v.audioBuffer[:0]
}
