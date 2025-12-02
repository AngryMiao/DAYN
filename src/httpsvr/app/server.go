package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"angrymiao-ai-server/src/configs"
	"angrymiao-ai-server/src/configs/database"
	"angrymiao-ai-server/src/core/chat"
	"angrymiao-ai-server/src/core/middleware"
	"angrymiao-ai-server/src/core/pool"
	"angrymiao-ai-server/src/core/providers"
	"angrymiao-ai-server/src/core/providers/auc"
	"angrymiao-ai-server/src/core/providers/llm"
	"angrymiao-ai-server/src/core/utils"
	"angrymiao-ai-server/src/httpsvr/bot"
	"angrymiao-ai-server/src/httpsvr/device"
	"angrymiao-ai-server/src/models"

	"github.com/gin-gonic/gin"
)

type AppService struct {
	logger        *utils.Logger
	config        *configs.Config
	deviceDB      *device.DeviceDB
	poolMgr       *pool.PoolManager
	botService    bot.BotConfigService
	friendService UserFriendService
}

func NewDefaultAppService(config *configs.Config, logger *utils.Logger) *AppService {
	db := database.GetDB()
	svc := &AppService{
		logger:        logger,
		config:        config,
		deviceDB:      device.NewDeviceDB(),
		botService:    bot.NewBotConfigService(db, logger),
		friendService: NewUserFriendService(db, logger),
	}
	// 初始化资源池管理器（若失败不阻断启动，延迟到首次请求再尝试）
	if pm, err := pool.NewPoolManager(config, logger); err == nil {
		svc.poolMgr = pm
	} else {
		logger.Warn("初始化资源池管理器失败，将在请求时重试: %v", err)
	}
	return svc
}

func (s *AppService) Start(ctx context.Context, engine *gin.Engine, apiGroup *gin.RouterGroup) {
	// 注册chat相关路由
	chatGroup := apiGroup.Group("/chat").Use(middleware.AmTokenJWTUserAuth())
	{
		chatGroup.POST("/send", s.handleChatSend)
		chatGroup.GET("/history", s.handleChatHistory)
	}

	appGroup := apiGroup.Group("/app").Use(middleware.AmTokenJWTUserAuth())
	{
		// 设备路由
		appGroup.GET("/devices", s.handleGetDevices)
		appGroup.GET("/media/home", s.handleGetHomeMedia)
		// 录音识别
		appGroup.POST("/audio/recognition", s.handleRecognition)
		appGroup.GET("/audio/recognition/:task_id", s.handleGetRecognitionResult)
	}

	// AUC回调
	apiGroup.POST("/app/callback", s.handleAUCCallback)
}

func (s *AppService) handleRecognition(c *gin.Context) {
	userID := c.GetUint("user_id")

	var req RecognitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Custom(c, http.StatusBadRequest, RecognitionResponse{Success: false, Message: "请求参数错误: " + err.Error()})
		return
	}

	// 查询是否有处理过录音文件
	userAucTask := models.AudioTask{}
	err := database.GetDB().Model(&models.AudioTask{}).
		Select("media_id").
		Where("user_id = ? AND media_id = ?", userID, req.MediaID).
		Find(&userAucTask).Error

	if err != nil {
		utils.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	if userAucTask.ID != 0 {
		utils.Custom(c, http.StatusInternalServerError, RecognitionResponse{Success: false, Message: "音频转录执行中"})
		return
	}

	// 查询媒体文件
	var audioData models.MediaUpload
	err = database.GetDB().Model(&models.MediaUpload{}).
		Where("user_id = ? AND id = ? AND file_type = ?", userID, req.MediaID, "audio").
		First(&audioData).Error

	if err != nil {
		utils.Custom(c, http.StatusInternalServerError, RecognitionResponse{Success: false, Message: "音频文件不存在"})
		return
	}

	// 获取AUC配置
	aucProviderName := s.config.SelectedModule["AUC"]
	if aucProviderName == "" {
		utils.Custom(c, http.StatusInternalServerError, RecognitionResponse{Success: false, Message: "AUC服务未配置"})
		return
	}

	aucConfig, ok := s.config.AUC[aucProviderName]
	if !ok {
		utils.Custom(c, http.StatusInternalServerError, RecognitionResponse{Success: false, Message: "AUC配置不存在"})
		return
	}

	aucType, ok := aucConfig["type"].(string)
	if !ok {
		utils.Custom(c, http.StatusInternalServerError, RecognitionResponse{Success: false, Message: "AUC配置错误"})
		return
	}

	// 创建AUC provider
	cfg := &auc.Config{
		Name: aucProviderName,
		Type: aucType,
		Data: aucConfig,
	}

	aucProvider, err := auc.Create(aucType, cfg, s.logger)
	if err != nil {
		s.logger.Error("创建AUC提供者失败: %v", err)
		utils.Custom(c, http.StatusInternalServerError, RecognitionResponse{Success: false, Message: "创建AUC服务失败"})
		return
	}

	if err := aucProvider.Initialize(); err != nil {
		s.logger.Error("初始化AUC提供者失败: %v", err)
		utils.Custom(c, http.StatusInternalServerError, RecognitionResponse{Success: false, Message: "初始化AUC服务失败"})
		return
	}
	defer aucProvider.Cleanup()

	// 提交AUC任务
	ctx := context.Background()
	taskID, err := aucProvider.SubmitTask(ctx, audioData.URL, fmt.Sprintf("%d", userID))
	if err != nil {
		s.logger.Error("提交AUC任务失败: %v", err)
		utils.Custom(c, http.StatusInternalServerError, RecognitionResponse{Success: false, Message: "提交识别任务失败"})
		return
	}

	// 创建AudioTask记录
	audioTask := models.AudioTask{
		UserID:    userID,
		DeviceID:  audioData.DeviceID,
		MediaID:   audioData.ID,
		AucType:   aucProviderName,
		AucTaskID: taskID,
		Status:    models.AudioTaskStatusProcessing,
	}

	if err := database.GetDB().Create(&audioTask).Error; err != nil {
		s.logger.Error("创建AudioTask记录失败: %v", err)
		utils.Custom(c, http.StatusInternalServerError, RecognitionResponse{Success: false, Message: "创建任务记录失败"})
		return
	}

	s.logger.Info("AUC任务已提交, TaskID: %s, MediaID: %d", taskID, req.MediaID)
	utils.Custom(c, http.StatusOK, RecognitionResponse{
		Success: true,
		Message: "识别任务已提交",
		TaskID:  taskID,
	})
}

func (s *AppService) handleGetRecognitionResult(c *gin.Context) {
	userID := c.GetUint("user_id")
	taskID := c.Param("task_id")

	// 查找对应的AudioTask
	var audioTask models.AudioTask
	err := database.GetDB().Where("user_id = ? AND auc_task_id = ?", userID, taskID).First(&audioTask).Error
	if err != nil {
		utils.Custom(c, http.StatusNotFound, gin.H{"success": false, "message": "任务不存在"})
		return
	}

	response := gin.H{
		"success":  true,
		"task_id":  audioTask.AucTaskID,
		"status":   audioTask.Status,
		"text":     audioTask.Text,
		"media_id": audioTask.MediaID,
		"auc_type": audioTask.AucType,
		"summary":  audioTask.Summary,
	}

	// 如果有关键点，解析并返回
	if len(audioTask.KeyPoints) > 0 {
		var keyPoints []string
		if err := json.Unmarshal(audioTask.KeyPoints, &keyPoints); err == nil {
			response["key_points"] = keyPoints
		}
	}

	// 如果有详细的识别结果，也返回
	if len(audioTask.ResultJSON) > 0 {
		var resultDetail map[string]interface{}
		if err := json.Unmarshal(audioTask.ResultJSON, &resultDetail); err == nil {
			response["result_detail"] = resultDetail
		}
	}

	utils.Custom(c, http.StatusOK, response)
}

func (s *AppService) handleAUCCallback(c *gin.Context) {
	var req AUCCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.logger.Error("AUC回调参数错误: %v", err)
		utils.Custom(c, http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	s.logger.Info("收到AUC回调, TaskID: %s, Code: %d", req.Resp.ID, req.Resp.Code)

	// 查找对应的AudioTask
	var audioTask models.AudioTask
	err := database.GetDB().Where("auc_task_id = ?", req.Resp.ID).First(&audioTask).Error
	if err != nil {
		s.logger.Error("查找AudioTask失败: %v, TaskID: %s", err, req.Resp.ID)
		utils.Custom(c, http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	// 更新任务状态
	// 豆包 AUC 返回 code=1000 表示成功
	if req.Resp.Code == 1000 {
		audioTask.Status = models.AudioTaskStatusCompleted
		audioTask.Text = req.Resp.Text

		// 保存完整的识别结果到 JSON 字段（包含 utterances、words、speaker 等详细信息）
		resultJSON, err := json.Marshal(req.Resp)
		if err != nil {
			s.logger.Warn("序列化识别结果失败: %v", err)
		} else {
			audioTask.ResultJSON = resultJSON
		}

		s.logger.Info("AUC任务完成, TaskID: %s, Text: %s, Utterances: %d",
			req.Resp.ID, req.Resp.Text, len(req.Resp.Utterances))

		// 调用AI生成摘要和关键点
		if summary, keyPoints, err := s.generateSummaryAndKeyPoints(req.Resp.Text); err != nil {
			s.logger.Warn("生成摘要失败: %v", err)
		} else {
			audioTask.Summary = summary
			if keyPointsJSON, err := json.Marshal(keyPoints); err == nil {
				audioTask.KeyPoints = keyPointsJSON
			}
		}
	} else {
		audioTask.Status = models.AudioTaskStatusFailed
		s.logger.Error("AUC任务失败, TaskID: %s, Code: %d, Message: %s",
			req.Resp.ID, req.Resp.Code, req.Resp.Message)
	}

	if err := database.GetDB().Save(&audioTask).Error; err != nil {
		s.logger.Error("更新AudioTask失败: %v", err)
		utils.Custom(c, http.StatusInternalServerError, gin.H{"error": "update failed"})
		return
	}

	utils.Custom(c, http.StatusOK, gin.H{"success": true})
}

func (s *AppService) handleGetHomeMedia(c *gin.Context) {
	userID := c.GetUint("user_id")

	// 读取分页参数
	pp := utils.ParsePageParams(c, 1, 8, 50)
	page := pp.Page
	pageSize := pp.PageSize

	// 读取 media_type 参数（可选：image/audio/video，支持逗号分隔多个类型，不传则返回所有）
	mediaType := c.Query("media_type")

	// 构建查询条件
	query := database.GetDB().Model(&models.MediaUpload{}).Where("user_id = ?", userID)

	// 如果指定了 media_type，添加过滤条件
	if mediaType != "" {
		// 支持逗号分隔的多个类型，如 "image,video"
		types := strings.Split(mediaType, ",")
		if len(types) > 1 {
			query = query.Where("file_type IN ?", types)
		} else {
			query = query.Where("file_type = ?", mediaType)
		}
	}

	// 统计总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		utils.Custom(c, http.StatusInternalServerError, GetHomeMediaResponse{Success: false, Message: "查询失败"})
		return
	}

	// 分页查询
	mediaList := make([]models.MediaUpload, 0)
	start, _ := utils.ComputeSliceRange(int(total), page, pageSize)
	if err := query.Order("created_at desc").
		Limit(pageSize).
		Offset(start).
		Find(&mediaList).Error; err != nil {
		s.logger.Error("查询媒体列表失败: %v", err)
		utils.Custom(c, http.StatusInternalServerError, GetHomeMediaResponse{Success: false, Message: "查询失败"})
		return
	}

	// 构建响应列表
	resultList := make([]MediaWithTask, 0, len(mediaList))

	// 如果是音频类型，需要关联查询 AudioTask
	if mediaType == "audio" {
		// 提取所有 media_id
		mediaIDs := make([]uint, 0, len(mediaList))
		for _, media := range mediaList {
			mediaIDs = append(mediaIDs, media.ID)
		}

		// 批量查询 AudioTask
		audioTasks := make([]models.AudioTask, 0)
		if len(mediaIDs) > 0 {
			database.GetDB().Where("media_id IN ? AND user_id = ?", mediaIDs, userID).Find(&audioTasks)
		}

		// 创建 media_id -> AudioTask 的映射
		taskMap := make(map[uint]*models.AudioTask)
		for i := range audioTasks {
			taskMap[audioTasks[i].MediaID] = &audioTasks[i]
		}

		// 合并数据
		for _, media := range mediaList {
			item := MediaWithTask{
				MediaUpload: media,
			}

			// 如果有对应的 AudioTask，添加任务信息
			if task, exists := taskMap[media.ID]; exists {
				item.TaskID = task.AucTaskID
				item.TaskStatus = task.Status
				item.TaskText = task.Text
				item.TaskSummary = task.Summary

				// 解析关键点
				if len(task.KeyPoints) > 0 {
					var keyPoints []string
					if err := json.Unmarshal(task.KeyPoints, &keyPoints); err == nil {
						item.TaskKeyPoints = keyPoints
					}
				}
			} else {
				item.TaskStatus = "ready"
			}

			resultList = append(resultList, item)
		}
	} else {
		// 非音频类型，直接转换
		for _, media := range mediaList {
			resultList = append(resultList, MediaWithTask{
				MediaUpload: media,
			})
		}
	}

	utils.Custom(c, http.StatusOK, GetHomeMediaResponse{Success: true, List: resultList, Total: total, Page: page, PageSize: pageSize})
}

func (s *AppService) handleGetDevices(c *gin.Context) {
	userID := c.GetUint("user_id")

	list, err := s.deviceDB.GetUserDevices(userID)
	if err != nil {
		s.logger.Error("查询用户设备失败: %v", err)
		utils.Custom(c, http.StatusInternalServerError, GetDevicesResponse{Success: false, Message: "查询失败"})
		return
	}

	resp := GetDevicesResponse{Success: true}
	for _, d := range list {
		resp.Devices = append(resp.Devices, toSummary(d))
	}
	utils.Custom(c, http.StatusOK, resp)
}

func toSummary(d models.Device) DeviceSummary {
	return DeviceSummary{
		DeviceID: d.DeviceID,
		Name:     d.Name,
		Online:   d.Online,
	}
}

// handleChatSend 处理聊天消息发送
func (s *AppService) handleChatSend(c *gin.Context) {
	var req ChatSendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Custom(c, http.StatusBadRequest, ChatSendResponse{Success: false, Message: "请求参数错误: " + err.Error()})
		return
	}

	userID := c.GetUint("user_id")

	// 使用 Postgres 作为对话记忆存储
	rm := chat.NewPostgresMemory(fmt.Sprintf("%d", userID))
	dialogueManager := chat.NewDialogueManager(s.logger, rm)

	dialogueManager.SetSystemMessage(s.config.DefaultPrompt)

	// 添加用户消息到对话历史
	dialogueManager.Put(chat.Message{
		Role:    "user",
		Content: req.Text,
	})

	// 获取对话历史（最近10条，失败则回退到内存对话）
	messages := make([]providers.Message, 0)
	storedMsgs, err := dialogueManager.GetStoredDialogue(8)
	var src []chat.Message
	if err != nil || len(storedMsgs) == 0 {
		if err != nil {
			s.logger.Warn("获取对话历史失败: %v", err)
		}
		src = dialogueManager.GetLLMDialogue()
	} else {
		src = storedMsgs
	}
	for _, msg := range src {
		messages = append(messages, providers.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// 获取或初始化资源池管理器
	if s.poolMgr == nil {
		if pm, e := pool.NewPoolManager(s.config, s.logger); e == nil {
			s.poolMgr = pm
		} else {
			s.logger.Error("初始化资源池失败: %v", e)
			utils.Custom(c, http.StatusInternalServerError, ChatSendResponse{Success: false, Message: "服务内部错误"})
			return
		}
	}

	// 从资源池获取LLM提供者
	set, err := s.poolMgr.GetProviderSet()
	if err != nil || set.LLM == nil {
		if err != nil {
			s.logger.Error("获取LLM提供者失败: %v", err)
		}
		utils.Custom(c, http.StatusInternalServerError, ChatSendResponse{Success: false, Message: "LLM服务不可用"})
		return
	}
	llmProvider := set.LLM

	// 如果指定了 bot_id，则应用用户级 LLM 配置
	if req.BotID != nil {
		userLLMConfig, err := s.getUserLLMConfigForBot(c.Request.Context(), userID, *req.BotID)
		if err != nil {
			s.logger.Error("获取用户Bot配置失败: %v", err)
			utils.Custom(c, http.StatusInternalServerError, ChatSendResponse{Success: false, Message: "获取Bot配置失败"})
			return
		}

		// 检查 APIKey 是否为空
		if userLLMConfig.APIKey == "" {
			s.logger.Warn("用户 %d 的Bot %d 缺少APIKey", userID, *req.BotID)
			c.JSON(http.StatusBadRequest, ChatSendResponse{
				Success:   false,
				Message:   "需要配置API密钥",
				ErrorCode: "MISSING_API_KEY",
				BotID:     req.BotID,
				BotConfig: &BotInfo{
					Name:      userLLMConfig.Name,
					Type:      userLLMConfig.Type,
					ModelName: userLLMConfig.ModelName,
					BaseURL:   userLLMConfig.BaseURL,
				},
			})
			return
		}

		// 应用用户级 LLM 配置
		if err := s.applyUserLLMConfig(llmProvider, userLLMConfig); err != nil {
			s.logger.Error("应用用户LLM配置失败: %v", err)
			utils.Custom(c, http.StatusInternalServerError, ChatSendResponse{Success: false, Message: "应用配置失败"})
			return
		}

		s.logger.Info("用户 %d 使用Bot %d 的配置进行聊天", userID, *req.BotID)
	}

	// 生成回复
	ctx := context.Background()
	sessionID := fmt.Sprintf("http_session_%d", userID)
	llmProvider.SetIdentityFlag("session", sessionID)
	responses, err := llmProvider.ResponseWithFunctions(ctx, sessionID, messages, nil)
	if err != nil {
		s.logger.Error("LLM生成回复失败: %v", err)
		utils.Custom(c, http.StatusInternalServerError, ChatSendResponse{Success: false, Message: "生成回复失败"})
		return
	}

	// 收集完整回复
	var fullReply strings.Builder
	for response := range responses {
		if response.Error != "" {
			s.logger.Error("LLM响应错误: %s", response.Error)
			utils.Custom(c, http.StatusInternalServerError, ChatSendResponse{Success: false, Message: "生成回复失败"})
			return
		}
		fullReply.WriteString(response.Content)
	}

	// 添加助手回复到对话历史
	dialogueManager.Put(chat.Message{
		Role:    "assistant",
		Content: fullReply.String(),
	})

	utils.Custom(c, http.StatusOK, ChatSendResponse{
		Success: true,
		Reply:   fullReply.String(),
	})
}

// handleChatHistory 获取聊天历史，支持分页
func (s *AppService) handleChatHistory(c *gin.Context) {
	// 解析分页参数
	pp := utils.ParsePageParams(c, 1, 20, 100)
	page := pp.Page
	pageSize := pp.PageSize

	userID := c.GetUint("user_id")

	// 从 Postgres 倒序分页读取历史
	pm := chat.NewPostgresMemory(fmt.Sprintf("%d", userID))
	pageItems, total64, err := pm.QueryMessages("DESC", page, pageSize)
	if err != nil {
		s.logger.Error("查询对话记忆失败: %v", err)
		utils.Custom(c, http.StatusInternalServerError, ChatHistoryResponse{Success: false, Message: "查询失败"})
		return
	}
	total := int(total64)

	// 为了展示从旧到新，需将当前页（DESC查询得到的）列表反转
	for i, j := 0, len(pageItems)-1; i < j; i, j = i+1, j-1 {
		pageItems[i], pageItems[j] = pageItems[j], pageItems[i]
	}

	utils.Custom(c, http.StatusOK, ChatHistoryResponse{
		Success:  true,
		Messages: pageItems,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// getUserLLMConfigForBot 根据 bot_id 和 user_id 获取用户级 LLM 配置
func (s *AppService) getUserLLMConfigForBot(ctx context.Context, userID uint, botID uint) (*llm.Config, error) {
	// 使用 bot service 的连表查询方法
	botLLMConfig, err := s.botService.GetUserBotLLMConfig(ctx, userID, botID)
	if err != nil {
		return nil, err
	}

	// 构建 LLM Config
	userLLMConfig := &llm.Config{
		Name:      fmt.Sprintf("user_%d_bot_%v_id_%d", userID, botLLMConfig.LLMType, botID),
		Type:      botLLMConfig.LLMProtocol,
		ModelName: botLLMConfig.ModelName,
		BaseURL:   botLLMConfig.BaseURL,
		APIKey:    "", // 默认为空，后续从 user_friend 或系统配置获取
		Extra:     make(map[string]interface{}),
	}

	// Bot 级别的配置覆盖（如果有）
	if botLLMConfig.Temperature != 0 {
		userLLMConfig.Temperature = float64(botLLMConfig.Temperature)
	}
	if botLLMConfig.MaxTokens != 0 {
		userLLMConfig.MaxTokens = botLLMConfig.MaxTokens
	}

	// 根据 requires_network 字段设置 enable_search
	if botLLMConfig.RequiresNetwork {
		userLLMConfig.Extra["enable_search"] = true
		s.logger.Info("Bot %d 启用联网搜索功能", botID)
	}

	// 用户级别的 AppKey 覆盖（优先级最高）
	if botLLMConfig.AppKey != "" {
		userLLMConfig.APIKey = botLLMConfig.AppKey
	} else {
		// 如果用户没有配置 AppKey，使用系统配置
		if systemLLMConfig, ok := s.config.LLM[botLLMConfig.LLMType]; ok {
			userLLMConfig.APIKey = systemLLMConfig.APIKey
		}
	}

	return userLLMConfig, nil
}

// applyUserLLMConfig 应用用户级 LLM 配置到 provider
func (s *AppService) applyUserLLMConfig(llmProvider providers.LLMProvider, userConfig *llm.Config) error {
	// 类型断言检查是否支持配置更新
	type ConfigurableLLMProvider interface {
		UpdateConfig(userConfig *llm.Config) error
	}

	if configurable, ok := llmProvider.(ConfigurableLLMProvider); ok {
		if err := configurable.UpdateConfig(userConfig); err != nil {
			return fmt.Errorf("更新LLM配置失败: %v", err)
		}
		s.logger.Info("成功应用用户LLM配置")
		return nil
	}

	s.logger.Warn("LLM provider 不支持动态配置更新")
	return nil
}

// generateSummaryAndKeyPoints 调用LLM生成摘要和关键点
func (s *AppService) generateSummaryAndKeyPoints(text string) (string, []string, error) {
	// 获取或初始化资源池管理器
	if s.poolMgr == nil {
		if pm, e := pool.NewPoolManager(s.config, s.logger); e == nil {
			s.poolMgr = pm
		} else {
			return "", nil, fmt.Errorf("初始化资源池失败: %v", e)
		}
	}

	// 从资源池获取LLM提供者
	set, err := s.poolMgr.GetProviderSet()
	if err != nil || set.LLM == nil {
		return "", nil, fmt.Errorf("获取LLM提供者失败: %v", err)
	}
	llmProvider := set.LLM

	// 构建提示词，要求返回JSON格式
	prompt := fmt.Sprintf(`请分析以下语音识别的文本内容，生成摘要和关键点。

文本内容：
%s

请以JSON格式返回结果，格式如下：
{
  "summary": "简短的摘要（不超过100字）",
  "key_points": ["关键点1", "关键点2", "关键点3"]
}

注意：
1. 摘要要简洁明了，突出核心内容
2. 关键点提取3-5个最重要的信息点
3. 只返回JSON，不要有其他文字`, text)

	messages := []providers.Message{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	// 调用LLM生成
	ctx := context.Background()
	sessionID := "summary_generation"
	llmProvider.SetIdentityFlag("session", sessionID)

	responses, err := llmProvider.ResponseWithFunctions(ctx, sessionID, messages, nil)
	if err != nil {
		return "", nil, fmt.Errorf("LLM生成失败: %v", err)
	}

	// 收集完整回复
	var fullReply strings.Builder
	for response := range responses {
		if response.Error != "" {
			return "", nil, fmt.Errorf("LLM响应错误: %s", response.Error)
		}
		fullReply.WriteString(response.Content)
	}

	// 解析JSON结果
	result := fullReply.String()

	// 尝试提取JSON（可能包含markdown代码块）
	jsonStr := result
	if idx := strings.Index(result, "{"); idx >= 0 {
		if endIdx := strings.LastIndex(result, "}"); endIdx > idx {
			jsonStr = result[idx : endIdx+1]
		}
	}

	var parsed struct {
		Summary   string   `json:"summary"`
		KeyPoints []string `json:"key_points"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		s.logger.Warn("解析LLM返回的JSON失败: %v, 原始内容: %s", err, result)
		// 如果解析失败，返回原始文本作为摘要
		return result, []string{}, nil
	}

	return parsed.Summary, parsed.KeyPoints, nil
}
