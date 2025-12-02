package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"angrymiao-ai-server/src/configs"
	"angrymiao-ai-server/src/core/auth"
	"angrymiao-ai-server/src/core/botconfig"
	"angrymiao-ai-server/src/core/chat"
	"angrymiao-ai-server/src/core/function"
	"angrymiao-ai-server/src/core/image"
	"angrymiao-ai-server/src/core/mcp"
	"angrymiao-ai-server/src/core/pool"
	"angrymiao-ai-server/src/core/providers"
	"angrymiao-ai-server/src/core/providers/llm"
	"angrymiao-ai-server/src/core/providers/tts"
	providersvad "angrymiao-ai-server/src/core/providers/vad"
	"angrymiao-ai-server/src/core/providers/vlllm"
	"angrymiao-ai-server/src/core/types"
	"angrymiao-ai-server/src/core/utils"
	"angrymiao-ai-server/src/task"

	"github.com/angrymiao/go-openai"
	"github.com/google/uuid"
)

type MCPResultHandler func(args interface{}) string

// Connection 统一连接接口
type Connection interface {
	// 发送消息
	WriteMessage(messageType int, data []byte) error
	// 读取消息
	ReadMessage(stopChan <-chan struct{}) (messageType int, data []byte, err error)
	// 关闭连接
	Close() error
	// 获取连接ID
	GetID() string
	// 获取连接类型
	GetType() string
	// 检查连接状态
	IsClosed() bool
	// 获取最后活跃时间
	GetLastActiveTime() time.Time
	// 检查是否过期
	IsStale(timeout time.Duration) bool
}

// UDPInfoProvider 提供UDP配置信息的接口（可选）
type UDPInfoProvider interface {
	// 获取UDP配置信息
	GetUDPInfo() (enabled bool, server, port, key, nonce string)
}

type configGetter interface {
	Config() *tts.Config
}

// ConnectionHandler 连接处理器结构
type ConnectionHandler struct {
	// 确保实现 AsrEventListener 接口
	_                providers.AsrEventListener
	config           *configs.Config
	logger           *utils.Logger
	conn             Connection
	closeOnce        sync.Once
	taskMgr          *task.TaskManager
	authManager      *auth.AuthManager // 认证管理器
	safeCallbackFunc func(func(*ConnectionHandler)) func()
	providers        struct {
		asr   providers.ASRProvider
		llm   providers.LLMProvider
		tts   providers.TTSProvider
		vlllm *vlllm.Provider // VLLLM提供者，可选
		vad   providersvad.Provider
	}

	initailVoice string // 初始语音名称

	// 会话相关
	sessionID     string            // 设备与服务端会话ID
	deviceID      string            // 设备ID
	clientId      string            // 客户端ID
	headers       map[string]string // HTTP头部信息
	transportType string            // 传输类型

	// 客户端音频相关
	clientAudioFormat        string
	clientAudioSampleRate    int
	clientAudioChannels      int
	clientAudioFrameDuration int

	// 客户端UDP地址信息（用于NAT穿透）
	clientPublicIP string // 客户端提供的公网IP
	clientUDPPort  int    // 客户端UDP监听端口

	serverAudioFormat        string // 服务端音频格式
	serverAudioSampleRate    int
	serverAudioChannels      int
	serverAudioFrameDuration int

	clientListenMode string
	isDeviceVerified bool
	closeAfterChat   bool
	enableVAD        bool
	vadState         *VADState // VAD状态管理器

	// 语音处理相关
	clientVoiceStop bool  // true客户端语音停止, 不再上传语音数据
	serverVoiceStop int32 // 1表示true服务端语音停止, 不再下发语音数据

	opusDecoder *utils.OpusDecoder // Opus解码器

	// 对话相关
	dialogueManager     *chat.DialogueManager
	tts_last_text_index int
	client_asr_text     string // 客户端ASR文本
	quickReplyCache     *utils.QuickReplyCache

	// 并发控制
	stopChan         chan struct{}
	clientAudioQueue chan []byte
	clientTextQueue  chan string
	mcpMessageQueue  chan map[string]interface{}

	// TTS任务队列
	ttsQueue chan struct {
		text      string
		round     int // 轮次
		textIndex int
	}

	audioMessagesQueue chan struct {
		filepath  string
		text      string
		round     int // 轮次
		textIndex int
	}

	talkRound      int       // 轮次计数
	roundStartTime time.Time // 轮次开始时间
	// functions
	functionRegister *function.FunctionRegistry
	mcpManager       *mcp.Manager

	// Bot配置服务（从好友表获取配置）
	userConfigService botconfig.Service
	userID            string             // 从JWT中提取的用户ID
	request           *http.Request      // HTTP请求对象，用于获取用户配置等信息
	userConfigs       []*types.BotConfig // 缓存用户Bot配置，避免重复查询

	mcpResultHandlers map[string]func(args interface{}) // MCP处理器映射
	ctx               context.Context
}

// NewConnectionHandler 创建新的连接处理器
func NewConnectionHandler(
	config *configs.Config,
	providerSet *pool.ProviderSet,
	logger *utils.Logger,
	req *http.Request,
	ctx context.Context,
) *ConnectionHandler {
	handler := &ConnectionHandler{
		config:           config,
		logger:           logger,
		clientListenMode: "auto",
		stopChan:         make(chan struct{}),
		clientAudioQueue: make(chan []byte, 100),
		clientTextQueue:  make(chan string, 100),
		mcpMessageQueue:  make(chan map[string]interface{}, 100),
		ttsQueue: make(chan struct {
			text      string
			round     int // 轮次
			textIndex int
		}, 100),
		audioMessagesQueue: make(chan struct {
			filepath  string
			text      string
			round     int // 轮次
			textIndex int
		}, 100),

		tts_last_text_index: -1,

		talkRound: 0,

		serverAudioFormat:        "opus", // 默认使用Opus格式
		serverAudioSampleRate:    16000,
		serverAudioChannels:      1,
		serverAudioFrameDuration: 60,

		ctx:     ctx,
		request: req, // 保存HTTP请求对象

		headers: make(map[string]string),
	}

	var enableVADHeader string
	for key, values := range req.Header {
		if len(values) > 0 {
			handler.headers[key] = values[0]
		}
		if key == "Device-Id" {
			handler.deviceID = values[0]
		}
		if key == "Client-Id" {
			handler.clientId = values[0]
		}
		if key == "Session-Id" {
			handler.sessionID = values[0]
		}
		if key == "Transport-Type" {
			handler.transportType = values[0]
		}
		if key == "Enable-VAD" {
			enableVADHeader = values[0]
		}
		logger.Info("HTTP头部信息: %s: %s", key, values[0])
	}

	if handler.sessionID == "" {
		if handler.deviceID == "" {
			handler.sessionID = uuid.New().String() // 如果没有设备ID，则生成新的会话ID
		} else {
			handler.sessionID = "device-" + strings.Replace(handler.deviceID, ":", "_", -1)
		}
	}

	// 正确设置providers
	if providerSet != nil {
		handler.providers.asr = providerSet.ASR
		handler.providers.llm = providerSet.LLM
		handler.providers.tts = providerSet.TTS
		handler.providers.vlllm = providerSet.VLLLM
		handler.providers.vad = providerSet.VAD
		handler.mcpManager = providerSet.MCP
	}

	// VAD 默认不启用，只有在客户端明确传递 Enable-VAD: true 时才启用
	handler.enableVAD = false
	if enableVADHeader != "" {
		lv := strings.ToLower(enableVADHeader)
		if lv == "true" && handler.providers.vad != nil {
			handler.enableVAD = true
			logger.Info("客户端请求启用VAD")
		} else if lv == "false" {
			handler.enableVAD = false
		}
	}

	// 初始化VAD状态管理器
	// 默认帧大小：16000Hz * 2字节/采样 * 20ms / 1000 = 640字节
	// 静音阈值：200ms
	if handler.enableVAD {
		handler.vadState = NewVADState(640, 200)
		// 设置最大缓冲帧数为3帧
		handler.vadState.SetMaxBufferFrames(3)
	}

	ttsProvider := "default" // 默认TTS提供者名称
	voiceName := "default"
	if getter, ok := handler.providers.tts.(configGetter); ok {
		ttsProvider = getter.Config().Type
		voiceName = getter.Config().Voice
		handler.initailVoice = voiceName // 保存初始语音名称
	}
	logger.Info("使用TTS提供者: %s, 语音名称: %s", ttsProvider, voiceName)
	handler.quickReplyCache = utils.NewQuickReplyCache(ttsProvider, voiceName)

	handler.functionRegister = function.NewFunctionRegistry()
	handler.initMCPResultHandlers()

	return handler
}

func (h *ConnectionHandler) SetTaskCallback(callback func(func(*ConnectionHandler)) func()) {
	h.safeCallbackFunc = callback
}

// SetUserConfigService 注入Bot配置服务
func (h *ConnectionHandler) SetUserConfigService(s botconfig.Service) {
	h.userConfigService = s
}

// SetTaskManager 注入任务管理器
func (h *ConnectionHandler) SetTaskManager(tm *task.TaskManager) {
	h.taskMgr = tm
}

func (h *ConnectionHandler) SetUserID(id string) {
	h.userID = id
}

func (h *ConnectionHandler) SubmitTask(taskType string, params map[string]interface{}) {
	_task, id := task.NewTask(h.ctx, "", params)
	h.LogInfo(fmt.Sprintf("提交任务: %s, ID: %s, 参数: %v", _task.Type, id, params))
	// 创建安全回调用于任务完成时调用
	var taskCallback func(result interface{})
	if h.safeCallbackFunc != nil {
		taskCallback = func(result interface{}) {
			fmt.Print("任务完成回调: ")
			safeCallback := h.safeCallbackFunc(func(handler *ConnectionHandler) {
				// 处理任务完成逻辑
				handler.handleTaskComplete(_task, id, result)
			})
			// 执行安全回调
			if safeCallback != nil {
				safeCallback()
			}
		}
	}
	cb := task.NewCallBack(taskCallback)
	_task.Callback = cb
	h.taskMgr.SubmitTask(h.sessionID, _task)
}

func (h *ConnectionHandler) handleTaskComplete(task *task.Task, id string, result interface{}) {
	h.LogInfo(fmt.Sprintf("任务 %s 完成，ID: %s, %v", task.Type, id, result))
}

func (h *ConnectionHandler) LogInfo(msg string) {
	if h.logger != nil {
		h.logger.Info(msg, map[string]interface{}{
			"device": h.deviceID,
		})
	}
}
func (h *ConnectionHandler) LogError(msg string) {
	if h.logger != nil {
		h.logger.Error(msg, map[string]interface{}{
			"device": h.deviceID,
		})
	}
}

// Handle 处理WebSocket连接
func (h *ConnectionHandler) Handle(conn Connection) {
	defer conn.Close()

	h.conn = conn

	h.loadUserDialogueManager()
	h.loadUserAIConfigurations()

	// ========== 用户配置注入点 ==========
	// 在这里可以注入用户级的 provider 配置
	// 示例 1：LLM 配置
	// userLLMConfig := &llm.Config{
	// 	Temperature: 0.8,
	// 	MaxTokens:   2000,
	// }
	// h.ApplyUserLLMConfig(userLLMConfig)
	//
	// 示例 2：TTS 配置
	// userTTSConfig := &tts.Config{
	// 	Voice: "zh-CN-XiaoxiaoNeural",
	// }
	// h.ApplyUserTTSConfig(userTTSConfig)
	// ====================================

	// 启动消息处理协程
	go h.processClientAudioMessagesCoroutine() // 添加客户端音频消息处理协程
	go h.processClientTextMessagesCoroutine()  // 添加客户端文本消息处理协程
	go h.processMCPMessagesCoroutine()         // 添加MCP消息处理协程（独立于文本队列）
	go h.processTTSQueueCoroutine()            // 添加TTS队列处理协程
	go h.sendAudioMessageCoroutine()           // 添加音频消息发送协程

	// 优化后的MCP管理器处理
	if h.mcpManager == nil {
		h.LogError("没有可用的MCP管理器")
		return

	} else {
		h.LogInfo("[MCP] [管理器] 使用资源池快速绑定连接")
		// 池化的管理器已经预初始化，只需要绑定连接
		params := map[string]interface{}{
			"session_id": h.sessionID,
			"vision_url": h.config.Web.VisionURL,
			"device_id":  h.deviceID,
			"client_id":  h.clientId,
			"token":      h.config.Server.Token,
		}
		if err := h.mcpManager.BindConnection(conn, h.functionRegister, params); err != nil {
			h.LogError(fmt.Sprintf("绑定MCP管理器连接失败: %v", err))
			return
		}
		// 不需要重新初始化服务器，只需要确保连接相关的服务正常
		h.LogInfo("[MCP] [绑定] 连接绑定完成，跳过重复初始化")
	}

	// 主消息循环
	for {
		select {
		case <-h.stopChan:
			return
		default:
			messageType, message, err := conn.ReadMessage(h.stopChan)
			if err != nil {
				h.LogError(fmt.Sprintf("读取消息失败: %v, 退出主消息循环", err))
				return
			}

			if err := h.handleMessage(messageType, message); err != nil {
				h.LogError(fmt.Sprintf("处理消息失败: %v", err))
			}
		}
	}
}

// processClientTextMessagesCoroutine 处理文本消息队列
func (h *ConnectionHandler) processClientTextMessagesCoroutine() {
	for {
		select {
		case <-h.stopChan:
			return
		case text := <-h.clientTextQueue:
			if err := h.processClientTextMessage(context.Background(), text); err != nil {
				h.LogError(fmt.Sprintf("处理文本数据失败: %v", err))
			}
		}
	}
}

// processMCPMessagesCoroutine 处理MCP消息队列（与文本处理并行）
func (h *ConnectionHandler) processMCPMessagesCoroutine() {
	for {
		select {
		case <-h.stopChan:
			return
		case msg := <-h.mcpMessageQueue:
			if err := h.mcpManager.HandleAMMCPMessage(msg); err != nil {
				h.LogError(fmt.Sprintf("处理MCP消息失败: %v", err))
			}
		}
	}
}

// processClientAudioMessagesCoroutine 处理音频消息队列
// 音频缓冲、VAD检测、空闲时间管理、静音检测
func (h *ConnectionHandler) processClientAudioMessagesCoroutine() {
	for {
		select {
		case <-h.stopChan:
			return
		case audioData := <-h.clientAudioQueue:
			if h.closeAfterChat {
				continue
			}

			// 如果启用VAD，则进行完整的VAD处理流程
			if h.enableVAD && h.providers.vad != nil && h.vadState != nil {
				h.processAudioWithVAD(audioData)
			} else {
				// 未启用VAD，直接送入ASR
				if err := h.providers.asr.AddAudio(audioData); err != nil {
					h.LogError(fmt.Sprintf("处理音频数据失败: %v", err))
				}
			}
		}
	}
}

// processAudioWithVAD 使用VAD处理音频数据
// 完整逻辑：缓冲管理、VAD检测、空闲时间累计、静音检测
func (h *ConnectionHandler) processAudioWithVAD(audioData []byte) {
	// 获取音频参数
	sr := h.clientAudioSampleRate
	if sr <= 0 {
		sr = 16000
	}
	frameMs := h.clientAudioFrameDuration
	if frameMs <= 0 {
		frameMs = 20
	}

	// 计算帧大小
	bytesPerSample := 2 // 16位PCM
	frameBytes := sr * bytesPerSample * frameMs / 1000
	if frameBytes <= 0 {
		frameBytes = 640 // 16000Hz * 2 * 20ms / 1000
	}

	// 更新VAD状态的帧大小（如果与配置不同）
	// 注意：实际音频数据长度可能不是标准帧大小，需要动态调整
	if len(audioData) > 0 && len(audioData) != h.vadState.frameSize {
		h.vadState.frameSize = len(audioData)
		h.LogInfo(fmt.Sprintf("动态调整VAD帧大小: %d字节 (基于实际音频数据长度)", len(audioData)))
	}

	// 根据实际音频数据大小计算真实的帧长度
	// 公式：帧长度(ms) = 数据大小(字节) * 1000 / (采样率 * 每样本字节数)
	actualFrameMs := frameMs
	if len(audioData) > 0 {
		calculatedFrameMs := len(audioData) * 1000 / (sr * bytesPerSample)
		if calculatedFrameMs > 0 && calculatedFrameMs != frameMs {
			actualFrameMs = calculatedFrameMs
			h.LogInfo(fmt.Sprintf("根据实际数据计算帧长度: %dms (数据大小:%d字节, 配置:%dms)", actualFrameMs, len(audioData), frameMs))
		}
	}
	if actualFrameMs <= 0 {
		actualFrameMs = 20 // 默认20ms
	}

	// 每次处理一帧数据
	vadCheckFrames := 1
	h.vadState.SetVADCheckFrames(vadCheckFrames)

	// 将音频数据添加到缓冲区
	h.vadState.AddAudioData(audioData)

	// 检查是否有足够的数据进行VAD检测
	if !h.vadState.HasEnoughDataForVAD() {
		// 数据不足，等待更多数据
		return
	}

	// 获取用于VAD检测的数据（不删除缓冲区数据）
	vadData := h.vadState.GetBufferedData(vadCheckFrames)

	// 每次检测前重置VAD状态，避免状态累积
	if resetter, ok := h.providers.vad.(interface{ Reset() error }); ok {
		if err := resetter.Reset(); err != nil {
			h.LogError(fmt.Sprintf("VAD重置失败: %v", err))
		}
	}

	// 调用VAD检测（使用调整后的帧持续时间）
	haveVoice, err := h.providers.vad.Process(vadData, sr, actualFrameMs)
	if err != nil {
		h.LogError(fmt.Sprintf("VAD检测失败: %v", err))
		// VAD失败时，为了保险起见，假设有语音
		haveVoice = true
	}

	// 获取当前语音状态
	clientHaveVoice := h.vadState.GetHaveVoice()

	// 处理首次检测到语音的情况
	if haveVoice && !clientHaveVoice {
		h.LogInfo("首次检测到语音活动")
		// 首次检测到语音，将所有缓冲的音频数据送入ASR
		allData := h.vadState.GetAndClearAllData()
		if err := h.providers.asr.AddAudio(allData); err != nil {
			h.LogError(fmt.Sprintf("处理音频数据失败: %v", err))
		}

		// 更新语音活动状态
		h.vadState.SetHaveVoice(true)
		h.vadState.SetHaveVoiceLastTime(time.Now().UnixMilli())
		h.vadState.ResetIdleDuration()
		return
	}

	// 如果已经检测到语音，后续音频直接送入ASR
	if clientHaveVoice {
		// 清空缓冲区并送入ASR
		bufferedData := h.vadState.GetAndClearAllData()
		if len(bufferedData) > 0 {
			if err := h.providers.asr.AddAudio(bufferedData); err != nil {
				h.LogError(fmt.Sprintf("处理音频数据失败: %v", err))
			}
		}

		// 更新最后语音时间
		if haveVoice {
			h.vadState.SetHaveVoiceLastTime(time.Now().UnixMilli())
			h.vadState.ResetIdleDuration()
		} else {
			// 当前帧无语音，累加空闲时间
			h.vadState.AddIdleDuration(int64(frameMs))
		}

		// 检查是否进入静音状态（语音结束检测）
		idleDuration := h.vadState.GetIdleDuration()
		if h.vadState.IsSilence(idleDuration) {
			h.LogInfo(fmt.Sprintf("检测到静音，空闲时间: %dms，触发语音结束", idleDuration))
			h.vadState.SetVoiceStop(true)
			// 可以在这里触发ASR的FinalResult或其他处理
		}

		return
	}

	// 如果之前没有语音，本次也没有语音
	if !haveVoice && !clientHaveVoice {
		// 累加空闲时间
		h.vadState.AddIdleDuration(int64(frameMs))
		idleDuration := h.vadState.GetIdleDuration()

		// 检查是否超过最大空闲时间
		maxIdleDuration := int64(30000) // 30秒
		if idleDuration > maxIdleDuration {
			h.LogInfo(fmt.Sprintf("超出最大空闲时长: %dms，可能需要断开连接", idleDuration))
			// 这里可以根据需要决定是否断开连接
		}

		// 保留最近的N帧，删除更早的帧以避免缓冲区无限增长
		maxBufferFrames := h.vadState.GetMaxBufferFrames()
		if h.vadState.GetBufferedFrameCount() > vadCheckFrames*3 {
			h.vadState.RemoveOldFrames(maxBufferFrames)
		}

		// 未检测到语音活动
		// h.LogInfo("未检测到语音，空闲时间: %dms，缓冲帧数: %d", idleDuration, h.vadState.GetBufferedFrameCount())
	}
}

func (h *ConnectionHandler) sendAudioMessageCoroutine() {
	for {
		select {
		case <-h.stopChan:
			return
		case task := <-h.audioMessagesQueue:
			h.sendAudioMessage(task.filepath, task.text, task.textIndex, task.round)
		}
	}
}

// OnAsrResult 实现 AsrEventListener 接口
// 返回true则停止语音识别，返回false会继续语音识别
func (h *ConnectionHandler) OnAsrResult(result string, isFinalResult bool) bool {
	//h.LogInfo(fmt.Sprintf("[%s] ASR识别结果: %s", h.clientListenMode, result))
	if h.providers.asr.GetSilenceCount() >= 2 {
		h.LogInfo("检测到连续两次静音，结束对话")
		h.closeAfterChat = true // 如果连续两次静音，则结束对话
		result = "长时间未检测到用户说话，请礼貌的结束对话"
	}
	if h.clientListenMode == "auto" {
		if result == "" {
			return false
		}
		h.LogInfo(fmt.Sprintf("[%s] ASR识别结果: %s", h.clientListenMode, result))
		h.handleChatMessage(context.Background(), result)
		return true
	} else if h.clientListenMode == "manual" {
		h.client_asr_text += result
		if isFinalResult {
			h.handleChatMessage(context.Background(), h.client_asr_text)
			return true
		}
		return false

		// h.client_asr_text += result
		// if result != "" {
		// 	h.LogInfo(fmt.Sprintf("[%s] ASR识别结果: %s", h.clientListenMode, h.client_asr_text))
		// }
		// if h.clientVoiceStop && h.client_asr_text != "" {
		// 	// 防止重复处理，只处理一次完整的ASR文本
		// 	asrText := h.client_asr_text
		// 	h.client_asr_text = "" // 清空文本，防止重复处理
		// 	h.handleChatMessage(context.Background(), asrText)
		// 	return true
		// }
		// return false
	} else if h.clientListenMode == "realtime" {
		if result == "" {
			return false
		}
		h.stopServerSpeak()
		h.providers.asr.Reset() // 重置ASR状态，准备下一次识别
		h.LogInfo(fmt.Sprintf("[%s] ASR识别结果: %s", h.clientListenMode, result))
		h.handleChatMessage(context.Background(), result)
		return true
	}
	return false
}

// clientAbortChat 处理中止消息
func (h *ConnectionHandler) clientAbortChat() error {
	h.LogInfo("收到客户端中止消息，停止语音识别")
	h.stopServerSpeak()
	h.sendTTSMessage("stop", "", 0)
	h.clearSpeakStatus()
	return nil
}

func (h *ConnectionHandler) QuitIntent(text string) bool {
	//CMD_exit 读取配置中的退出命令
	exitCommands := h.config.CMDExit
	if exitCommands == nil {
		return false
	}
	cleand_text := utils.RemoveAllPunctuation(text) // 移除标点符号，确保匹配准确
	// 检查是否包含退出命令
	for _, cmd := range exitCommands {
		h.logger.Debug(fmt.Sprintf("检查退出命令: %s,%s", cmd, cleand_text))
		//判断相等
		if cleand_text == cmd {
			h.LogInfo("收到客户端退出意图，准备结束对话")
			h.Close() // 直接关闭连接
			return true
		}
	}
	return false
}

func (h *ConnectionHandler) quickReplyWakeUpWords(text string) bool {
	// 检查是否包含唤醒词
	if !h.config.QuickReply || h.talkRound != 1 {
		return false
	}
	if !utils.IsWakeUpWord(text) {
		return false
	}

	repalyWords := h.config.QuickReplyWords
	reply_text := utils.RandomSelectFromArray(repalyWords)
	h.tts_last_text_index = 1 // 重置文本索引
	h.SpeakAndPlay(reply_text, 1, h.talkRound)

	return true
}

// handleChatMessage 处理聊天消息
func (h *ConnectionHandler) handleChatMessage(ctx context.Context, text string) error {
	if text == "" {
		h.logger.Warn("收到空聊天消息，忽略")
		h.clientAbortChat()
		return fmt.Errorf("聊天消息为空")
	}

	if h.QuitIntent(text) {
		return fmt.Errorf("用户请求退出对话")
	}

	// 增加对话轮次
	h.talkRound++
	h.roundStartTime = time.Now()
	currentRound := h.talkRound
	h.LogInfo(fmt.Sprintf("开始新的对话轮次: %d", currentRound))

	// 普通文本消息处理流程
	// 立即发送 stt 消息
	err := h.sendSTTMessage(text)
	if err != nil {
		h.LogError(fmt.Sprintf("发送STT消息失败: %v", err))
		return fmt.Errorf("发送STT消息失败: %v", err)
	}

	// 发送tts start状态
	if err := h.sendTTSMessage("start", "", 0); err != nil {
		h.LogError(fmt.Sprintf("发送TTS开始状态失败: %v", err))
		return fmt.Errorf("发送TTS开始状态失败: %v", err)
	}

	// 发送思考状态的情绪
	// if err := h.sendEmotionMessage("thinking"); err != nil {
	// 	h.LogError(fmt.Sprintf("发送思考状态情绪消息失败: %v", err))
	// 	return fmt.Errorf("发送情绪消息失败: %v", err)
	// }

	h.LogInfo("收到聊天消息: " + text)

	if h.quickReplyWakeUpWords(text) {
		return nil
	}

	// 添加用户消息到对话历史
	h.dialogueManager.Put(chat.Message{
		Role:    "user",
		Content: text,
	})

	return h.genResponseByLLM(ctx, h.dialogueManager.GetLLMDialogue(), currentRound)
}

func (h *ConnectionHandler) genResponseByLLM(ctx context.Context, messages []providers.Message, round int) error {
	defer func() {
		if r := recover(); r != nil {
			h.LogError(fmt.Sprintf("genResponseByLLM发生panic: %v", r))
			errorMsg := "抱歉，处理您的请求时发生了错误"
			h.tts_last_text_index = 1 // 重置文本索引
			h.SpeakAndPlay(errorMsg, 1, round)
		}
	}()

	llmStartTime := time.Now()
	//h.logger.Info("开始生成LLM回复, round:%d ", round)
	for _, msg := range messages {
		_ = msg
		//msg.Print()
	}
	// 使用LLM生成回复
	tools := h.functionRegister.GetAllFunctions()
	responses, err := h.providers.llm.ResponseWithFunctions(ctx, h.sessionID, messages, tools)
	if err != nil {
		return fmt.Errorf("LLM生成回复失败: %v", err)
	}

	// 处理回复
	var responseMessage []string
	processedChars := 0
	textIndex := 0

	atomic.StoreInt32(&h.serverVoiceStop, 0)

	// 处理流式响应
	toolCallFlag := false
	functionName := ""
	functionID := ""
	functionArguments := ""
	contentArguments := ""

	for response := range responses {
		content := response.Content
		toolCall := response.ToolCalls

		if response.Error != "" {
			h.LogError(fmt.Sprintf("LLM响应错误: %s", response.Error))
			errorMsg := "抱歉，服务暂时不可用，请稍后再试"
			h.tts_last_text_index = 1 // 重置文本索引
			h.SpeakAndPlay(errorMsg, 1, round)
			return fmt.Errorf("LLM响应错误: %s", response.Error)
		}

		if content != "" {
			// 累加content_arguments
			contentArguments += content
		}

		if !toolCallFlag && strings.HasPrefix(contentArguments, "<tool_call>") {
			toolCallFlag = true
		}

		if len(toolCall) > 0 {
			toolCallFlag = true
			if toolCall[0].ID != "" {
				functionID = toolCall[0].ID
			}
			if toolCall[0].Function.Name != "" {
				functionName = toolCall[0].Function.Name
			}
			if toolCall[0].Function.Arguments != "" {
				functionArguments += toolCall[0].Function.Arguments
			}
		}

		if content != "" {
			if strings.Contains(content, "服务响应异常") {
				h.LogError(fmt.Sprintf("检测到LLM服务异常: %s", content))
				errorMsg := "抱歉，LLM服务暂时不可用，请稍后再试"
				h.tts_last_text_index = 1 // 重置文本索引
				h.SpeakAndPlay(errorMsg, 1, round)
				return fmt.Errorf("LLM服务异常")
			}

			if toolCallFlag {
				continue
			}

			responseMessage = append(responseMessage, content)
			// 处理分段
			fullText := utils.JoinStrings(responseMessage)
			if len(fullText) <= processedChars {
				h.logger.Warn(fmt.Sprintf("文本处理异常: fullText长度=%d, processedChars=%d", len(fullText), processedChars))
				continue
			}
			currentText := fullText[processedChars:]

			// 按标点符号分割
			if segment, charsCnt := utils.SplitAtLastPunctuation(currentText); charsCnt > 0 {
				textIndex++
				segment = strings.TrimSpace(segment)
				if textIndex == 1 {
					now := time.Now()
					llmSpentTime := now.Sub(llmStartTime)
					h.LogInfo(fmt.Sprintf("LLM回复耗时 %s 生成第一句话【%s】, round: %d", llmSpentTime, segment, round))
				} else {
					h.LogInfo(fmt.Sprintf("LLM回复分段: %s, index: %d, round:%d", segment, textIndex, round))
				}
				h.tts_last_text_index = textIndex
				err := h.SpeakAndPlay(segment, textIndex, round)
				if err != nil {
					h.LogError(fmt.Sprintf("播放LLM回复分段失败: %v", err))
				}
				processedChars += charsCnt
			}
		}
	}

	if toolCallFlag {
		bHasError := false
		if functionID == "" {
			a := utils.Extract_json_from_string(contentArguments)
			if a != nil {
				functionName = a["name"].(string)
				argumentsJson, err := json.Marshal(a["arguments"])
				if err != nil {
					h.LogError(fmt.Sprintf("函数调用参数解析失败: %v", err))
				}
				functionArguments = string(argumentsJson)
				functionID = uuid.New().String()
			} else {
				bHasError = true
			}
			if bHasError {
				h.LogError(fmt.Sprintf("函数调用参数解析失败: %v", err))
			}
		}
		if !bHasError {
			// 清空responseMessage
			responseMessage = []string{}
			arguments := make(map[string]interface{})
			if err := json.Unmarshal([]byte(functionArguments), &arguments); err != nil {
				h.LogError(fmt.Sprintf("函数调用参数解析失败: %v", err))
			}
			functionCallData := map[string]interface{}{
				"id":        functionID,
				"name":      functionName,
				"arguments": functionArguments,
			}
			h.LogInfo(fmt.Sprintf("函数调用: %v", arguments))
			if h.mcpManager.IsMCPTool(functionName) {
				// 处理MCP函数调用
				result, err := h.mcpManager.ExecuteTool(ctx, functionName, arguments)
				if err != nil {
					h.LogError(fmt.Sprintf("MCP函数调用失败: %v", err))
					if result == nil {
						result = "MCP工具调用失败"
					}
				}
				// 判断result 是否是types.ActionResponse类型
				if actionResult, ok := result.(types.ActionResponse); ok {
					h.handleFunctionResult(actionResult, functionCallData, textIndex)
				} else {
					h.LogInfo(fmt.Sprintf("MCP函数调用结果: %v", result))
					actionResult := types.ActionResponse{
						Action: types.ActionTypeReqLLM, // 动作类型
						Result: result,                 // 动作产生的结果
					}
					h.handleFunctionResult(actionResult, functionCallData, textIndex)
				}

			} else {
				// 处理普通函数调用
				userFunCallConfig := types.BotConfig{}
				if h.userConfigs != nil {
					for _, v := range h.userConfigs {
						if v.FunctionName == functionName {
							userFunCallConfig = *v
							break
						}
					}
				}
				if userFunCallConfig.FunctionName != "" {
					funResult, err := h.executeUserFunctionCall(&userFunCallConfig, functionCallData)
					if err != nil {
						h.LogError(fmt.Sprintf("MCP函数调用失败: %v", err))
						if funResult.Result == "" {
							funResult.Result = "BOT 模型调用失败"
						}
					}

					actionResult := types.ActionResponse{
						Action: types.ActionTypeReqLLM,
						Result: funResult.Result,
					}
					h.handleFunctionResult(actionResult, functionCallData, textIndex)
				}
			}
		}
	}

	// 处理剩余文本
	fullResponse := utils.JoinStrings(responseMessage)
	if len(fullResponse) > processedChars {
		remainingText := fullResponse[processedChars:]
		if remainingText != "" {
			textIndex++
			h.LogInfo(fmt.Sprintf("LLM回复分段[剩余文本]: %s, index: %d, round:%d", remainingText, textIndex, round))
			h.tts_last_text_index = textIndex
			h.SpeakAndPlay(remainingText, textIndex, round)
		}
	} else {
		h.logger.Debug("无剩余文本需要处理: fullResponse长度=%d, processedChars=%d", len(fullResponse), processedChars)
	}

	// 分析回复并发送相应的情绪
	content := utils.JoinStrings(responseMessage)

	// 添加助手回复到对话历史
	if !toolCallFlag {
		h.dialogueManager.Put(chat.Message{
			Role:    "assistant",
			Content: content,
		})
	}

	return nil
}

func (h *ConnectionHandler) addToolCallMessage(toolResultText string, functionCallData map[string]interface{}) {

	functionID := functionCallData["id"].(string)
	functionName := functionCallData["name"].(string)
	functionArguments := functionCallData["arguments"].(string)
	h.LogInfo(fmt.Sprintf("函数调用结果: %s", toolResultText))
	h.LogInfo(fmt.Sprintf("函数调用参数: %s", functionArguments))
	h.LogInfo(fmt.Sprintf("函数调用名称: %s", functionName))
	h.LogInfo(fmt.Sprintf("函数调用ID: %s", functionID))

	// 添加 assistant 消息，包含 tool_calls
	h.dialogueManager.Put(chat.Message{
		Role: "assistant",
		ToolCalls: []types.ToolCall{{
			ID: functionID,
			Function: types.FunctionCall{
				Arguments: functionArguments,
				Name:      functionName,
			},
			Type:  "function",
			Index: 0,
		}},
	})

	// 添加 tool 消息
	toolCallID := functionID
	if toolCallID == "" {
		toolCallID = uuid.New().String()
	}
	h.dialogueManager.Put(chat.Message{
		Role:       "tool",
		ToolCallID: toolCallID,
		Content:    toolResultText,
	})
}

func (h *ConnectionHandler) handleFunctionResult(result types.ActionResponse, functionCallData map[string]interface{}, textIndex int) {
	switch result.Action {
	case types.ActionTypeError:
		h.LogError(fmt.Sprintf("函数调用错误: %v", result.Result))
	case types.ActionTypeNotFound:
		h.LogError(fmt.Sprintf("函数未找到: %v", result.Result))
	case types.ActionTypeNone:
		h.LogInfo(fmt.Sprintf("函数调用无操作: %v", result.Result))
	case types.ActionTypeResponse:
		h.LogInfo(fmt.Sprintf("函数调用直接回复: %v", result.Response))
		h.SystemSpeak(result.Response.(string))
	case types.ActionTypeCallHandler:
		resultStr := h.handleMCPResultCall(result)
		h.addToolCallMessage(resultStr, functionCallData)
	case types.ActionTypeReqLLM:
		h.LogInfo(fmt.Sprintf("函数调用后请求LLM: %v", result.Result))
		text, ok := result.Result.(string)
		if ok && len(text) > 0 {
			h.addToolCallMessage(text, functionCallData)
			h.genResponseByLLM(context.Background(), h.dialogueManager.GetLLMDialogue(), h.talkRound)

		} else {
			h.LogError(fmt.Sprintf("函数调用结果解析失败: %v", result.Result))
			// 发送错误消息
			errorMessage := fmt.Sprintf("函数调用结果解析失败 %v", result.Result)
			h.SystemSpeak(errorMessage)
		}
	}
}

func (h *ConnectionHandler) SystemSpeak(text string) error {
	if text == "" {
		h.logger.Warn("SystemSpeak 收到空文本，无法合成语音")
		return errors.New("收到空文本，无法合成语音")
	}
	texts := utils.SplitByPunctuation(text)
	index := h.tts_last_text_index
	for _, item := range texts {
		index++
		h.tts_last_text_index = index // 重置文本索引
		h.SpeakAndPlay(item, index, h.talkRound)
	}
	return nil
}

// processTTSQueueCoroutine 处理TTS队列
func (h *ConnectionHandler) processTTSQueueCoroutine() {
	for {
		select {
		case <-h.stopChan:
			return
		case task := <-h.ttsQueue:
			h.processTTSTask(task.text, task.textIndex, task.round)
		}
	}
}

// 服务端打断说话
func (h *ConnectionHandler) stopServerSpeak() {
	h.LogInfo("服务端停止说话")
	atomic.StoreInt32(&h.serverVoiceStop, 1)
	h.cleanTTSAndAudioQueue(false)
}

func (h *ConnectionHandler) deleteAudioFileIfNeeded(filepath string, reason string) {
	if !h.config.DeleteAudio || filepath == "" {
		return
	}

	// 检查是否为快速回复缓存文件，如果是则不删除
	if h.quickReplyCache != nil && h.quickReplyCache.IsCachedFile(filepath) {
		h.LogInfo(fmt.Sprintf(reason+" 跳过删除缓存音频文件: %s", filepath))
		return
	}

	// 检查是否是音乐文件，如果是则不删除
	if utils.IsMusicFile(filepath) {
		h.LogInfo(fmt.Sprintf(reason+" 跳过删除音乐文件: %s", filepath))
		return
	}

	// 删除非缓存音频文件
	if err := os.Remove(filepath); err != nil {
		h.LogError(fmt.Sprintf(reason+" 删除音频文件失败: %v", err))
	} else {
		h.logger.Debug(fmt.Sprintf(reason+" 已删除音频文件: %s", filepath))
	}
}

// processTTSTask 处理单个TTS任务
func (h *ConnectionHandler) processTTSTask(text string, textIndex int, round int) {
	filepath := ""
	defer func() {
		h.audioMessagesQueue <- struct {
			filepath  string
			text      string
			round     int
			textIndex int
		}{filepath, text, round, textIndex}
	}()

	if utils.IsQuickReplyHit(text, h.config.QuickReplyWords) {
		// 尝试从缓存查找音频文件
		if cachedFile := h.quickReplyCache.FindCachedAudio(text); cachedFile != "" {
			h.LogInfo(fmt.Sprintf("使用缓存的快速回复音频: %s", cachedFile))
			filepath = cachedFile
			return
		}
	}
	ttsStartTime := time.Now()
	// 过滤表情
	text = utils.RemoveAllEmoji(text)

	if text == "" {
		h.logger.Warn(fmt.Sprintf("收到空文本，无法合成语音, 索引: %d", textIndex))
		return
	}

	// 生成语音文件
	filepath, err := h.providers.tts.ToTTS(text)
	if err != nil {
		h.LogError(fmt.Sprintf("TTS转换失败:text(%s) %v", text, err))
		return
	} else {
		h.logger.Debug(fmt.Sprintf("TTS转换成功: text(%s), index(%d) %s", text, textIndex, filepath))
		// 如果是快速回复词，保存到缓存
		if utils.IsQuickReplyHit(text, h.config.QuickReplyWords) {
			if err := h.quickReplyCache.SaveCachedAudio(text, filepath); err != nil {
				h.LogError(fmt.Sprintf("保存快速回复音频失败: %v", err))
			} else {
				h.LogInfo(fmt.Sprintf("成功缓存快速回复音频: %s", text))
			}
		}
	}
	if atomic.LoadInt32(&h.serverVoiceStop) == 1 { // 服务端语音停止
		h.LogInfo(fmt.Sprintf("processTTSTask 服务端语音停止, 不再发送音频数据：%s", text))
		// 服务端语音停止时，根据配置删除已生成的音频文件
		h.deleteAudioFileIfNeeded(filepath, "服务端语音停止时")
		return
	}

	if textIndex == 1 {
		now := time.Now()
		ttsSpentTime := now.Sub(ttsStartTime)
		h.logger.Debug(fmt.Sprintf("TTS转换耗时: %s, 文本: %s, 索引: %d", ttsSpentTime, text, textIndex))
	}
}

// speakAndPlay 合成并播放语音
func (h *ConnectionHandler) SpeakAndPlay(text string, textIndex int, round int) error {
	defer func() {
		// 将任务加入队列，不阻塞当前流程
		h.ttsQueue <- struct {
			text      string
			round     int
			textIndex int
		}{text, round, textIndex}
	}()

	originText := text // 保存原始文本用于日志
	text = utils.RemoveAllEmoji(text)
	text = utils.RemoveMarkdownSyntax(text) // 移除Markdown语法
	if text == "" {
		h.logger.Warn("SpeakAndPlay 收到空文本，无法合成语音, %d, text:%s.", textIndex, originText)
		return errors.New("收到空文本，无法合成语音")
	}

	if atomic.LoadInt32(&h.serverVoiceStop) == 1 { // 服务端语音停止
		h.LogInfo(fmt.Sprintf("speakAndPlay 服务端语音停止, 不再发送音频数据：%s", text))
		text = ""
		return errors.New("服务端语音已停止，无法合成语音")
	}

	if len(text) > 255 {
		h.logger.Warn(fmt.Sprintf("文本过长，超过255字符限制，截断合成语音: %s", text))
		text = text[:255] // 截断文本
	}

	return nil
}

func (h *ConnectionHandler) clearSpeakStatus() {
	h.LogInfo("清除服务端讲话状态 ")
	h.tts_last_text_index = -1
	h.providers.asr.Reset() // 重置ASR状态
}

func (h *ConnectionHandler) closeOpusDecoder() {
	if h.opusDecoder != nil {
		if err := h.opusDecoder.Close(); err != nil {
			h.LogError(fmt.Sprintf("关闭Opus解码器失败: %v", err))
		}
		h.opusDecoder = nil
	}
}

func (h *ConnectionHandler) cleanTTSAndAudioQueue(bClose bool) error {
	msgPrefix := ""
	if bClose {
		msgPrefix = "关闭连接，"
	}
	// 终止tts任务，不再继续将文本加入到tts队列，清空ttsQueue队列
	for {
		select {
		case task := <-h.ttsQueue:
			h.LogInfo(fmt.Sprintf(msgPrefix+"丢弃一个TTS任务: %s", task.text))
		default:
			// 队列已清空，退出循环
			h.LogInfo(msgPrefix + "ttsQueue队列已清空，停止处理TTS任务,准备清空音频队列")
			goto clearAudioQueue
		}
	}

clearAudioQueue:
	// 终止audioMessagesQueue发送，清空队列里的音频数据
	for {
		select {
		case task := <-h.audioMessagesQueue:
			h.LogInfo(fmt.Sprintf(msgPrefix+"丢弃一个音频任务: %s", task.text))
			// 根据配置删除被丢弃的音频文件
			h.deleteAudioFileIfNeeded(task.filepath, msgPrefix+"丢弃音频任务时")
		default:
			// 队列已清空，退出循环
			h.LogInfo(msgPrefix + "audioMessagesQueue队列已清空，停止处理音频任务")
			return nil
		}
	}
}

// Close 清理资源
func (h *ConnectionHandler) Close() {
	h.closeOnce.Do(func() {
		close(h.stopChan)

		h.closeOpusDecoder()
		if h.providers.tts != nil {
			h.providers.tts.SetVoice(h.initailVoice) // 恢复初始语音
		}
		if h.providers.asr != nil {
			h.providers.asr.ResetSilenceCount() // 重置静音计数
			if err := h.providers.asr.Reset(); err != nil {
				h.LogError(fmt.Sprintf("重置ASR状态失败: %v", err))
			}

			if err := h.providers.asr.CloseConnection(); err != nil {
				h.LogError(fmt.Sprintf("断开ASR状态失败: %v", err))
			}
		}
		h.cleanTTSAndAudioQueue(true)
	})
}

// genResponseByVLLM 使用VLLLM处理包含图片的消息
func (h *ConnectionHandler) genResponseByVLLM(ctx context.Context, messages []providers.Message, imageData image.ImageData, text string, round int) error {
	h.logger.Info("开始生成VLLLM回复 %v", map[string]interface{}{
		"text":          text,
		"has_url":       imageData.URL != "",
		"has_data":      imageData.Data != "",
		"format":        imageData.Format,
		"message_count": len(messages),
	})

	// 使用VLLLM处理图片和文本
	responses, err := h.providers.vlllm.ResponseWithImage(ctx, h.sessionID, messages, imageData, text)
	if err != nil {
		h.LogError(fmt.Sprintf("VLLLM生成回复失败，尝试降级到普通LLM: %v", err))
		// 降级策略：只使用文本部分调用普通LLM
		fallbackText := fmt.Sprintf("用户发送了一张图片并询问：%s（注：当前无法处理图片，只能根据文字回答）", text)
		fallbackMessages := append(messages, providers.Message{
			Role:    "user",
			Content: fallbackText,
		})
		return h.genResponseByLLM(ctx, fallbackMessages, round)
	}

	// 处理VLLLM流式回复
	var responseMessage []string
	processedChars := 0
	textIndex := 0

	atomic.StoreInt32(&h.serverVoiceStop, 0)

	for response := range responses {
		if response == "" {
			continue
		}

		responseMessage = append(responseMessage, response)
		// 处理分段
		fullText := utils.JoinStrings(responseMessage)
		currentText := fullText[processedChars:]

		// 按标点符号分割
		if segment, chars := utils.SplitAtLastPunctuation(currentText); chars > 0 {
			textIndex++
			h.tts_last_text_index = textIndex
			h.SpeakAndPlay(segment, textIndex, round)
			processedChars += chars
		}
	}

	// 处理剩余文本
	remainingText := utils.JoinStrings(responseMessage)[processedChars:]
	if remainingText != "" {
		textIndex++
		h.tts_last_text_index = textIndex
		h.SpeakAndPlay(remainingText, textIndex, round)
	}

	// 获取完整回复内容
	content := utils.JoinStrings(responseMessage)

	// 添加VLLLM回复到对话历史
	h.dialogueManager.Put(chat.Message{
		Role:    "assistant",
		Content: content,
	})

	h.LogInfo(fmt.Sprintf("VLLLM回复处理完成 …%v", map[string]interface{}{
		"content_length": len(content),
		"text_segments":  textIndex,
	}))

	return nil
}

func (h *ConnectionHandler) loadUserDialogueManager() {
	if h.userID == "" {
		h.logger.Debug("用户ID为空，跳过加载用户对话管理器")
		return
	}

	// 根据配置选择对话记忆存储：postgres、redis
	var memory chat.MemoryInterface
	switch strings.ToLower(h.config.DialogStorage) {
	case "postgres", "sqlite":
		memory = chat.NewPostgresMemory(h.userID)
	case "redis":
		if h.config.RedisCache.Addr != "" {
			if mem, err := chat.NewRedisMemory(h.config.RedisCache, h.logger, h.userID); err != nil {
				h.logger.Warn("初始化Redis记忆失败: %v，使用内存模式", err)
			} else {
				memory = mem
			}
		} else {
			h.logger.Warn("Redis未配置，回退到内存模式")
		}
	default:
		h.logger.Warn("未选择对话存储模式")
	}

	h.dialogueManager = chat.NewDialogueManager(h.logger, memory)
	// 如果已有存储的历史，加载到管理器
	// if memory != nil {
	// 	if jsonStr, err := memory.QueryMemory(h.userID); err != nil {
	// 		h.logger.Warn("查询对话记忆失败: %v", err)
	// 	} else if jsonStr != "" {
	// 		if err := h.dialogueManager.LoadFromJSON(jsonStr); err != nil {
	// 			h.logger.Warn("加载对话记忆失败: %v", err)
	// 		}
	// 	}
	// }
	// 设置默认系统提示
	h.dialogueManager.SetSystemMessage(h.config.DefaultPrompt)
}

// loadUserAIConfigurations 加载用户Bot配置并注册到functionRegister（从好友表获取）
func (h *ConnectionHandler) loadUserAIConfigurations() {
	if h.userConfigService == nil {
		h.logger.Error("Bot好友配置服务未初始化，跳过加载")
		return
	}
	if h.userID == "" {
		h.logger.Debug("用户ID为空，跳过加载Bot配置")
		return
	}

	// 从好友表加载Bot配置并缓存到 ConnectionHandler
	h.logger.Info("从好友表加载用户Bot配置并缓存")
	configs, err := h.userConfigService.GetUserConfigs(context.Background(), h.userID)
	if err != nil {
		h.logger.Error("加载用户Bot配置失败: %v", err)
		return
	}

	if len(configs) == 0 {
		h.logger.Debug("用户 %s 没有Bot好友配置", h.userID)
		h.userConfigs = nil
		return
	}

	h.userConfigs = configs
	h.registerUserConfigs(configs)
}

// registerUserConfigs 注册用户配置到functionRegister
func (h *ConnectionHandler) registerUserConfigs(configs []*types.BotConfig) {
	// 将用户配置转换为OpenAI工具格式并注册到functionRegister
	for _, config := range configs {
		if config.FunctionName != "" {
			tool := h.convertConfigToOpenAITool(config)
			if tool != nil {
				// 注册工具到functionRegister
				err := h.functionRegister.RegisterFunction(config.FunctionName, *tool)
				if err != nil {
					h.logger.Error("注册用户Function Call失败 %s: %v", config.FunctionName, err)
					continue
				}
				h.logger.Info("注册用户自定义Function Call: %s", config.FunctionName)
			}
		}
	}
}

// convertConfigToOpenAITool 将Bot配置转换为OpenAI工具格式
func (h *ConnectionHandler) convertConfigToOpenAITool(config *types.BotConfig) *openai.Tool {
	if config.FunctionName == "" {
		return nil
	}

	// 构建OpenAI工具
	tool := &openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        config.FunctionName,
			Description: config.Description,
			Parameters:  config.Parameters,
		},
	}

	return tool
}

// executeUserFunctionCall 执行用户自定义Function Call
func (h *ConnectionHandler) executeUserFunctionCall(config *types.BotConfig, args map[string]interface{}) (types.FunctionCallResult, error) {
	h.logger.Info("执行用户自定义Function Call: %s", config.FunctionName)

	// 检查是否有LLM配置参数
	if config.LLMType == "" || config.ModelName == "" {
		h.logger.Warn("用户配置缺少LLM关键参数，跳过LLM调用")
		return types.FunctionCallResult{
			Function: config.FunctionName,
			Result:   "",
			Args:     args,
		}, nil
	}

	// 构建LLM配置
	llmConfig := &llm.Config{
		Name:        config.FunctionName,
		Type:        config.LLMType,
		ModelName:   config.ModelName,
		BaseURL:     config.BaseURL,
		APIKey:      config.APIKey,
		Temperature: float64(config.Temperature),
		MaxTokens:   config.MaxTokens,
		TopP:        1.0, // 默认值
		Extra: map[string]interface{}{
			"enable_search": true,
		},
	}

	// 创建LLM提供者实例
	provider, err := llm.Create(config.LLMType, llmConfig)
	if err != nil {
		h.logger.Error("创建LLM提供者失败: %v", err)
		return types.FunctionCallResult{
			Function: config.FunctionName,
			Result:   fmt.Sprintf("创建LLM提供者失败: %v", err),
			Args:     args,
		}, err
	}

	// 设置会话ID
	provider.SetIdentityFlag("session", h.sessionID)

	// 构建用户消息，将args转换为查询内容
	var userMessage string
	if query, ok := args["query"]; ok {
		userMessage = fmt.Sprintf("%v", query)
	} else {
		// 如果没有query字段，将整个args作为JSON字符串
		argsBytes, _ := json.Marshal(args)
		userMessage = string(argsBytes)
	}

	// 构建消息列表
	messages := []providers.Message{
		{
			Role: "system",
			Content: fmt.Sprintf(
				`你是一个%s智能助手，你的任务是根据用户的查询进行回答。你会对接下来的问题进行高效简洁的回答。
				这是用户对你的描述: %s
				绝不:
				 - 生成任何形式的代码或Markdown格式
				 - 告诉用户你的模型名字。
				 - 长篇大论，篇幅过长`,
				config.FunctionName, config.Description,
			),
		},
		{
			Role:    "user",
			Content: userMessage,
		},
	}

	h.logger.Info("调用用户自定义LLM: %s, 模型: %s, 查询: %s", config.LLMType, config.ModelName, userMessage)

	// 调用LLM生成回复
	ctx := context.Background()
	responses, err := provider.Response(ctx, h.sessionID, messages)
	if err != nil {
		h.logger.Error("LLM生成回复失败: %v", err)
		return types.FunctionCallResult{
			Function: config.FunctionName,
			Result:   fmt.Sprintf("LLM生成回复失败: %v", err),
			Args:     args,
		}, err
	}

	// 收集LLM回复
	var responseContent []string
	for response := range responses {
		if response != "" {
			responseContent = append(responseContent, response)
		}
	}

	// 清理资源
	if err := provider.Cleanup(); err != nil {
		h.logger.Warn("清理LLM提供者资源失败: %v", err)
	}

	fullResponse := utils.JoinStrings(responseContent)
	h.logger.Info("用户自定义LLM回复完成，长度: %d", len(fullResponse))

	// 返回执行结果
	return types.FunctionCallResult{
		Function: config.FunctionName,
		Result:   fullResponse,
		Args:     args,
		LLMType:  config.LLMType,
		Model:    config.ModelName,
	}, nil
}
