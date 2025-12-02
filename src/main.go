// @title 怒喵 API 文档
// @version 1.0
// @host localhost:8080
// @BasePath /api
package main

import (
	// 标准库
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	// 第三方库
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"

	// 项目内部包 - 配置相关
	"angrymiao-ai-server/src/configs"
	"angrymiao-ai-server/src/configs/database"
	"angrymiao-ai-server/src/httpsvr/bot"

	// 项目内部包 - 核心功能
	"angrymiao-ai-server/src/core/auth"
	"angrymiao-ai-server/src/core/auth/am_token"
	"angrymiao-ai-server/src/core/auth/store"
	"angrymiao-ai-server/src/core/botconfig"
	"angrymiao-ai-server/src/core/middleware"
	"angrymiao-ai-server/src/core/pool"
	"angrymiao-ai-server/src/core/transport"
	"angrymiao-ai-server/src/core/transport/grpcgateway"
	"angrymiao-ai-server/src/core/transport/mqtt"
	"angrymiao-ai-server/src/core/transport/websocket"
	"angrymiao-ai-server/src/core/utils"

	// 项目内部包 - 业务模块
	appApi "angrymiao-ai-server/src/httpsvr/app"
	"angrymiao-ai-server/src/httpsvr/device"
	"angrymiao-ai-server/src/httpsvr/ota"
	"angrymiao-ai-server/src/httpsvr/vision"
	"angrymiao-ai-server/src/task"

	// 文档
	_ "angrymiao-ai-server/src/docs"

	// AI 提供商 - 自动注册
	_ "angrymiao-ai-server/src/core/providers/asr/deepgram"
	_ "angrymiao-ai-server/src/core/providers/asr/doubao"
	_ "angrymiao-ai-server/src/core/providers/asr/gosherpa"
	_ "angrymiao-ai-server/src/core/providers/auc/doubao"
	_ "angrymiao-ai-server/src/core/providers/llm/coze"
	_ "angrymiao-ai-server/src/core/providers/llm/ollama"
	_ "angrymiao-ai-server/src/core/providers/llm/openai"
	_ "angrymiao-ai-server/src/core/providers/tts/deepgram"
	_ "angrymiao-ai-server/src/core/providers/tts/doubao"
	_ "angrymiao-ai-server/src/core/providers/tts/edge"
	_ "angrymiao-ai-server/src/core/providers/tts/gosherpa"
	_ "angrymiao-ai-server/src/core/providers/vad/webrtc"
	_ "angrymiao-ai-server/src/core/providers/vlllm/ollama"
	_ "angrymiao-ai-server/src/core/providers/vlllm/openai"
)

// Application 应用程序主结构体，管理整个应用的生命周期
type Application struct {
	config        *configs.Config
	logger        *utils.Logger
	db            *gorm.DB
	authManager   *auth.AuthManager
	serverManager *ServerManager
	ctx           context.Context
	cancel        context.CancelFunc
	group         *errgroup.Group
}

// ServerManager 服务管理器，负责管理所有服务的启动和关闭
type ServerManager struct {
	transportManager *transport.TransportManager
	httpServer       *http.Server
	logger           *utils.Logger
}

// ConfigurationManager 配置管理器，负责配置的加载和验证
type ConfigurationManager struct {
	config *configs.Config
	logger *utils.Logger
}

// NewApplication 创建新的应用程序实例
func NewApplication() *Application {
	ctx, cancel := context.WithCancel(context.Background())
	return &Application{
		ctx:    ctx,
		cancel: cancel,
	}
}

// Initialize 初始化应用程序
func (app *Application) Initialize() error {
	var err error

	// 初始化配置和日志
	if err = app.initializeConfigAndLogger(); err != nil {
		return fmt.Errorf("初始化配置和日志失败: %w", err)
	}

	// 验证配置
	if err = app.validateConfiguration(); err != nil {
		return fmt.Errorf("配置验证失败: %w", err)
	}

	// 初始化数据库
	if err = app.initializeDatabase(); err != nil {
		return fmt.Errorf("初始化数据库失败: %w", err)
	}

	// 初始化认证管理器
	if err = app.initializeAuthManager(); err != nil {
		return fmt.Errorf("初始化认证管理器失败: %w", err)
	}

	// 初始化服务管理器
	if err = app.initializeServerManager(); err != nil {
		return fmt.Errorf("初始化服务管理器失败: %w", err)
	}

	app.logger.Info("应用程序初始化完成")
	return nil
}

// initializeConfigAndLogger 初始化配置和日志系统
func (app *Application) initializeConfigAndLogger() error {
	// 加载配置，默认使用.config.yaml
	config, configPath, err := configs.LoadConfig(database.GetServerConfigDB())
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	app.config = config

	// 初始化日志系统
	logger, err := utils.NewLogger((*utils.LogCfg)(&config.Log))
	if err != nil {
		return fmt.Errorf("初始化日志系统失败: %w", err)
	}
	app.logger = logger
	utils.DefaultLogger = logger

	app.logger.Info("配置和日志系统初始化成功, 配置文件路径: %s", configPath)

	// 初始化AM jwt权限校验
	if err := am_token.Init(config); err != nil {
		app.logger.Error("初始化AM jwt失败: %v", err)
		return fmt.Errorf("初始化AM jwt失败: %w", err)
	}

	return nil
}

// validateConfiguration 验证配置的有效性
func (app *Application) validateConfiguration() error {
	if app.config == nil {
		return fmt.Errorf("配置对象为空")
	}

	// 验证Web端口配置
	if app.config.Web.Port <= 0 || app.config.Web.Port > 65535 {
		return fmt.Errorf("Web端口配置无效: %d", app.config.Web.Port)
	}

	app.logger.Info("配置验证通过")
	return nil
}

// initializeDatabase 初始化数据库连接
func (app *Application) initializeDatabase() error {
	db, _, err := database.InitDB(app.config)
	if err != nil {
		return fmt.Errorf("数据库连接失败: %w", err)
	}
	app.db = db

	database.SetLogger(app.logger)
	app.logger.Info("数据库连接初始化成功")
	return nil
}

// initializeAuthManager 初始化认证管理器
func (app *Application) initializeAuthManager() error {
	if !app.config.Server.Auth.Enabled {
		app.logger.Info("认证功能未启用")
		return nil
	}

	// 创建存储配置
	storeConfig := &store.StoreConfig{
		Type:     app.config.Server.Auth.Store.Type,
		ExpiryHr: app.config.Server.Auth.Store.Expiry,
		Config:   make(map[string]interface{}),
	}

	// 创建认证管理器
	authManager, err := auth.NewAuthManager(storeConfig, app.logger)
	if err != nil {
		return fmt.Errorf("创建认证管理器失败: %w", err)
	}
	app.authManager = authManager

	app.logger.Info("认证管理器初始化成功")
	return nil
}

// initializeServerManager 初始化服务管理器
func (app *Application) initializeServerManager() error {
	app.serverManager = &ServerManager{
		logger: app.logger,
	}
	return nil
}

// Start 启动应用程序
func (app *Application) Start() error {
	// 建立 errgroup，并基于当前 app.ctx 派生与组绑定的 ctx；
	// 当任一子任务返回错误或上游取消时，该 ctx 会同步取消。
	app.group, app.ctx = errgroup.WithContext(app.ctx)

	// 启动传输层服务
	if err := app.startTransportServer(); err != nil {
		return fmt.Errorf("启动传输层服务失败: %w", err)
	}

	// 启动HTTP服务
	if err := app.startHttpServer(); err != nil {
		return fmt.Errorf("启动HTTP服务失败: %w", err)
	}

	app.logger.Info("所有服务启动成功")
	return nil
}

// startTransportServer 启动传输层服务
func (app *Application) startTransportServer() error {
	// 初始化资源池管理器
	poolManager, err := pool.NewPoolManager(app.config, app.logger)
	if err != nil {
		return fmt.Errorf("初始化资源池管理器失败: %w", err)
	}

	// 初始化任务管理器
	taskMgr := task.NewTaskManager(task.ResourceConfig{
		MaxWorkers:        12,
		MaxTasksPerClient: 20,
	})
	taskMgr.Start()

	// 创建Bot配置服务（从好友表获取配置）
	userConfigService := botconfig.NewService(app.db, app.logger)

	// 创建传输管理器
	transportManager := transport.NewTransportManager(app.config, app.logger)
	app.serverManager.transportManager = transportManager

	// 创建连接处理器工厂
	handlerFactory := transport.NewDefaultConnectionHandlerFactory(
		app.config,
		poolManager,
		taskMgr,
		app.logger,
		userConfigService,
	)

	// 根据配置注册多个传输层
	if app.config.Transport.WebSocket.Enabled {
		wsTransport := websocket.NewWebSocketTransport(app.config, app.logger, userConfigService)
		wsTransport.SetConnectionHandler(handlerFactory)
		transportManager.RegisterTransport("websocket", wsTransport)
		app.logger.Info("WebSocket 传输层已注册")
	}
	if app.config.Transport.GrpcGateway.Enabled {
		gatewayTransport := grpcgateway.NewGrpcGatewayTransport(app.config, app.logger, handlerFactory)
		gatewayTransport.SetConnectionHandler(handlerFactory)
		transportManager.RegisterTransport("grpcgateway", gatewayTransport)
		app.logger.Info("GrpcGateway 传输层已注册")
	}
	if app.config.Transport.Mqtt.Enabled {
		mqttTransport := mqtt.NewMQTTTransport(app.config, app.logger)
		mqttTransport.SetConnectionHandler(handlerFactory)
		transportManager.RegisterTransport("mqtt", mqttTransport)
		app.logger.Info("MQTT 传输层已注册")
	}

	// 启动传输层服务
	app.group.Go(func() error {
		// 监听关闭信号
		go func() {
			<-app.ctx.Done()
			app.logger.Info("收到关闭信号，开始关闭所有传输层...")
			if err := transportManager.StopAll(); err != nil {
				app.logger.Error("关闭传输层失败: %v", err)
			} else {
				app.logger.Info("所有传输层已优雅关闭")
			}
		}()

		// 使用传输管理器启动服务
		if err := transportManager.StartAll(app.ctx); err != nil {
			if app.ctx.Err() != nil {
				return nil // 正常关闭
			}
			app.logger.Error("传输层运行失败: %v", err)
			return err
		}
		return nil
	})

	app.logger.Debug("传输层服务已成功启动")
	return nil
}

// startHttpServer 启动HTTP服务
func (app *Application) startHttpServer() error {
	// 初始化Gin引擎
	if app.config.Log.LogLevel == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.Default()
	router.SetTrustedProxies([]string{"0.0.0.0"})

	// 全局应用 CORS 中间件
	router.Use(middleware.CORS())

	// 注册路由
	if err := app.registerRoutes(router); err != nil {
		return fmt.Errorf("注册路由失败: %w", err)
	}

	// 创建HTTP服务器
	httpServer := &http.Server{
		Addr:    ":" + strconv.Itoa(app.config.Web.Port),
		Handler: router,
	}
	app.serverManager.httpServer = httpServer

	// 启动HTTP服务
	app.group.Go(func() error {
		app.logger.Info("Gin 服务已启动，访问地址: http://0.0.0.0:%d", app.config.Web.Port)

		// 在单独的 goroutine 中监听关闭信号
		go func() {
			<-app.ctx.Done()
			app.logger.Info("收到关闭信号，开始关闭HTTP服务...")

			// 创建关闭超时上下文
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				app.logger.Error("HTTP服务关闭失败: %v", err)
			} else {
				app.logger.Info("HTTP服务已优雅关闭")
			}
		}()

		// ListenAndServe 返回 ErrServerClosed 时表示正常关闭
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			app.logger.Error("HTTP 服务启动失败: %v", err)
			return err
		}
		return nil
	})

	return nil
}

// registerRoutes 注册所有路由
func (app *Application) registerRoutes(router *gin.Engine) error {
	// API路由全部挂载到/api前缀下
	apiGroup := router.Group("/api")
	// 统一跨域中间件
	apiGroup.Use(middleware.CORS())

	// 启动用户好友管理服务
	friendHandler := appApi.NewUserFriendHandler(app.db, app.logger)
	friendHandler.RegisterRoutes(apiGroup)

	// 启动Bot配置管理服务（需要 friendService 来检查 Bot 是否已添加）
	friendService := appApi.NewUserFriendService(app.db, app.logger)
	botHandler := bot.NewBotConfigHandler(app.db, app.logger, friendService)
	botHandler.RegisterRoutes(apiGroup)

	// 启动模型配置管理服务
	modelHandler := bot.NewModelConfigHandler(app.db, app.logger)
	modelHandler.RegisterRoutes(apiGroup)

	// 启动OTA服务
	otaService := ota.NewDefaultOTAService(app.config.Web.Websocket)
	otaService.Start(app.ctx, router, apiGroup)

	// 启动设备服务
	deviceService := device.NewDefaultDeviceService(app.config, app.logger)
	deviceService.Start(app.ctx, router, apiGroup)

	// 启动App服务
	appService := appApi.NewDefaultAppService(app.config, app.logger)
	appService.Start(app.ctx, router, apiGroup)

	// 启动Vision服务
	visionService, err := vision.NewDefaultVisionService(app.config, app.logger)
	if err != nil {
		app.logger.Error("Vision 服务初始化失败: %v", err)
	}
	if visionService != nil {
		visionService.Start(app.ctx, router, apiGroup)
	}

	// 注册Swagger文档路由
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return nil
}

// WaitForShutdown 等待关闭信号并执行优雅关机
func (app *Application) WaitForShutdown() {
	// 监听系统信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// 等待信号
	sig := <-sigChan
	app.logger.Info("接收到系统信号: %v，开始优雅关闭服务", sig)

	// 开始关闭流程
	app.Shutdown()
}

// Shutdown 执行优雅关机
func (app *Application) Shutdown() {
	// 取消上下文，通知所有服务开始关闭
	app.cancel()

	// 等待所有服务关闭，设置超时保护
	done := make(chan error, 1)
	go func() {
		done <- app.group.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			app.logger.Error("服务关闭过程中出现错误: %v", err)
			os.Exit(1)
		}
		app.logger.Info("所有服务已优雅关闭")
	case <-time.After(15 * time.Second):
		app.logger.Error("服务关闭超时，强制退出")
		os.Exit(1)
	}

	// 关闭认证管理器
	if app.authManager != nil {
		app.authManager.Close()
	}

	// 关闭日志系统
	if app.logger != nil {
		app.logger.Info("程序已成功退出")
		app.logger.Close()
	}
}

// main 程序入口点
func main() {
	// 创建应用程序实例
	app := NewApplication()

	// 初始化应用程序
	if err := app.Initialize(); err != nil {
		fmt.Printf("应用程序初始化失败: %v\n", err)
		os.Exit(1)
	}

	// 启动应用程序
	if err := app.Start(); err != nil {
		app.logger.Error("应用程序启动失败: %v", err)
		os.Exit(1)
	}

	// 等待关闭信号
	app.WaitForShutdown()
}
