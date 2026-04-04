package httpx

import "github.com/gin-gonic/gin"

const UnauthorizedCode = "UNAUTHORIZED"

func AbortUnauthorized(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"code":    UnauthorizedCode,
		"message": message,
	})
}
