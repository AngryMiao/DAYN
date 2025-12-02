package device

// BindDeviceRequest 设备绑定请求
type BindDeviceRequest struct {
	DeviceID string `json:"device_id"`
}

// BindDeviceResponse 设备绑定响应
type BindDeviceResponse struct {
	Success   bool   `json:"success"`              // 是否成功
	DeviceKey string `json:"device_key,omitempty"` // 设备长期密钥S（明文，仅下发一次）
	Token     string `json:"token,omitempty"`      // 30天有效期DeviceToken（成功时）
	Message   string `json:"message,omitempty"`    // 错误信息（失败时）
}

// UnbindDeviceRequest 设备解绑请求
type UnbindDeviceRequest struct {
	DeviceID string `json:"device_id" binding:"required"`
	BindKey  string `json:"bind_key" binding:"required"`
}

// UnbindDeviceResponse 设备解绑响应
type UnbindDeviceResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

type RefreshTokenResponse struct {
	Success bool   `json:"success"`
	Token   string `json:"token,omitempty"`
	Message string `json:"message,omitempty"`
}
