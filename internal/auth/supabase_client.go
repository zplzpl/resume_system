package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Session struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	User         any    `json:"user,omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type Client interface {
	PasswordLogin(ctx context.Context, email, password string) (*Session, error)
	Refresh(ctx context.Context, refreshToken string) (*Session, error)
	Logout(ctx context.Context, accessToken string) error
}

type SupabaseClient struct {
	baseURL string
	anonKey string
	client  *http.Client
}

func NewSupabaseClient(baseURL, anonKey string) *SupabaseClient {
	return &SupabaseClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		anonKey: anonKey,
		client: &http.Client{
			Timeout: 8 * time.Second,
		},
	}
}

func (s *SupabaseClient) PasswordLogin(ctx context.Context, email, password string) (*Session, error) {
	payload := map[string]string{
		"email":    email,
		"password": password,
	}
	return s.exchangeToken(ctx, "password", payload)
}

func (s *SupabaseClient) Refresh(ctx context.Context, refreshToken string) (*Session, error) {
	payload := map[string]string{
		"refresh_token": refreshToken,
	}
	return s.exchangeToken(ctx, "refresh_token", payload)
}

func (s *SupabaseClient) exchangeToken(ctx context.Context, grantType string, payload map[string]string) (*Session, error) {
	if s.baseURL == "" || s.anonKey == "" {
		return nil, fmt.Errorf("supabase url and anon key are required")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/auth/v1/token?grant_type=%s", s.baseURL, grantType),
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", s.anonKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("supabase auth error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var session Session
	if err := json.Unmarshal(respBody, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *SupabaseClient) Logout(ctx context.Context, accessToken string) error {
	if s.baseURL == "" || s.anonKey == "" {
		return fmt.Errorf("supabase url and anon key are required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/auth/v1/logout", nil)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", s.anonKey)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("supabase logout error: status=%d body=%s", resp.StatusCode, string(body))
	}
	return nil
}
