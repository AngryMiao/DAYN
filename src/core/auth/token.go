package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type AuthToken struct {
	secretKey []byte
	topicRoot string
}

func NewAuthToken(secretKey string) *AuthToken {
	// 添加验证，确保密钥不为空
	if secretKey == "" {
		fmt.Println("Error! secret key cannot be empty")
	}
	return &AuthToken{
		secretKey: []byte(secretKey),
		topicRoot: "am_topic", // 默认值
	}
}

// NewAuthTokenWithConfig 创建带配置的AuthToken
func NewAuthTokenWithConfig(secretKey string, topicRoot string) *AuthToken {
	if secretKey == "" {
		fmt.Println("Error! secret key cannot be empty")
	}
	if topicRoot == "" {
		topicRoot = "am_topic" // 默认值
	}
	return &AuthToken{
		secretKey: []byte(secretKey),
		topicRoot: topicRoot,
	}
}

// GenerateToken 生成JWT token，默认1小时有效期
func (at *AuthToken) GenerateToken(deviceID string) (string, error) {
	return at.GenerateTokenWithExpiry(0, deviceID, time.Hour)
}

// GenerateTokenWithExpiry 生成指定有效期的JWT token（包含MQTT ACL规则）
func (at *AuthToken) GenerateTokenWithExpiry(userID uint, deviceID string, expiry time.Duration) (string, error) {
	// 设置过期时间
	expireTime := time.Now().Add(expiry)

	// 使用deviceID作为username（MQTT需要）
	username := deviceID

	// 构建MQTT ACL规则（使用配置的topicRoot）
	acl := []map[string]any{
		{
			"permission": "allow",
			"action":     "publish",
			"topic":      fmt.Sprintf("%s/%s/+/in", at.topicRoot, username),
		},
		{
			"permission": "allow",
			"action":     "publish",
			"topic":      fmt.Sprintf("%s/%s/status/#", at.topicRoot, username),
		},
		{
			"permission": "allow",
			"action":     "subscribe",
			"topic":      fmt.Sprintf("%s/%s/+/out", at.topicRoot, username),
		},
		{
			"permission": "allow",
			"action":     "subscribe",
			"topic":      fmt.Sprintf("%s/%s/status/#", at.topicRoot, username),
		},
	}

	// 创建claims（包含MQTT所需字段和ACL规则）
	claims := jwt.MapClaims{
		"username":  username, // EMQX需要username字段
		"user_id":   userID,
		"device_id": deviceID,
		"acl":       acl, // MQTT ACL规则
		"exp":       expireTime.Unix(),
		"iat":       time.Now().Unix(),
	}

	// 创建token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// 使用密钥签名
	tokenString, err := token.SignedString(at.secretKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

// 校验设备token
func (at *AuthToken) VerifyToken(tokenString string, ignoreExpiry ...bool) (bool, string, uint, error) {
	if at == nil {
		return false, "", 0, errors.New("AuthToken instance is nil")
	}

	if at.secretKey == nil {
		return false, "", 0, errors.New("secret key is not initialized")
	}

	// 默认需要验证过期时间
	skipExpiry := false
	if len(ignoreExpiry) > 0 {
		skipExpiry = ignoreExpiry[0]
	}

	// 解析token
	parser := jwt.NewParser()
	if skipExpiry {
		parser = jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	}

	token, err := parser.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return at.secretKey, nil
	})
	if err != nil && !skipExpiry {
		return false, "", 0, fmt.Errorf("failed to parse token: %w", err)
	}

	// 如果忽略过期，则只检查签名是否有效
	if skipExpiry {
		if token == nil {
			return false, "", 0, errors.New("failed to parse token")
		}
	} else {
		// 验证token是否有效
		if !token.Valid {
			return false, "", 0, errors.New("invalid token")
		}
	}

	// 获取claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return false, "", 0, errors.New("invalid claims")
	}

	// 获取设备ID
	deviceID, ok := claims["device_id"].(string)
	if !ok {
		return false, "", 0, errors.New("invalid device_id in claims")
	}
	// 获取userID
	userID, ok := claims["user_id"].(float64)
	if !ok {
		return false, "", 0, errors.New("invalid user_id in claims")
	}

	return true, deviceID, uint(userID), nil
}
