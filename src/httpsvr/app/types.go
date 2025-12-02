package app

import (
	"angrymiao-ai-server/src/core/chat"
	"angrymiao-ai-server/src/models"
)

type GetDevicesResponse struct {
	Success bool            `json:"success"`
	Devices []DeviceSummary `json:"devices"`
	Message string          `json:"message,omitempty"`
}

type DeviceSummary struct {
	DeviceID string `json:"device_id"`
	Name     string `json:"name,omitempty"`
	Online   bool   `json:"online"`
}

type ChatSendRequest struct {
	Text  string `json:"text" binding:"required"`
	BotID *uint  `json:"bot_id" binding:"omitempty"`
}

type ChatSendResponse struct {
	Success   bool     `json:"success"`
	Message   string   `json:"message,omitempty"`
	Reply     string   `json:"reply,omitempty"`
	ErrorCode string   `json:"error_code,omitempty"`
	BotID     *uint    `json:"bot_id,omitempty"`
	BotConfig *BotInfo `json:"bot_config,omitempty"`
}

type BotInfo struct {
	Name      string `json:"name,omitempty"`
	Type      string `json:"type,omitempty"`
	ModelName string `json:"model_name,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
}

type ChatHistoryResponse struct {
	Success  bool           `json:"success"`
	Message  string         `json:"message,omitempty"`
	Messages []chat.Message `json:"messages"`
	Total    int            `json:"total"`
	Page     int            `json:"page,omitempty"`
	PageSize int            `json:"page_size,omitempty"`
}

// MediaWithTask 媒体文件及其关联的识别任务
type MediaWithTask struct {
	models.MediaUpload
	// 音频识别任务信息（仅当 file_type=audio 时有值）
	TaskID        string   `json:"task_id"`
	TaskStatus    string   `json:"task_status"`     // processing/completed/failed
	TaskText      string   `json:"task_text"`       // 识别结果文本
	TaskSummary   string   `json:"task_summary"`    // AI生成的摘要
	TaskKeyPoints []string `json:"task_key_points"` // AI提取的关键点
}

type GetHomeMediaResponse struct {
	Success  bool            `json:"success"`
	List     []MediaWithTask `json:"list"`
	Message  string          `json:"message,omitempty"`
	Total    int64           `json:"total,omitempty"`
	Page     int             `json:"page,omitempty"`
	PageSize int             `json:"page_size,omitempty"`
}

type RecognitionRequest struct {
	MediaID uint `json:"media_id" binding:"required"`
}

type RecognitionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	TaskID  string `json:"task_id,omitempty"`
}

type AUCCallbackRequest struct {
	Resp struct {
		ID         string      `json:"id"`
		Code       int         `json:"code"`
		Message    string      `json:"message,omitempty"`
		Text       string      `json:"text,omitempty"`
		Utterances []Utterance `json:"utterances,omitempty"`
	} `json:"resp"`
}

type Utterance struct {
	Text      string            `json:"text"`
	StartTime int               `json:"start_time"`
	EndTime   int               `json:"end_time"`
	Words     []Word            `json:"words,omitempty"`
	Additions map[string]string `json:"additions,omitempty"`
}

type Word struct {
	Text      string `json:"text"`
	StartTime int    `json:"start_time"`
	EndTime   int    `json:"end_time"`
}
