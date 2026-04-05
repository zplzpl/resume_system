package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zplzpl/resume_system/internal/config"
	"github.com/zplzpl/resume_system/internal/httpx"
)

func TestUnauthorizedWithoutToken(t *testing.T) {
	router := mustRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/candidates", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["code"] != httpx.UnauthorizedCode {
		t.Fatalf("expected code %q, got %v", httpx.UnauthorizedCode, body["code"])
	}
}

func TestInterviewerCannotCreateCandidate(t *testing.T) {
	router := mustRouter(t)
	token := signToken(t, "user_1", "interviewer")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/candidates", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["code"] != httpx.UnauthorizedCode {
		t.Fatalf("expected code %q, got %v", httpx.UnauthorizedCode, body["code"])
	}
}

func TestHRCanCreateCandidate(t *testing.T) {
	router := mustRouter(t)
	token := signToken(t, "user_2", "hr")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/candidates", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestSuperAdminCanListUsers(t *testing.T) {
	router := mustRouter(t)
	token := signToken(t, "user_3", "super_admin")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestUploadResumeSuccessAndGet(t *testing.T) {
	router := mustRouter(t)
	token := signToken(t, "user_hr", "hr")

	req := newMultipartRequest(t, http.MethodPost, "/api/v1/resumes/upload", nil, []multipartFile{
		{FieldName: "file", FileName: "alice_resume.pdf", Content: "Name: Alice Zhang\nEmail: alice@example.com\nPhone: +86 13800138000\nCurrent Company: ACME\nTitle: Backend Engineer\nSkills: Go, SQL\nExperience: 5 years\n"},
	})
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	var uploadResp struct {
		Resume struct {
			ID          string `json:"id"`
			CandidateID string `json:"candidate_id"`
			ParseStatus string `json:"parse_status"`
			Parsed      struct {
				FullName string `json:"full_name"`
				Email    string `json:"email"`
			} `json:"parsed_payload"`
		} `json:"resume"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &uploadResp); err != nil {
		t.Fatalf("unmarshal upload response: %v", err)
	}
	if uploadResp.Resume.ID == "" {
		t.Fatalf("expected resume id in response")
	}
	if uploadResp.Resume.ParseStatus != "success" {
		t.Fatalf("expected parse_status success, got %q", uploadResp.Resume.ParseStatus)
	}
	if uploadResp.Resume.Parsed.FullName != "Alice Zhang" {
		t.Fatalf("expected full name parsed, got %q", uploadResp.Resume.Parsed.FullName)
	}
	if uploadResp.Resume.CandidateID == "" {
		t.Fatalf("expected candidate_id to be mapped")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/resumes/"+uploadResp.Resume.ID, nil)
	getReq.Header.Set("Authorization", "Bearer "+token)
	getW := httptest.NewRecorder()
	router.ServeHTTP(getW, getReq)

	if getW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, getW.Code)
	}

	var getResp struct {
		Resume struct {
			ID          string `json:"id"`
			ParseStatus string `json:"parse_status"`
		} `json:"resume"`
	}
	if err := json.Unmarshal(getW.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("unmarshal get response: %v", err)
	}
	if getResp.Resume.ID != uploadResp.Resume.ID {
		t.Fatalf("expected same resume id, got %q", getResp.Resume.ID)
	}
	if getResp.Resume.ParseStatus != "success" {
		t.Fatalf("expected success parse status, got %q", getResp.Resume.ParseStatus)
	}
}

func TestUploadResumeBatchMixedStatus(t *testing.T) {
	router := mustRouter(t)
	token := signToken(t, "user_hr", "hr")

	req := newMultipartRequest(t, http.MethodPost, "/api/v1/resumes/upload/batch", nil, []multipartFile{
		{FieldName: "files", FileName: "good_resume.pdf", Content: "Name: Bob Li\nEmail: bob@example.com\n"},
		{FieldName: "files", FileName: "bad_resume.txt", Content: "this should fail by extension"},
	})
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp struct {
		Items []struct {
			FileName string `json:"file_name"`
			Resume   struct {
				ParseStatus   string `json:"parse_status"`
				FailureReason string `json:"failure_reason"`
			} `json:"resume"`
		} `json:"items"`
		Summary struct {
			Total   int `json:"total"`
			Success int `json:"success"`
			Failed  int `json:"failed"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal batch response: %v", err)
	}
	if resp.Summary.Total != 2 || resp.Summary.Success != 1 || resp.Summary.Failed != 1 {
		t.Fatalf("unexpected summary: %+v", resp.Summary)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}

	var badItemReason string
	for _, item := range resp.Items {
		if item.FileName == "bad_resume.txt" {
			badItemReason = item.Resume.FailureReason
			if item.Resume.ParseStatus != "failed" {
				t.Fatalf("expected bad resume failed status, got %q", item.Resume.ParseStatus)
			}
		}
	}
	if !strings.Contains(badItemReason, "only PDF/DOC/DOCX") {
		t.Fatalf("expected unsupported format reason, got %q", badItemReason)
	}
}

func TestRetryFailedResume(t *testing.T) {
	router := mustRouter(t)
	token := signToken(t, "user_hr", "hr")

	uploadReq := newMultipartRequest(t, http.MethodPost, "/api/v1/resumes/upload", nil, []multipartFile{
		{FieldName: "file", FileName: "retry_resume.pdf", Content: ""},
	})
	uploadReq.Header.Set("Authorization", "Bearer "+token)
	uploadW := httptest.NewRecorder()
	router.ServeHTTP(uploadW, uploadReq)

	if uploadW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, uploadW.Code, uploadW.Body.String())
	}

	var uploadResp struct {
		Resume struct {
			ID          string `json:"id"`
			StoragePath string `json:"storage_path"`
			ParseStatus string `json:"parse_status"`
		} `json:"resume"`
	}
	if err := json.Unmarshal(uploadW.Body.Bytes(), &uploadResp); err != nil {
		t.Fatalf("unmarshal upload response: %v", err)
	}
	if uploadResp.Resume.ParseStatus != "failed" {
		t.Fatalf("expected failed parse status, got %q", uploadResp.Resume.ParseStatus)
	}

	if err := os.WriteFile(uploadResp.Resume.StoragePath, []byte("Name: Retry OK\nEmail: retry@example.com\n"), 0o644); err != nil {
		t.Fatalf("rewrite stored file for retry: %v", err)
	}

	retryReq := httptest.NewRequest(http.MethodPost, "/api/v1/resumes/"+uploadResp.Resume.ID+"/retry", nil)
	retryReq.Header.Set("Authorization", "Bearer "+token)
	retryW := httptest.NewRecorder()
	router.ServeHTTP(retryW, retryReq)

	if retryW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, retryW.Code, retryW.Body.String())
	}

	var retryResp struct {
		Resume struct {
			ParseStatus string `json:"parse_status"`
			CandidateID string `json:"candidate_id"`
		} `json:"resume"`
	}
	if err := json.Unmarshal(retryW.Body.Bytes(), &retryResp); err != nil {
		t.Fatalf("unmarshal retry response: %v", err)
	}
	if retryResp.Resume.ParseStatus != "success" {
		t.Fatalf("expected retry parse status success, got %q", retryResp.Resume.ParseStatus)
	}
	if retryResp.Resume.CandidateID == "" {
		t.Fatalf("expected candidate id mapped after retry")
	}
}

func TestCandidateSearchWithCombinedFiltersAndStatusLayer(t *testing.T) {
	router := mustRouter(t)
	token := signToken(t, "user_hr", "hr")

	candidateA := uploadResumeAndGetCandidateID(t, router, token, "alice_resume.pdf", "Name: Alice Zhang\nEmail: alice@example.com\nCurrent Company: ACME Cloud\nEducation: Tsinghua University\nSkills: Go, SQL\n")
	candidateB := uploadResumeAndGetCandidateID(t, router, token, "bob_resume.pdf", "Name: Bob Li\nEmail: bob@example.com\nCurrent Company: Globex\nEducation: Stanford University\nSkills: Java, Spring\n")

	updateCandidateStatusLayer(t, router, token, candidateA, "screening")
	updateCandidateStatusLayer(t, router, token, candidateB, "interview")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/candidates?keyword=go&company=acme&school=tsinghua&status_layer=screening", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	var searchResp struct {
		Items []struct {
			ID          string `json:"id"`
			StatusLayer string `json:"status_layer"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &searchResp); err != nil {
		t.Fatalf("unmarshal search response: %v", err)
	}
	if len(searchResp.Items) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(searchResp.Items))
	}
	if searchResp.Items[0].ID != candidateA {
		t.Fatalf("expected candidate %s, got %s", candidateA, searchResp.Items[0].ID)
	}
	if searchResp.Items[0].StatusLayer != "screening" {
		t.Fatalf("expected status_layer screening, got %q", searchResp.Items[0].StatusLayer)
	}

	reqMulti := httptest.NewRequest(http.MethodGet, "/api/v1/candidates?status_layer=screening,interview", nil)
	reqMulti.Header.Set("Authorization", "Bearer "+token)
	wMulti := httptest.NewRecorder()
	router.ServeHTTP(wMulti, reqMulti)
	if wMulti.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, wMulti.Code, wMulti.Body.String())
	}
	var multiResp struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(wMulti.Body.Bytes(), &multiResp); err != nil {
		t.Fatalf("unmarshal multi search response: %v", err)
	}
	if len(multiResp.Items) != 2 {
		t.Fatalf("expected 2 candidates for multi status filter, got %d", len(multiResp.Items))
	}
}

func TestCandidateStatusLayerValidation(t *testing.T) {
	router := mustRouter(t)
	token := signToken(t, "user_hr", "hr")

	candidateID := uploadResumeAndGetCandidateID(t, router, token, "status_check_resume.pdf", "Name: Status Check\nEmail: status@example.com\nSkills: Go\n")

	badStatusReq := httptest.NewRequest(http.MethodPatch, "/api/v1/candidates/"+candidateID+"/status-layer", strings.NewReader(`{"status_layer":"unknown"}`))
	badStatusReq.Header.Set("Authorization", "Bearer "+token)
	badStatusReq.Header.Set("Content-Type", "application/json")
	badStatusW := httptest.NewRecorder()
	router.ServeHTTP(badStatusW, badStatusReq)
	if badStatusW.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, badStatusW.Code, badStatusW.Body.String())
	}

	badFilterReq := httptest.NewRequest(http.MethodGet, "/api/v1/candidates?status_layer=unknown", nil)
	badFilterReq.Header.Set("Authorization", "Bearer "+token)
	badFilterW := httptest.NewRecorder()
	router.ServeHTTP(badFilterW, badFilterReq)
	if badFilterW.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, badFilterW.Code, badFilterW.Body.String())
	}

	notFoundReq := httptest.NewRequest(http.MethodPatch, "/api/v1/candidates/cand_not_exist/status-layer", strings.NewReader(`{"status_layer":"screening"}`))
	notFoundReq.Header.Set("Authorization", "Bearer "+token)
	notFoundReq.Header.Set("Content-Type", "application/json")
	notFoundW := httptest.NewRecorder()
	router.ServeHTTP(notFoundW, notFoundReq)
	if notFoundW.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusNotFound, notFoundW.Code, notFoundW.Body.String())
	}
}

func TestInterviewSchedulingConflictAndCalendarViews(t *testing.T) {
	router := mustRouter(t)
	token := signToken(t, "user_hr", "hr")

	candidateA := uploadResumeAndGetCandidateID(t, router, token, "schedule_a.pdf", "Name: Candidate A\nEmail: a@example.com\nSkills: Go\n")
	candidateB := uploadResumeAndGetCandidateID(t, router, token, "schedule_b.pdf", "Name: Candidate B\nEmail: b@example.com\nSkills: Go\n")

	firstInterviewID := createInterview(t, router, token, map[string]any{
		"candidate_id":    candidateA,
		"interviewer_ids": []string{"iv_1", "iv_2"},
		"starts_at":       "2026-04-06T09:00:00Z",
		"ends_at":         "2026-04-06T10:00:00Z",
		"round":           "round-1",
	})
	if firstInterviewID == "" {
		t.Fatalf("expected interview id")
	}

	conflictSameCandidateReq := httptest.NewRequest(http.MethodPost, "/api/v1/interviews", strings.NewReader(`{"candidate_id":"`+candidateA+`","interviewer_ids":["iv_3"],"starts_at":"2026-04-06T09:30:00Z","ends_at":"2026-04-06T10:30:00Z"}`))
	conflictSameCandidateReq.Header.Set("Authorization", "Bearer "+token)
	conflictSameCandidateReq.Header.Set("Content-Type", "application/json")
	conflictSameCandidateW := httptest.NewRecorder()
	router.ServeHTTP(conflictSameCandidateW, conflictSameCandidateReq)
	if conflictSameCandidateW.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusConflict, conflictSameCandidateW.Code, conflictSameCandidateW.Body.String())
	}
	if !strings.Contains(conflictSameCandidateW.Body.String(), "candidate_time_conflict") {
		t.Fatalf("expected candidate conflict, got %s", conflictSameCandidateW.Body.String())
	}

	conflictInterviewerReq := httptest.NewRequest(http.MethodPost, "/api/v1/interviews", strings.NewReader(`{"candidate_id":"`+candidateB+`","interviewer_ids":["iv_1"],"starts_at":"2026-04-06T09:40:00Z","ends_at":"2026-04-06T10:20:00Z"}`))
	conflictInterviewerReq.Header.Set("Authorization", "Bearer "+token)
	conflictInterviewerReq.Header.Set("Content-Type", "application/json")
	conflictInterviewerW := httptest.NewRecorder()
	router.ServeHTTP(conflictInterviewerW, conflictInterviewerReq)
	if conflictInterviewerW.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusConflict, conflictInterviewerW.Code, conflictInterviewerW.Body.String())
	}
	if !strings.Contains(conflictInterviewerW.Body.String(), "interviewer_time_conflict") {
		t.Fatalf("expected interviewer conflict, got %s", conflictInterviewerW.Body.String())
	}

	secondInterviewID := createInterview(t, router, token, map[string]any{
		"candidate_id":    candidateB,
		"interviewer_ids": []string{"iv_3"},
		"starts_at":       "2026-04-07T11:00:00Z",
		"ends_at":         "2026-04-07T12:00:00Z",
		"round":           "round-2",
	})
	if secondInterviewID == "" {
		t.Fatalf("expected second interview id")
	}

	dayReq := httptest.NewRequest(http.MethodGet, "/api/v1/interviews/calendar?view=day&date=2026-04-06", nil)
	dayReq.Header.Set("Authorization", "Bearer "+token)
	dayW := httptest.NewRecorder()
	router.ServeHTTP(dayW, dayReq)
	if dayW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, dayW.Code, dayW.Body.String())
	}
	assertCalendarCount(t, dayW.Body.Bytes(), 1)

	weekReq := httptest.NewRequest(http.MethodGet, "/api/v1/interviews/calendar?view=week&date=2026-04-06", nil)
	weekReq.Header.Set("Authorization", "Bearer "+token)
	weekW := httptest.NewRecorder()
	router.ServeHTTP(weekW, weekReq)
	if weekW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, weekW.Code, weekW.Body.String())
	}
	assertCalendarCount(t, weekW.Body.Bytes(), 2)

	monthReq := httptest.NewRequest(http.MethodGet, "/api/v1/interviews/calendar?view=month&date=2026-04-01", nil)
	monthReq.Header.Set("Authorization", "Bearer "+token)
	monthW := httptest.NewRecorder()
	router.ServeHTTP(monthW, monthReq)
	if monthW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, monthW.Code, monthW.Body.String())
	}
	assertCalendarCount(t, monthW.Body.Bytes(), 2)
}

func TestInterviewUpdateTriggersNotificationAndStatusLink(t *testing.T) {
	router := mustRouter(t)
	token := signToken(t, "user_hr", "hr")

	candidateID := uploadResumeAndGetCandidateID(t, router, token, "schedule_update.pdf", "Name: Candidate Update\nEmail: update@example.com\nSkills: Go\n")
	interviewID := createInterview(t, router, token, map[string]any{
		"candidate_id":    candidateID,
		"interviewer_ids": []string{"iv_9"},
		"starts_at":       "2026-04-06T13:00:00Z",
		"ends_at":         "2026-04-06T14:00:00Z",
		"round":           "round-1",
	})

	updatePayload := `{"starts_at":"2026-04-06T15:00:00Z","ends_at":"2026-04-06T16:00:00Z","note":"rescheduled by hr"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/interviews/"+interviewID, strings.NewReader(updatePayload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp struct {
		NotificationsEnqueued int `json:"notifications_enqueued"`
		Interview             struct {
			Status string `json:"status"`
		} `json:"interview"`
		Candidate struct {
			StatusLayer string `json:"status_layer"`
		} `json:"candidate"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Interview.Status != "rescheduled" {
		t.Fatalf("expected status rescheduled, got %q", resp.Interview.Status)
	}
	if resp.NotificationsEnqueued != 4 {
		t.Fatalf("expected 4 notifications, got %d", resp.NotificationsEnqueued)
	}
	if resp.Candidate.StatusLayer != "interview" {
		t.Fatalf("expected candidate status_layer interview, got %q", resp.Candidate.StatusLayer)
	}
}

func mustRouter(t *testing.T) http.Handler {
	t.Helper()
	cfg := config.Config{
		Port:              "8080",
		SupabaseJWTSecret: "test-secret",
		ResumeStorageDir:  t.TempDir(),
	}
	router, err := NewRouter(cfg)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	return router
}

func uploadResumeAndGetCandidateID(t *testing.T, router http.Handler, token, fileName, content string) string {
	t.Helper()

	req := newMultipartRequest(t, http.MethodPost, "/api/v1/resumes/upload", nil, []multipartFile{
		{FieldName: "file", FileName: fileName, Content: content},
	})
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp struct {
		Candidate struct {
			ID string `json:"id"`
		} `json:"candidate"`
		Resume struct {
			ParseStatus string `json:"parse_status"`
		} `json:"resume"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal upload response: %v", err)
	}
	if resp.Resume.ParseStatus != "success" {
		t.Fatalf("expected parse status success, got %q", resp.Resume.ParseStatus)
	}
	if resp.Candidate.ID == "" {
		t.Fatalf("expected candidate id from upload response")
	}
	return resp.Candidate.ID
}

func updateCandidateStatusLayer(t *testing.T, router http.Handler, token, candidateID, status string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/candidates/"+candidateID+"/status-layer", strings.NewReader(`{"status_layer":"`+status+`"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}
}

func createInterview(t *testing.T, router http.Handler, token string, payload map[string]any) string {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/interviews", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp struct {
		NotificationsEnqueued int `json:"notifications_enqueued"`
		Interview             struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"interview"`
		Candidate struct {
			StatusLayer string `json:"status_layer"`
		} `json:"candidate"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal create interview response: %v", err)
	}
	if resp.Interview.ID == "" {
		t.Fatalf("expected interview id in response")
	}
	if resp.Interview.Status != "scheduled" {
		t.Fatalf("expected interview status scheduled, got %q", resp.Interview.Status)
	}
	if resp.Candidate.StatusLayer != "interview" {
		t.Fatalf("expected candidate status_layer interview, got %q", resp.Candidate.StatusLayer)
	}
	if resp.NotificationsEnqueued == 0 {
		t.Fatalf("expected notifications enqueued")
	}
	return resp.Interview.ID
}

func assertCalendarCount(t *testing.T, body []byte, want int) {
	t.Helper()
	var resp struct {
		Calendar struct {
			View      string `json:"view"`
			RangeFrom string `json:"range_from"`
			RangeTo   string `json:"range_to"`
			Items     []any  `json:"items"`
		} `json:"calendar"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal calendar response: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, resp.Calendar.RangeFrom); err != nil {
		t.Fatalf("invalid range_from: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, resp.Calendar.RangeTo); err != nil {
		t.Fatalf("invalid range_to: %v", err)
	}
	if len(resp.Calendar.Items) != want {
		t.Fatalf("expected %d items, got %d", want, len(resp.Calendar.Items))
	}
}

func signToken(t *testing.T, sub, role string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": sub,
		"app_metadata": map[string]any{
			"system_role": role,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	raw, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return raw
}

type multipartFile struct {
	FieldName string
	FileName  string
	Content   string
}

func newMultipartRequest(t *testing.T, method, target string, fields map[string]string, files []multipartFile) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write field %s: %v", key, err)
		}
	}

	for _, file := range files {
		part, err := writer.CreateFormFile(file.FieldName, file.FileName)
		if err != nil {
			t.Fatalf("create form file %s: %v", file.FileName, err)
		}
		if _, err := part.Write([]byte(file.Content)); err != nil {
			t.Fatalf("write file %s: %v", file.FileName, err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(method, target, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}
