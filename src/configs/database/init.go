package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"angrymiao-ai-server/src/configs"
	"angrymiao-ai-server/src/models"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	am_utils "angrymiao-ai-server/src/core/utils"

	gorm_logger "gorm.io/gorm/logger"
)

const (
	SystemConfigID = 1 // 系统配置的唯一ID
	ModuleConfigID = 1 // 模块配置的唯一ID
	ServerConfigID = 1 // 服务器配置的唯一ID
)

type DBLogger struct {
	logger *am_utils.Logger
}

func (l *DBLogger) LogMode(level gorm_logger.LogLevel) gorm_logger.Interface {
	return &DBLogger{
		logger: l.logger,
	}
}

func (l *DBLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	l.logger.Info(msg, data...)
}

func (l *DBLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	l.logger.Warn(msg, data...)
}

func (l *DBLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	l.logger.Error(msg, data...)
}

func (l *DBLogger) Trace(
	ctx context.Context,
	begin time.Time,
	fc func() (sql string, rowsAffected int64),
	err error,
) {
	sql, rows := fc()
	elapsed := time.Since(begin)
	if err != nil {
		if strings.Contains(err.Error(), "record not found") {
			// 忽略记录未找到的错误
			return
		}
		l.logger.Error("SQL Trace Error", map[string]interface{}{
			"sql":     sql,
			"rows":    rows,
			"elapsed": elapsed,
			"err":     err,
		})
	} else {
		l.logger.Debug("SQL Trace", map[string]interface{}{
			"sql":     sql,
			"rows":    rows,
			"elapsed": elapsed,
		})
	}
}

var (
	DB       *gorm.DB
	dbLogger *am_utils.Logger
)

func GetDB() *gorm.DB {
	if DB == nil {
		panic("数据库未初始化，请先调用 InitDB()")
	}
	return DB
}

// GetTxDB 获取一个新的事务DB（需手动Commit或Rollback）
func GetTxDB() *gorm.DB {
	if DB == nil {
		panic("数据库未初始化，请先调用 InitDB()")
	}
	return DB.Begin()
}

// InitDB 初始化数据库类型并连接
func InitDB(config *configs.Config) (*gorm.DB, string, error) {
	var (
		db     *gorm.DB
		err    error
		dbType string
	)

	// 先尝试加载配置文件获取数据库配置
	if config.DB.Dialect == "" {
		// 如果配置加载失败或未配置数据库，使用默认SQLite
		dbType = "sqlite"
		path := "./config.db"
		db, err = gorm.Open(sqlite.Open(path))
	} else {
		// 根据配置文件中的数据库类型进行连接
		dbType = config.DB.Dialect
		switch dbType {
		case "postgres":
			db, err = gorm.Open(postgres.Open(config.DB.DSN))
			fmt.Println("postgres 数据库连接成功")
		case "sqlite":
			db, err = gorm.Open(sqlite.Open(config.DB.DSN))
			fmt.Println("sqlite 数据库连接成功")
		default:
			return nil, "", fmt.Errorf("不支持的数据库类型: %s", dbType)
		}
	}

	if err != nil {
		return nil, "", fmt.Errorf("连接数据库失败: %w", err)
	}

	// 自动迁移所有表
	if err := migrateTables(db); err != nil {
		return nil, dbType, err
	}

	// 插入默认配置
	if err := InsertDefaultConfigIfNeeded(db); err != nil {
		fmt.Println("插入默认配置失败: %v", err)
	}

	DB = db

	NewServerConfigDB(db)

	return db, dbType, nil
}

// SetLogger 设置数据库日志记录器并显示数据库版本信息
func SetLogger(logger *am_utils.Logger) {
	dbLogger = logger
	DB.Logger = &DBLogger{logger: logger}

	// 根据数据库类型获取版本信息
	var version string
	var dbType string

	// 通过GORM的Dialector判断数据库类型
	dialectorName := DB.Dialector.Name()

	switch dialectorName {
	case "postgres":
		DB.Raw("SELECT version()").Scan(&version)
		dbType = "PostgreSQL"
	case "sqlite":
		DB.Raw("SELECT sqlite_version()").Scan(&version)
		dbType = "SQLite"
	default:
		version = "未知版本"
		dbType = dialectorName
	}

	logger.Info("%s 数据库连接成功，版本: %s", dbType, version)
}

// migrateTables 自动迁移模型表结构
func migrateTables(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.SystemConfig{},
		&models.User{},
		&models.UserSetting{},
		&models.ModuleConfig{},
		&models.Device{},
		&models.DialogueMessage{},
		&models.MediaUpload{},
		&models.AuthClient{},
		&models.AudioTask{},
		// 新的Bot配置系统模型
		&models.ModelConfig{},
		&models.BotConfig{},
		&models.UserFriend{},
	)
}

// InsertDefaultConfigIfNeeded 首次启动插入默认配置
func InsertDefaultConfigIfNeeded(db *gorm.DB) error {

	return nil
}
