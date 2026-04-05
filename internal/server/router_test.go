package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zplzpl/resume_system/internal/audit"
	"github.com/zplzpl/resume_system/internal/auth"
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

func TestInterviewerCanGetRecruitingDashboard(t *testing.T) {
	router := mustRouter(t)
	hrToken := signToken(t, "user_hr_dashboard", "hr")
	interviewerToken := signToken(t, "iv_dashboard_1", "interviewer")

	candidateID := uploadResumeAndGetCandidateID(t, router, hrToken, "dashboard_resume.pdf", "Name: Dashboard Candidate\nEmail: dashboard@example.com\nSkills: Go\n")
	interviewID := createInterview(t, router, hrToken, map[string]any{
		"candidate_id":    candidateID,
		"interviewer_ids": []string{"iv_dashboard_1"},
		"starts_at":       "2026-04-01T09:00:00Z",
		"ends_at":         "2026-04-01T10:00:00Z",
		"round":           "round-1",
	})
	submitEvaluation(t, router, interviewerToken, interviewID, map[string]any{
		"capability_scores": []map[string]any{
			{"dimension": "technical_depth", "score": 4, "comment": "solid"},
			{"dimension": "problem_solving", "score": 4, "comment": "good"},
			{"dimension": "communication", "score": 4, "comment": "clear"},
			{"dimension": "collaboration", "score": 5, "comment": "strong"},
		},
		"overall_comment": "good candidate",
		"conclusion":      "hire",
	}, http.StatusOK)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/recruiting-dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+interviewerToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp struct {
		Funnel struct {
			TotalCandidates int `json:"total_candidates"`
			Stages          []struct {
				Stage          string  `json:"stage"`
				CandidateCount int     `json:"candidate_count"`
				ConversionRate float64 `json:"conversion_rate"`
			} `json:"stages"`
		} `json:"funnel"`
		Efficiency struct {
			TotalFeedbackCount  int `json:"total_feedback_count"`
			InterviewerWorkload []struct {
				InterviewerID string `json:"interviewer_id"`
				FeedbackCount int    `json:"feedback_count"`
			} `json:"interviewer_workload"`
		} `json:"efficiency"`
		MetricDefinitions []struct {
			MetricID string `json:"metric_id"`
		} `json:"metric_definitions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal dashboard response: %v", err)
	}
	if resp.Funnel.TotalCandidates != 1 {
		t.Fatalf("expected total candidates 1, got %d", resp.Funnel.TotalCandidates)
	}
	if len(resp.Funnel.Stages) != 5 {
		t.Fatalf("expected 5 stages, got %d", len(resp.Funnel.Stages))
	}
	if resp.Efficiency.TotalFeedbackCount != 1 {
		t.Fatalf("expected total feedback count 1, got %d", resp.Efficiency.TotalFeedbackCount)
	}
	if len(resp.Efficiency.InterviewerWorkload) != 1 || resp.Efficiency.InterviewerWorkload[0].InterviewerID != "iv_dashboard_1" {
		t.Fatalf("unexpected interviewer workload: %+v", resp.Efficiency.InterviewerWorkload)
	}
	if len(resp.MetricDefinitions) < 4 {
		t.Fatalf("expected metric definitions, got %d", len(resp.MetricDefinitions))
	}
}

func TestRecruitingDashboardCSVExport(t *testing.T) {
	router := mustRouter(t)
	hrToken := signToken(t, "user_hr_dashboard_export", "hr")
	interviewerToken := signToken(t, "iv_dashboard_export_1", "interviewer")

	candidateID := uploadResumeAndGetCandidateID(t, router, hrToken, "dashboard_export_resume.pdf", "Name: Export Candidate\nEmail: export@example.com\nSkills: Go\n")
	interviewID := createInterview(t, router, hrToken, map[string]any{
		"candidate_id":    candidateID,
		"interviewer_ids": []string{"iv_dashboard_export_1"},
		"starts_at":       "2026-04-01T11:00:00Z",
		"ends_at":         "2026-04-01T12:00:00Z",
		"round":           "round-1",
	})
	submitEvaluation(t, router, interviewerToken, interviewID, map[string]any{
		"capability_scores": []map[string]any{
			{"dimension": "technical_depth", "score": 5, "comment": "excellent"},
			{"dimension": "problem_solving", "score": 4, "comment": "stable"},
			{"dimension": "communication", "score": 4, "comment": "clear"},
			{"dimension": "collaboration", "score": 4, "comment": "good"},
		},
		"overall_comment": "export ready",
		"conclusion":      "strong_hire",
	}, http.StatusOK)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/recruiting-dashboard/export.csv", nil)
	req.Header.Set("Authorization", "Bearer "+hrToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}
	if contentType := w.Header().Get("Content-Type"); !strings.Contains(contentType, "text/csv") {
		t.Fatalf("expected csv content-type, got %q", contentType)
	}
	if disposition := w.Header().Get("Content-Disposition"); !strings.Contains(disposition, "recruiting_dashboard.csv") {
		t.Fatalf("expected csv attachment filename, got %q", disposition)
	}

	records, err := csv.NewReader(strings.NewReader(w.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(records) < 2 {
		t.Fatalf("expected at least 2 csv rows, got %d", len(records))
	}
	if len(records[0]) < 4 || records[0][0] != "section" || records[0][1] != "metric" {
		t.Fatalf("unexpected csv header: %+v", records[0])
	}

	var hasTotalFeedback bool
	var hasDefinition bool
	for _, record := range records[1:] {
		if len(record) < 2 {
			continue
		}
		if record[1] == "total_feedback_count" {
			hasTotalFeedback = true
		}
		if record[0] == "definition" && record[1] == "stage_conversion_rate" {
			hasDefinition = true
		}
	}
	if !hasTotalFeedback {
		t.Fatalf("expected total_feedback_count row in csv")
	}
	if !hasDefinition {
		t.Fatalf("expected stage_conversion_rate definition row in csv")
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

func TestInterviewEvaluationSubmitArchiveAndCandidateLatestView(t *testing.T) {
	router := mustRouter(t)
	hrToken := signToken(t, "user_hr", "hr")

	candidateID := uploadResumeAndGetCandidateID(t, router, hrToken, "eval_resume.pdf", "Name: Eval Candidate\nEmail: eval@example.com\nSkills: Go\n")
	roundOneInterviewID := createInterview(t, router, hrToken, map[string]any{
		"candidate_id":    candidateID,
		"interviewer_ids": []string{"iv_eval_1", "iv_eval_2"},
		"starts_at":       "2026-04-08T09:00:00Z",
		"ends_at":         "2026-04-08T10:00:00Z",
		"round":           "round-1",
	})
	roundTwoInterviewID := createInterview(t, router, hrToken, map[string]any{
		"candidate_id":    candidateID,
		"interviewer_ids": []string{"iv_eval_1"},
		"starts_at":       "2026-04-10T09:00:00Z",
		"ends_at":         "2026-04-10T10:00:00Z",
		"round":           "round-2",
	})

	tokenEval1 := signToken(t, "iv_eval_1", "interviewer")
	tokenEval2 := signToken(t, "iv_eval_2", "interviewer")

	submitEvaluation(t, router, tokenEval1, roundOneInterviewID, map[string]any{
		"capability_scores": []map[string]any{
			{"dimension": "technical_depth", "score": 4, "comment": "solid backend fundamentals"},
			{"dimension": "problem_solving", "score": 4, "comment": "good decomposition"},
			{"dimension": "communication", "score": 3, "comment": "clear but concise"},
			{"dimension": "collaboration", "score": 4, "comment": "works well with peers"},
		},
		"overall_comment": "first pass looks strong",
		"conclusion":      "hire",
	}, http.StatusOK)

	submitEvaluation(t, router, tokenEval1, roundOneInterviewID, map[string]any{
		"capability_scores": []map[string]any{
			{"dimension": "technical_depth", "score": 5, "comment": "excellent depth"},
			{"dimension": "problem_solving", "score": 4, "comment": "fast reasoning"},
			{"dimension": "communication", "score": 4, "comment": "improved clarity"},
			{"dimension": "collaboration", "score": 5, "comment": "strong ownership"},
		},
		"overall_comment": "updated after follow-up questions",
		"conclusion":      "strong_hire",
	}, http.StatusOK)

	submitEvaluation(t, router, tokenEval2, roundOneInterviewID, map[string]any{
		"capability_scores": []map[string]any{
			{"dimension": "technical_depth", "score": 3, "comment": "ok depth"},
			{"dimension": "problem_solving", "score": 3, "comment": "acceptable"},
			{"dimension": "communication", "score": 4, "comment": "strong communication"},
			{"dimension": "collaboration", "score": 4, "comment": "team fit"},
		},
		"overall_comment": "overall acceptable",
		"conclusion":      "hold",
	}, http.StatusOK)

	submitEvaluation(t, router, tokenEval1, roundTwoInterviewID, map[string]any{
		"capability_scores": []map[string]any{
			{"dimension": "technical_depth", "score": 4, "comment": "stable"},
			{"dimension": "problem_solving", "score": 5, "comment": "excellent on tradeoffs"},
			{"dimension": "communication", "score": 4, "comment": "clear"},
			{"dimension": "collaboration", "score": 4, "comment": "good leadership"},
		},
		"overall_comment": "second round confirmation",
		"conclusion":      "hire",
	}, http.StatusOK)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/interviews/"+roundOneInterviewID+"/evaluations", nil)
	listReq.Header.Set("Authorization", "Bearer "+hrToken)
	listW := httptest.NewRecorder()
	router.ServeHTTP(listW, listReq)

	if listW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, listW.Code, listW.Body.String())
	}

	var listResp struct {
		Items []struct {
			InterviewerID string `json:"interviewer_id"`
			Version       int    `json:"version"`
			IsLatest      bool   `json:"is_latest"`
			ArchivedAt    string `json:"archived_at"`
			Conclusion    string `json:"conclusion"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listW.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if len(listResp.Items) != 3 {
		t.Fatalf("expected 3 archive items, got %d", len(listResp.Items))
	}

	var latestCount int
	for _, item := range listResp.Items {
		if item.IsLatest {
			latestCount++
		}
		if !item.IsLatest && item.ArchivedAt == "" {
			t.Fatalf("expected archived_at on historical version, got empty for %+v", item)
		}
	}
	if latestCount != 2 {
		t.Fatalf("expected 2 latest records in round-1 interview, got %d", latestCount)
	}

	latestReq := httptest.NewRequest(http.MethodGet, "/api/v1/candidates/"+candidateID+"/evaluations/latest", nil)
	latestReq.Header.Set("Authorization", "Bearer "+hrToken)
	latestW := httptest.NewRecorder()
	router.ServeHTTP(latestW, latestReq)
	if latestW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, latestW.Code, latestW.Body.String())
	}

	var latestResp struct {
		CandidateEvaluations struct {
			CandidateID      string `json:"candidate_id"`
			TotalLatestCount int    `json:"total_latest_count"`
			Rounds           []struct {
				Round       string `json:"round"`
				Evaluations []struct {
					IsLatest   bool   `json:"is_latest"`
					Conclusion string `json:"conclusion"`
				} `json:"evaluations"`
			} `json:"rounds"`
		} `json:"candidate_evaluations"`
	}
	if err := json.Unmarshal(latestW.Body.Bytes(), &latestResp); err != nil {
		t.Fatalf("unmarshal latest response: %v", err)
	}
	if latestResp.CandidateEvaluations.CandidateID != candidateID {
		t.Fatalf("expected candidate id %q, got %q", candidateID, latestResp.CandidateEvaluations.CandidateID)
	}
	if latestResp.CandidateEvaluations.TotalLatestCount != 3 {
		t.Fatalf("expected 3 latest items, got %d", latestResp.CandidateEvaluations.TotalLatestCount)
	}
	if len(latestResp.CandidateEvaluations.Rounds) != 2 {
		t.Fatalf("expected 2 rounds, got %d", len(latestResp.CandidateEvaluations.Rounds))
	}
	for _, round := range latestResp.CandidateEvaluations.Rounds {
		for _, item := range round.Evaluations {
			if !item.IsLatest {
				t.Fatalf("expected only latest evaluations in candidate latest view")
			}
		}
	}
}

func TestInterviewReportGenerateDeterministicAndExport(t *testing.T) {
	router := mustRouter(t)
	hrToken := signToken(t, "user_hr", "hr")
	interviewerToken := signToken(t, "iv_report_1", "interviewer")

	candidateID := uploadResumeAndGetCandidateID(t, router, hrToken, "report_resume.pdf", "Name: Report Candidate\nEmail: report@example.com\nCurrent Company: Acme\nTitle: Backend Engineer\nSkills: Go\n")
	interviewID := createInterview(t, router, hrToken, map[string]any{
		"candidate_id":    candidateID,
		"interviewer_ids": []string{"iv_report_1"},
		"starts_at":       "2026-04-12T09:00:00Z",
		"ends_at":         "2026-04-12T10:00:00Z",
		"round":           "round-1",
	})

	submitEvaluation(t, router, interviewerToken, interviewID, map[string]any{
		"capability_scores": []map[string]any{
			{"dimension": "technical_depth", "score": 5, "comment": "excellent"},
			{"dimension": "problem_solving", "score": 4, "comment": "good decomposition"},
			{"dimension": "communication", "score": 4, "comment": "clear"},
			{"dimension": "collaboration", "score": 5, "comment": "strong ownership"},
		},
		"overall_comment": "ready for next stage",
		"conclusion":      "strong_hire",
	}, http.StatusOK)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/candidates/"+candidateID+"/interview-report", nil)
	req.Header.Set("Authorization", "Bearer "+hrToken)
	firstW := httptest.NewRecorder()
	router.ServeHTTP(firstW, req)
	if firstW.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusCreated, firstW.Code, firstW.Body.String())
	}

	var firstResp struct {
		Report struct {
			ReportID              string  `json:"report_id"`
			AverageScore          float64 `json:"average_score"`
			HiringRecommendation  string  `json:"hiring_recommendation"`
			SourceEvaluationCount int     `json:"source_evaluation_count"`
			GeneratedAt           string  `json:"generated_at"`
			Candidate             struct {
				ID       string `json:"id"`
				FullName string `json:"full_name"`
			} `json:"candidate"`
		} `json:"report"`
	}
	if err := json.Unmarshal(firstW.Body.Bytes(), &firstResp); err != nil {
		t.Fatalf("unmarshal first report response: %v", err)
	}
	if firstResp.Report.ReportID == "" {
		t.Fatalf("expected report id")
	}
	if firstResp.Report.Candidate.ID != candidateID {
		t.Fatalf("expected candidate id %q, got %q", candidateID, firstResp.Report.Candidate.ID)
	}
	if firstResp.Report.AverageScore <= 0 {
		t.Fatalf("expected positive average score, got %f", firstResp.Report.AverageScore)
	}
	if firstResp.Report.HiringRecommendation == "" {
		t.Fatalf("expected hiring recommendation")
	}
	if firstResp.Report.SourceEvaluationCount != 1 {
		t.Fatalf("expected source evaluation count 1, got %d", firstResp.Report.SourceEvaluationCount)
	}

	reqRepeat := httptest.NewRequest(http.MethodPost, "/api/v1/candidates/"+candidateID+"/interview-report", nil)
	reqRepeat.Header.Set("Authorization", "Bearer "+hrToken)
	secondW := httptest.NewRecorder()
	router.ServeHTTP(secondW, reqRepeat)
	if secondW.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusCreated, secondW.Code, secondW.Body.String())
	}
	var secondResp struct {
		Report struct {
			ReportID    string `json:"report_id"`
			GeneratedAt string `json:"generated_at"`
		} `json:"report"`
	}
	if err := json.Unmarshal(secondW.Body.Bytes(), &secondResp); err != nil {
		t.Fatalf("unmarshal second report response: %v", err)
	}
	if firstResp.Report.ReportID != secondResp.Report.ReportID {
		t.Fatalf("expected deterministic report id, got %q vs %q", firstResp.Report.ReportID, secondResp.Report.ReportID)
	}
	if firstResp.Report.GeneratedAt != secondResp.Report.GeneratedAt {
		t.Fatalf("expected deterministic generated_at, got %q vs %q", firstResp.Report.GeneratedAt, secondResp.Report.GeneratedAt)
	}

	exportJSONReq := httptest.NewRequest(http.MethodGet, "/api/v1/interview-reports/"+firstResp.Report.ReportID+"/export?format=json", nil)
	exportJSONReq.Header.Set("Authorization", "Bearer "+hrToken)
	exportJSONW := httptest.NewRecorder()
	router.ServeHTTP(exportJSONW, exportJSONReq)
	if exportJSONW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, exportJSONW.Code, exportJSONW.Body.String())
	}
	if !strings.Contains(exportJSONW.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("expected json content-type, got %q", exportJSONW.Header().Get("Content-Type"))
	}
	if !strings.Contains(exportJSONW.Body.String(), `"final_comment"`) {
		t.Fatalf("expected final_comment in json export body")
	}

	exportMarkdownReq := httptest.NewRequest(http.MethodGet, "/api/v1/interview-reports/"+firstResp.Report.ReportID+"/export?format=markdown", nil)
	exportMarkdownReq.Header.Set("Authorization", "Bearer "+hrToken)
	exportMarkdownW := httptest.NewRecorder()
	router.ServeHTTP(exportMarkdownW, exportMarkdownReq)
	if exportMarkdownW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, exportMarkdownW.Code, exportMarkdownW.Body.String())
	}
	if !strings.Contains(exportMarkdownW.Body.String(), "## Score Details") {
		t.Fatalf("expected markdown score section")
	}
}

func TestInterviewReportGenerationRequiresEvaluations(t *testing.T) {
	router := mustRouter(t)
	hrToken := signToken(t, "user_hr", "hr")

	candidateID := uploadResumeAndGetCandidateID(t, router, hrToken, "report_no_eval_resume.pdf", "Name: No Eval\nEmail: no-eval@example.com\nSkills: Go\n")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/candidates/"+candidateID+"/interview-report", nil)
	req.Header.Set("Authorization", "Bearer "+hrToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no latest evaluations found for candidate") {
		t.Fatalf("expected explicit error message, got %s", w.Body.String())
	}
}

func TestInterviewEvaluationValidationAndPermission(t *testing.T) {
	router := mustRouter(t)
	hrToken := signToken(t, "user_hr", "hr")
	assignedInterviewerToken := signToken(t, "iv_assigned", "interviewer")
	unassignedInterviewerToken := signToken(t, "iv_other", "interviewer")

	candidateID := uploadResumeAndGetCandidateID(t, router, hrToken, "eval_validation.pdf", "Name: Validation Candidate\nEmail: validation@example.com\nSkills: Go\n")
	interviewID := createInterview(t, router, hrToken, map[string]any{
		"candidate_id":    candidateID,
		"interviewer_ids": []string{"iv_assigned"},
		"starts_at":       "2026-04-11T09:00:00Z",
		"ends_at":         "2026-04-11T10:00:00Z",
		"round":           "round-1",
	})

	forbiddenReqBody, _ := json.Marshal(map[string]any{
		"capability_scores": []map[string]any{
			{"dimension": "technical_depth", "score": 4},
			{"dimension": "problem_solving", "score": 4},
			{"dimension": "communication", "score": 4},
			{"dimension": "collaboration", "score": 4},
		},
		"overall_comment": "not assigned interviewer",
		"conclusion":      "hire",
	})
	forbiddenReq := httptest.NewRequest(http.MethodPost, "/api/v1/interviews/"+interviewID+"/evaluations", bytes.NewReader(forbiddenReqBody))
	forbiddenReq.Header.Set("Authorization", "Bearer "+unassignedInterviewerToken)
	forbiddenReq.Header.Set("Content-Type", "application/json")
	forbiddenW := httptest.NewRecorder()
	router.ServeHTTP(forbiddenW, forbiddenReq)
	if forbiddenW.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusForbidden, forbiddenW.Code, forbiddenW.Body.String())
	}
	var forbiddenResp map[string]any
	_ = json.Unmarshal(forbiddenW.Body.Bytes(), &forbiddenResp)
	if forbiddenResp["code"] != httpx.UnauthorizedCode {
		t.Fatalf("expected code %q, got %v", httpx.UnauthorizedCode, forbiddenResp["code"])
	}

	badTemplateReqBody, _ := json.Marshal(map[string]any{
		"capability_scores": []map[string]any{
			{"dimension": "technical_depth", "score": 6},
			{"dimension": "problem_solving", "score": 4},
			{"dimension": "communication", "score": 4},
		},
		"overall_comment": "bad template",
		"conclusion":      "hire",
	})
	badTemplateReq := httptest.NewRequest(http.MethodPost, "/api/v1/interviews/"+interviewID+"/evaluations", bytes.NewReader(badTemplateReqBody))
	badTemplateReq.Header.Set("Authorization", "Bearer "+assignedInterviewerToken)
	badTemplateReq.Header.Set("Content-Type", "application/json")
	badTemplateW := httptest.NewRecorder()
	router.ServeHTTP(badTemplateW, badTemplateReq)
	if badTemplateW.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, badTemplateW.Code, badTemplateW.Body.String())
	}
}

func TestInterviewQuestionRecommendationGenerateAndGet(t *testing.T) {
	router := mustRouter(t)
	hrToken := signToken(t, "user_hr", "hr")

	candidateID := uploadResumeAndGetCandidateID(t, router, hrToken, "question_resume.pdf", "Name: Question Candidate\nEmail: q@example.com\nCurrent Company: NovaTech\nTitle: Senior Backend Engineer\nSkills: Go, SQL, Redis\nExperience: 6 years\n")
	interviewID := createInterview(t, router, hrToken, map[string]any{
		"candidate_id":    candidateID,
		"interviewer_ids": []string{"iv_q_1"},
		"starts_at":       "2026-04-12T09:00:00Z",
		"ends_at":         "2026-04-12T10:00:00Z",
		"round":           "round-1",
	})

	payload, _ := json.Marshal(map[string]any{
		"job_title":       "Backend Engineer",
		"job_description": "The role requires Go and SQL expertise, system design for architecture, cross-functional collaboration, and service performance optimization.",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/interviews/"+interviewID+"/question-recommendations", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+hrToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp struct {
		Generated      bool `json:"generated"`
		Recommendation struct {
			InterviewID     string `json:"interview_id"`
			FallbackUsed    bool   `json:"fallback_used"`
			GeneratedSource string `json:"generated_source"`
			Questions       []struct {
				Category string `json:"category"`
			} `json:"questions"`
		} `json:"recommendation"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal recommendation response: %v", err)
	}
	if !resp.Generated {
		t.Fatalf("expected generated=true")
	}
	if resp.Recommendation.InterviewID != interviewID {
		t.Fatalf("expected interview id %q, got %q", interviewID, resp.Recommendation.InterviewID)
	}
	if resp.Recommendation.FallbackUsed {
		t.Fatalf("expected non-fallback recommendation")
	}
	if resp.Recommendation.GeneratedSource != "ai_synthesizer" {
		t.Fatalf("expected generated_source ai_synthesizer, got %q", resp.Recommendation.GeneratedSource)
	}
	if len(resp.Recommendation.Questions) < 4 {
		t.Fatalf("expected at least 4 questions, got %d", len(resp.Recommendation.Questions))
	}
	assertQuestionCategoryCoverage(t, resp.Recommendation.Questions)

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/interviews/"+interviewID+"/question-recommendations", nil)
	getReq.Header.Set("Authorization", "Bearer "+hrToken)
	getW := httptest.NewRecorder()
	router.ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, getW.Code, getW.Body.String())
	}

	var getResp struct {
		Recommendation struct {
			InterviewID string `json:"interview_id"`
			Questions   []any  `json:"questions"`
		} `json:"recommendation"`
	}
	if err := json.Unmarshal(getW.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("unmarshal get recommendation response: %v", err)
	}
	if getResp.Recommendation.InterviewID != interviewID {
		t.Fatalf("expected interview id %q, got %q", interviewID, getResp.Recommendation.InterviewID)
	}
	if len(getResp.Recommendation.Questions) == 0 {
		t.Fatalf("expected persisted recommendation questions")
	}
}

func TestInterviewQuestionRecommendationFallback(t *testing.T) {
	router := mustRouter(t)
	hrToken := signToken(t, "user_hr", "hr")

	candidateID := uploadResumeAndGetCandidateID(t, router, hrToken, "question_fallback_resume.pdf", "Name: Fallback Candidate\nEmail: fallback@example.com\nSkills: Go\n")
	interviewID := createInterview(t, router, hrToken, map[string]any{
		"candidate_id":    candidateID,
		"interviewer_ids": []string{"iv_q_fb"},
		"starts_at":       "2026-04-13T09:00:00Z",
		"ends_at":         "2026-04-13T10:00:00Z",
		"round":           "round-1",
	})

	payload, _ := json.Marshal(map[string]any{
		"job_title": "Platform Engineer",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/interviews/"+interviewID+"/question-recommendations", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+hrToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp struct {
		Recommendation struct {
			FallbackUsed    bool   `json:"fallback_used"`
			FallbackReason  string `json:"fallback_reason"`
			GeneratedSource string `json:"generated_source"`
			Questions       []struct {
				Category string `json:"category"`
			} `json:"questions"`
		} `json:"recommendation"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal fallback recommendation response: %v", err)
	}
	if !resp.Recommendation.FallbackUsed {
		t.Fatalf("expected fallback recommendation")
	}
	if resp.Recommendation.GeneratedSource != "template_fallback" {
		t.Fatalf("expected generated_source template_fallback, got %q", resp.Recommendation.GeneratedSource)
	}
	if resp.Recommendation.FallbackReason == "" {
		t.Fatalf("expected fallback reason")
	}
	if len(resp.Recommendation.Questions) < 4 {
		t.Fatalf("expected at least 4 fallback questions, got %d", len(resp.Recommendation.Questions))
	}
	assertQuestionCategoryCoverage(t, resp.Recommendation.Questions)
}

func TestLoginAndResumeDeleteAreAudited(t *testing.T) {
	router := mustRouterWithAuthClient(t, &stubAuthClient{
		passwordLoginResp: &auth.Session{
			AccessToken:  "access_1",
			TokenType:    "bearer",
			ExpiresIn:    3600,
			RefreshToken: "refresh_1",
			User: map[string]any{
				"id":    "auth_user_1",
				"email": "hr@example.com",
			},
		},
	})

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"hr@example.com","password":"pass"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	router.ServeHTTP(loginW, loginReq)
	if loginW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, loginW.Code, loginW.Body.String())
	}

	hrToken := signToken(t, "user_hr", "hr")
	_, resumeID := uploadResumeAndGetIDs(t, router, hrToken, "to_delete.pdf", "Name: To Delete\nEmail: delete@example.com\nSkills: Go\n")

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/resumes/"+resumeID, nil)
	deleteReq.Header.Set("Authorization", "Bearer "+hrToken)
	deleteW := httptest.NewRecorder()
	router.ServeHTTP(deleteW, deleteReq)
	if deleteW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, deleteW.Code, deleteW.Body.String())
	}

	loginAuditReq := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?action_type="+audit.ActionAuthLogin+"&operator_id=auth_user_1", nil)
	loginAuditReq.Header.Set("Authorization", "Bearer "+hrToken)
	loginAuditW := httptest.NewRecorder()
	router.ServeHTTP(loginAuditW, loginAuditReq)
	if loginAuditW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, loginAuditW.Code, loginAuditW.Body.String())
	}
	var loginAuditResp struct {
		Items []struct {
			ActionType string `json:"action_type"`
			OperatorID string `json:"operator_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(loginAuditW.Body.Bytes(), &loginAuditResp); err != nil {
		t.Fatalf("unmarshal login audit response: %v", err)
	}
	if len(loginAuditResp.Items) != 1 {
		t.Fatalf("expected 1 login audit record, got %d", len(loginAuditResp.Items))
	}
	if loginAuditResp.Items[0].ActionType != audit.ActionAuthLogin {
		t.Fatalf("unexpected action type: %q", loginAuditResp.Items[0].ActionType)
	}
	if loginAuditResp.Items[0].OperatorID != "auth_user_1" {
		t.Fatalf("unexpected operator_id: %q", loginAuditResp.Items[0].OperatorID)
	}

	deleteAuditReq := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?action_type="+audit.ActionResumeDelete+"&object_id="+resumeID, nil)
	deleteAuditReq.Header.Set("Authorization", "Bearer "+hrToken)
	deleteAuditW := httptest.NewRecorder()
	router.ServeHTTP(deleteAuditW, deleteAuditReq)
	if deleteAuditW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, deleteAuditW.Code, deleteAuditW.Body.String())
	}
	var deleteAuditResp struct {
		Items []struct {
			ActionType string            `json:"action_type"`
			OperatorID string            `json:"operator_id"`
			ObjectID   string            `json:"object_id"`
			Metadata   map[string]string `json:"metadata"`
		} `json:"items"`
	}
	if err := json.Unmarshal(deleteAuditW.Body.Bytes(), &deleteAuditResp); err != nil {
		t.Fatalf("unmarshal delete audit response: %v", err)
	}
	if len(deleteAuditResp.Items) != 1 {
		t.Fatalf("expected 1 delete audit record, got %d", len(deleteAuditResp.Items))
	}
	if deleteAuditResp.Items[0].ActionType != audit.ActionResumeDelete {
		t.Fatalf("unexpected action type: %q", deleteAuditResp.Items[0].ActionType)
	}
	if deleteAuditResp.Items[0].OperatorID != "user_hr" {
		t.Fatalf("unexpected operator_id: %q", deleteAuditResp.Items[0].OperatorID)
	}
	if deleteAuditResp.Items[0].ObjectID != resumeID {
		t.Fatalf("unexpected object_id: %q", deleteAuditResp.Items[0].ObjectID)
	}
}

func TestEvaluationModificationIsAuditedAndFilterable(t *testing.T) {
	router := mustRouter(t)
	hrToken := signToken(t, "user_hr", "hr")
	interviewerToken := signToken(t, "iv_eval_mod", "interviewer")

	candidateID := uploadResumeAndGetCandidateID(t, router, hrToken, "eval_mod_resume.pdf", "Name: Eval Mod\nEmail: eval_mod@example.com\nSkills: Go\n")
	interviewID := createInterview(t, router, hrToken, map[string]any{
		"candidate_id":    candidateID,
		"interviewer_ids": []string{"iv_eval_mod"},
		"starts_at":       "2026-04-15T09:00:00Z",
		"ends_at":         "2026-04-15T10:00:00Z",
		"round":           "round-1",
	})

	submitEvaluation(t, router, interviewerToken, interviewID, map[string]any{
		"capability_scores": []map[string]any{
			{"dimension": "technical_depth", "score": 4, "comment": "good"},
			{"dimension": "problem_solving", "score": 4, "comment": "good"},
			{"dimension": "communication", "score": 4, "comment": "good"},
			{"dimension": "collaboration", "score": 4, "comment": "good"},
		},
		"overall_comment": "first version",
		"conclusion":      "hire",
	}, http.StatusOK)
	submitEvaluation(t, router, interviewerToken, interviewID, map[string]any{
		"capability_scores": []map[string]any{
			{"dimension": "technical_depth", "score": 5, "comment": "better"},
			{"dimension": "problem_solving", "score": 4, "comment": "stable"},
			{"dimension": "communication", "score": 4, "comment": "clear"},
			{"dimension": "collaboration", "score": 5, "comment": "strong"},
		},
		"overall_comment": "second version",
		"conclusion":      "strong_hire",
	}, http.StatusOK)

	modifyReq := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?action_type="+audit.ActionInterviewEvaluationModify+"&operator_id=iv_eval_mod", nil)
	modifyReq.Header.Set("Authorization", "Bearer "+hrToken)
	modifyW := httptest.NewRecorder()
	router.ServeHTTP(modifyW, modifyReq)
	if modifyW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, modifyW.Code, modifyW.Body.String())
	}
	var modifyResp struct {
		Items []struct {
			ActionType string            `json:"action_type"`
			Metadata   map[string]string `json:"metadata"`
		} `json:"items"`
	}
	if err := json.Unmarshal(modifyW.Body.Bytes(), &modifyResp); err != nil {
		t.Fatalf("unmarshal modify audit response: %v", err)
	}
	if len(modifyResp.Items) != 1 {
		t.Fatalf("expected 1 modify audit record, got %d", len(modifyResp.Items))
	}
	if modifyResp.Items[0].ActionType != audit.ActionInterviewEvaluationModify {
		t.Fatalf("unexpected action type: %q", modifyResp.Items[0].ActionType)
	}
	if modifyResp.Items[0].Metadata["version"] != "2" {
		t.Fatalf("expected version=2 metadata, got %+v", modifyResp.Items[0].Metadata)
	}

	submitReq := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?action_type="+audit.ActionInterviewEvaluationSubmit+"&operator_id=iv_eval_mod", nil)
	submitReq.Header.Set("Authorization", "Bearer "+hrToken)
	submitW := httptest.NewRecorder()
	router.ServeHTTP(submitW, submitReq)
	if submitW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, submitW.Code, submitW.Body.String())
	}
	var submitResp struct {
		Items []struct {
			Metadata map[string]string `json:"metadata"`
		} `json:"items"`
	}
	if err := json.Unmarshal(submitW.Body.Bytes(), &submitResp); err != nil {
		t.Fatalf("unmarshal submit audit response: %v", err)
	}
	if len(submitResp.Items) != 1 {
		t.Fatalf("expected 1 submit audit record, got %d", len(submitResp.Items))
	}
	if submitResp.Items[0].Metadata["version"] != "1" {
		t.Fatalf("expected version=1 metadata, got %+v", submitResp.Items[0].Metadata)
	}

	forbiddenReq := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	forbiddenReq.Header.Set("Authorization", "Bearer "+interviewerToken)
	forbiddenW := httptest.NewRecorder()
	router.ServeHTTP(forbiddenW, forbiddenReq)
	if forbiddenW.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusForbidden, forbiddenW.Code, forbiddenW.Body.String())
	}

	fromFuture := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	emptyReq := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?action_type="+audit.ActionInterviewEvaluationModify+"&from="+fromFuture, nil)
	emptyReq.Header.Set("Authorization", "Bearer "+hrToken)
	emptyW := httptest.NewRecorder()
	router.ServeHTTP(emptyW, emptyReq)
	if emptyW.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, emptyW.Code, emptyW.Body.String())
	}
	var emptyResp struct {
		Items []any `json:"items"`
	}
	if err := json.Unmarshal(emptyW.Body.Bytes(), &emptyResp); err != nil {
		t.Fatalf("unmarshal empty audit response: %v", err)
	}
	if len(emptyResp.Items) != 0 {
		t.Fatalf("expected empty result for future window, got %d", len(emptyResp.Items))
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

func mustRouterWithAuthClient(t *testing.T, authClient auth.Client) http.Handler {
	t.Helper()
	cfg := config.Config{
		Port:              "8080",
		SupabaseJWTSecret: "test-secret",
		ResumeStorageDir:  t.TempDir(),
	}
	router, err := newRouterWithAuthClient(cfg, authClient)
	if err != nil {
		t.Fatalf("new router with auth client: %v", err)
	}
	return router
}

func uploadResumeAndGetCandidateID(t *testing.T, router http.Handler, token, fileName, content string) string {
	t.Helper()
	candidateID, _ := uploadResumeAndGetIDs(t, router, token, fileName, content)
	return candidateID
}

func uploadResumeAndGetIDs(t *testing.T, router http.Handler, token, fileName, content string) (string, string) {
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
			ID          string `json:"id"`
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
	if resp.Resume.ID == "" {
		t.Fatalf("expected resume id from upload response")
	}
	return resp.Candidate.ID, resp.Resume.ID
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

func submitEvaluation(t *testing.T, router http.Handler, token, interviewID string, payload map[string]any, expectedStatus int) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal evaluation payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/interviews/"+interviewID+"/evaluations", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != expectedStatus {
		t.Fatalf("expected status %d, got %d body=%s", expectedStatus, w.Code, w.Body.String())
	}
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

func assertQuestionCategoryCoverage(t *testing.T, questions []struct {
	Category string `json:"category"`
}) {
	t.Helper()
	var hasExperience bool
	var hasCapability bool
	for _, item := range questions {
		if item.Category == "experience_follow_up" {
			hasExperience = true
		}
		if item.Category == "capability_assessment" {
			hasCapability = true
		}
	}
	if !hasExperience || !hasCapability {
		t.Fatalf("expected category coverage (experience_follow_up + capability_assessment), got %+v", questions)
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

type stubAuthClient struct {
	passwordLoginResp *auth.Session
	passwordLoginErr  error
	refreshResp       *auth.Session
	refreshErr        error
	logoutErr         error
}

func (s *stubAuthClient) PasswordLogin(ctx context.Context, email, password string) (*auth.Session, error) {
	return s.passwordLoginResp, s.passwordLoginErr
}

func (s *stubAuthClient) Refresh(ctx context.Context, refreshToken string) (*auth.Session, error) {
	return s.refreshResp, s.refreshErr
}

func (s *stubAuthClient) Logout(ctx context.Context, accessToken string) error {
	return s.logoutErr
}
