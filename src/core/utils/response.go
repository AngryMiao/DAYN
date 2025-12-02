package utils

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// UnifiedResponse 统一响应结构体
type UnifiedResponse struct {
	Code    int         `json:"code"`              // HTTP状态码
	Success bool        `json:"success"`           // 是否成功
	Message string      `json:"message,omitempty"` // 消息描述
	Data    interface{} `json:"data,omitempty"`    // 数据负载
	Error   string      `json:"error,omitempty"`   // 错误详情
}

// Success 返回成功响应
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, UnifiedResponse{
		Code:    http.StatusOK,
		Success: true,
		Message: "操作成功",
		Data:    data,
	})
}

// SuccessWithMessage 返回带自定义消息的成功响应
func SuccessWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, UnifiedResponse{
		Code:    http.StatusOK,
		Success: true,
		Message: message,
		Data:    data,
	})
}

// Error 返回错误响应
func Error(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, UnifiedResponse{
		Code:    statusCode,
		Success: false,
		Message: message,
	})
}

// ErrorWithDetail 返回带详细错误信息的错误响应
func ErrorWithDetail(c *gin.Context, statusCode int, message string, err error) {
	resp := UnifiedResponse{
		Code:    statusCode,
		Success: false,
		Message: message,
	}
	if err != nil {
		resp.Error = err.Error()
	}
	c.JSON(statusCode, resp)
}

// Custom 返回自定义响应（保持向后兼容）
// 使用此方法可以包装现有的返回结构，不改变其内部格式
func Custom(c *gin.Context, statusCode int, data interface{}) {
	success := statusCode >= 200 && statusCode < 300
	c.JSON(statusCode, UnifiedResponse{
		Code:    statusCode,
		Success: success,
		Data:    data,
	})
}

// CustomWithMessage 返回带消息的自定义响应
func CustomWithMessage(c *gin.Context, statusCode int, message string, data interface{}) {
	success := statusCode >= 200 && statusCode < 300
	c.JSON(statusCode, UnifiedResponse{
		Code:    statusCode,
		Success: success,
		Message: message,
		Data:    data,
	})
}
