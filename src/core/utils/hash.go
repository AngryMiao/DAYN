package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// GenerateUniqueHash 生成唯一的哈希值
// 使用 SHA256(input + timestamp + random) 生成唯一标识
// 参数 input 可以是任意字符串，用于增加哈希的可追溯性
// 返回32个字符的十六进制字符串
func GenerateUniqueHash(input string) (string, error) {
	// 生成16字节随机数
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("生成随机数失败: %v", err)
	}

	// 组合数据：输入 + 纳秒时间戳 + 随机数
	timestamp := time.Now().UnixNano()
	data := fmt.Sprintf("%s:%d:%s", input, timestamp, hex.EncodeToString(randomBytes))

	// 计算 SHA256 哈希
	hash := sha256.Sum256([]byte(data))

	// 返回16字节的十六进制字符串（32个字符）
	return hex.EncodeToString(hash[:16]), nil
}

// GenerateShortHash 生成短哈希值（16个字符）
// 适用于需要更短标识符的场景
func GenerateShortHash(input string) (string, error) {
	// 生成8字节随机数
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("生成随机数失败: %v", err)
	}

	// 组合数据
	timestamp := time.Now().UnixNano()
	data := fmt.Sprintf("%s:%d:%s", input, timestamp, hex.EncodeToString(randomBytes))

	// 计算 SHA256 哈希
	hash := sha256.Sum256([]byte(data))

	// 返回8字节的十六进制字符串（16个字符）
	return hex.EncodeToString(hash[:8]), nil
}
