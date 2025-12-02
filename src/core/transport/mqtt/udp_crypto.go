package mqtt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

// GenerateAESKey 生成16字节的AES-128密钥
func GenerateAESKey() ([16]byte, error) {
	var key [16]byte
	if _, err := rand.Read(key[:]); err != nil {
		return key, fmt.Errorf("生成AES密钥失败: %v", err)
	}
	return key, nil
}

// GenerateConnID 生成4字节的连接ID
func GenerateConnID() ([4]byte, error) {
	var connID [4]byte
	if _, err := rand.Read(connID[:]); err != nil {
		return connID, fmt.Errorf("生成连接ID失败: %v", err)
	}
	return connID, nil
}

// GenerateNonceTemplate 生成8字节的nonce模板（connID 4字节 + timestamp 4字节）
func GenerateNonceTemplate(connID [4]byte) [8]byte {
	var nonce [8]byte
	copy(nonce[0:4], connID[:])
	// timestamp使用当前时间戳的低4字节
	timestamp := uint32(getCurrentTimestamp())
	binary.BigEndian.PutUint32(nonce[4:8], timestamp)
	return nonce
}

// getCurrentTimestamp 获取当前时间戳（秒）
func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}

// EncryptAESCTR 使用AES-CTR模式加密数据
// nonce: 16字节完整nonce
// key: 16字节AES密钥
// plaintext: 待加密的明文
func EncryptAESCTR(nonce []byte, key []byte, plaintext []byte) ([]byte, error) {
	if len(nonce) != 16 {
		return nil, fmt.Errorf("nonce长度必须为16字节")
	}
	if len(key) != 16 {
		return nil, fmt.Errorf("密钥长度必须为16字节")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建AES cipher失败: %v", err)
	}

	ciphertext := make([]byte, len(plaintext))
	stream := cipher.NewCTR(block, nonce)
	stream.XORKeyStream(ciphertext, plaintext)

	return ciphertext, nil
}

// DecryptAESCTR 使用AES-CTR模式解密数据
// nonce: 16字节完整nonce
// key: 16字节AES密钥
// ciphertext: 待解密的密文
func DecryptAESCTR(nonce []byte, key []byte, ciphertext []byte) ([]byte, error) {
	// AES-CTR模式下，加密和解密是相同的操作
	return EncryptAESCTR(nonce, key, ciphertext)
}

// BuildFullNonce 构建16字节完整nonce
// 格式: [type(1B)][reserved(1B)][length(2B)][connID(4B)][timestamp(4B)][seq(4B)]
func BuildFullNonce(nonceTemplate [8]byte, dataLen int, seq uint32) []byte {
	nonce := make([]byte, 16)
	nonce[0] = 0x01 // 包类型
	nonce[1] = 0x00 // 保留
	binary.BigEndian.PutUint16(nonce[2:4], uint16(dataLen))
	copy(nonce[4:12], nonceTemplate[:]) // connID(4B) + timestamp(4B)
	binary.BigEndian.PutUint32(nonce[12:16], seq)
	return nonce
}

// ExtractNonceInfo 从16字节nonce中提取信息
func ExtractNonceInfo(nonce []byte) (connID []byte, seq uint32, dataLen uint16, err error) {
	if len(nonce) < 16 {
		return nil, 0, 0, fmt.Errorf("nonce长度不足16字节")
	}

	dataLen = binary.BigEndian.Uint16(nonce[2:4])
	connID = nonce[4:8] // 只取connID部分（4字节）
	seq = binary.BigEndian.Uint32(nonce[12:16])

	return connID, seq, dataLen, nil
}
