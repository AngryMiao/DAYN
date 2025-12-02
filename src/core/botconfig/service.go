package botconfig

import (
	"context"
	"fmt"
	"strconv"

	"angrymiao-ai-server/src/core/types"
	"angrymiao-ai-server/src/core/utils"
	"angrymiao-ai-server/src/models"

	"gorm.io/gorm"
)

// Service Bot配置服务接口
type Service interface {
	GetUserConfigs(ctx context.Context, userID string) ([]*types.BotConfig, error)
	GetActiveConfigs(ctx context.Context, userID string) ([]*types.BotConfig, error)
	GetBotFriendConfig(ctx context.Context, userID uint, botConfigID uint) (*types.BotConfig, error)
}

// DefaultService 默认Bot配置服务实现
type DefaultService struct {
	db     *gorm.DB
	logger *utils.Logger
}

// NewService 创建Bot配置服务实例
func NewService(db *gorm.DB, logger *utils.Logger) Service {
	return &DefaultService{
		db:     db,
		logger: logger,
	}
}

// GetUserConfigs 获取用户的所有Bot配置
func (s *DefaultService) GetUserConfigs(ctx context.Context, userID string) ([]*types.BotConfig, error) {
	uid, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		s.logger.Error("无效的用户ID: %s", userID)
		return nil, fmt.Errorf("无效的用户ID")
	}

	var friends []models.UserFriend
	err = s.db.WithContext(ctx).
		Where("user_id = ? AND friend_type = ?", uint(uid), "bot").
		Order("priority DESC, created_at DESC").
		Find(&friends).Error

	if err != nil {
		s.logger.Error("查询用户Bot好友失败: %v", err)
		return nil, err
	}

	if len(friends) == 0 {
		return []*types.BotConfig{}, nil
	}

	return s.assembleBotConfigs(ctx, userID, friends)
}

// GetActiveConfigs 获取用户的活跃Bot配置
func (s *DefaultService) GetActiveConfigs(ctx context.Context, userID string) ([]*types.BotConfig, error) {
	uid, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		s.logger.Error("无效的用户ID: %s", userID)
		return nil, fmt.Errorf("无效的用户ID")
	}

	var friends []models.UserFriend
	err = s.db.WithContext(ctx).
		Where("user_id = ? AND friend_type = ? AND is_active = ?", uint(uid), "bot", true).
		Order("priority DESC, created_at DESC").
		Find(&friends).Error

	if err != nil {
		s.logger.Error("查询用户活跃Bot好友失败: %v", err)
		return nil, err
	}

	if len(friends) == 0 {
		return []*types.BotConfig{}, nil
	}

	return s.assembleBotConfigs(ctx, userID, friends)
}

// assembleBotConfigs 组装Bot配置（从好友表+bot_configs+model_configs）
func (s *DefaultService) assembleBotConfigs(ctx context.Context, userID string, friends []models.UserFriend) ([]*types.BotConfig, error) {
	botConfigIDs := make([]uint, 0, len(friends))
	friendMap := make(map[uint]*models.UserFriend)
	for i := range friends {
		if friends[i].BotConfigID != nil {
			botConfigIDs = append(botConfigIDs, *friends[i].BotConfigID)
			friendMap[*friends[i].BotConfigID] = &friends[i]
		}
	}

	if len(botConfigIDs) == 0 {
		return []*types.BotConfig{}, nil
	}

	var botConfigs []models.BotConfig
	if err := s.db.WithContext(ctx).Where("id IN ?", botConfigIDs).Find(&botConfigs).Error; err != nil {
		s.logger.Error("查询Bot配置失败: %v", err)
		return nil, err
	}

	modelIDs := make([]uint, 0, len(botConfigs))
	for i := range botConfigs {
		modelIDs = append(modelIDs, botConfigs[i].ModelID)
	}

	var modelConfigs []models.ModelConfig
	if err := s.db.WithContext(ctx).Where("id IN ?", modelIDs).Find(&modelConfigs).Error; err != nil {
		s.logger.Error("查询模型配置失败: %v", err)
		return nil, err
	}

	modelConfigMap := make(map[uint]*models.ModelConfig)
	for i := range modelConfigs {
		modelConfigMap[modelConfigs[i].ID] = &modelConfigs[i]
	}

	var configs []*types.BotConfig
	for _, botConfig := range botConfigs {
		friend := friendMap[botConfig.ID]
		modelConfig := modelConfigMap[botConfig.ModelID]

		if friend == nil || modelConfig == nil {
			continue
		}

		configs = append(configs, &types.BotConfig{
			ID:           botConfig.ID,
			UserID:       userID,
			LLMType:      modelConfig.LLMType,
			ModelName:    modelConfig.ModelName,
			APIKey:       friend.AppKey,
			BaseURL:      modelConfig.BaseURL,
			MaxTokens:    botConfig.MaxTokens,
			Temperature:  botConfig.Temperature,
			FunctionName: botConfig.FunctionName,
			Description:  botConfig.Description,
			Parameters:   botConfig.Parameters,
			MCPServerURL: botConfig.MCPServerURL,
			IsActive:     friend.IsActive,
			Priority:     friend.Priority,
			BotHash:      botConfig.BotHash,
			CreatedAt:    botConfig.CreatedAt,
			UpdatedAt:    botConfig.UpdatedAt,
		})
	}

	return configs, nil
}

// GetBotFriendConfig 获取用户指定的Bot好友配置
func (s *DefaultService) GetBotFriendConfig(ctx context.Context, userID uint, botConfigID uint) (*types.BotConfig, error) {
	// 查询用户的Bot好友关系
	var friend models.UserFriend
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND bot_config_id = ? AND friend_type = ?", userID, botConfigID, "bot").
		First(&friend).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("Bot好友不存在")
		}
		s.logger.Error("查询Bot好友失败: %v", err)
		return nil, err
	}

	// 查询Bot配置
	var botConfig models.BotConfig
	if err := s.db.WithContext(ctx).First(&botConfig, botConfigID).Error; err != nil {
		s.logger.Error("查询Bot配置失败: %v", err)
		return nil, err
	}

	// 查询模型配置
	var modelConfig models.ModelConfig
	if err := s.db.WithContext(ctx).First(&modelConfig, botConfig.ModelID).Error; err != nil {
		s.logger.Error("查询模型配置失败: %v", err)
		return nil, err
	}

	return &types.BotConfig{
		ID:           botConfig.ID,
		UserID:       fmt.Sprintf("%d", userID),
		LLMType:      modelConfig.LLMType,
		ModelName:    modelConfig.ModelName,
		APIKey:       friend.AppKey, // 使用用户好友表中的AppKey
		BaseURL:      modelConfig.BaseURL,
		MaxTokens:    botConfig.MaxTokens,
		Temperature:  botConfig.Temperature,
		FunctionName: botConfig.FunctionName,
		Description:  botConfig.Description,
		Parameters:   botConfig.Parameters,
		MCPServerURL: botConfig.MCPServerURL,
		IsActive:     friend.IsActive,
		Priority:     friend.Priority,
		BotHash:      botConfig.BotHash,
		CreatedAt:    botConfig.CreatedAt,
		UpdatedAt:    botConfig.UpdatedAt,
	}, nil
}
