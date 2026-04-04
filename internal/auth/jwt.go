package auth

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zplzpl/resume_system/internal/rbac"
)

type User struct {
	ID     string
	Role   rbac.Role
	Claims jwt.MapClaims
}

type JWTVerifier struct {
	secret []byte
}

func NewJWTVerifier(secret string) (*JWTVerifier, error) {
	if secret == "" {
		return nil, errors.New("supabase jwt secret is required")
	}
	return &JWTVerifier{secret: []byte(secret)}, nil
}

func (v *JWTVerifier) VerifyToken(raw string) (*User, error) {
	token, err := jwt.Parse(raw, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %s", token.Method.Alg())
		}
		return v.secret, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("token invalid")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid token claims")
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, errors.New("token missing sub claim")
	}
	role := resolveRole(claims)
	if !rbac.IsKnownRole(role) {
		return nil, fmt.Errorf("unknown role: %s", role)
	}
	return &User{
		ID:     sub,
		Role:   role,
		Claims: claims,
	}, nil
}

func resolveRole(claims jwt.MapClaims) rbac.Role {
	if role, ok := claims["system_role"].(string); ok && role != "" {
		return rbac.Role(role)
	}

	appMetadata, ok := claims["app_metadata"].(map[string]any)
	if !ok {
		return ""
	}

	if role, ok := appMetadata["system_role"].(string); ok && role != "" {
		return rbac.Role(role)
	}
	return ""
}
