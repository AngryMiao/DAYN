package chat

import (
	"encoding/json"
	"strings"

	"angrymiao-ai-server/src/core/types"
	"angrymiao-ai-server/src/core/utils"
)

type Message = types.Message

// DialogueManager 管理对话上下文和历史
type DialogueManager struct {
	logger   *utils.Logger
	dialogue []Message
	memory   MemoryInterface
}

// NewDialogueManager 创建对话管理器实例
func NewDialogueManager(logger *utils.Logger, memory MemoryInterface) *DialogueManager {
	return &DialogueManager{
		logger:   logger,
		dialogue: make([]Message, 0),
		memory:   memory,
	}
}

func (dm *DialogueManager) SetSystemMessage(systemMessage string) {
	if systemMessage == "" {
		return
	}

	// 如果对话中已经有系统消息，则更新其内容
	if len(dm.dialogue) > 0 && dm.dialogue[0].Role == "system" {
		dm.dialogue[0].Content = systemMessage
		return
	}

	// 添加新的系统消息到对话开头（系统消息不落库）
	dm.dialogue = append([]Message{
		{Role: "system", Content: systemMessage},
	}, dm.dialogue...)
}

func (dm *DialogueManager) RemoveSecondMessageForToolType() {
	// 如果第二条的类型是"role": "tool",则移除这条
	if len(dm.dialogue) < 2 || dm.dialogue[1].Role != "tool" {
		return
	}
	dm.dialogue = append(dm.dialogue[:1], dm.dialogue[2:]...)
}

// 保留最近的几条对话消息
func (dm *DialogueManager) KeepRecentMessages(maxMessages int) {
	if maxMessages <= 0 || len(dm.dialogue) <= maxMessages {
		return
	}
	// 保留system消息和最近的 maxMessages 条消息
	if len(dm.dialogue) > 0 && dm.dialogue[0].Role == "system" {
		// 保留system消息
		dm.dialogue = append(dm.dialogue[:1], dm.dialogue[len(dm.dialogue)-maxMessages:]...)
		dm.RemoveSecondMessageForToolType()
		return
	}
	// 如果没有system消息，直接保留最近的 maxMessages 条消息
	if len(dm.dialogue) > maxMessages {
		dm.dialogue = dm.dialogue[len(dm.dialogue)-maxMessages:]
	}
}

// GetRecentMessages 获取最近的对话消息
// 如果 maxMessages <= 0，则返回全部对话消息
func (dm *DialogueManager) GetRecentMessages(maxMessages int) []Message {
	if maxMessages <= 0 || len(dm.dialogue) <= maxMessages {
		return dm.dialogue
	}
	// 保留system消息和最近的 maxMessages 条消息
	if len(dm.dialogue) > 0 && dm.dialogue[0].Role == "system" {
		// 保留system消息
		return append([]Message{dm.dialogue[0]}, dm.dialogue[len(dm.dialogue)-maxMessages:]...)
	}
	return dm.dialogue
}

// Put 添加新消息到对话
func (dm *DialogueManager) Put(message Message) {
	dm.dialogue = append(dm.dialogue, message)

	// 仅在非system且内容非空时持久化追加保存
	if dm.memory != nil {
		if (message.Role == "user" || message.Role == "assistant") && strings.TrimSpace(message.Content) != "" {
			if err := dm.memory.SaveMemory([]Message{message}); err != nil {
				dm.logger.Warn("保存对话失败: %v", err)
			}
		}
	}
}

func (dm *DialogueManager) GetLastTwoMessages() []Message {
	if len(dm.dialogue) < 2 {
		return nil
	}
	return dm.dialogue[len(dm.dialogue)-2:]
}

// GetLLMDialogue 获取完整对话历史
func (dm *DialogueManager) GetLLMDialogue() []Message {
	return dm.dialogue
}

// LoadFromJSON 用JSON字符串覆盖加载对话（保留现有system消息）
func (dm *DialogueManager) LoadFromJSON(jsonStr string) error {
	if strings.TrimSpace(jsonStr) == "" {
		return nil
	}
	var msgs []Message
	if err := json.Unmarshal([]byte(jsonStr), &msgs); err != nil {
		return err
	}
	// 保留已有的 system 消息（若存在且位于首位）
	if len(dm.dialogue) > 0 && dm.dialogue[0].Role == "system" {
		dm.dialogue = append([]Message{dm.dialogue[0]}, msgs...)
	} else {
		dm.dialogue = msgs
	}
	return nil
}

// LoadFromStorage 从持久化存储加载对话到内存（覆盖当前非system内容）
func (dm *DialogueManager) LoadFromStorage() error {
	if dm.memory == nil {
		return nil
	}
	jsonStr, err := dm.memory.QueryMemory("")
	if err != nil {
		return err
	}
	return dm.LoadFromJSON(jsonStr)
}

// GetStoredDialogue 直接从存储读取并返回对话（不改变内存状态）
// limit<=0 表示获取全部；>0 表示仅返回存储中的最近 limit 条消息
func (dm *DialogueManager) GetStoredDialogue(limit int) ([]Message, error) {
	if dm.memory == nil {
		return dm.GetLLMDialogue(), nil
	}
	msgs, err := dm.memory.QueryMessagesLimit(limit)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return dm.GetLLMDialogue(), nil
	}
	// 若当前内存首条是 system，则在返回结果前加上
	if len(dm.dialogue) > 0 && dm.dialogue[0].Role == "system" {
		return append([]Message{dm.dialogue[0]}, msgs...), nil
	}
	return msgs, nil
}

// GetLLMDialogueWithMemory 获取带记忆的对话
func (dm *DialogueManager) GetLLMDialogueWithMemory(memoryStr string) []Message {
	if memoryStr == "" {
		return dm.GetLLMDialogue()
	}

	memoryMsg := Message{
		Role:    "system",
		Content: memoryStr,
	}

	dialogue := make([]Message, 0, len(dm.dialogue)+1)
	dialogue = append(dialogue, memoryMsg)
	dialogue = append(dialogue, dm.dialogue...)

	return dialogue
}

// Clear 清空对话历史
func (dm *DialogueManager) Clear() {
	dm.dialogue = make([]Message, 0)
	if dm.memory != nil {
		if err := dm.memory.ClearMemory(); err != nil {
			dm.logger.Warn("清空记忆失败: %v", err)
		}
	}
}

func (dm *DialogueManager) Length() int {
	return len(dm.dialogue)
}

// ToJSON 将对话历史转换为JSON字符串
func (dm *DialogueManager) ToJSON(keepSystemPrompt bool) (string, error) {
	dialogue := dm.dialogue
	if !keepSystemPrompt && len(dialogue) > 0 && dialogue[0].Role == "system" {
		// 如果不保留系统消息，则移除第一条消息
		dialogue = dialogue[1:]
	}
	bytes, err := json.Marshal(dialogue)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
