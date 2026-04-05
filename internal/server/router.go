package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zplzpl/resume_system/internal/auth"
	"github.com/zplzpl/resume_system/internal/config"
	"github.com/zplzpl/resume_system/internal/httpx"
	"github.com/zplzpl/resume_system/internal/rbac"
	"github.com/zplzpl/resume_system/internal/resume"
)

type handler struct {
	authClient auth.Client
	resumeSvc  *resume.Service
}

type createCandidateRequest struct {
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
}

type updateCandidateStatusLayerRequest struct {
	StatusLayer string `json:"status_layer"`
}

func NewRouter(cfg config.Config) (*gin.Engine, error) {
	if cfg.SupabaseJWTSecret == "" {
		return nil, errors.New("SUPABASE_JWT_SECRET is required")
	}

	storage, err := resume.NewLocalStorage(cfg.ResumeStorageDir)
	if err != nil {
		return nil, fmt.Errorf("init resume storage: %w", err)
	}

	h := &handler{
		authClient: auth.NewSupabaseClient(cfg.SupabaseURL, cfg.SupabaseAnonKey),
		resumeSvc:  resume.NewService(resume.NewMemoryRepository(), storage, resume.NewHeuristicParser()),
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
		protected.PATCH("/candidates/:id/status-layer", auth.RequirePermission(rbac.ActionCandidateWrite), h.updateCandidateStatusLayer)
		protected.POST("/resumes/upload", auth.RequirePermission(rbac.ActionCandidateWrite), h.uploadResume)
		protected.POST("/resumes/upload/batch", auth.RequirePermission(rbac.ActionCandidateWrite), h.uploadResumeBatch)
		protected.GET("/resumes/:id", auth.RequirePermission(rbac.ActionCandidateRead), h.getResume)
		protected.POST("/resumes/:id/retry", auth.RequirePermission(rbac.ActionCandidateWrite), h.retryResume)
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
	statusParams := append([]string{}, c.QueryArray("status_layer")...)
	statusParams = append(statusParams, c.QueryArray("status")...)
	statusList, err := resume.ParseCandidateStatusLayers(statusParams)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": err.Error()})
		return
	}

	options := resume.CandidateSearchOptions{
		Keyword:    strings.TrimSpace(c.Query("keyword")),
		Skill:      strings.TrimSpace(c.Query("skill")),
		Company:    strings.TrimSpace(c.Query("company")),
		School:     strings.TrimSpace(c.Query("school")),
		StatusList: statusList,
	}
	c.JSON(http.StatusOK, gin.H{"items": h.resumeSvc.SearchCandidates(options)})
}

func (h *handler) createCandidate(c *gin.Context) {
	var req createCandidateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "invalid request body"})
		return
	}

	candidate := h.resumeSvc.CreateManualCandidate(req.FullName, req.Email, req.Phone)
	c.JSON(http.StatusOK, gin.H{"created": true, "candidate": candidate})
}

func (h *handler) updateCandidateStatusLayer(c *gin.Context) {
	var req updateCandidateStatusLayerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "invalid request body"})
		return
	}

	candidate, err := h.resumeSvc.UpdateCandidateStatusLayer(c.Param("id"), req.StatusLayer)
	if err != nil {
		switch {
		case resume.IsCandidateNotFound(err):
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": err.Error()})
		case resume.IsInvalidStatusLayer(err):
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"updated": true, "candidate": candidate})
}

func (h *handler) uploadResume(c *gin.Context) {
	user := auth.MustUser(c)
	if user == nil {
		httpx.AbortUnauthorized(c, http.StatusUnauthorized, "missing auth context")
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "file is required"})
		return
	}

	f, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "UPLOAD_OPEN_FAILED", "message": err.Error()})
		return
	}
	defer f.Close()

	candidateID := strings.TrimSpace(c.PostForm("candidate_id"))
	result, err := h.resumeSvc.Upload(fileHeader.Filename, f, user.ID, candidateID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "UPLOAD_FAILED", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *handler) uploadResumeBatch(c *gin.Context) {
	user := auth.MustUser(c)
	if user == nil {
		httpx.AbortUnauthorized(c, http.StatusUnauthorized, "missing auth context")
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "invalid multipart form"})
		return
	}

	files := form.File["files"]
	if len(files) == 0 {
		files = form.File["file"]
	}
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "at least one file is required"})
		return
	}

	candidateID := strings.TrimSpace(c.PostForm("candidate_id"))
	items := make([]gin.H, 0, len(files))
	success := 0
	failed := 0

	for idx, fileHeader := range files {
		f, openErr := fileHeader.Open()
		if openErr != nil {
			failed++
			items = append(items, gin.H{
				"index":     idx,
				"file_name": fileHeader.Filename,
				"error":     openErr.Error(),
			})
			continue
		}

		result, uploadErr := h.resumeSvc.Upload(fileHeader.Filename, f, user.ID, candidateID)
		_ = f.Close()
		if uploadErr != nil {
			failed++
			items = append(items, gin.H{
				"index":     idx,
				"file_name": fileHeader.Filename,
				"error":     uploadErr.Error(),
			})
			continue
		}

		if result.Resume.ParseStatus == resume.ParseStatusSuccess {
			success++
		} else {
			failed++
		}

		items = append(items, gin.H{
			"index":     idx,
			"file_name": fileHeader.Filename,
			"resume":    result.Resume,
			"candidate": result.Candidate,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"items": items,
		"summary": gin.H{
			"total":   len(files),
			"success": success,
			"failed":  failed,
		},
	})
}

func (h *handler) getResume(c *gin.Context) {
	result, ok := h.resumeSvc.GetResume(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": "resume not found"})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *handler) retryResume(c *gin.Context) {
	result, err := h.resumeSvc.Retry(c.Param("id"))
	if err != nil {
		message := err.Error()
		switch {
		case strings.Contains(message, "not found"):
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": message})
		case strings.Contains(message, "not in failed status"):
			c.JSON(http.StatusConflict, gin.H{"code": "INVALID_STATUS", "message": message})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": message})
		}
		return
	}
	c.JSON(http.StatusOK, result)
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
