package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zplzpl/resume_system/internal/httpx"
	"github.com/zplzpl/resume_system/internal/rbac"
)

const userContextKey = "auth_user"

func RequireAuth(verifier *JWTVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" || token == authHeader {
			httpx.AbortUnauthorized(c, http.StatusUnauthorized, "missing bearer token")
			return
		}

		user, err := verifier.VerifyToken(token)
		if err != nil {
			httpx.AbortUnauthorized(c, http.StatusUnauthorized, "invalid access token")
			return
		}
		c.Set(userContextKey, user)
		c.Next()
	}
}

func RequirePermission(action rbac.Action) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := MustUser(c)
		if user == nil {
			httpx.AbortUnauthorized(c, http.StatusUnauthorized, "missing auth context")
			return
		}
		if !rbac.Can(user.Role, action) {
			httpx.AbortUnauthorized(c, http.StatusForbidden, "insufficient permissions")
			return
		}
		c.Next()
	}
}

func MustUser(c *gin.Context) *User {
	v, ok := c.Get(userContextKey)
	if !ok {
		return nil
	}
	user, _ := v.(*User)
	return user
}
