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
