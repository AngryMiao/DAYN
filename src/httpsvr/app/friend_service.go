package app

import (
	"context"
	"fmt"
	"time"

	"angrymiao-ai-server/src/core/utils"
	"angrymiao-ai-server/src/models"

	"gorm.io/gorm"
)

// UserFriendService 用户好友服务接口
type UserFriendService interface {
	// 好友管理
	AddBotFriend(ctx context.Context, userID uint, botConfigID uint, appKey string, alias string) error
	RemoveBotFriend(ctx context.Context, userID uint, botConfigID uint) error
	GetUserBotFriends(ctx context.Context, userID uint) ([]*models.UserBotFriendResponse, error)
	UpdateBotFriendPriority(ctx context.Context, userID uint, botConfigID uint, priority int) error
	ToggleBotFriendStatus(ctx context.Context, userID uint, botConfigID uint, isActive bool) error
	UpdateBotFriendAppKey(ctx context.Context, userID uint, botConfigID uint, appKey string) error

	// 查询
	IsBotAdded(ctx context.Context, userID uint, botConfigID uint) (bool, error)
	GetBotFriendAppKey(ctx context.Context, userID uint, botConfigID uint) (string, error)
}

// DefaultUserFriendService 默认用户好友服务实现
type DefaultUserFriendService struct {
	db     *gorm.DB
	logger *utils.Logger
}

// NewUserFriendService 创建用户好友服务实例
func NewUserFriendService(db *gorm.DB, logger *utils.Logger) UserFriendService {
	return &DefaultUserFriendService{
		db:     db,
		logger: logger,
	}
}

// AddBotFriend 添加Bot好友
func (s *DefaultUserFriendService) AddBotFriend(ctx context.Context, userID uint, botConfigID uint, appKey string, alias string) error {
	// 检查是否已添加
	isAdded, err := s.IsBotAdded(ctx, userID, botConfigID)
	if err != nil {
		return err
	}
	if isAdded {
		return fmt.Errorf("已添加该Bot好友")
	}

	// 创建好友关系
	friend := &models.UserFriend{
		UserID:      userID,
		FriendType:  "bot",
		BotConfigID: &botConfigID,
		AppKey:      appKey,
		Alias:       alias,
		Priority:    0,
		IsActive:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.db.WithContext(ctx).Create(friend).Error; err != nil {
		s.logger.Error("添加Bot好友失败: %v", err)
		return err
	}

	s.logger.Info("用户 %d 添加Bot好友成功 (BotConfigID: %d)", userID, botConfigID)
	return nil
}

// RemoveBotFriend 删除Bot好友
func (s *DefaultUserFriendService) RemoveBotFriend(ctx context.Context, userID uint, botConfigID uint) error {
	result := s.db.WithContext(ctx).
		Where("user_id = ? AND bot_config_id = ? AND friend_type = ?", userID, botConfigID, "bot").
		Delete(&models.UserFriend{})

	if result.Error != nil {
		s.logger.Error("删除Bot好友失败: %v", result.Error)
		return result.Error
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("Bot好友不存在")
	}

	s.logger.Info("用户 %d 删除Bot好友成功 (BotConfigID: %d)", userID, botConfigID)
	return nil
}

// GetUserBotFriends 获取用户的Bot好友列表
func (s *DefaultUserFriendService) GetUserBotFriends(ctx context.Context, userID uint) ([]*models.UserBotFriendResponse, error) {
	var friends []models.UserFriend
	var responses []*models.UserBotFriendResponse

	// 查询用户的Bot好友，并预加载Bot配置信息
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND friend_type = ?", userID, "bot").
		Order("priority DESC, created_at DESC").
		Find(&friends).Error

	if err != nil {
		return nil, err
	}

	// 获取关联的Bot配置
	for _, friend := range friends {
		if friend.BotConfigID == nil {
			continue
		}

		var botConfig models.BotConfig
		if err := s.db.WithContext(ctx).First(&botConfig, *friend.BotConfigID).Error; err != nil {
			s.logger.Warn("获取Bot配置失败 (ID: %d): %v", *friend.BotConfigID, err)
			continue
		}

		response := &models.UserBotFriendResponse{
			ID:          friend.ID,
			UserID:      friend.UserID,
			BotConfigID: *friend.BotConfigID,
			AppKey:      friend.AppKey,
			Alias:       friend.Alias,
			Priority:    friend.Priority,
			IsActive:    friend.IsActive,
			BotConfig:   botConfig.ToResponse(),
			CreatedAt:   friend.CreatedAt,
			UpdatedAt:   friend.UpdatedAt,
		}

		responses = append(responses, response)
	}

	return responses, nil
}

// UpdateBotFriendPriority 更新Bot好友优先级
func (s *DefaultUserFriendService) UpdateBotFriendPriority(ctx context.Context, userID uint, botConfigID uint, priority int) error {
	result := s.db.WithContext(ctx).Model(&models.UserFriend{}).
		Where("user_id = ? AND bot_config_id = ? AND friend_type = ?", userID, botConfigID, "bot").
		Updates(map[string]interface{}{
			"priority":   priority,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("Bot好友不存在")
	}

	s.logger.Info("用户 %d 更新Bot好友优先级成功 (BotConfigID: %d, Priority: %d)", userID, botConfigID, priority)
	return nil
}

// ToggleBotFriendStatus 切换Bot好友状态
func (s *DefaultUserFriendService) ToggleBotFriendStatus(ctx context.Context, userID uint, botConfigID uint, isActive bool) error {
	result := s.db.WithContext(ctx).Model(&models.UserFriend{}).
		Where("user_id = ? AND bot_config_id = ? AND friend_type = ?", userID, botConfigID, "bot").
		Updates(map[string]interface{}{
			"is_active":  isActive,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("Bot好友不存在")
	}

	status := "禁用"
	if isActive {
		status = "启用"
	}
	s.logger.Info("用户 %d 切换Bot好友状态成功 (BotConfigID: %d, Status: %s)", userID, botConfigID, status)
	return nil
}

// UpdateBotFriendAppKey 更新Bot好友的AppKey
func (s *DefaultUserFriendService) UpdateBotFriendAppKey(ctx context.Context, userID uint, botConfigID uint, appKey string) error {
	result := s.db.WithContext(ctx).Model(&models.UserFriend{}).
		Where("user_id = ? AND bot_config_id = ? AND friend_type = ?", userID, botConfigID, "bot").
		Updates(map[string]interface{}{
			"app_key":    appKey,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("Bot好友不存在")
	}

	s.logger.Info("用户 %d 更新Bot好友AppKey成功 (BotConfigID: %d)", userID, botConfigID)
	return nil
}

// IsBotAdded 检查用户是否已添加Bot
func (s *DefaultUserFriendService) IsBotAdded(ctx context.Context, userID uint, botConfigID uint) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&models.UserFriend{}).
		Where("user_id = ? AND bot_config_id = ? AND friend_type = ?", userID, botConfigID, "bot").
		Count(&count).Error

	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetBotFriendAppKey 获取Bot好友的AppKey
func (s *DefaultUserFriendService) GetBotFriendAppKey(ctx context.Context, userID uint, botConfigID uint) (string, error) {
	var friend models.UserFriend
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND bot_config_id = ? AND friend_type = ?", userID, botConfigID, "bot").
		First(&friend).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", fmt.Errorf("Bot好友不存在")
		}
		return "", err
	}

	return friend.AppKey, nil
}
