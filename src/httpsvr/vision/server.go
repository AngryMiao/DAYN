package vision

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"angrymiao-ai-server/src/configs"
	"angrymiao-ai-server/src/configs/database"
	"angrymiao-ai-server/src/core/auth"
	"angrymiao-ai-server/src/core/image"
	"angrymiao-ai-server/src/core/middleware"
	"angrymiao-ai-server/src/core/providers"
	"angrymiao-ai-server/src/core/providers/vlllm"
	"angrymiao-ai-server/src/core/utils"
	"angrymiao-ai-server/src/models"

	"github.com/gin-gonic/gin"
)

const (
	// 最大文件大小为5MB
	MAX_FILE_SIZE = 5 * 1024 * 1024
)

type DefaultVisionService struct {
	logger   *utils.Logger
	config   *configs.Config
	vlllmMap map[string]*vlllm.Provider // 支持多个VLLLM provider
}

// NewDefaultVisionService 构造函数
func NewDefaultVisionService(config *configs.Config, logger *utils.Logger) (*DefaultVisionService, error) {
	service := &DefaultVisionService{
		logger:   logger,
		config:   config,
		vlllmMap: make(map[string]*vlllm.Provider),
	}

	// 初始化VLLLM providers
	if err := service.initVLLMProviders(); err != nil {
		return nil, fmt.Errorf("初始化VLLLM providers失败: %v", err)
	}

	return service, nil
}

// initVLLMProviders 初始化VLLLM providers
func (s *DefaultVisionService) initVLLMProviders() error {
	// 先看配置中的VLLLM provider
	selected_vlllm := s.config.SelectedModule["VLLLM"]
	if selected_vlllm == "" {
		s.logger.Warn("请设置好VLLLM provider配置")
		return fmt.Errorf("请设置好VLLLM provider配置")
	}

	vlllmConfig := s.config.VLLLM[selected_vlllm]

	// 创建VLLLM provider配置
	providerConfig := &vlllm.Config{
		Type:        vlllmConfig.Type,
		ModelName:   vlllmConfig.ModelName,
		BaseURL:     vlllmConfig.BaseURL,
		APIKey:      vlllmConfig.APIKey,
		Temperature: vlllmConfig.Temperature,
		MaxTokens:   vlllmConfig.MaxTokens,
		TopP:        vlllmConfig.TopP,
		Security:    vlllmConfig.Security,
	}

	// 创建provider实例
	provider, err := vlllm.NewProvider(providerConfig, s.logger)
	if err != nil {
		s.logger.Warn(fmt.Sprintf("创建VLLLM provider 失败: %v", err))

	}

	// 初始化provider
	if err := provider.Initialize(); err != nil {
		s.logger.Warn(fmt.Sprintf("初始化VLLLM provider失败: %v", err))

	}

	s.vlllmMap[selected_vlllm] = provider
	s.logger.Info(fmt.Sprintf("VLLLM provider %s 初始化成功", selected_vlllm))

	if len(s.vlllmMap) == 0 {
		s.logger.Error("没有可用的VLLLM provider，请检查配置")
		return fmt.Errorf("没有可用的VLLLM provider")
	}

	return nil
}

// Start 实现 VisionService 接口，注册所有 Vision 相关路由
func (s *DefaultVisionService) Start(ctx context.Context, engine *gin.Engine, apiGroup *gin.RouterGroup) {
	// Vision 主接口（GET用于状态检查，POST用于图片分析）

	visionGroup := apiGroup.Group("vision").Use(middleware.DeviceTokenAuth(
		auth.NewAuthToken(s.config.Server.Token),
		s.logger))
	{
		visionGroup.POST("", s.handlePost)
		visionGroup.POST("/upload/sign", s.handleUploadSign)
		visionGroup.POST("/upload/complete", s.handleUploadComplete)
	}

}

// 上传文件签名请求
func (s *DefaultVisionService) handleUploadSign(c *gin.Context) {
	var body BodyOSSSign
	if err := c.ShouldBindJSON(&body); err != nil {
		s.respondError(c, http.StatusBadRequest, "请求参数格式错误: "+err.Error())
		return
	}

	userID := c.GetUint("userID")
	encryptedID := utils.HashUserIDWithSalt(userID, s.config.Server.Token, 12)

	filename := utils.GenerateRandomKey(32)
	nowDate := time.Now().Format("20060102")

	dir := fmt.Sprintf("%s/%s/%s", encryptedID, body.FileType, nowDate)
	path := fmt.Sprint(dir, filename, "_", ".", body.FileSuffix)

	now := time.Now().Unix()
	expireEnd := now + s.config.OSS.Expiration
	var tokenExpire = time.Unix(expireEnd, 0).UTC().Format("2006-01-02T15:04:05Z")

	var config ConfigStruct
	config.Expiration = tokenExpire

	var condition []string
	condition = append(condition, "eq")
	condition = append(condition, "$key")
	condition = append(condition, path)
	config.Conditions = append(config.Conditions, condition)

	var policyToken PolicyToken

	// calculate signature
	result, err := json.Marshal(config)
	if err != err {
		s.logger.Warn(fmt.Sprintf("Vision请求解析失败: %v", err))
		s.respondError(c, http.StatusBadRequest, "请求参数格式错误: "+err.Error())
		return
	}
	deByte := base64.StdEncoding.EncodeToString(result)
	h := hmac.New(func() hash.Hash { return sha1.New() }, []byte(s.config.OSS.AccessKeySecret))
	_, _ = io.WriteString(h, deByte)

	signedStr := base64.StdEncoding.EncodeToString(h.Sum(nil))

	policyToken.AccessKeyId = s.config.OSS.AccessKeyID
	policyToken.Host = s.config.OSS.Host
	policyToken.Expire = expireEnd
	policyToken.Signature = signedStr
	policyToken.Path = path
	policyToken.Policy = deByte

	utils.Custom(c, http.StatusOK, UploadSignResponse{
		Success: true,
		Result:  policyToken,
		Message: "上传签名获取成功",
	})
}

// 上传完成回调，保存媒体信息
func (s *DefaultVisionService) handleUploadComplete(c *gin.Context) {
	var body BodyUploadComplete
	if err := c.ShouldBindJSON(&body); err != nil {
		s.respondError(c, http.StatusBadRequest, "请求参数格式错误: "+err.Error())
		return
	}

	userID := c.GetUint("userID")
	deviceID := c.GetHeader("Device-Id")

	// 生成可访问URL，如果客户端未传url，则用OSS Host + path
	url := body.URL
	if url == "" {
		host := strings.TrimRight(s.config.OSS.Host, "/")
		path := strings.TrimLeft(body.Path, "/")
		if host != "" && path != "" {
			url = host + "/" + path
		}
	}

	title := time.Now().Format("2006-01-02-15-04")
	rec := models.MediaUpload{
		UserID:          userID,
		DeviceID:        deviceID,
		Title:           title,
		FileType:        body.FileType,
		Path:            body.Path,
		URL:             url,
		Size:            body.Size,
		MimeType:        body.MimeType,
		DurationSeconds: body.DurationSeconds,
		Width:           body.Width,
		Height:          body.Height,
	}

	if err := database.GetDB().Create(&rec).Error; err != nil {
		s.respondError(c, http.StatusInternalServerError, "保存上传记录失败: "+err.Error())
		return
	}

	utils.Custom(c, http.StatusOK, UploadCompleteResponse{
		Success: true,
		Result: struct {
			ID  uint   `json:"id"`
			URL string `json:"url"`
		}{ID: rec.ID, URL: rec.URL},
		Message: "上传记录保存成功",
	})
}

// handlePost 处理POST请求（图片分析）
func (s *DefaultVisionService) handlePost(c *gin.Context) {
	deviceID := c.GetHeader("Device-Id")

	// 解析multipart表单
	req, err := s.parseMultipartRequest(c, deviceID)
	if err != nil {
		s.respondError(c, http.StatusBadRequest, err.Error())
		s.logger.Warn(fmt.Sprintf("Vision请求解析失败: %v", err))
		return
	}

	s.logger.Debug("收到Vision分析请求 %v", map[string]interface{}{
		"device_id":  req.DeviceID,
		"client_id":  req.ClientID,
		"question":   req.Question,
		"image_size": len(req.Image),
		"image_path": req.ImagePath,
		"file_type":  req.FileType,
		"url":        req.URL,
	})

	// 处理图片分析
	result, err := s.processVisionRequest(req)

	// 返回成功响应
	response := VisionResponse{
		Success: true,
		Result:  result,
	}

	if err != nil {
		s.respondError(c, http.StatusInternalServerError, err.Error())
		s.logger.Warn(fmt.Sprintf("Vision请求处理失败: %v", err))
		// 返回成功响应
		response.Success = false
		response.Message = err.Error()
		response.Result = "" // 清空结果
	}

	s.logger.Info("Vision分析结果%t: %s", response.Success, response.Result)
	utils.Custom(c, http.StatusOK, response)
}

// parseMultipartRequest 解析multipart表单请求
func (s *DefaultVisionService) parseMultipartRequest(c *gin.Context, deviceID string) (*VisionRequest, error) {
	// 解析multipart表单
	err := c.Request.ParseMultipartForm(MAX_FILE_SIZE)
	if err != nil {
		return nil, fmt.Errorf("解析multipart表单失败: %v", err)
	}

	// 打印所有的form字段
	s.logger.Info("解析到的form字段:")
	if c.Request.MultipartForm != nil {
		// 打印所有文本字段
		for key, values := range c.Request.MultipartForm.Value {
			s.logger.Info(fmt.Sprintf("文本字段 %s: %v", key, values))
		}
		// 打印所有文件字段
		for key, files := range c.Request.MultipartForm.File {
			s.logger.Info(fmt.Sprintf("文件字段 %s: 共%d个文件", key, len(files)))
			for i, file := range files {
				s.logger.Info(fmt.Sprintf("  文件%d: %s (大小: %d bytes)", i+1, file.Filename, file.Size))
			}
		}
	}

	// 获取question字段
	question := c.Request.FormValue("question")
	if question == "" {
		return nil, fmt.Errorf("缺少问题字段")
	}

	fileType := c.Request.FormValue("file_type")
	if fileType == "" {
		return nil, fmt.Errorf("缺少文件类型字段")
	}

	if fileType != "url" && fileType != "file" {
		return nil, fmt.Errorf("文件类型必须为url或file")
	}

	url := ""
	imageData := []byte{}
	saveImageToFile := ""

	if fileType == "url" {
		url = c.Request.FormValue("file_url")
		if url == "" {
			return nil, fmt.Errorf("缺少URL字段")
		}
	}

	if fileType == "file" {
		// 获取图片文件
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			return nil, fmt.Errorf("缺少图片文件: %v", err)
		}
		defer file.Close()

		// 检查文件大小
		if header.Size > MAX_FILE_SIZE {
			return nil, fmt.Errorf("图片大小超过限制，最大允许%dMB", MAX_FILE_SIZE/1024/1024)
		}

		// 读取图片数据
		imageData, err = io.ReadAll(file)
		if err != nil {
			return nil, fmt.Errorf("读取图片数据失败: %v", err)
		}

		if len(imageData) == 0 {
			return nil, fmt.Errorf("图片数据为空")
		}

		// 验证图片格式
		if !s.isValidImageFile(imageData) {
			return nil, fmt.Errorf("不支持的文件格式，请上传有效的图片文件（支持JPEG、PNG、GIF、BMP、TIFF、WEBP格式）")
		}

		// 将图片保存在本地
		saveImageToFile, err = s.saveImageToFile(imageData, deviceID)
		if err != nil {
			return nil, fmt.Errorf("保存图片文件失败(%s): %v", saveImageToFile, err)
		}
	}

	return &VisionRequest{
		Question:  question,
		Image:     imageData,
		DeviceID:  deviceID,
		ClientID:  c.GetHeader("Client-Id"),
		ImagePath: saveImageToFile, // 保存的图片路径
		FileType:  fileType,
		URL:       url,
	}, nil
}

func (s *DefaultVisionService) saveImageToFile(imageData []byte, deviceID string) (string, error) {
	// 生成唯一的文件名
	device_id_format := strings.ReplaceAll(deviceID, ":", "_")
	filename := fmt.Sprintf("%s_%d.%s", device_id_format, time.Now().Unix(), s.detectImageFormat(imageData))
	filepath := fmt.Sprintf("uploads/%s", filename)

	// 确保uploads目录存在
	if err := os.MkdirAll("uploads", os.ModePerm); err != nil {
		return "", fmt.Errorf("创建uploads目录失败: %v", err)
	}

	// 保存图片文件
	if err := os.WriteFile(filepath, imageData, 0644); err != nil {
		return "", fmt.Errorf("保存图片文件失败: %v", err)
	}

	s.logger.Info(fmt.Sprintf("图片已保存到: %s", filepath))
	return filepath, nil
}

// processVisionRequest 处理视觉分析请求
func (s *DefaultVisionService) processVisionRequest(req *VisionRequest) (string, error) {
	// 选择VLLLM provider
	provider := s.selectProvider("")
	if provider == nil {
		return "", fmt.Errorf("没有可用的视觉分析模型")
	}

	imageData := image.ImageData{}
	if req.FileType == "url" {
		imageData.URL = req.URL
	}

	if req.FileType == "file" {
		// 将图片转换为base64
		imageBase64 := base64.StdEncoding.EncodeToString(req.Image)

		imageData.Data = imageBase64
		imageData.Format = s.detectImageFormat(req.Image)
	}

	// 调用VLLLM provider
	messages := []providers.Message{} // 空的历史消息
	responseChan, err := provider.ResponseWithImage(context.Background(), "", messages, imageData, req.Question)
	if err != nil {
		return "", fmt.Errorf("调用VLLLM失败: %v", err)
	}

	// 收集所有响应内容
	var result strings.Builder
	for content := range responseChan {
		result.WriteString(content)
	}
	s.logger.Info(fmt.Sprintf("VLLLM分析结果: %s", result.String()))

	return result.String(), nil
}

// selectProvider 选择VLLLM provider
func (s *DefaultVisionService) selectProvider(modelName string) *vlllm.Provider {
	// 如果指定了模型名，尝试找到对应的provider
	if modelName != "" {
		if provider, exists := s.vlllmMap[modelName]; exists {
			return provider
		}
	}

	// 否则返回第一个可用的provider
	for _, provider := range s.vlllmMap {
		return provider
	}

	return nil
}

// isValidImageFile 检查是否为有效的图片文件
func (s *DefaultVisionService) isValidImageFile(data []byte) bool {
	if len(data) < 8 {
		return false
	}

	// 检查常见图片格式的文件头
	return s.hasJPEGHeader(data) ||
		s.hasPNGHeader(data) ||
		s.hasGIFHeader(data) ||
		s.hasBMPHeader(data) ||
		s.hasWebPHeader(data)
}

// hasJPEGHeader 检查JPEG文件头
func (s *DefaultVisionService) hasJPEGHeader(data []byte) bool {
	return len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8
}

// hasPNGHeader 检查PNG文件头
func (s *DefaultVisionService) hasPNGHeader(data []byte) bool {
	return len(data) >= 8 &&
		data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 &&
		data[4] == 0x0D && data[5] == 0x0A && data[6] == 0x1A && data[7] == 0x0A
}

// hasGIFHeader 检查GIF文件头
func (s *DefaultVisionService) hasGIFHeader(data []byte) bool {
	return len(data) >= 6 &&
		((data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 && data[4] == 0x37 && data[5] == 0x61) ||
			(data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 && data[4] == 0x39 && data[5] == 0x61))
}

// hasBMPHeader 检查BMP文件头
func (s *DefaultVisionService) hasBMPHeader(data []byte) bool {
	return len(data) >= 2 && data[0] == 0x42 && data[1] == 0x4D
}

// hasWebPHeader 检查WebP文件头
func (s *DefaultVisionService) hasWebPHeader(data []byte) bool {
	return len(data) >= 12 &&
		data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50
}

// detectImageFormat 检测图片格式
func (s *DefaultVisionService) detectImageFormat(data []byte) string {
	if s.hasJPEGHeader(data) {
		return "jpeg"
	}
	if s.hasPNGHeader(data) {
		return "png"
	}
	if s.hasGIFHeader(data) {
		return "gif"
	}
	if s.hasBMPHeader(data) {
		return "bmp"
	}
	if s.hasWebPHeader(data) {
		return "webp"
	}
	return "jpeg" // 默认格式
}

// addCORSHeaders 添加CORS头
func (s *DefaultVisionService) addCORSHeaders(c *gin.Context) {
	c.Header("Access-Control-Allow-Headers", "client-id, content-type, device-id, authorization")
	c.Header("Access-Control-Allow-Credentials", "true")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
}

// respondError 返回错误响应
func (s *DefaultVisionService) respondError(c *gin.Context, statusCode int, message string) {
	response := VisionResponse{
		Success: false,
		Message: message,
	}
	utils.Custom(c, statusCode, response)
}

// Cleanup 清理资源
func (s *DefaultVisionService) Cleanup() error {
	for name, provider := range s.vlllmMap {
		if err := provider.Cleanup(); err != nil {
			s.logger.Warn(fmt.Sprintf("清理VLLLM provider %s 失败: %v", name, err))
		}
	}
	s.logger.Info("Vision服务清理完成")
	return nil
}
