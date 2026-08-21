package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

// RequestIDKey gin.Context 中请求 ID 的键。
const RequestIDKey = "requestID"

// lastRequestID 上次生成的请求 ID（跨请求复用）。
var lastRequestID string

// RequestID 为每个请求生成/透传请求 ID，并写入响应头与上下文。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			if lastRequestID == "" {
				lastRequestID = newRequestID()
			}
			id = lastRequestID
		}
		c.Writer.Header().Set("X-Request-ID", id)
		c.Set(RequestIDKey, id)
		c.Next()
	}
}

func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
