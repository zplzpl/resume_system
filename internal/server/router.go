package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zplzpl/resume_system/internal/audit"
	"github.com/zplzpl/resume_system/internal/auth"
	"github.com/zplzpl/resume_system/internal/config"
	"github.com/zplzpl/resume_system/internal/httpx"
	"github.com/zplzpl/resume_system/internal/interview"
	"github.com/zplzpl/resume_system/internal/rbac"
	"github.com/zplzpl/resume_system/internal/report"
	"github.com/zplzpl/resume_system/internal/resume"
)

type handler struct {
	authClient   auth.Client
	auditSvc     *audit.Service
	resumeSvc    *resume.Service
	interviewSvc *interview.Service
	reportSvc    *report.Service
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

type capabilityScorePayload struct {
	Dimension string `json:"dimension"`
	Score     int    `json:"score"`
	Comment   string `json:"comment"`
}

type submitInterviewEvaluationRequest struct {
	CapabilityScores []capabilityScorePayload `json:"capability_scores"`
	OverallComment   string                   `json:"overall_comment"`
	Conclusion       string                   `json:"conclusion"`
}

type generateInterviewQuestionRecommendationRequest struct {
	JobTitle       string `json:"job_title"`
	JobDescription string `json:"job_description"`
}

type startInterviewTranscriptSessionRequest struct {
	Provider string `json:"provider"`
}

type appendInterviewTranscriptRequest struct {
	SessionID   string  `json:"session_id"`
	SpeakerRole string  `json:"speaker_role"`
	SpeakerID   string  `json:"speaker_id"`
	Text        string  `json:"text"`
	IsFinal     bool    `json:"is_final"`
	StartedAt   *string `json:"started_at"`
	EndedAt     *string `json:"ended_at"`
}

type markInterviewTranscriptInterruptedRequest struct {
	Reason string `json:"reason"`
}

func NewRouter(cfg config.Config) (*gin.Engine, error) {
	return newRouterWithAuthClient(cfg, auth.NewSupabaseClient(cfg.SupabaseURL, cfg.SupabaseAnonKey))
}

func newRouterWithAuthClient(cfg config.Config, authClient auth.Client) (*gin.Engine, error) {
	if cfg.SupabaseJWTSecret == "" {
		return nil, errors.New("SUPABASE_JWT_SECRET is required")
	}

	storage, err := resume.NewLocalStorage(cfg.ResumeStorageDir)
	if err != nil {
		return nil, fmt.Errorf("init resume storage: %w", err)
	}

	h := &handler{
		authClient:   authClient,
		auditSvc:     audit.NewService(audit.NewMemoryRepository()),
		resumeSvc:    resume.NewService(resume.NewMemoryRepository(), storage, resume.NewHeuristicParser()),
		interviewSvc: interview.NewService(interview.NewMemoryRepository()),
		reportSvc:    report.NewService(nil),
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
		protected.DELETE("/resumes/:id", auth.RequirePermission(rbac.ActionCandidateWrite), h.deleteResume)
		protected.POST("/resumes/:id/retry", auth.RequirePermission(rbac.ActionCandidateWrite), h.retryResume)
		protected.POST("/interviews", auth.RequirePermission(rbac.ActionInterviewManage), h.createInterview)
		protected.PATCH("/interviews/:id", auth.RequirePermission(rbac.ActionInterviewManage), h.updateInterview)
		protected.GET("/interviews/calendar", auth.RequirePermission(rbac.ActionInterviewManage), h.getInterviewCalendar)
		protected.POST("/interviews/:id/transcriptions/sessions", auth.RequirePermission(rbac.ActionInterviewManage), h.startInterviewTranscriptSession)
		protected.POST("/interviews/:id/transcriptions", auth.RequirePermission(rbac.ActionInterviewManage), h.appendInterviewTranscript)
		protected.GET("/interviews/:id/transcriptions", auth.RequirePermission(rbac.ActionInterviewManage), h.getInterviewTranscriptions)
		protected.POST("/interviews/:id/transcriptions/sessions/:sessionID/interrupted", auth.RequirePermission(rbac.ActionInterviewManage), h.markInterviewTranscriptInterrupted)
		protected.POST("/interviews/:id/transcriptions/sessions/:sessionID/reconnect", auth.RequirePermission(rbac.ActionInterviewManage), h.reconnectInterviewTranscriptSession)
		protected.POST("/interviews/:id/evaluations", auth.RequirePermission(rbac.ActionInterviewManage), h.submitInterviewEvaluation)
		protected.GET("/interviews/:id/evaluations", auth.RequirePermission(rbac.ActionInterviewManage), h.listInterviewEvaluations)
		protected.POST("/interviews/:id/question-recommendations", auth.RequirePermission(rbac.ActionInterviewManage), h.generateInterviewQuestionRecommendation)
		protected.GET("/interviews/:id/question-recommendations", auth.RequirePermission(rbac.ActionInterviewManage), h.getInterviewQuestionRecommendation)
		protected.GET("/candidates/:id/evaluations/latest", auth.RequirePermission(rbac.ActionInterviewManage), h.getCandidateLatestEvaluations)
		protected.POST("/candidates/:id/interview-report", auth.RequirePermission(rbac.ActionInterviewManage), h.generateInterviewReport)
		protected.GET("/interview-reports/:id/export", auth.RequirePermission(rbac.ActionInterviewManage), h.exportInterviewReport)
		protected.GET("/admin/users", auth.RequirePermission(rbac.ActionUserManage), h.listUsers)
		protected.GET("/audit-logs", auth.RequirePermission(rbac.ActionAuditRead), h.listAuditLogs)
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
	h.auditSvc.Record(audit.RecordInput{
		ActionType: audit.ActionAuthLogin,
		OperatorID: resolveLoginOperatorID(session, req.Email),
		ObjectType: "session",
		Metadata: map[string]string{
			"auth_method": "password",
		},
	})
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
	operatorID := ""
	if user := auth.MustUser(c); user != nil {
		operatorID = user.ID
	}
	h.auditSvc.Record(audit.RecordInput{
		ActionType: audit.ActionAuthLogout,
		OperatorID: operatorID,
		ObjectType: "session",
	})
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

func (h *handler) deleteResume(c *gin.Context) {
	user := auth.MustUser(c)
	if user == nil {
		httpx.AbortUnauthorized(c, http.StatusUnauthorized, "missing auth context")
		return
	}

	record, err := h.resumeSvc.DeleteResume(c.Param("id"))
	if err != nil {
		switch {
		case resume.IsResumeNotFound(err):
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"code": "DELETE_FAILED", "message": err.Error()})
		}
		return
	}

	h.auditSvc.Record(audit.RecordInput{
		ActionType: audit.ActionResumeDelete,
		OperatorID: user.ID,
		ObjectType: "resume",
		ObjectID:   record.ID,
		Metadata: map[string]string{
			"candidate_id": record.CandidateID,
			"file_name":    record.FileName,
		},
	})

	c.JSON(http.StatusOK, gin.H{
		"deleted":   true,
		"resume_id": record.ID,
	})
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

func (h *handler) startInterviewTranscriptSession(c *gin.Context) {
	var req startInterviewTranscriptSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "invalid request body"})
		return
	}

	session, err := h.interviewSvc.StartTranscriptSession(c.Param("id"), interview.StartTranscriptSessionRequest{
		Provider: req.Provider,
	})
	if err != nil {
		if interview.IsInterviewNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"started": true,
		"session": session,
	})
}

func (h *handler) appendInterviewTranscript(c *gin.Context) {
	var req appendInterviewTranscriptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "invalid request body"})
		return
	}

	startedAt, err := parseBodyOptionalRFC3339(req.StartedAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "started_at must be RFC3339"})
		return
	}
	endedAt, err := parseBodyOptionalRFC3339(req.EndedAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "ended_at must be RFC3339"})
		return
	}

	segment, session, err := h.interviewSvc.AppendTranscript(c.Param("id"), interview.AppendTranscriptRequest{
		SessionID:   req.SessionID,
		SpeakerRole: req.SpeakerRole,
		SpeakerID:   req.SpeakerID,
		Text:        req.Text,
		IsFinal:     req.IsFinal,
		StartedAt:   startedAt,
		EndedAt:     endedAt,
	})
	if err != nil {
		switch {
		case interview.IsInterviewNotFound(err), errors.Is(err, interview.ErrTranscriptionSessionNotFound):
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": err.Error()})
		case errors.Is(err, interview.ErrTranscriptionSessionInactive):
			c.JSON(http.StatusConflict, gin.H{
				"code":               "TRANSCRIPTION_INTERRUPTED",
				"message":            err.Error(),
				"reconnect_required": true,
			})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"accepted": true,
		"segment":  segment,
		"session":  session,
	})
}

func (h *handler) getInterviewTranscriptions(c *gin.Context) {
	sinceSeq := int64(0)
	if rawSince := strings.TrimSpace(c.Query("since_seq")); rawSince != "" {
		parsed, err := strconv.ParseInt(rawSince, 10, 64)
		if err != nil || parsed < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "since_seq must be a non-negative integer"})
			return
		}
		sinceSeq = parsed
	}

	limit := 50
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "limit must be a positive integer"})
			return
		}
		limit = parsed
	}

	stream, err := h.interviewSvc.StreamTranscripts(c.Param("id"), interview.StreamTranscriptsRequest{
		SessionID:     c.Query("session_id"),
		SinceSequence: sinceSeq,
		Limit:         limit,
	})
	if err != nil {
		switch {
		case interview.IsInterviewNotFound(err), errors.Is(err, interview.ErrTranscriptionSessionNotFound):
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": err.Error()})
		}
		return
	}

	status := http.StatusOK
	if stream.ReconnectRequired {
		status = http.StatusConflict
	}
	c.JSON(status, gin.H{
		"stream": stream,
	})
}

func (h *handler) markInterviewTranscriptInterrupted(c *gin.Context) {
	var req markInterviewTranscriptInterruptedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "invalid request body"})
		return
	}

	session, err := h.interviewSvc.MarkTranscriptSessionInterrupted(c.Param("id"), c.Param("sessionID"), req.Reason)
	if err != nil {
		switch {
		case interview.IsInterviewNotFound(err), errors.Is(err, interview.ErrTranscriptionSessionNotFound):
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"updated": true,
		"session": session,
	})
}

func (h *handler) reconnectInterviewTranscriptSession(c *gin.Context) {
	session, err := h.interviewSvc.ReconnectTranscriptSession(c.Param("id"), c.Param("sessionID"))
	if err != nil {
		switch {
		case interview.IsInterviewNotFound(err), errors.Is(err, interview.ErrTranscriptionSessionNotFound):
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"reconnected": true,
		"session":     session,
	})
}

func (h *handler) submitInterviewEvaluation(c *gin.Context) {
	user := auth.MustUser(c)
	if user == nil {
		httpx.AbortUnauthorized(c, http.StatusUnauthorized, "missing auth context")
		return
	}

	var req submitInterviewEvaluationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "invalid request body"})
		return
	}

	capabilityScores := make([]interview.CapabilityScore, 0, len(req.CapabilityScores))
	for _, item := range req.CapabilityScores {
		capabilityScores = append(capabilityScores, interview.CapabilityScore{
			Dimension: item.Dimension,
			Score:     item.Score,
			Comment:   item.Comment,
		})
	}

	result, err := h.interviewSvc.SubmitEvaluation(c.Param("id"), user.ID, interview.SubmitEvaluationRequest{
		CapabilityScores: capabilityScores,
		OverallComment:   req.OverallComment,
		Conclusion:       req.Conclusion,
	})
	if err != nil {
		switch {
		case interview.IsInterviewNotFound(err):
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": err.Error()})
		case errors.Is(err, interview.ErrEvaluationInterviewerNotAssigned):
			httpx.AbortUnauthorized(c, http.StatusForbidden, err.Error())
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": err.Error()})
		}
		return
	}

	actionType := audit.ActionInterviewEvaluationSubmit
	if result.Version > 1 {
		actionType = audit.ActionInterviewEvaluationModify
	}
	h.auditSvc.Record(audit.RecordInput{
		ActionType: actionType,
		OperatorID: user.ID,
		ObjectType: "interview_evaluation",
		ObjectID:   result.ID,
		Metadata: map[string]string{
			"interview_id":   result.InterviewID,
			"candidate_id":   result.CandidateID,
			"interviewer_id": result.InterviewerID,
			"conclusion":     string(result.Conclusion),
			"version":        strconv.Itoa(result.Version),
		},
	})

	c.JSON(http.StatusOK, gin.H{
		"submitted":  true,
		"evaluation": result,
	})
}

func (h *handler) listInterviewEvaluations(c *gin.Context) {
	evaluations, err := h.interviewSvc.ListEvaluations(c.Param("id"))
	if err != nil {
		if interview.IsInterviewNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"interview_id": c.Param("id"),
		"items":        evaluations,
	})
}

func (h *handler) getCandidateLatestEvaluations(c *gin.Context) {
	candidateID := strings.TrimSpace(c.Param("id"))
	if !h.resumeSvc.CandidateExists(candidateID) {
		c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": "candidate not found"})
		return
	}

	view := h.interviewSvc.BuildCandidateLatestEvaluationsView(candidateID)
	c.JSON(http.StatusOK, gin.H{"candidate_evaluations": view})
}

func (h *handler) generateInterviewQuestionRecommendation(c *gin.Context) {
	var req generateInterviewQuestionRecommendationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "invalid request body"})
		return
	}

	item, err := h.interviewSvc.GetInterview(c.Param("id"))
	if err != nil {
		if interview.IsInterviewNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": err.Error()})
		return
	}

	candidate, ok := h.resumeSvc.GetCandidate(item.CandidateID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": "candidate not found"})
		return
	}

	recommendation, err := h.interviewSvc.GenerateQuestionRecommendation(item.ID, interview.CandidateSnapshot{
		ID:                    candidate.ID,
		FullName:              candidate.FullName,
		CurrentCompany:        candidate.CurrentCompany,
		CurrentTitle:          candidate.CurrentTitle,
		HighestEducation:      candidate.HighestEducation,
		TotalExperienceMonths: candidate.TotalExperienceMonths,
		Skills:                candidate.Skills,
	}, interview.GenerateQuestionRecommendationRequest{
		JobTitle:       req.JobTitle,
		JobDescription: req.JobDescription,
	})
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
		"generated":       true,
		"recommendation":  recommendation,
		"candidate":       candidate,
		"interview_round": item.Round,
	})
}

func (h *handler) getInterviewQuestionRecommendation(c *gin.Context) {
	recommendation, err := h.interviewSvc.GetQuestionRecommendation(c.Param("id"))
	if err != nil {
		switch {
		case interview.IsInterviewNotFound(err):
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": err.Error()})
		case interview.IsQuestionRecommendationNotFound(err):
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"recommendation": recommendation,
	})
}

func (h *handler) generateInterviewReport(c *gin.Context) {
	candidateID := strings.TrimSpace(c.Param("id"))
	candidate, ok := h.resumeSvc.GetCandidate(candidateID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": "candidate not found"})
		return
	}

	view := h.interviewSvc.BuildCandidateLatestEvaluationsView(candidateID)
	if len(view.LatestEvaluations) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "BAD_REQUEST",
			"message": "cannot generate interview report: no latest evaluations found for candidate",
		})
		return
	}

	evaluations := make([]report.EvaluationSnapshot, 0, len(view.LatestEvaluations))
	for _, item := range view.LatestEvaluations {
		scores := make([]report.CapabilityScore, 0, len(item.CapabilityScores))
		for _, score := range item.CapabilityScores {
			scores = append(scores, report.CapabilityScore{
				Dimension: score.Dimension,
				Score:     score.Score,
				Comment:   score.Comment,
			})
		}
		evaluations = append(evaluations, report.EvaluationSnapshot{
			InterviewID:      item.InterviewID,
			Round:            item.Round,
			InterviewerID:    item.InterviewerID,
			AverageScore:     item.AverageScore,
			CapabilityScores: scores,
			OverallComment:   item.OverallComment,
			Conclusion:       string(item.Conclusion),
			SubmittedAt:      item.SubmittedAt,
		})
	}

	generatedBy := ""
	if user := auth.MustUser(c); user != nil {
		generatedBy = user.ID
	}

	generated, err := h.reportSvc.Generate(report.GenerateRequest{
		Candidate: report.CandidateSnapshot{
			ID:             candidate.ID,
			FullName:       candidate.FullName,
			Email:          candidate.Email,
			Phone:          candidate.Phone,
			CurrentTitle:   candidate.CurrentTitle,
			CurrentCompany: candidate.CurrentCompany,
			StatusLayer:    string(candidate.StatusLayer),
		},
		Evaluations: evaluations,
		GeneratedBy: generatedBy,
	})
	if err != nil {
		var validationErr report.ValidationErrors
		if errors.As(err, &validationErr) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "BAD_REQUEST",
				"message": "report generation request is invalid",
				"details": gin.H{"fields": validationErr},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": "INTERNAL_ERROR", "message": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"generated": true,
		"report":    generated,
	})
}

func (h *handler) exportInterviewReport(c *gin.Context) {
	reportID := strings.TrimSpace(c.Param("id"))
	if reportID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "report id is required"})
		return
	}

	fileName, contentType, content, err := h.reportSvc.Export(reportID, c.Query("format"))
	if err != nil {
		switch {
		case errors.Is(err, report.ErrReportNotFound):
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": "report not found"})
		case errors.Is(err, report.ErrUnsupportedExportFormat):
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "unsupported export format: use json or markdown"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"code": "INTERNAL_ERROR", "message": err.Error()})
		}
		return
	}

	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", `attachment; filename="`+fileName+`"`)
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write(content)
}

func (h *handler) listAuditLogs(c *gin.Context) {
	limit := 100
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "limit must be a positive integer"})
			return
		}
		limit = parsed
	}

	from, err := parseOptionalRFC3339(c.Query("from"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "from must be RFC3339"})
		return
	}
	to, err := parseOptionalRFC3339(c.Query("to"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "to must be RFC3339"})
		return
	}

	items := h.auditSvc.Query(audit.QueryFilter{
		ActionType: strings.TrimSpace(c.Query("action_type")),
		OperatorID: strings.TrimSpace(c.Query("operator_id")),
		ObjectType: strings.TrimSpace(c.Query("object_type")),
		ObjectID:   strings.TrimSpace(c.Query("object_id")),
		From:       from,
		To:         to,
		Limit:      limit,
	})
	c.JSON(http.StatusOK, gin.H{
		"items": items,
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

func parseOptionalRFC3339(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func parseBodyOptionalRFC3339(raw *string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	return parseOptionalRFC3339(*raw)
}

func resolveLoginOperatorID(session *auth.Session, fallback string) string {
	if session != nil {
		if userMap, ok := session.User.(map[string]any); ok {
			if id, ok := userMap["id"].(string); ok && strings.TrimSpace(id) != "" {
				return strings.TrimSpace(id)
			}
			if email, ok := userMap["email"].(string); ok && strings.TrimSpace(email) != "" {
				return strings.TrimSpace(email)
			}
		}
	}
	return strings.TrimSpace(fallback)
}

func (h *handler) listUsers(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"items": []gin.H{
			{"id": "user_001", "role": rbac.RoleSuperAdmin},
		},
	})
}
