package bot

import (
	"context"
	"fmt"
	"time"

	"angrymiao-ai-server/src/core/utils"
	"angrymiao-ai-server/src/models"

	"gorm.io/gorm"
)

// ModelConfigService 模型配置服务接口
type ModelConfigService interface {
	// CRUD操作
	CreateModelConfig(ctx context.Context, config *models.ModelConfig) error
	GetModelConfigByID(ctx context.Context, id uint) (*models.ModelConfig, error)
	UpdateModelConfig(ctx context.Context, config *models.ModelConfig) error
	DeleteModelConfig(ctx context.Context, id uint) error

	// 查询
	ListModelConfigs(ctx context.Context, includePublic bool) ([]*models.ModelConfig, error)
	FindOrCreateModelConfig(ctx context.Context, llmType, modelName, baseURL string) (*models.ModelConfig, error)
}

// DefaultModelConfigService 默认模型配置服务实现
type DefaultModelConfigService struct {
	db     *gorm.DB
	logger *utils.Logger
}

// NewModelConfigService 创建模型配置服务实例
func NewModelConfigService(db *gorm.DB, logger *utils.Logger) ModelConfigService {
	return &DefaultModelConfigService{
		db:     db,
		logger: logger,
	}
}

// CreateModelConfig 创建模型配置
func (s *DefaultModelConfigService) CreateModelConfig(ctx context.Context, config *models.ModelConfig) error {
	config.CreatedAt = time.Now()
	config.UpdatedAt = time.Now()

	if err := s.db.WithContext(ctx).Create(config).Error; err != nil {
		s.logger.Error("创建模型配置失败: %v", err)
		return err
	}

	s.logger.Info("创建模型配置成功: %s/%s (ID: %d)", config.LLMType, config.ModelName, config.ID)
	return nil
}

// GetModelConfigByID 根据ID获取模型配置
func (s *DefaultModelConfigService) GetModelConfigByID(ctx context.Context, id uint) (*models.ModelConfig, error) {
	var config models.ModelConfig
	err := s.db.WithContext(ctx).First(&config, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("模型配置不存在")
		}
		return nil, err
	}
	return &config, nil
}

// UpdateModelConfig 更新模型配置
func (s *DefaultModelConfigService) UpdateModelConfig(ctx context.Context, config *models.ModelConfig) error {
	config.UpdatedAt = time.Now()

	if err := s.db.WithContext(ctx).Save(config).Error; err != nil {
		s.logger.Error("更新模型配置失败: %v", err)
		return err
	}

	s.logger.Info("更新模型配置成功: %s/%s (ID: %d)", config.LLMType, config.ModelName, config.ID)
	return nil
}

// DeleteModelConfig 删除模型配置（软删除）
func (s *DefaultModelConfigService) DeleteModelConfig(ctx context.Context, id uint) error {
	result := s.db.WithContext(ctx).Delete(&models.ModelConfig{}, id)
	if result.Error != nil {
		s.logger.Error("删除模型配置失败: %v", result.Error)
		return result.Error
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("模型配置不存在")
	}

	s.logger.Info("删除模型配置成功 (ID: %d)", id)
	return nil
}

// ListModelConfigs 获取模型配置列表
func (s *DefaultModelConfigService) ListModelConfigs(ctx context.Context, includePublic bool) ([]*models.ModelConfig, error) {
	var configs []*models.ModelConfig
	query := s.db.WithContext(ctx)

	if includePublic {
		query = query.Where("is_public = ?", true)
	}

	err := query.Order("created_at DESC").Find(&configs).Error
	return configs, err
}

// FindOrCreateModelConfig 查找或创建模型配置
func (s *DefaultModelConfigService) FindOrCreateModelConfig(ctx context.Context, llmType, modelName, baseURL string) (*models.ModelConfig, error) {
	var config models.ModelConfig

	// 先尝试查找现有配置
	err := s.db.WithContext(ctx).Where(
		"llm_type = ? AND model_name = ? AND base_url = ?",
		llmType, modelName, baseURL,
	).First(&config).Error

	if err == nil {
		// 找到了，直接返回
		return &config, nil
	}

	if err != gorm.ErrRecordNotFound {
		// 其他错误
		return nil, err
	}

	// 没找到，创建新的
	config = models.ModelConfig{
		LLMType:   llmType,
		ModelName: modelName,
		BaseURL:   baseURL,
		IsPublic:  false,
	}

	if err := s.CreateModelConfig(ctx, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
