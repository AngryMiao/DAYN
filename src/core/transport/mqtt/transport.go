package mqtt

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"angrymiao-ai-server/src/configs"
	"angrymiao-ai-server/src/core/auth"
	"angrymiao-ai-server/src/core/transport"
	"angrymiao-ai-server/src/core/utils"
	"angrymiao-ai-server/src/httpsvr/device"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MQTTTransport MQTT传输层实现
type MQTTTransport struct {
	cfg         *configs.Config
	logger      *utils.Logger
	factory     transport.ConnectionHandlerFactory
	client      mqtt.Client
	udpServer   *UDPServer      // UDP服务器（可选）
	connections sync.Map        // key=deviceID:sessionID -> *MQTTConnection
	handlers    sync.Map        // key=deviceID:sessionID -> transport.ConnectionHandler
	authToken   *auth.AuthToken // JWT认证工具
}

func NewMQTTTransport(cfg *configs.Config, logger *utils.Logger) *MQTTTransport {
	t := &MQTTTransport{cfg: cfg, logger: logger}
	// 使用配置中的 topic_root
	topicRoot := cfg.Transport.Mqtt.TopicRoot
	if topicRoot == "" {
		topicRoot = "am_topic" // 默认值
	}
	t.authToken = auth.NewAuthTokenWithConfig(cfg.Server.Token, topicRoot)
	return t
}

func (t *MQTTTransport) GetType() string { return "mqtt" }

func (t *MQTTTransport) SetConnectionHandler(f transport.ConnectionHandlerFactory) { t.factory = f }

func (t *MQTTTransport) GetActiveConnectionCount() int {
	count := 0
	t.connections.Range(func(_, _ any) bool { count++; return true })
	return count
}

// Start 启动MQTT传输层：连接Broker并订阅入站主题
func (t *MQTTTransport) Start(ctx context.Context) error {
	if t.factory == nil {
		return fmt.Errorf("连接处理器工厂未设置")
	}
	if !t.cfg.Transport.Mqtt.Enabled {
		return fmt.Errorf("MQTT未启用")
	}

	// 如果启用UDP，先启动UDP服务器
	if t.cfg.Transport.Mqtt.UDP.Enabled {
		t.udpServer = NewUDPServer(t.cfg, t.logger)
		if err := t.udpServer.Start(); err != nil {
			return fmt.Errorf("启动UDP服务器失败: %v", err)
		}
		t.logger.Info("UDP服务器已启动（用于音频数据传输）")
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(t.cfg.Transport.Mqtt.Broker)
	clientID := fmt.Sprintf("%s-%d", t.cfg.Transport.Mqtt.ClientIDPrefix, time.Now().UnixNano())
	opts.SetClientID(clientID)

	// 设置认证信息
	if u := t.cfg.Transport.Mqtt.Username; u != "" {
		opts.SetUsername(u)
		t.logger.Info("MQTT 使用认证连接: username=%s", u)
	} else {
		t.logger.Warn("MQTT 未配置用户名，使用匿名连接")
	}
	if p := t.cfg.Transport.Mqtt.Password; p != "" {
		opts.SetPassword(p)
	}
	opts.SetAutoReconnect(true)

	// TLS配置（可选）
	if t.cfg.Transport.Mqtt.TLS.Enabled {
		ls := &tls.Config{InsecureSkipVerify: t.cfg.Transport.Mqtt.TLS.SkipVerify}
		// CA 根证书
		if caf := t.cfg.Transport.Mqtt.TLS.CAFile; caf != "" {
			pem, err := os.ReadFile(caf)
			if err != nil {
				return fmt.Errorf("读取CA文件失败: %v", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return fmt.Errorf("加载CA证书失败")
			}
			ls.RootCAs = pool
		}
		// 客户端证书
		if certFile := t.cfg.Transport.Mqtt.TLS.CertFile; certFile != "" {
			keyFile := t.cfg.Transport.Mqtt.TLS.KeyFile
			cert, err := tls.LoadX509KeyPair(certFile, keyFile)
			if err != nil {
				return fmt.Errorf("加载客户端证书失败: %v", err)
			}
			ls.Certificates = []tls.Certificate{cert}
		}
		opts.SetTLSConfig(ls)
	}

	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		t.logger.Warn("MQTT连接丢失: %v", err)
	})
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		prefix := strings.TrimSuffix(t.cfg.Transport.Mqtt.TopicRoot, "/")
		inSuffix := strings.TrimPrefix(t.cfg.Transport.Mqtt.InSuffix, "/")
		// 示例：ws_asr/+/+/in
		inTopic := fmt.Sprintf("%s/+/+/%s", prefix, inSuffix)
		t.logger.Info("MQTT已连接，订阅主题: %s", inTopic)
		tk := c.Subscribe(inTopic, byte(t.cfg.Transport.Mqtt.Qos), t.onMessage)
		tk.Wait()
		if err := tk.Error(); err != nil {
			t.logger.Error("订阅失败: %v", err)
		}
		// 订阅心跳与连接状态（LWT）主题：ws_asr/+/status/heartbeat 与 ws_asr/+/status/connection
		hbTopic := fmt.Sprintf("%s/+/status/heartbeat", prefix)
		t.logger.Info("MQTT订阅心跳主题: %s", hbTopic)
		tk2 := c.Subscribe(hbTopic, byte(t.cfg.Transport.Mqtt.Qos), t.onHeartbeatMessage)
		tk2.Wait()
		if err := tk2.Error(); err != nil {
			t.logger.Error("订阅心跳失败: %v", err)
		}

		connTopic := fmt.Sprintf("%s/+/status/connection", prefix)
		t.logger.Info("MQTT订阅连接状态主题: %s", connTopic)
		tk3 := c.Subscribe(connTopic, byte(t.cfg.Transport.Mqtt.Qos), t.onConnectionMessage)
		tk3.Wait()
		if err := tk3.Error(); err != nil {
			t.logger.Error("订阅连接状态失败: %v", err)
		}
	})

	client := mqtt.NewClient(opts)
	con := client.Connect()
	con.Wait()
	if err := con.Error(); err != nil {
		return fmt.Errorf("MQTT连接失败: %v", err)
	}
	t.client = client
	// 监听关闭信号
	go func() {
		<-ctx.Done()
		_ = t.Stop()
	}()

	t.logger.Info("MQTT传输层已启动: %s", t.cfg.Transport.Mqtt.Broker)
	return nil
}

// Stop 停止MQTT传输层
func (t *MQTTTransport) Stop() error {
	if t.client != nil && t.client.IsConnected() {
		t.client.Disconnect(250)
	}

	// 停止UDP服务器
	if t.udpServer != nil {
		t.udpServer.Stop()
	}

	// 关闭所有会话
	t.handlers.Range(func(k, v any) bool {
		if h, ok := v.(transport.ConnectionHandler); ok {
			h.Close()
		}
		t.handlers.Delete(k)
		return true
	})
	t.connections.Range(func(k, v any) bool {
		if c, ok := v.(*MQTTConnection); ok {
			_ = c.Close()
		}
		t.connections.Delete(k)
		return true
	})
	t.logger.Info("MQTT传输层已停止")
	return nil
}

// onMessage 处理设备入站消息
func (t *MQTTTransport) onMessage(_ mqtt.Client, msg mqtt.Message) {
	deviceID, sessionID, ok := t.extractIDs(msg.Topic())
	if !ok {
		t.logger.Warn("MQTT主题不匹配，忽略: %s", msg.Topic())
		return
	}
	key := deviceID + ":" + sessionID

	// 检查连接是否已存在
	_, exists := t.connections.Load(key)

	// 如果连接不存在，则需要解析首条消息中的headers进行认证
	if !exists {
		// 解析首条消息中的头包装（header 模式）：{"headers": {...}, "payload": {...}}
		var wrapper struct {
			Headers map[string]string `json:"headers"`
			Payload interface{}       `json:"payload"`
		}

		// 1. 强制要求消息必须包含headers字段
		if err := json.Unmarshal(msg.Payload(), &wrapper); err != nil {
			t.logger.Warn("首条消息格式错误，无法解析JSON: deviceID=%s, error=%v", deviceID, err)
			t.sendErrorResponse(deviceID, sessionID, "首条消息格式错误，必须为有效的JSON格式")
			return
		}

		// 2. 强制要求headers字段必须存在且不为空
		if len(wrapper.Headers) == 0 {
			t.logger.Warn("首条消息缺少headers字段: deviceID=%s", deviceID)
			t.sendErrorResponse(deviceID, sessionID, "连接失败：首条消息必须包含headers字段")
			return
		}

		t.logger.Info("收到MQTT首条消息headers: deviceID=%s, headers=%v", deviceID, wrapper.Headers)
		// 3. 强制要求Token字段必须存在
		token, ok := wrapper.Headers["Token"]
		if !ok || token == "" {
			t.logger.Warn("连接失败：缺少Token字段: deviceID=%s, sessionID=%s", deviceID, sessionID)
			t.sendErrorResponse(deviceID, sessionID, "连接失败：headers中必须包含Token字段")
			return
		}

		// 4. 验证Token
		if t.authToken == nil {
			t.logger.Error("认证管理器未初始化")
			t.sendErrorResponse(deviceID, sessionID, "服务器配置错误")
			return
		}

		valid, tokenDevID, userID, err := t.authToken.VerifyToken(token)
		if err != nil || !valid {
			t.logger.Warn("Token验证失败: deviceID=%s, sessionID=%s, error=%v", deviceID, sessionID, err)
			t.sendErrorResponse(deviceID, sessionID, "连接失败：Token验证失败，请检查Token是否有效")
			return
		}

		if tokenDevID != deviceID {
			t.logger.Warn("设备ID与Token不匹配: 请求deviceID=%s, token中deviceID=%s", deviceID, tokenDevID)
			t.sendErrorResponse(deviceID, sessionID, "连接失败：设备ID与Token不匹配")
			return
		}

		t.logger.Info("MQTT连接验证成功: deviceID=%s, sessionID=%s, userID=%d", deviceID, sessionID, userID)
		conn := t.newConnection(deviceID, sessionID)
		if conn == nil {
			return
		}
		req := &http.Request{Header: http.Header{}}

		// 设置基础headers（使用标准命名）
		req.Header.Set("Device-Id", deviceID)
		req.Header.Set("Session-Id", sessionID)
		req.Header.Set("Transport-Type", "mqtt")

		// 从wrapper.Headers中提取Client-Id（使用标准命名）
		clientID := key // 默认使用 deviceID:sessionID
		if cid, ok := wrapper.Headers["Client-Id"]; ok && cid != "" {
			clientID = cid
		}
		req.Header.Set("Client-Id", clientID)

		// 直接填充所有来自客户端的headers（不做命名转换）
		for k, v := range wrapper.Headers {
			req.Header.Set(k, v)
		}

		// 将内部 payload 作为首条实际消息
		var payloadToPush []byte
		if wrapper.Payload != nil {
			if b, err := json.Marshal(wrapper.Payload); err == nil {
				payloadToPush = b
			}
		}

		if t.factory == nil {
			t.logger.Error("连接处理器工厂未设置")
			_ = conn.Close()
			return
		}
		handler := t.factory.CreateHandler(conn, req)
		if handler == nil {
			t.logger.Error("创建连接处理器失败")
			_ = conn.Close()
			return
		}

		// 检查header中是否请求UDP传输（Udp-Enabled: true）
		fmt.Printf("onMessage: transport=%p, t.udpServer=%p\n", t, t.udpServer)
		if req.Header.Get("Udp-Enabled") == "true" && t.udpServer != nil {
			// 创建UDP会话
			udpSession, err := t.udpServer.CreateSession(deviceID, sessionID)
			if err != nil {
				t.logger.Error("创建UDP会话失败: %v", err)
				t.sendErrorResponse(deviceID, sessionID, fmt.Sprintf("服务器配置错误:%v", err))
			} else {
				// 将UDP会话和服务器信息设置到连接，并标记启用UDP
				conn.SetUDPSession(
					udpSession,
					t.udpServer.externalHost,
					fmt.Sprintf("%d", t.udpServer.externalPort),
				)
				t.logger.Info("创建UDP会话成功: deviceID=%s, sessionID=%s", deviceID, sessionID)

				// 如果客户端在headers中提供了UDP地址信息，立即发起探测
				if clientIP := req.Header.Get("Udp-Client-Ip"); clientIP != "" {
					if clientPortStr := req.Header.Get("Udp-Client-Port"); clientPortStr != "" {
						// 解析端口
						var clientPort int
						if _, err := fmt.Sscanf(clientPortStr, "%d", &clientPort); err == nil && clientPort > 0 {
							t.logger.Info("客户端提供UDP地址，发起探测: deviceID=%s, addr=%s:%d",
								deviceID, clientIP, clientPort)
							// 异步发起探测，不阻塞连接建立
							go func() {
								if err := t.udpServer.ProbeClientAddress(udpSession, clientIP, clientPort); err != nil {
									t.logger.Warn("UDP地址探测失败: %v", err)
								}
							}()
						}
					}
				}
			}
		}

		// 设置userID到ConnectionHandler（Token已在前面验证过）
		if userID != 0 {
			if adapter, ok := handler.(*transport.ConnectionContextAdapter); ok {
				ch := adapter.GetConnectionHandler()
				ch.SetUserID(fmt.Sprintf("%d", userID))
			}
		}

		t.connections.Store(key, conn)
		t.handlers.Store(key, handler)
		// 标记会话在线
		device.GetPresenceManager().SetSessionOnline(deviceID, sessionID)
		go func() {
			defer func() {
				t.handlers.Delete(key)
				t.connections.Delete(key)
				handler.Close()
				// 标记会话离线
				device.GetPresenceManager().SetSessionOffline(deviceID, sessionID)
			}()
			handler.Handle()
		}()

		// 投递首条消息的payload到连接
		if len(payloadToPush) > 0 {
			mt := inferMessageType(payloadToPush)
			conn.PushIncoming(mt, payloadToPush)
		}
	} else {
		// 连接已存在，直接投递消息（后续消息不需要headers包装）
		if v, ok := t.connections.Load(key); ok {
			if conn, ok := v.(*MQTTConnection); ok {
				mt := inferMessageType(msg.Payload())
				conn.PushIncoming(mt, msg.Payload())
				// 更新会话活跃时间
				device.GetPresenceManager().TouchSession(deviceID, sessionID)
			}
		}
	}
}

// onHeartbeatMessage 处理心跳消息：主题形如 prefix/{deviceID}/status/heartbeat
func (t *MQTTTransport) onHeartbeatMessage(_ mqtt.Client, msg mqtt.Message) {
	deviceID := t.extractDeviceIDFromStatusTopic(msg.Topic())
	if deviceID == "" {
		t.logger.Warn("心跳主题不匹配，忽略: %s", msg.Topic())
		return
	}
	// 支持 JSON 或简易文本
	var m map[string]interface{}
	hb := device.HeartbeatMetrics{}
	if json.Unmarshal(msg.Payload(), &m) == nil {
		if ts, ok := m["ts"].(float64); ok {
			hb.Timestamp = int64(ts)
		}
		if bat, ok := m["battery"].(float64); ok {
			hb.Battery = bat
		}
		if tmp, ok := m["temp"].(float64); ok {
			hb.Temp = tmp
		}
		if net, ok := m["net"].(string); ok {
			hb.Net = net
		}
		if rssi, ok := m["rssi"].(float64); ok {
			hb.RSSI = int(rssi)
		}
	} else {
		// 非JSON载荷，至少标记时间
		hb.Timestamp = time.Now().Unix()
	}
	device.GetPresenceManager().UpdateHeartbeat(deviceID, hb)
}

// onConnectionMessage 处理连接状态（LWT）：主题形如 prefix/{deviceID}/status/connection
// 载荷支持字符串 "online"/"offline" 或 JSON {"status":"online|offline", "ts":...}
func (t *MQTTTransport) onConnectionMessage(_ mqtt.Client, msg mqtt.Message) {
	deviceID := t.extractDeviceIDFromStatusTopic(msg.Topic())
	if deviceID == "" {
		t.logger.Warn("连接状态主题不匹配，忽略: %s", msg.Topic())
		return
	}
	status := ""
	var m map[string]interface{}
	if json.Unmarshal(msg.Payload(), &m) == nil {
		if s, ok := m["status"].(string); ok {
			status = s
		}
	} else {
		// 尝试按纯文本解析
		s := strings.TrimSpace(string(msg.Payload()))
		status = strings.ToLower(s)
	}
	switch status {
	case "online":
		device.GetPresenceManager().SetDeviceConnectionState(deviceID, true)
	case "offline":
		device.GetPresenceManager().SetDeviceConnectionState(deviceID, false)
	default:
		// 未知状态，忽略
	}
}

// extractDeviceIDFromStatusTopic 从 status/* 主题中提取设备ID
func (t *MQTTTransport) extractDeviceIDFromStatusTopic(topic string) string {
	parts := strings.Split(topic, "/")
	if len(parts) < 4 { // prefix/deviceID/status/{heartbeat|connection}
		return ""
	}
	// 倒数第3个为 deviceID
	return parts[len(parts)-3]
}

// sendErrorResponse 发送错误响应到设备
func (t *MQTTTransport) sendErrorResponse(deviceID, sessionID, errorMsg string) {
	prefix := strings.TrimSuffix(t.cfg.Transport.Mqtt.TopicRoot, "/")
	outSuffix := strings.TrimPrefix(t.cfg.Transport.Mqtt.OutSuffix, "/")
	outTopic := fmt.Sprintf("%s/%s/%s/%s", prefix, deviceID, sessionID, outSuffix)

	errorResponse := map[string]interface{}{
		"type":    "error",
		"message": errorMsg,
		"code":    "AUTH_FAILED",
	}

	data, err := json.Marshal(errorResponse)
	if err != nil {
		t.logger.Error("序列化错误响应失败: %v", err)
		return
	}

	token := t.client.Publish(outTopic, byte(t.cfg.Transport.Mqtt.Qos), false, data)
	if token != nil {
		token.Wait()
		if err := token.Error(); err != nil {
			t.logger.Error("发送错误响应失败: %v", err)
		}
	}
}

// newConnection 创建新的会话连接
func (t *MQTTTransport) newConnection(deviceID, sessionID string) *MQTTConnection {
	if t.client == nil || !t.client.IsConnected() {
		t.logger.Error("MQTT客户端未连接，无法创建连接")
		return nil
	}
	prefix := strings.TrimSuffix(t.cfg.Transport.Mqtt.TopicRoot, "/")
	outSuffix := strings.TrimPrefix(t.cfg.Transport.Mqtt.OutSuffix, "/")
	outTopic := fmt.Sprintf("%s/%s/%s/%s", prefix, deviceID, sessionID, outSuffix)
	connID := fmt.Sprintf("%s/%s", deviceID, sessionID)
	return NewMQTTConnection(t.client, connID, outTopic, t.cfg.Transport.Mqtt.Qos)
}

// extractIDs 从主题中解析 deviceID 与 sessionID
func (t *MQTTTransport) extractIDs(topic string) (string, string, bool) {
	parts := strings.Split(topic, "/")
	if len(parts) < 3 {
		return "", "", false
	}
	// 末尾应为 InSuffix
	inSuffix := strings.TrimPrefix(t.cfg.Transport.Mqtt.InSuffix, "/")
	if parts[len(parts)-1] != inSuffix {
		return "", "", false
	}
	sessionID := parts[len(parts)-2]
	deviceID := parts[len(parts)-3]
	return deviceID, sessionID, true
}

// inferMessageType 基于payload内容推断消息类型：文本=1，二进制=2
func inferMessageType(payload []byte) int {
	if len(payload) == 0 {
		return 1
	}
	if utf8.Valid(payload) {
		// 尝试作为JSON解析，若成功按文本处理
		var m interface{}
		if json.Unmarshal(payload, &m) == nil {
			return 1
		}
		// 有效UTF-8但非JSON，当作纯文本
		return 1
	}
	return 2
}
