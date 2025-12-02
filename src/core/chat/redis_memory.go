package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"angrymiao-ai-server/src/configs"
	"angrymiao-ai-server/src/core/utils"
	"github.com/redis/go-redis/v9"
)

// RedisMemory 使用Redis持久化对话记忆（哈希：key 固定，field=用户ID）
type RedisMemory struct {
	client  *redis.Client
	hashKey string
	field   string
	ctx     context.Context
	logger  *utils.Logger
	ttl     time.Duration
}

// NewRedisMemory 创建Redis记忆存储（哈希模式，按 userID 作为 field）
func NewRedisMemory(cfg configs.RedisConfig, logger *utils.Logger, userID string) (*RedisMemory, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("Redis地址未配置")
	}
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("Redis连接失败: %v", err)
	}

	service := cfg.Service
	if service == "" {
		service = "ai"
	}
	hashKey := fmt.Sprintf("%s:dialogue", service)

	return &RedisMemory{
		client:  client,
		hashKey: hashKey,
		field:   userID,
		ctx:     ctx,
		logger:  logger,
		ttl:     0,
	}, nil
}

// QueryMemory 查询对话记忆（返回JSON字符串）
func (rm *RedisMemory) QueryMemory(_ string) (string, error) {
	val, err := rm.client.HGet(rm.ctx, rm.hashKey, rm.field).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return val, nil
}

// SaveMemory 保存对话记忆（哈希 field=用户ID）
func (rm *RedisMemory) SaveMemory(dialogue []Message) error {
	bytes, err := json.Marshal(dialogue)
	if err != nil {
		return err
	}
	if err := rm.client.HSet(rm.ctx, rm.hashKey, rm.field, bytes).Err(); err != nil {
		return err
	}
	if rm.ttl > 0 {
		_ = rm.client.Expire(rm.ctx, rm.hashKey, rm.ttl).Err()
	}
	return nil
}

// ClearMemory 清空记忆（删除该用户的 field）
func (rm *RedisMemory) ClearMemory() error {
	return rm.client.HDel(rm.ctx, rm.hashKey, rm.field).Err()
}

// QueryMessagesLimit 从Redis获取最近 limit 条消息（基于存储的JSON数组）
// 目前数据以整段JSON存储在哈希字段中，这里读取后进行切片。
func (rm *RedisMemory) QueryMessagesLimit(limit int) ([]Message, error) {
	val, err := rm.client.HGet(rm.ctx, rm.hashKey, rm.field).Result()
	if err == redis.Nil {
		return []Message{}, nil
	}
	if err != nil {
		return nil, err
	}
	if val == "" {
		return []Message{}, nil
	}
	var msgs []Message
	if err := json.Unmarshal([]byte(val), &msgs); err != nil {
		return nil, err
	}
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}
	return msgs, nil
}