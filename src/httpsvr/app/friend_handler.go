package app

import (
	"net/http"
	"strconv"

	"angrymiao-ai-server/src/core/middleware"
	"angrymiao-ai-server/src/core/utils"
	"angrymiao-ai-server/src/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// UserFriendHandler 用户好友处理器
type UserFriendHandler struct {
	friendService UserFriendService
	logger        *utils.Logger
}

// NewUserFriendHandler 创建用户好友处理器
func NewUserFriendHandler(db *gorm.DB, logger *utils.Logger) *UserFriendHandler {
	return &UserFriendHandler{
		friendService: NewUserFriendService(db, logger),
		logger:        logger,
	}
}

// RegisterRoutes 注册用户好友路由
func (h *UserFriendHandler) RegisterRoutes(apiGroup *gin.RouterGroup) {
	friendGroup := apiGroup.Group("/friends/bots").Use(middleware.AmTokenJWTUserAuth())
	{
		friendGroup.POST("", h.AddBotFriend)
		friendGroup.DELETE("/:bot_config_id", h.RemoveBotFriend)
		friendGroup.GET("", h.GetBotFriends)
		friendGroup.PATCH("/:bot_config_id/appkey", h.UpdateAppKey)
		friendGroup.PATCH("/:bot_config_id/priority", h.UpdatePriority)
		friendGroup.PATCH("/:bot_config_id/toggle", h.ToggleStatus)
	}
}

// AddBotFriend 添加Bot好友
// @Summary 添加Bot好友
// @Description 添加Bot为好友
// @Tags 用户好友管理
// @Accept json
// @Produce json
// @Param friend body models.AddBotFriendRequest true "好友信息"
// @Success 201 {object} map[string]interface{} "添加成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/friends/bots [post]
func (h *UserFriendHandler) AddBotFriend(c *gin.Context) {
	userID := h.getUserID(c)

	var req models.AddBotFriendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, http.StatusBadRequest, "请求参数格式错误", err)
		return
	}

	if err := h.friendService.AddBotFriend(c.Request.Context(), userID, req.BotConfigID, req.AppKey, req.Alias); err != nil {
		if err.Error() == "已添加该Bot好友" {
			h.respondError(c, http.StatusBadRequest, "已添加该Bot好友", err)
		} else {
			h.respondError(c, http.StatusInternalServerError, "添加Bot好友失败", err)
		}
		return
	}

	h.logger.Info("用户 %d 添加Bot好友成功 (BotConfigID: %d)", userID, req.BotConfigID)
	c.JSON(http.StatusCreated, gin.H{
		"code":    201,
		"message": "添加Bot好友成功",
	})
}

// RemoveBotFriend 删除Bot好友
// @Summary 删除Bot好友
// @Description 删除Bot好友关系
// @Tags 用户好友管理
// @Accept json
// @Produce json
// @Param bot_config_id path int true "Bot配置ID"
// @Success 200 {object} map[string]interface{} "删除成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 404 {object} map[string]interface{} "好友不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/friends/bots/{bot_config_id} [delete]
func (h *UserFriendHandler) RemoveBotFriend(c *gin.Context) {
	userID := h.getUserID(c)
	botConfigID, err := strconv.ParseUint(c.Param("bot_config_id"), 10, 32)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "无效的Bot配置ID", err)
		return
	}

	if err := h.friendService.RemoveBotFriend(c.Request.Context(), userID, uint(botConfigID)); err != nil {
		if err.Error() == "Bot好友不存在" {
			h.respondError(c, http.StatusNotFound, "Bot好友不存在", err)
		} else {
			h.respondError(c, http.StatusInternalServerError, "删除Bot好友失败", err)
		}
		return
	}

	h.logger.Info("用户 %d 删除Bot好友成功 (BotConfigID: %d)", userID, botConfigID)
	h.respondSuccess(c, gin.H{
		"message": "删除Bot好友成功",
	})
}

// GetBotFriends 获取Bot好友列表
// @Summary 获取Bot好友列表
// @Description 获取用户的所有Bot好友
// @Tags 用户好友管理
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "成功"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/friends/bots [get]
func (h *UserFriendHandler) GetBotFriends(c *gin.Context) {
	userID := h.getUserID(c)

	friends, err := h.friendService.GetUserBotFriends(c.Request.Context(), userID)
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "获取Bot好友列表失败", err)
		return
	}

	h.respondSuccess(c, gin.H{
		"friends": friends,
		"total":   len(friends),
	})
}

// UpdateAppKey 更新Bot好友的AppKey
// @Summary 更新Bot好友的AppKey
// @Description 更新Bot好友的LLM API密钥
// @Tags 用户好友管理
// @Accept json
// @Produce json
// @Param bot_config_id path int true "Bot配置ID"
// @Param appkey body models.UpdateBotFriendAppKeyRequest true "AppKey信息"
// @Success 200 {object} map[string]interface{} "更新成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 404 {object} map[string]interface{} "好友不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/friends/bots/{bot_config_id}/appkey [patch]
func (h *UserFriendHandler) UpdateAppKey(c *gin.Context) {
	userID := h.getUserID(c)
	botConfigID, err := strconv.ParseUint(c.Param("bot_config_id"), 10, 32)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "无效的Bot配置ID", err)
		return
	}

	var req models.UpdateBotFriendAppKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, http.StatusBadRequest, "请求参数格式错误", err)
		return
	}

	if err := h.friendService.UpdateBotFriendAppKey(c.Request.Context(), userID, uint(botConfigID), req.AppKey); err != nil {
		if err.Error() == "Bot好友不存在" {
			h.respondError(c, http.StatusNotFound, "Bot好友不存在", err)
		} else {
			h.respondError(c, http.StatusInternalServerError, "更新AppKey失败", err)
		}
		return
	}

	h.logger.Info("用户 %d 更新Bot好友AppKey成功 (BotConfigID: %d)", userID, botConfigID)
	h.respondSuccess(c, gin.H{
		"message": "更新AppKey成功",
	})
}

// UpdatePriority 更新Bot好友优先级
// @Summary 更新Bot好友优先级
// @Description 更新Bot好友的优先级
// @Tags 用户好友管理
// @Accept json
// @Produce json
// @Param bot_config_id path int true "Bot配置ID"
// @Param priority body models.UpdateBotFriendPriorityRequest true "优先级信息"
// @Success 200 {object} map[string]interface{} "更新成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 404 {object} map[string]interface{} "好友不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/friends/bots/{bot_config_id}/priority [patch]
func (h *UserFriendHandler) UpdatePriority(c *gin.Context) {
	userID := h.getUserID(c)
	botConfigID, err := strconv.ParseUint(c.Param("bot_config_id"), 10, 32)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "无效的Bot配置ID", err)
		return
	}

	var req models.UpdateBotFriendPriorityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, http.StatusBadRequest, "请求参数格式错误", err)
		return
	}

	if err := h.friendService.UpdateBotFriendPriority(c.Request.Context(), userID, uint(botConfigID), req.Priority); err != nil {
		if err.Error() == "Bot好友不存在" {
			h.respondError(c, http.StatusNotFound, "Bot好友不存在", err)
		} else {
			h.respondError(c, http.StatusInternalServerError, "更新优先级失败", err)
		}
		return
	}

	h.logger.Info("用户 %d 更新Bot好友优先级成功 (BotConfigID: %d, Priority: %d)", userID, botConfigID, req.Priority)
	h.respondSuccess(c, gin.H{
		"message":  "更新优先级成功",
		"priority": req.Priority,
	})
}

// ToggleStatus 切换Bot好友状态
// @Summary 切换Bot好友状态
// @Description 启用或禁用Bot好友
// @Tags 用户好友管理
// @Accept json
// @Produce json
// @Param bot_config_id path int true "Bot配置ID"
// @Param status body models.ToggleBotFriendStatusRequest true "状态信息"
// @Success 200 {object} map[string]interface{} "操作成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 404 {object} map[string]interface{} "好友不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/friends/bots/{bot_config_id}/toggle [patch]
func (h *UserFriendHandler) ToggleStatus(c *gin.Context) {
	userID := h.getUserID(c)
	botConfigID, err := strconv.ParseUint(c.Param("bot_config_id"), 10, 32)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "无效的Bot配置ID", err)
		return
	}

	var req models.ToggleBotFriendStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, http.StatusBadRequest, "请求参数格式错误", err)
		return
	}

	if err := h.friendService.ToggleBotFriendStatus(c.Request.Context(), userID, uint(botConfigID), req.IsActive); err != nil {
		if err.Error() == "Bot好友不存在" {
			h.respondError(c, http.StatusNotFound, "Bot好友不存在", err)
		} else {
			h.respondError(c, http.StatusInternalServerError, "切换状态失败", err)
		}
		return
	}

	status := "禁用"
	if req.IsActive {
		status = "启用"
	}
	h.logger.Info("用户 %d 切换Bot好友状态成功 (BotConfigID: %d, Status: %s)", userID, botConfigID, status)
	h.respondSuccess(c, gin.H{
		"message":   "切换状态成功",
		"is_active": req.IsActive,
	})
}

// getUserID 从上下文获取用户ID
func (h *UserFriendHandler) getUserID(c *gin.Context) uint {
	if userID, exists := c.Get("user_id"); exists {
		if uid, ok := userID.(uint); ok {
			return uid
		}
	}
	h.logger.Error("无法从上下文中获取用户ID")
	return 0
}

// respondSuccess 返回成功响应
func (h *UserFriendHandler) respondSuccess(c *gin.Context, data interface{}) {
	utils.Success(c, data)
}

// respondError 返回错误响应
func (h *UserFriendHandler) respondError(c *gin.Context, statusCode int, message string, err error) {
	if err != nil {
		h.logger.Error("%s: %v", message, err)
	} else {
		h.logger.Error("%s", message)
	}
	utils.ErrorWithDetail(c, statusCode, message, err)
}
