package mqtt

import (
	"crypto/aes"
	"encoding/hex"
	"fmt"
	"time"
)

// NewUDPSession 创建新的UDP会话
func NewUDPSession(deviceID, sessionID string, aesKey [16]byte, nonce [8]byte, connID string) (*UDPSession, error) {
	// 创建AES cipher block
	block, err := aes.NewCipher(aesKey[:])
	if err != nil {
		return nil, fmt.Errorf("创建AES cipher失败: %v", err)
	}

	session := &UDPSession{
		ID:          fmt.Sprintf("%s:%s", deviceID, sessionID),
		ConnID:      connID,
		DeviceID:    deviceID,
		SessionID:   sessionID,
		AESKey:      aesKey,
		Nonce:       nonce,
		Block:       block,
		RecvChannel: make(chan []byte, 100), // 缓冲区大小100
		SendChannel: make(chan []byte, 100),
		CreatedAt:   time.Now(),
		LastActive:  time.Now(),
		Status:      "active",
		LocalSeq:    0,
		RemoteSeq:   0,
	}

	return session, nil
}

// GetAESKeyAndNonce 返回hex格式的密钥和完整nonce（16字节）
// 客户端会从返回的16字节nonce中提取[4:12]位置的8字节作为模板
func (s *UDPSession) GetAESKeyAndNonce() (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	keyHex := hex.EncodeToString(s.AESKey[:])

	// 构造16字节完整nonce，格式与demo一致：
	// [0x01, 0x00][0x00, 0x00][8字节nonce模板][0x00, 0x00, 0x00, 0x00]
	fullNonce := make([]byte, 16)
	fullNonce[0] = 0x01               // 包类型
	fullNonce[1] = 0x00               // 保留
	fullNonce[2] = 0x00               // 长度高字节
	fullNonce[3] = 0x00               // 长度低字节
	copy(fullNonce[4:12], s.Nonce[:]) // 8字节nonce模板
	fullNonce[12] = 0x00              // 序列号（初始为0）
	fullNonce[13] = 0x00
	fullNonce[14] = 0x00
	fullNonce[15] = 0x00

	nonceHex := hex.EncodeToString(fullNonce)

	return keyHex, nonceHex
}

// Encrypt 加密数据并返回完整的UDP数据包（nonce + 加密数据）
func (s *UDPSession) Encrypt(data []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Status != "active" {
		return nil, fmt.Errorf("会话已关闭")
	}

	// 递增序列号
	s.LocalSeq++

	// 生成完整nonce
	fullNonce := s.generateNonce(len(data), s.LocalSeq)

	// 使用AES-CTR加密
	encrypted, err := EncryptAESCTR(fullNonce, s.AESKey[:], data)
	if err != nil {
		return nil, fmt.Errorf("加密失败: %v", err)
	}

	// 组装UDP数据包: [16字节nonce][加密数据]
	packet := make([]byte, 16+len(encrypted))
	copy(packet[0:16], fullNonce)
	copy(packet[16:], encrypted)

	s.LastActive = time.Now()
	return packet, nil
}

// Decrypt 解密UDP数据包（提取nonce、验证序列号、解密数据）
func (s *UDPSession) Decrypt(packet []byte) ([]byte, error) {
	if len(packet) < 16 {
		return nil, fmt.Errorf("数据包长度不足16字节")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Status != "active" {
		return nil, fmt.Errorf("会话已关闭")
	}

	// 提取nonce和加密数据
	nonce := packet[0:16]
	encrypted := packet[16:]

	// 提取nonce信息
	_, seq, dataLen, err := ExtractNonceInfo(nonce)
	if err != nil {
		return nil, fmt.Errorf("解析nonce失败: %v", err)
	}

	// 验证数据长度
	if int(dataLen) != len(encrypted) {
		return nil, fmt.Errorf("数据长度不匹配: 期望%d, 实际%d", dataLen, len(encrypted))
	}

	// 验证序列号（防止重放攻击，但允许第一个包）
	// 使用 < 而不是 <= 以允许序列号从0或1开始
	if s.RemoteSeq > 0 && seq < s.RemoteSeq {
		return nil, fmt.Errorf("序列号无效: 期望>=%d, 实际%d", s.RemoteSeq, seq)
	}
	s.RemoteSeq = seq

	// 解密数据
	decrypted, err := DecryptAESCTR(nonce, s.AESKey[:], encrypted)
	if err != nil {
		return nil, fmt.Errorf("解密失败: %v", err)
	}

	s.LastActive = time.Now()
	return decrypted, nil
}

// generateNonce 生成16字节完整nonce
// 格式: [type(1B)][reserved(1B)][length(2B)][connID(4B)][timestamp(4B)][seq(4B)]
func (s *UDPSession) generateNonce(dataLen int, seq uint32) []byte {
	return BuildFullNonce(s.Nonce, dataLen, seq)
}

// SendAudioData 非阻塞发送音频数据到SendChannel
func (s *UDPSession) SendAudioData(data []byte) (bool, error) {
	if s.Status != "active" {
		return false, fmt.Errorf("会话已关闭")
	}

	select {
	case s.SendChannel <- data:
		return true, nil
	default:
		// 通道满，丢弃数据
		return false, fmt.Errorf("发送通道已满")
	}
}

// RecvData 非阻塞接收数据到RecvChannel
func (s *UDPSession) RecvData(data []byte) (bool, error) {
	if s.Status != "active" {
		return false, fmt.Errorf("会话已关闭")
	}

	select {
	case s.RecvChannel <- data:
		return true, nil
	default:
		// 通道满，丢弃数据
		return false, fmt.Errorf("接收通道已满")
	}
}

// Destroy 销毁会话，关闭通道
func (s *UDPSession) Destroy() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Status == "closed" {
		return
	}

	s.Status = "closed"
	close(s.RecvChannel)
	close(s.SendChannel)
}

// IsActive 检查会话是否活跃
func (s *UDPSession) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Status == "active"
}

// GetLastActiveTime 获取最后活跃时间
func (s *UDPSession) GetLastActiveTime() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.LastActive
}
