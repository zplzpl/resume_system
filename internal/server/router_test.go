package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func mustRouter(t *testing.T) http.Handler {
	t.Helper()
	cfg := config.Config{
		Port:              "8080",
		SupabaseJWTSecret: "test-secret",
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
