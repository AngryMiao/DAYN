package models

import (
	//"gorm.io/gorm"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// 系统全局配置（只保存一条记录）
type SystemConfig struct {
	ID               uint `gorm:"primaryKey"`
	SelectedASR      string
	SelectedTTS      string
	SelectedLLM      string
	SelectedVLLLM    string
	Prompt           string         `gorm:"type:text"`
	QuickReplyWords  datatypes.JSON // 存储为 JSON 数组
	DeleteAudio      bool
	UsePrivateConfig bool
}

// 用户
type User struct {
	ID       uint   `gorm:"primaryKey"`
	Username string `gorm:"uniqueIndex;not null"`
	Password string // 建议加密
	Role     string // 可选值：admin/user
	Setting  UserSetting
}

// 用户设置
type UserSetting struct {
	ID              uint `gorm:"primaryKey"`
	UserID          uint `gorm:"uniqueIndex"` // 一对一
	SelectedASR     string
	SelectedTTS     string
	SelectedLLM     string
	SelectedVLLLM   string
	PromptOverride  string `gorm:"type:text"`
	QuickReplyWords datatypes.JSON
}

// 模块配置（可选）
type ModuleConfig struct {
	ID          uint   `gorm:"primaryKey"`
	Name        string `gorm:"uniqueIndex;not null"` // 模块名
	Type        string
	ConfigJSON  datatypes.JSON
	Public      bool
	Description string
	Enabled     bool
}

type ServerConfig struct {
	ID     uint   `gorm:"primaryKey"`
	CfgStr string `gorm:"type:text"`
}

type Device struct {
	ID               uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	DeviceID         string         `gorm:"type:varchar(64);not null;uniqueIndex:uniq_device_binding" json:"device_id"`
	UserID           uint           `gorm:"not null;index" json:"user_id"`
	BindKey          string         `gorm:"type:varchar(512);not null" json:"binding_key"`
	IsActive         bool           `gorm:"not null;default:true" json:"is_active"`
	AgentID          *uint          `gorm:"index"                                  json:"agentID"`          // 外键关联 Agent
	Name             string         `gorm:"not null"                               json:"name"`             // 设备名称
	MacAddress       string         `gorm:"type:varchar(255);uniqueIndex;not null" json:"macAddress"`       // mac地址
	ClientID         string         `gorm:"type:varchar(255);uniqueIndex;not null" json:"clientId"`         // 客户端唯一标识
	Version          string         `                                              json:"version"`          // 设备固件版本号
	OTA              bool           `gorm:"default:true"                           json:"ota"`              // 是否支持OTA升级
	RegisterTime     int64          `                                              json:"-"`                // 注册时间戳
	LastActiveTime   int64          `                                              json:"-"`                // 最后活跃时间戳
	RegisterTimeV2   time.Time      `                                              json:"registerTimeV2"`   // 注册时间
	LastActiveTimeV2 time.Time      `                                              json:"lastActiveTimeV2"` // 最后活跃时间
	Online           bool           `                                              json:"online"`           // 在线状态
	AuthCode         string         `                                              json:"authCode"`         // 认证码
	AuthStatus       string         `                                              json:"authStatus"`       // 认证状态，可选值：pending
	BoardType        string         `                                              json:"boardType"`        // 主板类型
	ChipModelName    string         `                                              json:"chipModelName"`    // 芯片型号名称
	Channel          int            `                                              json:"channel"`          // WiFi 频道
	SSID             string         `                                              json:"ssid"`             // WiFi SSID
	Application      string         `                                              json:"application"`      // 应用名称
	Language         string         `gorm:"default:'zh-CN'"                        json:"language"`         // 语言，默认为中文
	DeviceCode       string         `                                              json:"deviceCode"`       // 设备码，简化版的设备唯一标识
	DeletedAt        gorm.DeletedAt `gorm:"index"                                  json:"-"`                // 软删除字段
	Extra            string         `gorm:"type:text"                              json:"extra"`            // 额外信息，JSON格式
	Conversationid   string         `                                              json:"conversationId"`   // 关联的对话AgentDialog的ID
	Mode             string         `                                              json:"mode"`             // 模式:chat/listen/ban
	CreateAt         time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdateAt         time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// 媒体上传记录表
type MediaUpload struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	UserID          uint           `gorm:"index" json:"user_id"`
	DeviceID        string         `gorm:"index;type:varchar(255)" json:"device_id"`
	FileType        string         `gorm:"type:varchar(32)" json:"file_type"`
	Title           string         `gorm:"type:varchar(255)" json:"title"`
	Path            string         `gorm:"type:text" json:"path"`
	URL             string         `gorm:"type:text" json:"url"`
	Size            int64          `json:"size"`
	MimeType        string         `gorm:"type:varchar(128)" json:"mime_type"`
	DurationSeconds *float64       `json:"duration_seconds,omitempty"`
	Width           *int           `json:"width,omitempty"`
	Height          *int           `json:"height,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// AudioTask 状态常量
const (
	AudioTaskStatusProcessing = "processing"
	AudioTaskStatusCompleted  = "completed"
	AudioTaskStatusFailed     = "failed"
)

// 音频文件识别任务表
type AudioTask struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	UserID     uint           `gorm:"index" json:"user_id"`
	DeviceID   string         `gorm:"index;type:varchar(255)" json:"device_id"`
	MediaID    uint           `gorm:"index" json:"media_id"`
	AucType    string         `json:"auc_type"`
	AucTaskID  string         `gorm:"uniqueIndex" json:"auc_task_id"`
	Text       string         `gorm:"type:text" json:"text"`
	Status     string         `gorm:"type:varchar(20);default:'processing';check:status IN ('processing','completed','failed')" json:"status"`
	ResultJSON datatypes.JSON `gorm:"type:json" json:"result_json,omitempty"` // 保存完整的识别结果（包含 utterances、words 等）
	Summary    string         `json:"summary"`
	KeyPoints  datatypes.JSON `json:"key_points"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}
