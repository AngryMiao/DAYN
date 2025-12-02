package core

import (
	"angrymiao-ai-server/src/configs/database"
	"angrymiao-ai-server/src/core/chat"
	"angrymiao-ai-server/src/core/image"
	"angrymiao-ai-server/src/core/media"
	"angrymiao-ai-server/src/core/providers"
	"angrymiao-ai-server/src/core/utils"
	"angrymiao-ai-server/src/httpsvr/device"
	"angrymiao-ai-server/src/models"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// handleMessage 处理接收到的消息
func (h *ConnectionHandler) handleMessage(messageType int, message []byte) error {
	switch messageType {
	case 1: // 文本消息
		// 优先尝试解析为 JSON，若为 MCP 消息则投递到独立队列，避免文本处理协程阻塞
		var msgJSON interface{}
		if err := json.Unmarshal(message, &msgJSON); err == nil {
			if msgMap, ok := msgJSON.(map[string]interface{}); ok {
				if msgType, ok := msgMap["type"].(string); ok && msgType == "mcp" {
					h.mcpMessageQueue <- msgMap
					return nil
				}
			}
		}
		h.clientTextQueue <- string(message)
		return nil
	case 2: // 二进制消息（音频数据）
		actualAudioData := message
		if h.clientAudioFormat == "pcm" {
			// 直接将PCM数据放入队列
			h.clientAudioQueue <- actualAudioData
		} else if h.clientAudioFormat == "opus" {
			// 检查是否初始化了opus解码器
			if h.opusDecoder != nil {
				// 解码opus数据为PCM
				decodedData, err := h.opusDecoder.Decode(actualAudioData)
				if err != nil {
					h.logger.Error(fmt.Sprintf("解码Opus音频失败: %v", err))
					// 即使解码失败，也尝试将原始数据传递给ASR处理
					h.clientAudioQueue <- actualAudioData
				} else {
					// 解码成功，将PCM数据放入队列
					h.logger.Debug(fmt.Sprintf("Opus解码成功: %d bytes -> %d bytes", len(actualAudioData), len(decodedData)))
					if len(decodedData) > 0 {
						h.clientAudioQueue <- decodedData
						h.LogInfo(fmt.Sprintf("✓ Opus解码后的PCM数据已放入队列: size=%d", len(decodedData)))
					}
				}
			} else {
				// 没有解码器，直接传递原始数据
				h.clientAudioQueue <- actualAudioData
				h.LogInfo(fmt.Sprintf("✓ 原始音频数据已放入队列（无解码器）: size=%d", len(actualAudioData)))
			}
		}
		return nil
	default:
		h.logger.Error(fmt.Sprintf("未知的消息类型: %d", messageType))
		return fmt.Errorf("未知的消息类型: %d", messageType)
	}
}

// processClientTextMessage 处理文本数据
func (h *ConnectionHandler) processClientTextMessage(ctx context.Context, text string) error {
	// 解析JSON消息
	var msgJSON interface{}
	if err := json.Unmarshal([]byte(text), &msgJSON); err != nil {
		return h.conn.WriteMessage(1, []byte(text))
	}

	// 检查是否为整数类型
	if _, ok := msgJSON.(float64); ok {
		return h.conn.WriteMessage(1, []byte(text))
	}

	// 解析为map类型处理具体消息
	msgMap, ok := msgJSON.(map[string]interface{})
	if !ok {
		return fmt.Errorf("消息格式错误")
	}

	// 根据消息类型分发处理
	msgType, ok := msgMap["type"].(string)
	if !ok {
		return fmt.Errorf("消息类型错误")
	}

	switch msgType {
	case "hello":
		return h.handleHelloMessage(msgMap)
	case "abort":
		return h.clientAbortChat()
	case "listen":
		return h.handleListenMessage(msgMap)
	case "chat":
		msgText, ok := msgMap["text"].(string)
		if !ok {
			return fmt.Errorf("消息格式错误")
		}
		return h.handleChatMessage(ctx, msgText)
	case "heartbeat":
		return h.handleHeartbeatMessage(msgMap)
	case "device_status":
		return h.handleDeviceStatusMessage(msgMap)
	case "vision":
		return h.handleVisionMessage(msgMap)
	case "media_upload":
		return h.handleMediaUpload(msgMap)
	case "image":
		return h.handleImageMessage(ctx, msgMap)
	case "mcp":
		return h.mcpManager.HandleAMMCPMessage(msgMap)
	default:
		h.logger.Warn("=== 未知消息类型 ===", map[string]interface{}{
			"unknown_type": msgType,
			"full_message": msgMap,
		})
		return fmt.Errorf("未知的消息类型: %s", msgType)
	}
}

func (h *ConnectionHandler) handleMediaUpload(msgMap map[string]interface{}) error {
	// 解析base64数据
	base64Data, ok := msgMap["media_base64"].(string)
	if !ok || base64Data == "" {
		return fmt.Errorf("缺少base64_data字段")
	}

	// 解析文件类型（image/video/audio）
	fileType, ok := msgMap["media_type"].(string)
	if !ok || fileType == "" {
		return fmt.Errorf("缺少file_type字段")
	}

	// 验证文件类型
	fileType = strings.ToLower(fileType)
	if fileType != "image" && fileType != "video" && fileType != "audio" {
		return fmt.Errorf("不支持的文件类型: %s，仅支持 image、video、audio", fileType)
	}

	h.LogInfo(fmt.Sprintf("收到媒体上传请求: type=%s, device=%s, size=%d bytes",
		fileType, h.deviceID, len(base64Data)))

	// 使用媒体上传器处理上传
	result, err := h.uploadMedia(base64Data, fileType)
	if err != nil {
		h.LogError(fmt.Sprintf("媒体上传失败: %v", err))
		return h.sendMediaUploadResponse(false, "", "", fileType, "", err.Error())
	}

	h.LogInfo(fmt.Sprintf("媒体文件上传成功: url=%s, suffix=%s", result.URL, result.Suffix))

	// 解码base64数据用于提取元数据
	fileData, err := utils.DecodeBase64(base64Data)
	if err != nil {
		h.LogError(fmt.Sprintf("解码base64数据失败: %v", err))
		fileData = nil
	}

	// 保存上传记录到数据库
	if err := h.saveMediaUploadRecord(result, fileData); err != nil {
		h.LogError(fmt.Sprintf("保存媒体上传记录失败: %v", err))
		// 即使保存失败，也返回上传成功的响应（因为文件已经上传成功）
	}

	// 发送上传成功响应
	return h.sendMediaUploadResponse(true, result.URL, result.Path, fileType, result.Suffix, "")
}

// uploadMedia 上传媒体文件（内部方法）
func (h *ConnectionHandler) uploadMedia(base64Data, fileType string) (*media.UploadResult, error) {
	uploader := media.NewUploader(h.config, h.logger)

	return uploader.Upload(&media.UploadRequest{
		Base64Data: base64Data,
		FileType:   fileType,
		UserID:     h.userID,
		DeviceID:   h.deviceID,
	})
}

// sendMediaUploadResponse 发送媒体上传响应
func (h *ConnectionHandler) sendMediaUploadResponse(success bool, url, path, fileType, suffix, errMsg string) error {
	response := map[string]interface{}{
		"type":      "media_upload_result",
		"success":   success,
		"file_type": fileType,
		"timestamp": time.Now().Unix(),
	}

	if success {
		response["url"] = url
		response["path"] = path
		response["suffix"] = suffix
	} else {
		response["error"] = errMsg
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("序列化响应失败: %v", err)
	}

	return h.conn.WriteMessage(1, responseJSON)
}

func (h *ConnectionHandler) handleVisionMessage(msgMap map[string]interface{}) error {
	// 处理视觉消息
	cmd := msgMap["cmd"].(string)
	if cmd == "gen_pic" {
	} else if cmd == "gen_video" {
	} else if cmd == "read_img" {
	}
	return nil
}

// handleHelloMessage 处理欢迎消息
// 客户端会上传语音格式和采样率等信息
func (h *ConnectionHandler) handleHelloMessage(msgMap map[string]interface{}) error {
	h.LogInfo("收到客户端欢迎消息: " + fmt.Sprintf("%v", msgMap))

	// 获取客户端编码格式
	if audioParams, ok := msgMap["audio_params"].(map[string]interface{}); ok {
		if format, ok := audioParams["format"].(string); ok {
			h.clientAudioFormat = format
			if format == "pcm" {
				// 客户端使用PCM格式，服务端也使用PCM格式
				h.serverAudioFormat = "pcm"
			}
		}
		if sampleRate, ok := audioParams["sample_rate"].(float64); ok {
			h.clientAudioSampleRate = int(sampleRate)
		}
		if channels, ok := audioParams["channels"].(float64); ok {
			h.clientAudioChannels = int(channels)
		}
		if frameDuration, ok := audioParams["frame_duration"].(float64); ok {
			h.clientAudioFrameDuration = int(frameDuration)
		}
		h.LogInfo(fmt.Sprintf("客户端音频参数: format=%s, sample_rate=%d, channels=%d, frame_duration=%d",
			h.clientAudioFormat, h.clientAudioSampleRate, h.clientAudioChannels, h.clientAudioFrameDuration))
	}

	// 处理客户端提供的UDP地址信息（用于NAT穿透）
	if udpInfo, ok := msgMap["udp_client_info"].(map[string]interface{}); ok {
		if publicIP, ok := udpInfo["public_ip"].(string); ok {
			h.LogInfo(fmt.Sprintf("客户端提供的公网IP: %s", publicIP))
			// 存储客户端提供的公网IP，用于UDP探测
			h.clientPublicIP = publicIP
		}
		if udpPort, ok := udpInfo["udp_port"].(float64); ok {
			h.LogInfo(fmt.Sprintf("客户端UDP监听端口: %d", int(udpPort)))
			h.clientUDPPort = int(udpPort)
		}
	}

	h.sendHelloMessage()
	h.closeOpusDecoder()
	// 初始化opus解码器
	opusDecoder, err := utils.NewOpusDecoder(&utils.OpusDecoderConfig{
		SampleRate:  h.clientAudioSampleRate, // 客户端使用24kHz采样率
		MaxChannels: h.clientAudioChannels,   // 单声道音频
	})
	if err != nil {
		h.logger.Error(fmt.Sprintf("初始化Opus解码器失败: %v", err))
	} else {
		h.opusDecoder = opusDecoder
		h.LogInfo("Opus解码器初始化成功")
	}

	// 在 hello 消息处理时就设置 ASR listener，避免依赖 listen 消息
	// 这样即使客户端不发送 listen 消息，ASR 也能正常工作
	if h.providers.asr != nil {
		h.providers.asr.SetListener(h)
		h.LogInfo("ASR listener 已设置（在 hello 消息中）")
	} else {
		h.LogError("providers.asr 为 nil，无法设置 listener")
	}

	return nil
}

// handleHeartbeatMessage 处理心跳消息并更新在线状态
func (h *ConnectionHandler) handleHeartbeatMessage(msgMap map[string]interface{}) error {
	hb := device.HeartbeatMetrics{}
	if ts, ok := msgMap["ts"].(float64); ok {
		hb.Timestamp = int64(ts)
	}
	if bat, ok := msgMap["battery"].(float64); ok {
		hb.Battery = bat
	}
	if tmp, ok := msgMap["temp"].(float64); ok {
		hb.Temp = tmp
	}
	if net, ok := msgMap["net"].(string); ok {
		hb.Net = net
	}
	if rssi, ok := msgMap["rssi"].(float64); ok {
		hb.RSSI = int(rssi)
	}

	device.GetPresenceManager().UpdateHeartbeat(h.deviceID, hb)
	device.GetPresenceManager().TouchSession(h.deviceID, h.sessionID)
	h.LogInfo(fmt.Sprintf("收到客户端心跳: device=%s, session=%s", h.deviceID, h.sessionID))
	return nil
}

// handleDeviceStatusMessage 处理设备状态上报（仅运行态与委托持久化）
func (h *ConnectionHandler) handleDeviceStatusMessage(msgMap map[string]interface{}) error {
	if h.deviceID == "" {
		return fmt.Errorf("设备ID缺失，无法更新设备状态")
	}

	// 运行态：设置设备连接状态（默认 true，若消息携带online则按消息值）
	online := true
	if on, ok := msgMap["online"].(bool); ok {
		online = on
	}
	device.GetPresenceManager().SetDeviceConnectionState(h.deviceID, online)

	// 委托设备服务层持久化设备状态
	if err := device.NewDeviceDB().UpdateDeviceStatus(h.deviceID, msgMap, h.userID); err != nil {
		h.LogError(fmt.Sprintf("设备状态持久化失败: %v", err))
		return err
	}
	h.LogInfo(fmt.Sprintf("设备状态已更新: device=%s, online=%v", h.deviceID, online))
	return nil
}

// handleListenMessage 处理语音相关消息
func (h *ConnectionHandler) handleListenMessage(msgMap map[string]interface{}) error {

	// 处理state参数
	state, ok := msgMap["state"].(string)
	if !ok {
		return fmt.Errorf("listen消息缺少state参数")
	}

	// 处理mode参数
	if mode, ok := msgMap["mode"].(string); ok {
		h.clientListenMode = mode
		h.LogInfo(fmt.Sprintf("客户端拾音模式：%s， %s", h.clientListenMode, state))
		if h.providers.asr != nil {
			h.providers.asr.SetListener(h)
		}
	}

	switch state {
	case "start":
		if h.client_asr_text != "" && h.clientListenMode == "manual" {
			h.clientAbortChat()
		}
		h.client_asr_text = ""
	case "stop":
		// 重置ASR状态，停止语音识别
		h.providers.asr.SendLastAudio([]byte{}) // 发送空数据标记结束
		h.LogInfo("客户端停止语音识别")
		// if h.providers.asr != nil {
		// 	if err := h.providers.asr.Reset(); err != nil {
		// 		h.LogError(fmt.Sprintf("重置ASR状态失败: %v", err))
		// 	}
		// }
		// h.LogInfo("客户端停止语音识别")
	case "detect":
		text, hasText := msgMap["text"].(string)

		if hasText && text != "" {
			// 只有文本，使用普通LLM处理
			h.LogInfo(fmt.Sprintf("检测到纯文本消息，使用LLM处理 %v", map[string]interface{}{
				"text": text,
			}))
			return h.handleChatMessage(context.Background(), text)
		} else {
			// 既没有图片也没有文本
			h.logger.Warn("detect消息既没有text也没有image参数")
			return fmt.Errorf("detect消息缺少text或image参数")
		}
	}
	return nil
}

// handleImageMessage 处理图片消息
func (h *ConnectionHandler) handleImageMessage(ctx context.Context, msgMap map[string]interface{}) error {
	// 增加对话轮次
	h.talkRound++
	currentRound := h.talkRound
	h.LogInfo(fmt.Sprintf("开始新的图片对话轮次: %d", currentRound))

	// 检查是否有VLLLM Provider
	if h.providers.vlllm == nil {
		h.logger.Warn("未配置VLLLM服务，图片消息将被忽略")
		return h.conn.WriteMessage(1, []byte("系统暂不支持图片处理功能"))
	}

	// 解析文本内容
	text, ok := msgMap["text"].(string)
	if !ok {
		text = "请描述这张图片" // 默认提示
	}

	// 解析图片数据
	imageDataMap, ok := msgMap["image_data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("缺少图片数据")
	}

	imageData := image.ImageData{}
	if url, ok := imageDataMap["url"].(string); ok {
		imageData.URL = url
	}
	if data, ok := imageDataMap["data"].(string); ok {
		imageData.Data = data
	}
	if format, ok := imageDataMap["format"].(string); ok {
		imageData.Format = format
	}

	// 验证图片数据
	if imageData.URL == "" && imageData.Data == "" {
		return fmt.Errorf("图片数据为空")
	}

	h.LogInfo(fmt.Sprintf("收到图片消息 %v", map[string]interface{}{
		"text":        text,
		"has_url":     imageData.URL != "",
		"has_data":    imageData.Data != "",
		"format":      imageData.Format,
		"data_length": len(imageData.Data),
	}))

	// 立即发送STT消息
	err := h.sendSTTMessage(text)
	if err != nil {
		h.logger.Error(fmt.Sprintf("发送STT消息失败: %v", err))
		return fmt.Errorf("发送STT消息失败: %v", err)
	}

	// 发送TTS开始状态
	if err := h.sendTTSMessage("start", "", 0); err != nil {
		h.logger.Error(fmt.Sprintf("发送TTS开始状态失败: %v", err))
		return fmt.Errorf("发送TTS开始状态失败: %v", err)
	}

	// 发送思考状态的情绪
	// if err := h.sendEmotionMessage("thinking"); err != nil {
	// 	h.logger.Error(fmt.Sprintf("发送思考状态情绪消息失败: %v", err))
	// 	return fmt.Errorf("发送情绪消息失败: %v", err)
	// }

	// 添加用户消息到对话历史（包含图片信息的描述）
	userMessage := fmt.Sprintf("%s [用户发送了一张%s格式的图片]", text, imageData.Format)
	h.dialogueManager.Put(chat.Message{
		Role:    "user",
		Content: userMessage,
	})

	// 获取对话历史
	messages := make([]providers.Message, 0)
	for _, msg := range h.dialogueManager.GetLLMDialogue() {
		// 排除包含图片信息的最后一条消息，因为我们要用VLLLM处理
		if msg.Role == "user" && strings.Contains(msg.Content, "[用户发送了一张") {
			continue
		}
		messages = append(messages, providers.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	return h.genResponseByVLLM(ctx, messages, imageData, text, currentRound)
}

// saveMediaUploadRecord 保存媒体上传记录到数据库
func (h *ConnectionHandler) saveMediaUploadRecord(result *media.UploadResult, fileData []byte) error {
	// 将 userID 从 string 转换为 uint
	userIDUint, err := utils.StringToUint(h.userID)
	if err != nil {
		return fmt.Errorf("用户ID转换失败: %v", err)
	}

	// 根据文件后缀生成 MIME 类型
	mimeType := utils.GetMimeType(result.FileType, result.Suffix)

	// 生成默认标题（格式：2025-11-21-17-55）
	title := time.Now().Format("2006-01-02-15-04")

	// 创建媒体上传记录
	record := models.MediaUpload{
		UserID:   userIDUint,
		DeviceID: h.deviceID,
		FileType: result.FileType,
		Title:    title,
		Path:     result.Path,
		URL:      result.URL,
		Size:     result.Size,
		MimeType: mimeType,
	}

	// 提取元数据
	if len(fileData) > 0 {
		switch result.FileType {
		case "image":
			// 获取图片尺寸
			if width, height, err := utils.GetImageDimensions(fileData); err == nil {
				record.Width = &width
				record.Height = &height
				h.LogInfo(fmt.Sprintf("图片尺寸: %dx%d", width, height))
			} else {
				h.LogInfo(fmt.Sprintf("无法获取图片尺寸: %v", err))
			}

		case "audio":
			// 获取音频时长
			if duration, err := utils.GetAudioDuration(fileData, result.Suffix); err == nil {
				record.DurationSeconds = &duration
				h.LogInfo(fmt.Sprintf("音频时长: %.2f秒", duration))
			} else {
				h.LogInfo(fmt.Sprintf("无法获取音频时长: %v", err))
			}

		case "video":
			// 获取视频时长
			if duration, err := utils.GetVideoDuration(fileData, result.Suffix); err == nil {
				record.DurationSeconds = &duration
				h.LogInfo(fmt.Sprintf("视频时长: %.2f秒", duration))
			} else {
				h.LogInfo(fmt.Sprintf("无法获取视频时长: %v", err))
			}
		}
	}

	// 保存到数据库
	if err := database.GetDB().Create(&record).Error; err != nil {
		return fmt.Errorf("数据库保存失败: %v", err)
	}

	h.LogInfo(fmt.Sprintf("媒体上传记录已保存: id=%d, type=%s, size=%d", record.ID, record.FileType, record.Size))
	return nil
}
