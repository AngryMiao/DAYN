package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"angrymiao-ai-server/src/configs"
	"angrymiao-ai-server/src/core/middleware"
	"angrymiao-ai-server/src/core/providers/llm"
	"angrymiao-ai-server/src/core/types"
	"angrymiao-ai-server/src/core/utils"
	"angrymiao-ai-server/src/models"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// BotConfigHandler Bot配置处理器
type BotConfigHandler struct {
	botService    BotConfigService
	modelService  ModelConfigService
	friendService interface {
		IsBotAdded(ctx context.Context, userID uint, botConfigID uint) (bool, error)
	}
	logger *utils.Logger
}

// NewBotConfigHandler 创建Bot配置处理器
func NewBotConfigHandler(db *gorm.DB, logger *utils.Logger, friendService interface {
	IsBotAdded(ctx context.Context, userID uint, botConfigID uint) (bool, error)
}) *BotConfigHandler {
	return &BotConfigHandler{
		botService:    NewBotConfigService(db, logger),
		modelService:  NewModelConfigService(db, logger),
		friendService: friendService,
		logger:        logger,
	}
}

// RegisterRoutes 注册Bot配置路由
func (h *BotConfigHandler) RegisterRoutes(apiGroup *gin.RouterGroup) {
	botGroup := apiGroup.Group("/bots").Use(middleware.AmTokenJWTUserAuth())
	{
		botGroup.POST("", h.CreateBotConfig)
		botGroup.GET("/:id", h.GetBotConfig)
		botGroup.PUT("/:id", h.UpdateBotConfig)
		botGroup.DELETE("/:id", h.DeleteBotConfig)
		botGroup.GET("/search", h.SearchBots)
		botGroup.GET("/my", h.GetMyBots)
	}
}

// CreateBotConfig 创建Bot配置
// @Summary 创建Bot配置
// @Description 创建新的Bot配置
// @Tags Bot配置管理
// @Accept json
// @Produce json
// @Param config body models.CreateBotConfigRequest true "配置信息"
// @Success 201 {object} map[string]interface{} "创建成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/v2/bots [post]
func (h *BotConfigHandler) CreateBotConfig(c *gin.Context) {
	userID := h.getUserID(c)

	var req models.CreateBotConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, http.StatusBadRequest, "请求参数格式错误", err)
		return
	}

	// 验证模型配置是否存在
	_, err := h.modelService.GetModelConfigByID(c.Request.Context(), req.ModelID)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "模型配置不存在", err)
		return
	}

	// 生成bot_hash（使用 FunctionName 作为 Bot 名称）
	botHash, err := h.botService.GenerateBotHash(userID, req.FunctionName)
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "生成Bot Hash失败", err)
		return
	}

	// 设置默认可见性
	visibility := req.Visibility
	if visibility == "" {
		visibility = "private"
	}

	// 验证可见性值
	if visibility != "private" && visibility != "public" {
		h.respondError(c, http.StatusBadRequest, "无效的可见性值", nil)
		return
	}

	// 设置默认Bot类型
	botType := req.BotType
	if botType == "" {
		botType = "llm"
	}

	// 验证Bot类型
	if botType != "llm" && botType != "image" && botType != "tts" && botType != "asr" {
		h.respondError(c, http.StatusBadRequest, "无效的Bot类型，必须是 llm/image/tts/asr 之一", nil)
		return
	}

	// 构建Bot配置对象
	config := &models.BotConfig{
		CreatorID:       userID,
		BotHash:         botHash,
		Visibility:      visibility,
		ModelID:         req.ModelID,
		BotType:         botType,
		RequiresNetwork: req.RequiresNetwork,
		MaxTokens:       req.MaxTokens,
		Temperature:     req.Temperature,
		FunctionName:    req.FunctionName,
		Description:     req.Description,
		MCPServerURL:    req.MCPServerURL,
	}

	// 处理参数JSON
	if req.Parameters != nil {
		parametersJSON, err := json.Marshal(req.Parameters)
		if err != nil {
			h.respondError(c, http.StatusBadRequest, "参数格式错误", err)
			return
		}
		config.Parameters = datatypes.JSON(parametersJSON)
	}

	if req.Parameters == nil {
		go h.generateLLMFunctionParameters(config, config.FunctionName, config.Description)
	}

	if err := h.botService.CreateBotConfig(c.Request.Context(), config); err != nil {
		h.respondError(c, http.StatusInternalServerError, "创建Bot配置失败", err)
		return
	}

	h.logger.Info("用户 %d 创建Bot配置成功: %s (ID: %d)", userID, config.FunctionName, config.ID)

	c.JSON(http.StatusCreated, gin.H{
		"code":    201,
		"message": "Bot配置创建成功",
		"data":    config.ToResponse(),
	})
}

// GetBotConfig 获取Bot配置详情
// @Summary 获取Bot配置详情
// @Description 根据ID获取Bot配置详情
// @Tags Bot配置管理
// @Accept json
// @Produce json
// @Param id path int true "Bot配置ID"
// @Success 200 {object} map[string]interface{} "成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 404 {object} map[string]interface{} "配置不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/v2/bots/{id} [get]
func (h *BotConfigHandler) GetBotConfig(c *gin.Context) {
	userID := h.getUserID(c)
	configID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "无效的配置ID", err)
		return
	}

	config, err := h.botService.GetBotConfigByID(c.Request.Context(), uint(configID))
	if err != nil {
		if err.Error() == "Bot配置不存在" {
			h.respondError(c, http.StatusNotFound, "Bot配置不存在", err)
		} else {
			h.respondError(c, http.StatusInternalServerError, "获取Bot配置失败", err)
		}
		return
	}

	// 检查权限：private Bot只有创建者可以查看
	if config.Visibility == "private" && config.CreatorID != userID {
		h.respondError(c, http.StatusForbidden, "无权限查看此Bot配置", nil)
		return
	}

	response := config.ToResponse()

	// 检查用户是否已添加
	if h.friendService != nil {
		isAdded, _ := h.friendService.IsBotAdded(c.Request.Context(), userID, config.ID)
		response.IsAdded = isAdded
	}

	h.respondSuccess(c, gin.H{
		"config": response,
	})
}

// UpdateBotConfig 更新Bot配置
// @Summary 更新Bot配置
// @Description 更新指定的Bot配置
// @Tags Bot配置管理
// @Accept json
// @Produce json
// @Param id path int true "Bot配置ID"
// @Param config body models.UpdateBotConfigRequest true "更新信息"
// @Success 200 {object} map[string]interface{} "更新成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 403 {object} map[string]interface{} "无权限"
// @Failure 404 {object} map[string]interface{} "配置不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/v2/bots/{id} [put]
func (h *BotConfigHandler) UpdateBotConfig(c *gin.Context) {
	userID := h.getUserID(c)
	configID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "无效的配置ID", err)
		return
	}

	var req models.UpdateBotConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, http.StatusBadRequest, "请求参数格式错误", err)
		return
	}

	// 获取现有配置
	config, err := h.botService.GetBotConfigByID(c.Request.Context(), uint(configID))
	oldDescription := config.Description
	if err != nil {
		if err.Error() == "Bot配置不存在" {
			h.respondError(c, http.StatusNotFound, "Bot配置不存在", err)
		} else {
			h.respondError(c, http.StatusInternalServerError, "获取Bot配置失败", err)
		}
		return
	}

	// 检查权限
	if config.CreatorID != userID {
		h.respondError(c, http.StatusForbidden, "只有创建者可以修改Bot配置", nil)
		return
	}

	// 更新字段
	if req.Visibility != nil {
		if *req.Visibility != "private" && *req.Visibility != "public" {
			h.respondError(c, http.StatusBadRequest, "无效的可见性值", nil)
			return
		}
		config.Visibility = *req.Visibility
	}
	if req.BotType != nil {
		if *req.BotType != "text" && *req.BotType != "image" && *req.BotType != "tts" && *req.BotType != "asr" {
			h.respondError(c, http.StatusBadRequest, "无效的Bot类型，必须是 text/image/tts/asr 之一", nil)
			return
		}
		config.BotType = *req.BotType
	}
	if req.RequiresNetwork != nil {
		config.RequiresNetwork = *req.RequiresNetwork
	}
	if req.MaxTokens != nil {
		config.MaxTokens = *req.MaxTokens
	}
	if req.Temperature != nil {
		config.Temperature = *req.Temperature
	}
	if req.FunctionName != nil {
		config.FunctionName = *req.FunctionName
	}
	if req.Description != nil {
		config.Description = *req.Description
	}
	if req.MCPServerURL != nil {
		config.MCPServerURL = *req.MCPServerURL
	}

	// 处理参数JSON
	if req.Parameters != nil {
		parametersJSON, err := json.Marshal(req.Parameters)
		if err != nil {
			h.respondError(c, http.StatusBadRequest, "参数格式错误", err)
			return
		}
		config.Parameters = datatypes.JSON(parametersJSON)
	}

	if err := h.botService.UpdateBotConfig(c.Request.Context(), config); err != nil {
		h.respondError(c, http.StatusInternalServerError, "更新Bot配置失败", err)
		return
	}

	if *req.Description != oldDescription {
		go h.generateLLMFunctionParameters(config, config.FunctionName, config.Description)
	}

	h.logger.Info("用户 %d 更新Bot配置成功: %s (ID: %d)", userID, config.FunctionName, config.ID)
	h.respondSuccess(c, gin.H{
		"config": config.ToResponse(),
	})
}

// DeleteBotConfig 删除Bot配置
// @Summary 删除Bot配置
// @Description 删除指定的Bot配置
// @Tags Bot配置管理
// @Accept json
// @Produce json
// @Param id path int true "Bot配置ID"
// @Success 200 {object} map[string]interface{} "删除成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 403 {object} map[string]interface{} "无权限"
// @Failure 404 {object} map[string]interface{} "配置不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/v2/bots/{id} [delete]
func (h *BotConfigHandler) DeleteBotConfig(c *gin.Context) {
	userID := h.getUserID(c)
	configID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "无效的配置ID", err)
		return
	}

	if err := h.botService.DeleteBotConfig(c.Request.Context(), uint(configID), userID); err != nil {
		if err.Error() == "Bot配置不存在" {
			h.respondError(c, http.StatusNotFound, "Bot配置不存在", err)
		} else if err.Error() == "无权限删除此Bot配置" {
			h.respondError(c, http.StatusForbidden, "只有创建者可以删除Bot配置", err)
		} else {
			h.respondError(c, http.StatusInternalServerError, "删除Bot配置失败", err)
		}
		return
	}

	h.logger.Info("用户 %d 删除Bot配置成功 (ID: %d)", userID, configID)
	h.respondSuccess(c, gin.H{
		"message": "Bot配置删除成功",
	})
}

// SearchBots 搜索Bot配置
// @Summary 搜索Bot配置
// @Description 搜索Bot配置，支持bot_hash精确搜索和bot_name模糊搜索
// @Tags Bot配置管理
// @Accept json
// @Produce json
// @Param query query string true "搜索关键词"
// @Param search_type query string false "搜索类型：hash/name/description，默认为name"
// @Success 200 {object} map[string]interface{} "成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/v2/bots/search [get]
func (h *BotConfigHandler) SearchBots(c *gin.Context) {
	userID := h.getUserID(c)
	query := c.Query("query")
	searchType := c.DefaultQuery("search_type", "name")

	if query == "" {
		h.respondError(c, http.StatusBadRequest, "搜索关键词不能为空", nil)
		return
	}

	configs, err := h.botService.SearchBots(c.Request.Context(), userID, query, searchType)
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "搜索Bot配置失败", err)
		return
	}

	// 转换为响应格式并标识是否已添加
	var responses []*models.BotConfigResponse
	for _, config := range configs {
		response := config.ToResponse()

		// 检查用户是否已添加
		if h.friendService != nil {
			isAdded, _ := h.friendService.IsBotAdded(c.Request.Context(), userID, config.ID)
			response.IsAdded = isAdded
		}

		responses = append(responses, response)
	}

	h.respondSuccess(c, gin.H{
		"bots":  responses,
		"total": len(responses),
	})
}

// GetMyBots 获取我创建的Bot列表
// @Summary 获取我创建的Bot列表
// @Description 获取当前用户创建的所有Bot配置
// @Tags Bot配置管理
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "成功"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/v2/bots/my [get]
func (h *BotConfigHandler) GetMyBots(c *gin.Context) {
	userID := h.getUserID(c)

	configs, err := h.botService.GetUserCreatedBots(c.Request.Context(), userID)
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "获取Bot列表失败", err)
		return
	}

	// 转换为响应格式
	var responses []*models.BotConfigResponse
	for _, config := range configs {
		responses = append(responses, config.ToResponse())
	}

	h.respondSuccess(c, gin.H{
		"bots":  responses,
		"total": len(responses),
	})
}

// getUserID 从上下文获取用户ID
func (h *BotConfigHandler) getUserID(c *gin.Context) uint {
	if userID, exists := c.Get("user_id"); exists {
		if uid, ok := userID.(uint); ok {
			return uid
		}
	}
	h.logger.Error("无法从上下文中获取用户ID")
	return 0
}

// respondSuccess 返回成功响应
func (h *BotConfigHandler) respondSuccess(c *gin.Context, data interface{}) {
	utils.Success(c, data)
}

// respondError 返回错误响应
func (h *BotConfigHandler) respondError(c *gin.Context, statusCode int, message string, err error) {
	if err != nil {
		h.logger.Error("%s: %v", message, err)
	} else {
		h.logger.Error("%s", message)
	}
	utils.ErrorWithDetail(c, statusCode, message, err)
}

func (h *BotConfigHandler) generateLLMFunctionParameters(config *models.BotConfig, configName, description string) {
	h.logger.Info("为Function Call配置自动生成Parameters: %s", configName)

	generatedParams, err := h.generateParametersWithLLM(configName, description)
	if err != nil {
		h.logger.Warn("自动生成Parameters失败: %v", err)
	}
	parametersJSON, err := json.Marshal(generatedParams)
	if err != nil {
		h.logger.Info("生成的参数格式错误: %s", configName)
		return
	}
	config.Parameters = datatypes.JSON(parametersJSON)
	h.logger.Info("成功为Function Call配置生成Parameters: %s,%s", configName, string(parametersJSON))

	// 更新配置
	if err := h.botService.UpdateBotConfig(context.Background(), config); err != nil {
		h.logger.Error("更新配置参数失败: %v", err)
	}
	return
}

// generateParametersWithLLM 使用LLM生成Function Call的Parameters JSON Schema
func (h *BotConfigHandler) generateParametersWithLLM(configName, description string) (map[string]interface{}, error) {
	// 获取全局配置
	cfg := configs.Cfg
	if cfg == nil {
		return nil, fmt.Errorf("无法获取系统配置")
	}

	// 获取选定的LLM类型
	selectedLLM := cfg.SelectedModule["LLM"]
	if selectedLLM == "" {
		return nil, fmt.Errorf("未配置选定的LLM")
	}

	// 获取LLM配置
	llmConfig, exists := cfg.LLM[selectedLLM]
	if !exists {
		return nil, fmt.Errorf("找不到LLM配置: %s", selectedLLM)
	}

	// 创建LLM配置
	providerConfig := &llm.Config{
		Name:        selectedLLM,
		Type:        llmConfig.Type,
		ModelName:   llmConfig.ModelName,
		BaseURL:     llmConfig.BaseURL,
		APIKey:      llmConfig.APIKey,
		Temperature: llmConfig.Temperature,
		MaxTokens:   llmConfig.MaxTokens,
		TopP:        llmConfig.TopP,
		Extra:       llmConfig.Extra,
	}

	// 创建LLM提供者实例
	provider, err := llm.Create(llmConfig.Type, providerConfig)
	if err != nil {
		return nil, fmt.Errorf("创建LLM提供者失败: %v", err)
	}
	defer provider.Cleanup()

	// 构建提示词
	prompt := fmt.Sprintf(`请为以下Function Call生成合适的JSON Schema参数定义：

Function名称: %s
Function描述: %s

要求：
1. 返回标准的JSON Schema格式
2. 包含type、properties、required字段
3. 根据Function的名称和描述推断合理的参数
4. 只返回JSON格式，不要包含其他文字说明
5. 确保JSON格式正确且可解析

示例格式：
{
  "type": "object",
  "properties": {
    "param1": {
      "type": "string",
      "description": "参数1描述"
    },
    "param2": {
      "type": "integer",
      "description": "参数2描述"
    }
  },
  "required": ["param1"]
}`, configName, description)

	// 构建消息
	messages := []types.Message{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	// 调用LLM
	ctx, cancel := context.WithTimeout(context.Background(), 30*1000000000) // 30秒超时
	defer cancel()

	responseChan, err := provider.Response(ctx, "param-gen", messages)
	if err != nil {
		return nil, fmt.Errorf("调用LLM失败: %v", err)
	}

	// 收集响应
	var responseText strings.Builder
	for chunk := range responseChan {
		responseText.WriteString(chunk)
	}

	response := strings.TrimSpace(responseText.String())
	if response == "" {
		return nil, fmt.Errorf("LLM返回空响应")
	}

	// 尝试解析JSON
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(response), &params); err != nil {
		// 如果直接解析失败，尝试提取JSON部分
		response = h.extractJSONFromResponse(response)
		if err := json.Unmarshal([]byte(response), &params); err != nil {
			return nil, fmt.Errorf("解析LLM响应JSON失败: %v, 响应内容: %s", err, response)
		}
	}

	// 验证JSON Schema基本结构
	if err := h.validateJSONSchema(params); err != nil {
		return nil, fmt.Errorf("生成的JSON Schema格式不正确: %v", err)
	}

	return params, nil
}

// extractJSONFromResponse 从响应中提取JSON部分
func (h *BotConfigHandler) extractJSONFromResponse(response string) string {
	// 查找第一个 { 和最后一个 }
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")

	if start != -1 && end != -1 && end > start {
		return response[start : end+1]
	}

	return response
}

// validateJSONSchema 验证JSON Schema基本结构
func (h *BotConfigHandler) validateJSONSchema(schema map[string]interface{}) error {
	// 检查必需的字段
	if _, ok := schema["type"]; !ok {
		return fmt.Errorf("缺少type字段")
	}

	if schemaType, ok := schema["type"].(string); !ok || schemaType != "object" {
		return fmt.Errorf("type字段必须为object")
	}

	// properties字段是可选的，但如果存在必须是对象
	if properties, exists := schema["properties"]; exists {
		if _, ok := properties.(map[string]interface{}); !ok {
			return fmt.Errorf("properties字段必须是对象")
		}
	}

	// required字段是可选的，但如果存在必须是数组
	if required, exists := schema["required"]; exists {
		if _, ok := required.([]interface{}); !ok {
			return fmt.Errorf("required字段必须是数组")
		}
	}

	return nil
}
