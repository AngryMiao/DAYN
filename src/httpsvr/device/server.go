package device

import (
	"context"
	"net/http"
	"strings"
	"time"

	"angrymiao-ai-server/src/configs"
	"angrymiao-ai-server/src/core/auth"
	"angrymiao-ai-server/src/core/auth/am_token"
	"angrymiao-ai-server/src/core/utils"

	"github.com/gin-gonic/gin"
)

type DefaultDeviceService struct {
	logger    *utils.Logger
	config    *configs.Config
	authToken *auth.AuthToken
	deviceDB  *DeviceDB
}

// NewDefaultDeviceService 构造函数
func NewDefaultDeviceService(config *configs.Config, logger *utils.Logger) *DefaultDeviceService {
	// 初始化AuthToken，使用配置中的 topic_root
	topicRoot := config.Transport.Mqtt.TopicRoot
	if topicRoot == "" {
		topicRoot = "am_topic" // 默认值
	}
	authToken := auth.NewAuthTokenWithConfig(config.Server.Token, topicRoot)

	// 初始化设备绑定数据库操作
	deviceDB := NewDeviceDB()

	return &DefaultDeviceService{
		logger:    logger,
		config:    config,
		authToken: authToken,
		deviceDB:  deviceDB,
	}
}

// Start 注册 Device 相关路由
func (s *DefaultDeviceService) Start(ctx context.Context, engine *gin.Engine, apiGroup *gin.RouterGroup) {
	apiGroup.POST("/device/bind", s.handleDeviceBind)
	apiGroup.POST("/device/unbind", s.handleDeviceUnbind)
	apiGroup.GET("/device/refresh_token", s.handleDeviceRefToken)
}

// @Summary 刷新设备token
// @Description 刷新设备的JWT token，延长有效期
// @Tags Device
// @Accept json
// @Produce json
// @Param device_id query string true "设备ID"
// @Param token query string true "当前设备token"
// @Param bind_key query string true "设备绑定密钥"
// @Success 200 {object} RefreshTokenResponse "刷新成功"
// @Failure 400 {object} RefreshTokenResponse "请求参数错误"
// @Failure 401 {object} RefreshTokenResponse "认证失败"
// @Failure 500 {object} RefreshTokenResponse "服务器内部错误"
// @Router /device/refresh_token [get]
func (s *DefaultDeviceService) handleDeviceRefToken(c *gin.Context) {
	// 获取get请求参数
	deviceID := c.Query("device_id")
	if deviceID == "" {
		s.respondError(c, http.StatusBadRequest, "设备ID不能为空")
		return
	}

	token := c.Query("token")
	if token == "" {
		s.respondError(c, http.StatusBadRequest, "token不能为空")
		return
	}

	bindKey := c.Query("bind_key")
	if bindKey == "" {
		s.respondError(c, http.StatusBadRequest, "bind_key不能为空")
		return
	}

	deviceInfo, err := s.deviceDB.GetDevice(deviceID)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "设备不存在")
		return
	}

	// 验证绑定密钥
	if deviceInfo.BindKey != bindKey {
		s.respondError(c, http.StatusInternalServerError, "绑定密钥验证失败")
		return
	}

	// 使用Casbin进行JWT token验证

	_, tokenDeviceID, tokenUserID, err := s.authToken.VerifyToken(token, true)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "token验证失败: "+err.Error())
		return
	}

	if tokenDeviceID != deviceID || tokenUserID != deviceInfo.UserID {
		s.respondError(c, http.StatusInternalServerError, "token设备ID与请求设备ID不匹配")
		return
	}

	// 生成新的7天有效期token
	sevenDays := 7 * 24 * time.Hour
	newToken, err := s.authToken.GenerateTokenWithExpiry(deviceInfo.UserID, deviceID, sevenDays)
	if err != nil {
		s.logger.Error("生成新token失败: %v", err)
		s.respondError(c, http.StatusInternalServerError, "生成新token失败")
		return
	}

	s.logger.Info("刷新设备token成功 - 用户ID: %d, 设备ID: %s", deviceInfo.UserID, deviceID)

	// 返回成功响应
	utils.Custom(c, http.StatusOK, RefreshTokenResponse{
		Success: true,
		Token:   newToken,
	})
}

// @Summary 设备绑定
// @Description 绑定设备到用户账户，生成设备密钥和token
// @Tags Device
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer <UserJWT>"
// @Param body body BindDeviceRequest true "设备绑定请求"
// @Success 200 {object} BindDeviceResponse "绑定成功"
// @Failure 400 {object} BindDeviceResponse "请求参数错误"
// @Failure 401 {object} BindDeviceResponse "认证失败"
// @Failure 500 {object} BindDeviceResponse "服务器内部错误"
// @Router /device/bind [post]
func (s *DefaultDeviceService) handleDeviceBind(c *gin.Context) {
	// 验证认证
	authHeader := c.GetHeader("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		s.respondError(c, http.StatusUnauthorized, "无效的认证token或token已过期")
		return
	}

	token := authHeader[7:] // 移除"Bearer "前缀

	// 使用Casbin进行JWT token验证
	claims, err := am_token.ParseToken(token)
	if err != nil {
		s.respondError(c, http.StatusUnauthorized, "token验证失败: "+err.Error())
		return
	}

	// 从claims中获取用户ID
	userID := uint(claims.UserID)

	// 解析请求体
	var body BindDeviceRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		s.respondError(c, http.StatusBadRequest, "请求参数格式错误: "+err.Error())
		return
	}

	// 验证设备ID
	if !ValidateDeviceID(body.DeviceID) {
		s.respondError(c, http.StatusBadRequest, "设备ID格式无效")
		return
	}

	// 是否绑定过设备
	exists, err := s.deviceDB.IsDeviceBound(body.DeviceID)
	if err != nil {
		s.logger.Error("检查设备绑定失败: %v", err)
		s.respondError(c, http.StatusInternalServerError, "检查设备绑定失败")
		return
	}
	if exists {
		s.respondError(c, http.StatusBadRequest, "设备已绑定")
		return
	}

	// 生成绑定密钥，使用HMAC(device_id, server_secret)算法
	bindKey := GenerateBindKey(body.DeviceID, s.config.Server.Token, userID)

	// 生成7天有效期的token
	sevenDays := 7 * 24 * time.Hour
	deviceToken, err := s.authToken.GenerateTokenWithExpiry(userID, body.DeviceID, sevenDays)
	if err != nil {
		s.logger.Error("生成设备token失败: %v", err)
		s.respondError(c, http.StatusInternalServerError, "生成设备token失败")
		return
	}

	// 保存绑定信息到数据库
	if err := s.deviceDB.SaveDevice(body.DeviceID, userID, bindKey); err != nil {
		s.logger.Error("保存设备绑定信息失败: %v", err)
		s.respondError(c, http.StatusInternalServerError, "保存绑定信息失败")
		return
	}

	s.logger.Info("设备绑定成功 - 用户ID: %d, 设备ID: %s", userID, body.DeviceID)

	// 返回成功响应
	utils.Custom(c, http.StatusOK, BindDeviceResponse{
		Success:   true,
		DeviceKey: bindKey,
		Token:     deviceToken,
	})
}

// handleDeviceUnbind 处理设备解绑请求
// @Summary 设备解绑
// @Description 解绑指定设备，将数据库中is_active字段设置为false
// @Tags Device
// @Accept json
// @Produce json
// @Param body body UnbindDeviceRequest true "设备解绑请求"
// @Success 200 {object} UnbindDeviceResponse "解绑成功"
// @Failure 400 {object} UnbindDeviceResponse "请求参数错误"
// @Failure 404 {object} UnbindDeviceResponse "设备未找到"
// @Failure 500 {object} UnbindDeviceResponse "服务器内部错误"
// @Router /device/unbind [post]
func (s *DefaultDeviceService) handleDeviceUnbind(c *gin.Context) {
	// 解析请求体
	var body UnbindDeviceRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		s.logger.Error("解绑请求参数解析失败: %v", err)
		s.respondUnbindError(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	// 验证设备ID格式
	if !ValidateDeviceID(body.DeviceID) {
		s.logger.Error("无效的设备ID格式: %s", body.DeviceID)
		s.respondUnbindError(c, http.StatusBadRequest, "设备ID格式无效")
		return
	}

	// 检查设备是否存在且已绑定
	existingBind, err := s.deviceDB.GetDevice(body.DeviceID)
	if err != nil {
		s.logger.Error("查询设备绑定信息失败: %v", err)
		s.respondUnbindError(c, http.StatusNotFound, "设备未找到或未绑定")
		return
	}

	if existingBind.BindKey != body.BindKey {
		s.respondUnbindError(c, http.StatusBadRequest, "绑定key错误")
		return
	}

	// 执行解绑操作
	if err := s.deviceDB.UnbindDevice(body.DeviceID, body.BindKey); err != nil {
		s.logger.Error("设备解绑失败: %v", err)
		s.respondUnbindError(c, http.StatusInternalServerError, "解绑操作失败")
		return
	}

	s.logger.Info("设备解绑成功: %s (用户ID: %d)", body.DeviceID, existingBind.UserID)

	// 返回成功响应
	utils.Custom(c, http.StatusOK, UnbindDeviceResponse{
		Success: true,
		Message: "设备解绑成功",
	})
}

// respondUnbindError 返回解绑错误响应
func (s *DefaultDeviceService) respondUnbindError(c *gin.Context, statusCode int, message string) {
	utils.Custom(c, statusCode, UnbindDeviceResponse{
		Success: false,
		Message: message,
	})
}

// respondError 返回错误响应
func (s *DefaultDeviceService) respondError(c *gin.Context, statusCode int, message string) {
	utils.Custom(c, statusCode, BindDeviceResponse{
		Success: false,
		Message: message,
	})
}
