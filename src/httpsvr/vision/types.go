package vision

// VisionRequest Vision分析请求结构（从multipart表单解析）
type VisionRequest struct {
	Question  string // 问题文本（从表单字段获取）
	Image     []byte // 图片数据（从文件字段获取）
	DeviceID  string // 设备ID（从请求头获取）
	ClientID  string // 客户端ID（从请求头获取）
	ImagePath string // 图片路径
	FileType  string // 文件类型（url或file）
	URL       string // 文件URL（当FileType为url时）
}

// VisionResponse Vision标准响应结构
type VisionResponse struct {
	Success bool   `json:"success"`           // 是否成功
	Result  string `json:"result,omitempty"`  // 分析结果（成功时）
	Message string `json:"message,omitempty"` // 错误信息（失败时）
}

// VisionStatusResponse Vision状态响应结构
type VisionStatusResponse struct {
	Message string // 状态信息（纯文本）
}

// AuthVerifyResult 认证验证结果
type AuthVerifyResult struct {
	IsValid  bool
	DeviceID string
	UserID   uint
}

type BodyOSSSign struct {
	FileType   string `json:"file_type" binding:"required,oneof=image video audio"`
	FileSuffix string `json:"file_suffix" binding:"required"`
}

type PolicyToken struct {
	AccessKeyId string `json:"access_id"`
	Host        string `json:"host"`
	Expire      int64  `json:"expire"`
	Signature   string `json:"signature"`
	Policy      string `json:"policy"`
	Path        string `json:"path"`
}

type ConfigStruct struct {
	Expiration string     `json:"expiration"`
	Conditions [][]string `json:"conditions"`
}

type UploadSignResponse struct {
	Success bool        `json:"success"`
	Result  PolicyToken `json:"result"`
	Message string      `json:"message"`
}

// 客户端上传完成后上报的请求体
type BodyUploadComplete struct {
	FileType        string   `json:"file_type" binding:"required,oneof=image video audio"` // 文件类型
	Path            string   `json:"path" binding:"required"`                              // 文件路径
	URL             string   `json:"url"`                                                  // 可选；若未提供则以OSS Host + path 生成
	Size            int64    `json:"size"`                                                 // 文件大小
	MimeType        string   `json:"mime_type"`                                            // 文件MIME类型
	DurationSeconds *float64 `json:"duration_seconds"`                                     // 视频时长
	Width           *int     `json:"width"`                                                // 图片宽度
	Height          *int     `json:"height"`                                               // 图片高度
}

type UploadCompleteResponse struct {
	Success bool   `json:"success"`
	Result  any    `json:"result"`
	Message string `json:"message"`
}
