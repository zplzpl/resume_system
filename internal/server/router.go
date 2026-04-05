package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zplzpl/resume_system/internal/auth"
	"github.com/zplzpl/resume_system/internal/config"
	"github.com/zplzpl/resume_system/internal/httpx"
	"github.com/zplzpl/resume_system/internal/interview"
	"github.com/zplzpl/resume_system/internal/rbac"
	"github.com/zplzpl/resume_system/internal/resume"
)

type handler struct {
	authClient   auth.Client
	resumeSvc    *resume.Service
	interviewSvc *interview.Service
}

type createCandidateRequest struct {
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
}

type updateCandidateStatusLayerRequest struct {
	StatusLayer string `json:"status_layer"`
}

type createInterviewRequest struct {
	CandidateID    string   `json:"candidate_id"`
	InterviewerIDs []string `json:"interviewer_ids"`
	StartsAt       string   `json:"starts_at"`
	EndsAt         string   `json:"ends_at"`
	Round          string   `json:"round"`
	Note           string   `json:"note"`
}

type updateInterviewRequest struct {
	CandidateID    *string   `json:"candidate_id"`
	InterviewerIDs *[]string `json:"interviewer_ids"`
	StartsAt       *string   `json:"starts_at"`
	EndsAt         *string   `json:"ends_at"`
	Round          *string   `json:"round"`
	Status         *string   `json:"status"`
	Note           *string   `json:"note"`
}

type submitCandidateResponseRequest struct {
	Action           string `json:"action"`
	ProposedStartsAt string `json:"proposed_starts_at"`
	ProposedEndsAt   string `json:"proposed_ends_at"`
	Note             string `json:"note"`
}

type reviewRescheduleRequest struct {
	Decision string `json:"decision"`
	Note     string `json:"note"`
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
		authClient:   auth.NewSupabaseClient(cfg.SupabaseURL, cfg.SupabaseAnonKey),
		resumeSvc:    resume.NewService(resume.NewMemoryRepository(), storage, resume.NewHeuristicParser()),
		interviewSvc: interview.NewService(interview.NewMemoryRepository()),
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
	v1.POST("/interview-responses/:token", h.submitCandidateResponse)
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
		protected.PATCH("/interviews/:id", auth.RequirePermission(rbac.ActionInterviewManage), h.updateInterview)
		protected.GET("/interviews/calendar", auth.RequirePermission(rbac.ActionInterviewManage), h.getInterviewCalendar)
		protected.POST("/interviews/:id/reschedule-review", auth.RequirePermission(rbac.ActionInterviewManage), h.reviewInterviewReschedule)
		protected.GET("/interviews/:id/process-records", auth.RequirePermission(rbac.ActionInterviewManage), h.listInterviewProcessRecords)
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
	var req createInterviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "invalid request body"})
		return
	}

	if !h.resumeSvc.CandidateExists(req.CandidateID) {
		c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": "candidate not found"})
		return
	}

	startsAt, err := time.Parse(time.RFC3339, strings.TrimSpace(req.StartsAt))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "starts_at must be RFC3339"})
		return
	}
	endsAt, err := time.Parse(time.RFC3339, strings.TrimSpace(req.EndsAt))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "ends_at must be RFC3339"})
		return
	}

	user := auth.MustUser(c)
	createdBy := ""
	if user != nil {
		createdBy = user.ID
	}

	result, err := h.interviewSvc.Create(interview.CreateRequest{
		CandidateID:    req.CandidateID,
		InterviewerIDs: req.InterviewerIDs,
		StartsAt:       startsAt,
		EndsAt:         endsAt,
		Round:          req.Round,
		Note:           req.Note,
		CreatedBy:      createdBy,
	})
	if err != nil {
		switch {
		case interview.IsConflictError(err):
			conflictErr := interview.ExtractConflictError(err)
			c.JSON(http.StatusConflict, gin.H{
				"code":      "SCHEDULE_CONFLICT",
				"message":   err.Error(),
				"conflicts": conflictErr.Conflicts,
			})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": err.Error()})
		}
		return
	}

	candidate, statusErr := h.resumeSvc.UpdateCandidateStatusLayer(result.Interview.CandidateID, string(resume.CandidateStatusInterview))
	if statusErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "STATUS_LINK_FAILED", "message": statusErr.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"scheduled":              true,
		"interview":              result.Interview,
		"notifications":          result.Notifications,
		"notifications_enqueued": result.NotificationsEnqueued,
		"candidate":              candidate,
	})
}

func (h *handler) updateInterview(c *gin.Context) {
	var req updateInterviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "invalid request body"})
		return
	}

	if req.CandidateID != nil && !h.resumeSvc.CandidateExists(*req.CandidateID) {
		c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": "candidate not found"})
		return
	}

	updateReq := interview.UpdateRequest{
		CandidateID:    req.CandidateID,
		InterviewerIDs: req.InterviewerIDs,
		Round:          req.Round,
		Status:         req.Status,
		Note:           req.Note,
	}

	if req.StartsAt != nil {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.StartsAt))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "starts_at must be RFC3339"})
			return
		}
		updateReq.StartsAt = &parsed
	}
	if req.EndsAt != nil {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.EndsAt))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "ends_at must be RFC3339"})
			return
		}
		updateReq.EndsAt = &parsed
	}

	result, err := h.interviewSvc.Update(c.Param("id"), updateReq)
	if err != nil {
		switch {
		case interview.IsInterviewNotFound(err):
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": err.Error()})
		case interview.IsConflictError(err):
			conflictErr := interview.ExtractConflictError(err)
			c.JSON(http.StatusConflict, gin.H{
				"code":      "SCHEDULE_CONFLICT",
				"message":   err.Error(),
				"conflicts": conflictErr.Conflicts,
			})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": err.Error()})
		}
		return
	}

	candidate, statusErr := h.resumeSvc.UpdateCandidateStatusLayer(result.Interview.CandidateID, string(resume.CandidateStatusInterview))
	if statusErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "STATUS_LINK_FAILED", "message": statusErr.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"updated":                true,
		"interview":              result.Interview,
		"notifications":          result.Notifications,
		"notifications_enqueued": result.NotificationsEnqueued,
		"candidate":              candidate,
	})
}

func (h *handler) getInterviewCalendar(c *gin.Context) {
	view, err := interview.ParseCalendarView(c.Query("view"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": err.Error()})
		return
	}

	anchor, err := parseCalendarAnchor(c.Query("date"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"calendar": h.interviewSvc.Calendar(view, anchor),
	})
}

func (h *handler) submitCandidateResponse(c *gin.Context) {
	var req submitCandidateResponseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "invalid request body"})
		return
	}

	svcReq := interview.CandidateResponseRequest{
		Action: req.Action,
		Note:   req.Note,
	}
	if raw := strings.TrimSpace(req.ProposedStartsAt); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "proposed_starts_at must be RFC3339"})
			return
		}
		svcReq.ProposedStartsAt = &parsed
	}
	if raw := strings.TrimSpace(req.ProposedEndsAt); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "proposed_ends_at must be RFC3339"})
			return
		}
		svcReq.ProposedEndsAt = &parsed
	}

	result, err := h.interviewSvc.SubmitCandidateResponse(c.Param("token"), svcReq)
	if err != nil {
		switch {
		case interview.IsCandidateTokenNotFound(err):
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"submitted":              true,
		"interview":              result.Interview,
		"reschedule_request":     result.RescheduleRequest,
		"notifications":          result.Notifications,
		"notifications_enqueued": result.NotificationsEnqueued,
		"process_records":        result.ProcessRecords,
	})
}

func (h *handler) reviewInterviewReschedule(c *gin.Context) {
	var req reviewRescheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "invalid request body"})
		return
	}

	user := auth.MustUser(c)
	processedBy := ""
	if user != nil {
		processedBy = user.ID
	}

	result, err := h.interviewSvc.ReviewReschedule(c.Param("id"), interview.ReviewRescheduleRequest{
		Decision:    req.Decision,
		ProcessedBy: processedBy,
		Note:        req.Note,
	})
	if err != nil {
		switch {
		case interview.IsInterviewNotFound(err):
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": err.Error()})
		case interview.IsNoPendingRescheduleReview(err):
			c.JSON(http.StatusConflict, gin.H{"code": "NO_PENDING_REVIEW", "message": err.Error()})
		case interview.IsConflictError(err):
			conflictErr := interview.ExtractConflictError(err)
			c.JSON(http.StatusConflict, gin.H{
				"code":      "SCHEDULE_CONFLICT",
				"message":   err.Error(),
				"conflicts": conflictErr.Conflicts,
			})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": err.Error()})
		}
		return
	}

	candidate, statusErr := h.resumeSvc.UpdateCandidateStatusLayer(result.Interview.CandidateID, string(resume.CandidateStatusInterview))
	if statusErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "STATUS_LINK_FAILED", "message": statusErr.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"reviewed":               true,
		"interview":              result.Interview,
		"reschedule_request":     result.RescheduleRequest,
		"notifications":          result.Notifications,
		"notifications_enqueued": result.NotificationsEnqueued,
		"process_records":        result.ProcessRecords,
		"candidate":              candidate,
	})
}

func (h *handler) listInterviewProcessRecords(c *gin.Context) {
	records, err := h.interviewSvc.ProcessRecords(c.Param("id"))
	if err != nil {
		switch {
		case interview.IsInterviewNotFound(err):
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": records,
	})
}

func parseCalendarAnchor(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Now().UTC(), nil
	}

	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("date must be YYYY-MM-DD or RFC3339")
}

func (h *handler) listUsers(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"items": []gin.H{
			{"id": "user_001", "role": rbac.RoleSuperAdmin},
		},
	})
}
