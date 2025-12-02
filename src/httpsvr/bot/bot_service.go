package bot

import (
	"context"
	"fmt"
	"time"

	"angrymiao-ai-server/src/core/utils"
	"angrymiao-ai-server/src/models"

	"gorm.io/gorm"
)

// BotConfigService Bot配置服务接口
type BotConfigService interface {
	// CRUD操作
	CreateBotConfig(ctx context.Context, config *models.BotConfig) error
	GetBotConfigByID(ctx context.Context, id uint) (*models.BotConfig, error)
	GetBotConfigByHash(ctx context.Context, botHash string) (*models.BotConfig, error)
	UpdateBotConfig(ctx context.Context, config *models.BotConfig) error
	DeleteBotConfig(ctx context.Context, id uint, userID uint) error

	// 搜索和查询
	SearchBots(ctx context.Context, userID uint, query string, searchType string) ([]*models.BotConfig, error)
	GetUserCreatedBots(ctx context.Context, userID uint) ([]*models.BotConfig, error)

	// 权限验证
	CheckBotPermission(ctx context.Context, botID uint, userID uint) (bool, error)

	// 业务逻辑
	GenerateBotHash(creatorID uint, configName string) (string, error)

	// 用户级配置
	GetUserBotLLMConfig(ctx context.Context, userID uint, botID uint) (*UserBotLLMConfig, error)
}

// DefaultBotConfigService 默认Bot配置服务实现
type DefaultBotConfigService struct {
	db     *gorm.DB
	logger *utils.Logger
}

// NewBotConfigService 创建Bot配置服务实例
func NewBotConfigService(db *gorm.DB, logger *utils.Logger) BotConfigService {
	return &DefaultBotConfigService{
		db:     db,
		logger: logger,
	}
}

// CreateBotConfig 创建Bot配置
func (s *DefaultBotConfigService) CreateBotConfig(ctx context.Context, config *models.BotConfig) error {
	config.CreatedAt = time.Now()
	config.UpdatedAt = time.Now()

	if err := s.db.WithContext(ctx).Create(config).Error; err != nil {
		s.logger.Error("创建Bot配置失败: %v", err)
		return err
	}

	s.logger.Info("用户 %d 创建Bot配置成功: %s (ID: %d)", config.CreatorID, config.FunctionName, config.ID)
	return nil
}

// GetBotConfigByID 根据ID获取Bot配置
func (s *DefaultBotConfigService) GetBotConfigByID(ctx context.Context, id uint) (*models.BotConfig, error) {
	var config models.BotConfig
	err := s.db.WithContext(ctx).First(&config, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("Bot配置不存在")
		}
		return nil, err
	}
	return &config, nil
}

// GetBotConfigByHash 根据BotHash获取Bot配置
func (s *DefaultBotConfigService) GetBotConfigByHash(ctx context.Context, botHash string) (*models.BotConfig, error) {
	var config models.BotConfig
	err := s.db.WithContext(ctx).Where("bot_hash = ?", botHash).First(&config).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("Bot配置不存在")
		}
		return nil, err
	}
	return &config, nil
}

// UpdateBotConfig 更新Bot配置
func (s *DefaultBotConfigService) UpdateBotConfig(ctx context.Context, config *models.BotConfig) error {
	config.UpdatedAt = time.Now()

	if err := s.db.WithContext(ctx).Save(config).Error; err != nil {
		s.logger.Error("更新Bot配置失败: %v", err)
		return err
	}

	s.logger.Info("用户 %d 更新Bot配置成功: %s (ID: %d)", config.CreatorID, config.FunctionName, config.ID)
	return nil
}

// DeleteBotConfig 删除Bot配置
func (s *DefaultBotConfigService) DeleteBotConfig(ctx context.Context, id uint, userID uint) error {
	// 先检查权限
	hasPermission, err := s.CheckBotPermission(ctx, id, userID)
	if err != nil {
		return err
	}
	if !hasPermission {
		return fmt.Errorf("无权限删除此Bot配置")
	}

	// 删除Bot配置
	result := s.db.WithContext(ctx).Delete(&models.BotConfig{}, id)
	if result.Error != nil {
		s.logger.Error("删除Bot配置失败: %v", result.Error)
		return result.Error
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("Bot配置不存在")
	}

	// 同时删除所有用户与该Bot的好友关系
	s.db.WithContext(ctx).Where("bot_config_id = ? AND friend_type = ?", id, "bot").Delete(&models.UserFriend{})

	s.logger.Info("删除Bot配置成功 (ID: %d)", id)
	return nil
}

// SearchBots 搜索Bot配置
func (s *DefaultBotConfigService) SearchBots(ctx context.Context, userID uint, query string, searchType string) ([]*models.BotConfig, error) {
	var configs []*models.BotConfig
	db := s.db.WithContext(ctx)

	// 根据搜索类型构建查询
	switch searchType {
	case "hash":
		// bot_hash精确搜索
		db = db.Where("bot_hash = ?", query)
	case "name":
		// bot_name模糊搜索
		db = db.Where("function_name LIKE ?", "%"+query+"%")
	case "description":
		// description模糊搜索
		db = db.Where("description LIKE ?", "%"+query+"%")
	default:
		// 默认：name或description模糊搜索
		db = db.Where("function_name LIKE ? OR description LIKE ?", "%"+query+"%", "%"+query+"%")
	}

	// 权限过滤：public Bot对所有用户可见，private Bot只对创建者可见
	db = db.Where("visibility = ? OR creator_id = ?", "public", userID)

	err := db.Order("created_at DESC").Find(&configs).Error
	return configs, err
}

// GetUserCreatedBots 获取用户创建的Bot列表
func (s *DefaultBotConfigService) GetUserCreatedBots(ctx context.Context, userID uint) ([]*models.BotConfig, error) {
	var configs []*models.BotConfig
	err := s.db.WithContext(ctx).
		Where("creator_id = ?", userID).
		Order("created_at DESC").
		Find(&configs).Error
	return configs, err
}

// CheckBotPermission 检查用户是否有权限操作Bot
func (s *DefaultBotConfigService) CheckBotPermission(ctx context.Context, botID uint, userID uint) (bool, error) {
	var config models.BotConfig
	err := s.db.WithContext(ctx).First(&config, botID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, fmt.Errorf("Bot配置不存在")
		}
		return false, err
	}

	// 只有创建者有权限
	return config.CreatorID == userID, nil
}

// GenerateBotHash 生成Bot Hash
func (s *DefaultBotConfigService) GenerateBotHash(creatorID uint, configName string) (string, error) {
	input := fmt.Sprintf("%d:%s:%d", creatorID, configName, time.Now().UnixNano())
	return utils.GenerateUniqueHash(input)
}

// UserBotLLMConfig 用户Bot的LLM配置（连表查询结果）
type UserBotLLMConfig struct {
	// user_friends 表字段
	UserFriendID uint   `gorm:"column:user_friend_id"`
	AppKey       string `gorm:"column:app_key"`
	IsActive     bool   `gorm:"column:is_active"`

	// bot_configs 表字段
	BotConfigID     uint    `gorm:"column:bot_config_id"`
	Temperature     float32 `gorm:"column:temperature"`
	MaxTokens       int     `gorm:"column:max_tokens"`
	RequiresNetwork bool    `gorm:"column:requires_network"`

	// model_configs 表字段
	ModelID     uint   `gorm:"column:model_id"`
	LLMType     string `gorm:"column:llm_type"`
	ModelName   string `gorm:"column:model_name"`
	BaseURL     string `gorm:"column:base_url"`
	LLMProtocol string `gorm:"column:llm_protocol"`
}

// GetUserBotLLMConfig 获取用户Bot的LLM配置（一条SQL连表查询）
func (s *DefaultBotConfigService) GetUserBotLLMConfig(ctx context.Context, userID uint, botID uint) (*UserBotLLMConfig, error) {
	var config UserBotLLMConfig

	// 一条SQL连表查询：user_friends JOIN bot_configs JOIN model_configs
	// 使用 Scan 而不是 First，避免 GORM 自动添加 ORDER BY
	err := s.db.WithContext(ctx).
		Table("user_friends AS uf").
		Select(`
			uf.id AS user_friend_id,
			uf.app_key,
			uf.is_active,
			bc.id AS bot_config_id,
			bc.temperature,
			bc.max_tokens,
			bc.requires_network,
			mc.id AS model_id,
			mc.llm_type,
			mc.model_name,
			mc.base_url,
			mc.llm_protocol
		`).
		Joins("INNER JOIN bot_configs AS bc ON uf.bot_config_id = bc.id").
		Joins("INNER JOIN model_configs AS mc ON bc.model_id = mc.id").
		Where("uf.user_id = ? AND uf.bot_config_id = ? AND uf.friend_type = ? AND uf.is_active = ?",
			userID, botID, "bot", true).
		Limit(1).
		Scan(&config).Error

	if err != nil {
		s.logger.Error("查询用户Bot配置失败: %v", err)
		return nil, fmt.Errorf("查询用户Bot配置失败: %v", err)
	}

	// 检查是否查询到数据（通过主键判断）
	if config.UserFriendID == 0 {
		return nil, fmt.Errorf("用户未添加该Bot或Bot已禁用")
	}

	return &config, nil
}
