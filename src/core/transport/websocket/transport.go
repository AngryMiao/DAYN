package websocket

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"angrymiao-ai-server/src/configs"
	"angrymiao-ai-server/src/core/auth"
	"angrymiao-ai-server/src/core/auth/am_token"
	"angrymiao-ai-server/src/core/botconfig"
	"angrymiao-ai-server/src/core/transport"
	"angrymiao-ai-server/src/core/utils"
	"angrymiao-ai-server/src/httpsvr/device"

	"github.com/gorilla/websocket"
)

// WebSocketTransport WebSocket传输层实现
type WebSocketTransport struct {
	config            *configs.Config
	server            *http.Server
	logger            *utils.Logger
	connHandler       transport.ConnectionHandlerFactory
	activeConnections sync.Map
	upgrader          *websocket.Upgrader
	authToken         *auth.AuthToken // JWT认证工具
	userConfigService botconfig.Service
}

// NewWebSocketTransport 创建WebSocket传输层
func NewWebSocketTransport(config *configs.Config, logger *utils.Logger, userConfigService botconfig.Service) *WebSocketTransport {
	return &WebSocketTransport{
		config: config,
		logger: logger,
		upgrader: &websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许所有来源，生产环境应该更严格
			},
		},
		authToken:         auth.NewAuthToken(config.Server.Token), // 初始化JWT认证工具
		userConfigService: userConfigService,
	}
}

// Start 启动WebSocket传输层
func (t *WebSocketTransport) Start(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", t.config.Transport.WebSocket.IP, t.config.Transport.WebSocket.Port)

	mux := http.NewServeMux()
	mux.HandleFunc("/", t.handleWebSocket)
	// App 专用入口，使用 AM Token 做认证
	mux.HandleFunc("/ws/app", t.handleAppWebSocket)

	t.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	t.logger.Info("启动WebSocket传输层 ws://%s", addr)

	// 监听关闭信号
	go func() {
		<-ctx.Done()
		t.Stop()
	}()

	if err := t.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("WebSocket传输层启动失败: %v", err)
	}

	return nil
}

// Stop 停止WebSocket传输层
func (t *WebSocketTransport) Stop() error {
	if t.server != nil {
		t.logger.Info("WebSocket传输层...")

		// 关闭所有活动连接
		t.activeConnections.Range(func(key, value interface{}) bool {
			if handler, ok := value.(transport.ConnectionHandler); ok {
				handler.Close()
			}
			t.activeConnections.Delete(key)
			return true
		})

		return t.server.Close()
	}
	return nil
}

// SetConnectionHandler 设置连接处理器工厂
func (t *WebSocketTransport) SetConnectionHandler(handler transport.ConnectionHandlerFactory) {
	t.connHandler = handler
}

// GetActiveConnectionCount 获取活跃连接数
func (t *WebSocketTransport) GetActiveConnectionCount() int {
	count := 0
	t.activeConnections.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// GetType 获取传输类型
func (t *WebSocketTransport) GetType() string {
	return "websocket"
}

// verifyJWTAuth 验证JWT认证并返回用户ID
func (t *WebSocketTransport) verifyJWTAuth(r *http.Request) (uint, error) {
	// 获取Authorization头
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return 0, fmt.Errorf("缺少或无效的Authorization头")
	}

	token := authHeader[7:] // 移除"Bearer "前缀

	// 验证JWT token
	isValid, deviceID, userID, err := t.authToken.VerifyToken(token)
	if err != nil || !isValid {
		return 0, fmt.Errorf("JWT token验证失败: %v", err)
	}

	// 检查设备ID匹配
	requestDeviceID := r.Header.Get("Device-Id")
	if requestDeviceID != deviceID {
		return 0, fmt.Errorf("设备ID与token不匹配: 请求=%s, token=%s", requestDeviceID, deviceID)
	}

	t.logger.Info("用户认证成功: userID=%d, deviceID=%s", userID, deviceID)

	return userID, nil
}

// verifyAMJWTAuth 使用 AM Token 验证并返回用户ID
func (t *WebSocketTransport) verifyAMJWTAuth(r *http.Request) (uint, error) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return 0, fmt.Errorf("缺少或无效的Authorization头")
	}
	token := authHeader[7:]
	claims, err := am_token.ParseToken(token)
	if err != nil {
		return 0, fmt.Errorf("签名验证失败: %v", err)
	}
	if claims == nil {
		return 0, fmt.Errorf("签名解析失败")
	}
	if claims.UserID <= 0 {
		return 0, fmt.Errorf("签名不包含有效的用户ID")
	}
	return uint(claims.UserID), nil
}

// handleWebSocket 处理WebSocket连接
func (t *WebSocketTransport) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 从URL参数中获取header信息（用于支持WebSocket连接时传递自定义header）
	if t.config.Transport.WebSocket.Browser {
		query := r.URL.Query()
		if deviceId := query.Get("Device-Id"); deviceId != "" {
			r.Header.Set("Device-Id", deviceId)
		}
		if clientId := query.Get("Client-Id"); clientId != "" {
			r.Header.Set("Client-Id", clientId)
		}
		if sessionId := query.Get("Session-Id"); sessionId != "" {
			r.Header.Set("Session-Id", sessionId)
		}
		if transportType := query.Get("Transport-Type"); transportType != "" {
			r.Header.Set("Transport-Type", transportType)
		}
		if token := query.Get("Token"); token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("Token", token)
		}
		if vad := query.Get("Enable-VAD"); vad != "" {
			r.Header.Set("Enable-VAD", vad)
		}
	}

	// 验证JWT认证并获取用户ID
	userID, err := t.verifyJWTAuth(r)
	if err != nil {
		t.logger.Warn("WebSocket认证失败: %v device-id: %s", err, r.Header.Get("Device-Id"))
		http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// 认证成功后，直接在连接处理器上绑定用户ID
	t.logger.Info("WebSocket认证成功: device-id=%s, user-id=%d", r.Header.Get("Device-Id"), userID)

	conn, err := t.upgrader.Upgrade(w, r, nil)
	if err != nil {
		t.logger.Error("WebSocket升级失败: %v", err)
		return
	}

	clientID := fmt.Sprintf("%p", conn)
	t.logger.Info("收到WebSocket连接请求: %s", r.Header.Get("Device-Id"))
	wsConn := NewWebSocketConnection(clientID, conn)

	// 若请求未提供 Session-Id，则使用 clientID 作为会话ID
	sessionID := r.Header.Get("Session-Id")
	if sessionID == "" {
		sessionID = clientID
		r.Header.Set("Session-Id", sessionID)
	}
	deviceID := r.Header.Get("Device-Id")

	if t.connHandler == nil {
		t.logger.Error("连接处理器工厂未设置")
		conn.Close()
		return
	}

	handler := t.connHandler.CreateHandler(wsConn, r)
	if handler == nil {
		t.logger.Error("创建连接处理器失败")
		conn.Close()
		return
	}
	// 绑定用户ID到具体的 ConnectionHandler
	if adapter, ok := handler.(*transport.ConnectionContextAdapter); ok {
		ch := adapter.GetConnectionHandler()
		ch.SetUserID(fmt.Sprintf("%d", userID))
	} else {
		t.logger.Warn("连接处理器类型非预期，无法绑定用户ID")
	}

	t.activeConnections.Store(clientID, handler)
	t.logger.Info("WebSocket客户端 %s 连接已建立，资源已分配", clientID)

	// 标记会话在线
	device.GetPresenceManager().SetSessionOnline(deviceID, sessionID)

	// 启动连接处理，并在结束时清理资源
	go func() {
		defer func() {
			// 连接结束时清理
			t.activeConnections.Delete(clientID)
			handler.Close()
			// 标记会话离线
			device.GetPresenceManager().SetSessionOffline(deviceID, sessionID)
		}()

		handler.Handle()
	}()
}

// handleAppWebSocket 处理 App 专用 WebSocket 连接（使用 AM Token 认证）
func (t *WebSocketTransport) handleAppWebSocket(w http.ResponseWriter, r *http.Request) {
	// 支持从 query 注入 header
	if t.config.Transport.WebSocket.Browser {
		query := r.URL.Query()
		if deviceId := query.Get("Device-Id"); deviceId != "" {
			r.Header.Set("Device-Id", deviceId)
		}
		if clientId := query.Get("Client-Id"); clientId != "" {
			r.Header.Set("Client-Id", clientId)
		}
		if sessionId := query.Get("Session-Id"); sessionId != "" {
			r.Header.Set("Session-Id", sessionId)
		}
		if token := query.Get("Token"); token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("Token", token)
		}
		if vad := query.Get("Enable-VAD"); vad != "" {
			r.Header.Set("Enable-VAD", vad)
		}
	}

	// 标记传输类型为 app
	r.Header.Set("Transport-Type", "WebSocket-App")

	// 使用 AM Token 校验用户
	userID, err := t.verifyAMJWTAuth(r)
	if err != nil {
		t.logger.Warn("[APP] WebSocket认证失败: %v", err)
		http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// 升级 WebSocket
	conn, err := t.upgrader.Upgrade(w, r, nil)
	if err != nil {
		t.logger.Error("[APP] WebSocket升级失败: %v", err)
		return
	}

	clientID := fmt.Sprintf("%p", conn)
	deviceID := r.Header.Get("Device-Id")
	if deviceID == "" {
		deviceID = fmt.Sprintf("app-%d", userID)
		r.Header.Set("Device-Id", deviceID)
	}

	wsConn := NewWebSocketConnection(clientID, conn)

	if t.connHandler == nil {
		t.logger.Error("[APP] 连接处理器工厂未设置")
		conn.Close()
		return
	}

	// 创建处理器并绑定用户ID
	handler := t.connHandler.CreateHandler(wsConn, r)
	if handler == nil {
		t.logger.Error("[APP] 创建连接处理器失败")
		conn.Close()
		return
	}
	if adapter, ok := handler.(*transport.ConnectionContextAdapter); ok {
		ch := adapter.GetConnectionHandler()
		ch.SetUserID(fmt.Sprintf("%d", userID))
	} else {
		t.logger.Warn("[APP] 连接处理器类型非预期，无法绑定用户ID")
	}

	// 记录活跃连接
	t.activeConnections.Store(clientID, handler)
	t.logger.Info("[APP] WebSocket客户端 %s 连接已建立，device-id=%s", clientID, deviceID)

	// Session 处理
	sessionID := r.Header.Get("Session-Id")
	if sessionID == "" {
		sessionID = clientID
		r.Header.Set("Session-Id", sessionID)
	}
	device.GetPresenceManager().SetSessionOnline(deviceID, sessionID)

	// 启动连接处理，并在结束时清理资源
	go func() {
		defer func() {
			t.activeConnections.Delete(clientID)
			handler.Close()
			device.GetPresenceManager().SetSessionOffline(deviceID, sessionID)
		}()
		handler.Handle()
	}()
}
