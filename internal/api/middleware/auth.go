package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// APIKeyAuth checks either Authorization: Bearer <token> or X-API-Key.
func APIKeyAuth(token string) gin.HandlerFunc {
	expected := strings.TrimSpace(token)
	return func(c *gin.Context) {
		if expected == "" {
			c.Next()
			return
		}

		provided := strings.TrimSpace(c.GetHeader("X-API-Key"))
		if provided == "" {
			provided = strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "api authorization token is missing or invalid"})
			return
		}

		c.Next()
	}
}
