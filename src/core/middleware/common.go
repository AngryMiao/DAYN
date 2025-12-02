package middleware

import (
	"net/http"
	"strings"

	"angrymiao-ai-server/src/core/auth"
	am_token "angrymiao-ai-server/src/core/auth/am_token"
	"angrymiao-ai-server/src/core/utils"

	"github.com/gin-gonic/gin"
)

// CORS 返回一个统一的跨域中间件
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// 允许所有来源，或者你可以指定特定的来源
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
		} else {
			c.Header("Access-Control-Allow-Origin", "*")
		}

		// 统一允许的头与方法
		c.Header("Access-Control-Allow-Headers", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400") // 24小时

		// 处理 OPTIONS 预检请求
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// AmTokenJWTUserAuth 使用用户JWT进行认证，设置 user_id 到上下文
func AmTokenJWTUserAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "无效的认证token或token已过期"})
			c.Abort()
			return
		}

		token := authHeader[7:]

		claims, err := am_token.ParseToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "token验证失败: " + err.Error()})
			c.Abort()
			return
		}

		c.Set("user_id", uint(claims.UserID))
		c.Set("jwt_claims", claims)
		c.Next()
	}
}

// DeviceTokenAuth 使用设备Token校验，并校验 Device-Id 头与 token 一致
func DeviceTokenAuth(authToken *auth.AuthToken, logger *utils.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "无效的认证token或token已过期"})
			c.Abort()
			return
		}

		token := authHeader[7:]
		isValid, deviceID, userID, err := authToken.VerifyToken(token)
		if err != nil || !isValid {
			if logger != nil {
				logger.Warn("JWTDeviceAuth 验证失败: %v", err)
			}
			c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "无效的认证token或token已过期"})
			c.Abort()
			return
		}

		requestDeviceID := c.GetHeader("Device-Id")
		if requestDeviceID != deviceID {
			if logger != nil {
				logger.Warn("设备ID与token不匹配: 请求=%s, token=%s", requestDeviceID, deviceID)
			}
			c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "设备ID与token不匹配"})
			c.Abort()
			return
		}

		// 与 Vision 保持一致的上下文键
		c.Set("deviceID", deviceID)
		c.Set("userID", userID)
		c.Next()
	}
}
