package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zplzpl/resume_system/internal/auth"
	"github.com/zplzpl/resume_system/internal/config"
	"github.com/zplzpl/resume_system/internal/httpx"
	"github.com/zplzpl/resume_system/internal/rbac"
)

type handler struct {
	authClient auth.Client
}

func NewRouter(cfg config.Config) (*gin.Engine, error) {
	if cfg.SupabaseJWTSecret == "" {
		return nil, errors.New("SUPABASE_JWT_SECRET is required")
	}

	h := &handler{
		authClient: auth.NewSupabaseClient(cfg.SupabaseURL, cfg.SupabaseAnonKey),
	}

	verifier, err := auth.NewJWTVerifier(cfg.SupabaseJWTSecret)
	if err != nil {
		return nil, err
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := r.Group("/api/v1")
	authGroup := v1.Group("/auth")
	{
		authGroup.POST("/login", h.login)
		authGroup.POST("/refresh", h.refresh)
		authGroup.POST("/logout", auth.RequireAuth(verifier), h.logout)
	}

	protected := v1.Group("/")
	protected.Use(auth.RequireAuth(verifier))
	{
		protected.GET("/me", h.me)
		protected.GET("/candidates", auth.RequirePermission(rbac.ActionCandidateRead), h.listCandidates)
		protected.POST("/candidates", auth.RequirePermission(rbac.ActionCandidateWrite), h.createCandidate)
		protected.POST("/interviews", auth.RequirePermission(rbac.ActionInterviewManage), h.createInterview)
		protected.GET("/admin/users", auth.RequirePermission(rbac.ActionUserManage), h.listUsers)
	}

	return r, nil
}

func (h *handler) login(c *gin.Context) {
	var req auth.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "invalid request body"})
		return
	}
	session, err := h.authClient.PasswordLogin(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "AUTH_LOGIN_FAILED", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, session)
}

func (h *handler) refresh(c *gin.Context) {
	var req auth.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "invalid request body"})
		return
	}
	session, err := h.authClient.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "AUTH_REFRESH_FAILED", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, session)
}

func (h *handler) logout(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	token, ok := strings.CutPrefix(authHeader, "Bearer ")
	if !ok || token == "" {
		httpx.AbortUnauthorized(c, http.StatusUnauthorized, "missing bearer token")
		return
	}
	if err := h.authClient.Logout(c.Request.Context(), token); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": "AUTH_LOGOUT_FAILED", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *handler) me(c *gin.Context) {
	user := auth.MustUser(c)
	if user == nil {
		httpx.AbortUnauthorized(c, http.StatusUnauthorized, "missing auth context")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user_id":     user.ID,
		"role":        user.Role,
		"permissions": rbac.Permissions(user.Role),
	})
}

func (h *handler) listCandidates(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"items": []gin.H{
			{"id": "cand_001", "name": "Alice"},
		},
	})
}

func (h *handler) createCandidate(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"created": true})
}

func (h *handler) createInterview(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"scheduled": true})
}

func (h *handler) listUsers(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"items": []gin.H{
			{"id": "user_001", "role": rbac.RoleSuperAdmin},
		},
	})
}
