package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/response"
)

type contextKey string

const (
	ClaimsContextKey contextKey = "jwt_claims"
)

// JWTClaims represents standard claims payload in the token.
type JWTClaims struct {
	Subject   string      `json:"sub,omitempty"`
	Email     string      `json:"email,omitempty"`
	Features  interface{} `json:"features,omitempty"`
	Roles     interface{} `json:"roles,omitempty"`
	Exp       int64       `json:"exp,omitempty"`
	Iat       int64       `json:"iat,omitempty"`
}

// HasFeature returns true if the claims include the required feature.
func (c *JWTClaims) HasFeature(requiredFeature string) bool {
	if requiredFeature == "" {
		return true
	}

	checkItem := func(val interface{}) bool {
		switch v := val.(type) {
		case string:
			for _, part := range strings.Split(v, ",") {
				if strings.TrimSpace(part) == requiredFeature || strings.TrimSpace(part) == "ADMIN" || strings.TrimSpace(part) == "*" {
					return true
				}
			}
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok {
					if s == requiredFeature || s == "ADMIN" || s == "*" {
						return true
					}
				}
			}
		case []string:
			for _, s := range v {
				if s == requiredFeature || s == "ADMIN" || s == "*" {
					return true
				}
			}
		}
		return false
	}

	if checkItem(c.Features) {
		return true
	}
	if checkItem(c.Roles) {
		return true
	}

	return false
}

// AuthMiddleware creates a middleware that validates JWT authentication and authorization.
type AuthMiddleware struct {
	jwtSecret []byte
}

// NewAuthMiddleware initializes the auth middleware with an optional JWT secret.
func NewAuthMiddleware(secret string) *AuthMiddleware {
	if secret == "" {
		secret = os.Getenv("JWT_SECRET")
	}
	return &AuthMiddleware{
		jwtSecret: []byte(secret),
	}
}

// RequireFeature wraps an http.HandlerFunc ensuring the caller has a valid JWT and the required feature.
func (a *AuthMiddleware) RequireFeature(requiredFeature string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			response.WriteError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "missing authorization header", nil)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			response.WriteError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "invalid authorization header format", nil)
			return
		}

		tokenString := strings.TrimSpace(parts[1])
		if tokenString == "" {
			response.WriteError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "empty bearer token", nil)
			return
		}

		claims, err := a.validateToken(tokenString)
		if err != nil {
			response.WriteError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "invalid or expired token", nil)
			return
		}

		if !claims.HasFeature(requiredFeature) {
			details := []response.ErrorDetail{
				{
					Field:   "feature",
					Message: "required feature missing: " + requiredFeature,
				},
			}
			response.WriteError(w, r, http.StatusForbidden, "FORBIDDEN", "access denied: insufficient permissions", details)
			return
		}

		ctx := context.WithValue(r.Context(), ClaimsContextKey, claims)
		next(w, r.WithContext(ctx))
	}
}

var (
	errInvalidTokenFormat = errors.New("invalid token format")
	errSignatureMismatch  = errors.New("signature mismatch")
	errTokenExpired       = errors.New("token expired")
)

func (a *AuthMiddleware) validateToken(tokenString string) (*JWTClaims, error) {
	tokenParts := strings.Split(tokenString, ".")
	if len(tokenParts) != 3 {
		return nil, errInvalidTokenFormat
	}

	// Verify signature if secret is set
	if len(a.jwtSecret) > 0 {
		signingInput := tokenParts[0] + "." + tokenParts[1]
		h := hmac.New(sha256.New, a.jwtSecret)
		h.Write([]byte(signingInput))
		expectedSig := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

		if !hmac.Equal([]byte(expectedSig), []byte(tokenParts[2])) {
			return nil, errSignatureMismatch
		}
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(tokenParts[1])
	if err != nil {
		// Try standard padding as fallback
		payloadJSON, err = base64.URLEncoding.DecodeString(tokenParts[1])
		if err != nil {
			return nil, err
		}
	}

	var claims JWTClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, err
	}

	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return nil, errTokenExpired
	}

	return &claims, nil
}
