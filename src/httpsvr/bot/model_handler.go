package bot

import (
	"net/http"
	"strconv"

	"angrymiao-ai-server/src/core/middleware"
	"angrymiao-ai-server/src/core/utils"
	"angrymiao-ai-server/src/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ModelConfigHandler 模型配置处理器
type ModelConfigHandler struct {
	modelService ModelConfigService
	logger       *utils.Logger
}

// NewModelConfigHandler 创建模型配置处理器
func NewModelConfigHandler(db *gorm.DB, logger *utils.Logger) *ModelConfigHandler {
	return &ModelConfigHandler{
		modelService: NewModelConfigService(db, logger),
		logger:       logger,
	}
}

// RegisterRoutes 注册模型配置路由
func (h *ModelConfigHandler) RegisterRoutes(apiGroup *gin.RouterGroup) {
	modelGroup := apiGroup.Group("/models").Use(middleware.AmTokenJWTUserAuth())
	{
		modelGroup.POST("", h.CreateModel)
		modelGroup.GET("", h.ListModels)
		modelGroup.GET("/:id", h.GetModel)
		modelGroup.PUT("/:id", h.UpdateModel)
		modelGroup.DELETE("/:id", h.DeleteModel)
	}
}

// CreateModel 创建模型配置
// @Summary 创建模型配置
// @Description 创建新的模型配置
// @Tags 模型配置管理
// @Accept json
// @Produce json
// @Param model body object true "模型配置信息"
// @Success 201 {object} map[string]interface{} "创建成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/v2/models [post]
func (h *ModelConfigHandler) CreateModel(c *gin.Context) {
	var req struct {
		LLMType     string `json:"llm_type" binding:"required"`
		ModelName   string `json:"model_name" binding:"required"`
		LLMProtocol string `json:"llm_protocol" binding:"required"` // llm 的协议类型【openai,ollama】
		BaseURL     string `json:"base_url"`
		Description string `json:"description"`
		IsPublic    bool   `json:"is_public"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, http.StatusBadRequest, "请求参数格式错误", err)
		return
	}

	config := &models.ModelConfig{
		LLMType:     req.LLMType,
		ModelName:   req.ModelName,
		LLMProtocol: req.LLMProtocol,
		BaseURL:     req.BaseURL,
		Description: req.Description,
		IsPublic:    req.IsPublic,
	}

	if err := h.modelService.CreateModelConfig(c.Request.Context(), config); err != nil {
		h.respondError(c, http.StatusInternalServerError, "创建模型配置失败", err)
		return
	}

	h.logger.Info("创建模型配置成功: %s/%s (ID: %d)", config.LLMType, config.ModelName, config.ID)
	c.JSON(http.StatusCreated, gin.H{
		"code":    201,
		"message": "模型配置创建成功",
		"data":    config,
	})
}

// GetModel 获取模型配置详情
// @Summary 获取模型配置详情
// @Description 根据ID获取模型配置详情
// @Tags 模型配置管理
// @Accept json
// @Produce json
// @Param id path int true "模型配置ID"
// @Success 200 {object} map[string]interface{} "成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 404 {object} map[string]interface{} "配置不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/v2/models/{id} [get]
func (h *ModelConfigHandler) GetModel(c *gin.Context) {
	configID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "无效的配置ID", err)
		return
	}

	config, err := h.modelService.GetModelConfigByID(c.Request.Context(), uint(configID))
	if err != nil {
		if err.Error() == "模型配置不存在" {
			h.respondError(c, http.StatusNotFound, "模型配置不存在", err)
		} else {
			h.respondError(c, http.StatusInternalServerError, "获取模型配置失败", err)
		}
		return
	}

	h.respondSuccess(c, gin.H{
		"model": config,
	})
}

// UpdateModel 更新模型配置
// @Summary 更新模型配置
// @Description 更新指定的模型配置
// @Tags 模型配置管理
// @Accept json
// @Produce json
// @Param id path int true "模型配置ID"
// @Param model body object true "更新信息"
// @Success 200 {object} map[string]interface{} "更新成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 404 {object} map[string]interface{} "配置不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/v2/models/{id} [put]
func (h *ModelConfigHandler) UpdateModel(c *gin.Context) {
	configID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "无效的配置ID", err)
		return
	}

	var req struct {
		LLMType     *string `json:"llm_type"`
		ModelName   *string `json:"model_name"`
		BaseURL     *string `json:"base_url"`
		Description *string `json:"description"`
		IsPublic    *bool   `json:"is_public"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, http.StatusBadRequest, "请求参数格式错误", err)
		return
	}

	// 获取现有配置
	config, err := h.modelService.GetModelConfigByID(c.Request.Context(), uint(configID))
	if err != nil {
		if err.Error() == "模型配置不存在" {
			h.respondError(c, http.StatusNotFound, "模型配置不存在", err)
		} else {
			h.respondError(c, http.StatusInternalServerError, "获取模型配置失败", err)
		}
		return
	}

	// 更新字段
	if req.LLMType != nil {
		config.LLMType = *req.LLMType
	}
	if req.ModelName != nil {
		config.ModelName = *req.ModelName
	}
	if req.BaseURL != nil {
		config.BaseURL = *req.BaseURL
	}
	if req.Description != nil {
		config.Description = *req.Description
	}
	if req.IsPublic != nil {
		config.IsPublic = *req.IsPublic
	}

	if err := h.modelService.UpdateModelConfig(c.Request.Context(), config); err != nil {
		h.respondError(c, http.StatusInternalServerError, "更新模型配置失败", err)
		return
	}

	h.logger.Info("更新模型配置成功: %s/%s (ID: %d)", config.LLMType, config.ModelName, config.ID)
	h.respondSuccess(c, gin.H{
		"model": config,
	})
}

// DeleteModel 删除模型配置（软删除）
// @Summary 删除模型配置
// @Description 删除指定的模型配置（软删除）
// @Tags 模型配置管理
// @Accept json
// @Produce json
// @Param id path int true "模型配置ID"
// @Success 200 {object} map[string]interface{} "删除成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 404 {object} map[string]interface{} "配置不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/v2/models/{id} [delete]
func (h *ModelConfigHandler) DeleteModel(c *gin.Context) {
	configID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "无效的配置ID", err)
		return
	}

	if err := h.modelService.DeleteModelConfig(c.Request.Context(), uint(configID)); err != nil {
		if err.Error() == "模型配置不存在" {
			h.respondError(c, http.StatusNotFound, "模型配置不存在", err)
		} else {
			h.respondError(c, http.StatusInternalServerError, "删除模型配置失败", err)
		}
		return
	}

	h.logger.Info("删除模型配置成功 (ID: %d)", configID)
	h.respondSuccess(c, gin.H{
		"message": "模型配置删除成功",
	})
}

// ListModels 获取模型配置列表
// @Summary 获取模型配置列表
// @Description 获取模型配置列表
// @Tags 模型配置管理
// @Accept json
// @Produce json
// @Param include_public query bool false "是否只包含公共模型"
// @Success 200 {object} map[string]interface{} "成功"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/v2/models [get]
func (h *ModelConfigHandler) ListModels(c *gin.Context) {
	includePublic := c.DefaultQuery("include_public", "false") == "true"

	configs, err := h.modelService.ListModelConfigs(c.Request.Context(), includePublic)
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "获取模型配置列表失败", err)
		return
	}

	h.respondSuccess(c, gin.H{
		"models": configs,
		"total":  len(configs),
	})
}

// respondSuccess 返回成功响应
func (h *ModelConfigHandler) respondSuccess(c *gin.Context, data interface{}) {
	utils.Success(c, data)
}

// respondError 返回错误响应
func (h *ModelConfigHandler) respondError(c *gin.Context, statusCode int, message string, err error) {
	if err != nil {
		h.logger.Error("%s: %v", message, err)
	} else {
		h.logger.Error("%s", message)
	}
	utils.ErrorWithDetail(c, statusCode, message, err)
}
