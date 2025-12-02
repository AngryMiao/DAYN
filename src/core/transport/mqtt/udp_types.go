package mqtt

import (
	"crypto/cipher"
	"net"
	"sync"
	"time"
)

// UDPSession UDP会话，存储会话信息和加密密钥
type UDPSession struct {
	ID          string       // 会话唯一标识
	ConnID      string       // 4字节连接ID的hex字符串
	DeviceID    string       // 设备ID
	SessionID   string       // 会话ID
	AESKey      [16]byte     // AES-128密钥
	Nonce       [8]byte      // 8字节nonce模板（connID 4字节 + timestamp 4字节）
	RemoteAddr  *net.UDPAddr // 设备UDP地址
	LocalSeq    uint32       // 本地序列号（发送）
	RemoteSeq   uint32       // 远程序列号（接收）
	Block       cipher.Block // AES cipher block
	RecvChannel chan []byte  // 接收音频数据通道
	SendChannel chan []byte  // 发送音频数据通道
	CreatedAt   time.Time    // 创建时间
	LastActive  time.Time    // 最后活跃时间
	Status      string       // 会话状态：active/closed
	mu          sync.Mutex   // 保护并发访问
}

// incomingMsg 内部消息结构
type incomingMsg struct {
	messageType int
	data        []byte
}

// ClientHelloMessage 客户端Hello消息
type ClientHelloMessage struct {
	Type        string      `json:"type"`         // "hello"
	Version     int         `json:"version"`      // 协议版本
	Transport   string      `json:"transport"`    // "udp" 或 "mqtt"
	AudioParams AudioParams `json:"audio_params"` // 音频参数
}

// AudioParams 音频参数
type AudioParams struct {
	Format        string `json:"format"`         // "opus"
	SampleRate    int    `json:"sample_rate"`    // 采样率
	Channels      int    `json:"channels"`       // 声道数
	FrameDuration int    `json:"frame_duration"` // 帧时长(ms)
}

// ServerHelloUDPResponse 服务器Hello响应（UDP模式）
type ServerHelloUDPResponse struct {
	Type        string      `json:"type"`         // "hello"
	Version     int         `json:"version"`      // 协议版本
	SessionID   string      `json:"session_id"`   // 会话ID
	Transport   string      `json:"transport"`    // "udp"
	UDP         UDPInfo     `json:"udp"`          // UDP配置信息
	AudioParams AudioParams `json:"audio_params"` // 音频参数
}

// UDPInfo UDP配置信息
type UDPInfo struct {
	Server     string `json:"server"`     // UDP服务器地址
	Port       int    `json:"port"`       // UDP服务器端口
	Encryption string `json:"encryption"` // "aes-ctr"
	Key        string `json:"key"`        // AES密钥(hex)
	Nonce      string `json:"nonce"`      // 完整nonce(hex)
}

// ServerHelloMQTTResponse 服务器Hello响应（纯MQTT模式）
type ServerHelloMQTTResponse struct {
	Type        string      `json:"type"`         // "hello"
	Version     int         `json:"version"`      // 协议版本
	SessionID   string      `json:"session_id"`   // 会话ID
	Transport   string      `json:"transport"`    // "mqtt"
	AudioParams AudioParams `json:"audio_params"` // 音频参数
}
