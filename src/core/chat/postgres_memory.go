package chat

import (
	"encoding/json"
	"strings"

	"angrymiao-ai-server/src/configs/database"
	"angrymiao-ai-server/src/models"

	"gorm.io/gorm"
)

// PostgresMemory 使用数据库存储用户对话消息（按 userID，每条消息一行）
type PostgresMemory struct {
	db     *gorm.DB
	userID string
}

// NewPostgresMemory 创建 Postgres 记忆存储
func NewPostgresMemory(userID string) *PostgresMemory {
	return &PostgresMemory{db: database.GetDB(), userID: userID}
}

// QueryMemory 查询用户对话记忆（返回JSON字符串）
// 不从数据库存JSON，按行读取并拼装为 []Message，再转为JSON返回
func (m *PostgresMemory) QueryMemory(_ string) (string, error) {
	if m.db == nil {
		return "", nil
	}
	var rows []models.DialogueMessage
	if err := m.db.Where("user_id = ?", m.userID).Order("created_at ASC").Find(&rows).Error; err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}

	messages := make([]Message, 0, len(rows))
	for _, r := range rows {
		messages = append(messages, Message{
			Role:       r.Role,
			Content:    r.Content,
			ToolCallID: r.ToolCallID,
			ToolCalls:  nil, // 不记录 ToolCalls 内容
		})
	}
	bytes, err := json.Marshal(messages)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// QueryMessages 支持分页与排序的查询
// order: "ASC" 或 "DESC"（其他值按 ASC 处理）
func (m *PostgresMemory) QueryMessages(order string, page, pageSize int) ([]Message, int64, error) {
	if m.db == nil {
		return nil, 0, nil
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	// 统计总数
	var total int64
	if err := m.db.Model(&models.DialogueMessage{}).
		Where("user_id = ?", m.userID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 排序
	ord := strings.ToUpper(strings.TrimSpace(order))
	orderBy := "created_at ASC"
	if ord == "DESC" {
		orderBy = "created_at DESC"
	}

	// 分页
	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	var rows []models.DialogueMessage
	if err := m.db.Where("user_id = ?", m.userID).
		Order(orderBy).
		Limit(pageSize).
		Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	// 收集所有非零的 BotID
	botIDs := make([]uint, 0)
	for _, r := range rows {
		if r.BotID > 0 {
			botIDs = append(botIDs, r.BotID)
		}
	}

	// 批量查询 BotConfig 获取 bot 名称
	botNameMap := make(map[uint]string)
	if len(botIDs) > 0 {
		var configs []models.BotConfig
		if err := m.db.Select("id, function_name").
			Where("id IN ?", botIDs).
			Find(&configs).Error; err == nil {
			for _, cfg := range configs {
				botNameMap[cfg.ID] = cfg.FunctionName
			}
		}
	}

	messages := make([]Message, 0, len(rows))
	for _, r := range rows {
		msg := Message{
			Role:       r.Role,
			Content:    r.Content,
			ToolCallID: r.ToolCallID,
			ToolCalls:  nil,
			BotID:      r.BotID,
		}

		// 设置 bot 名称
		if r.BotID == 0 {
			msg.BotName = "AM official"
		} else if name, exists := botNameMap[r.BotID]; exists {
			msg.BotName = name
		} else {
			msg.BotName = "Unknown Bot"
		}

		messages = append(messages, msg)
	}
	return messages, total, nil
}

// SaveMemory 保存用户对话记忆（追加写入，不删除旧记录）
// 仅保存传入的消息切片（通常为单条），不做预查询。
func (m *PostgresMemory) SaveMemory(dialogue []Message) error {
	if m.db == nil {
		return nil
	}
	if len(dialogue) == 0 {
		return nil
	}
	rows := make([]models.DialogueMessage, 0, len(dialogue))
	for _, msg := range dialogue {
		rows = append(rows, models.DialogueMessage{
			UserID:     m.userID,
			Index:      0,
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		})
	}
	return m.db.Create(&rows).Error
}

// ClearMemory 清空用户对话记忆
func (m *PostgresMemory) ClearMemory() error {
	if m.db == nil {
		return nil
	}
	return m.db.Where("user_id = ?", m.userID).Delete(&models.DialogueMessage{}).Error
}

// QueryMessagesLimit 直接获取最近 limit 条消息（limit<=0 返回全部）
func (m *PostgresMemory) QueryMessagesLimit(limit int) ([]Message, error) {
	if m.db == nil {
		return nil, nil
	}
	var rows []models.DialogueMessage
	if limit > 0 {
		// 先按时间倒序拿最近 limit 条
		if err := m.db.Where("user_id = ?", m.userID).
			Order("created_at DESC").
			Limit(limit).
			Find(&rows).Error; err != nil {
			return nil, err
		}
		// 反转为时间正序
		for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
			rows[i], rows[j] = rows[j], rows[i]
		}
	} else {
		// 全量时按时间正序
		if err := m.db.Where("user_id = ?", m.userID).
			Order("created_at ASC").
			Find(&rows).Error; err != nil {
			return nil, err
		}
	}

	// 收集所有非零的 BotID
	botIDs := make([]uint, 0)
	for _, r := range rows {
		if r.BotID > 0 {
			botIDs = append(botIDs, r.BotID)
		}
	}

	// 批量查询 BotConfig 获取 bot 名称
	botNameMap := make(map[uint]string)
	if len(botIDs) > 0 {
		var configs []models.BotConfig
		if err := m.db.Select("id, function_name").
			Where("id IN ?", botIDs).
			Find(&configs).Error; err == nil {
			for _, cfg := range configs {
				botNameMap[cfg.ID] = cfg.FunctionName
			}
		}
	}

	messages := make([]Message, 0, len(rows))
	for _, r := range rows {
		msg := Message{
			Role:       r.Role,
			Content:    r.Content,
			ToolCallID: r.ToolCallID,
			ToolCalls:  nil,
			BotID:      r.BotID,
		}

		// 设置 bot 名称
		if r.BotID == 0 {
			msg.BotName = "AM official"
		} else if name, exists := botNameMap[r.BotID]; exists {
			msg.BotName = name
		} else {
			msg.BotName = "Unknown Bot"
		}

		messages = append(messages, msg)
	}
	return messages, nil
}
