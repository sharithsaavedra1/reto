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

var (
	errInvalidTokenFormat = errors.New("invalid token format")
	errInvalidHeader      = errors.New("invalid token header or unsupported algorithm")
	errSignatureMismatch  = errors.New("signature mismatch")
	errTokenExpired       = errors.New("token expired")
	errTokenNotYetValid   = errors.New("token not yet valid (nbf)")
	errMissingSecret      = errors.New("JWT secret is unconfigured")
)

// JWTHeader represents the header of a JWT.
type JWTHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// JWTClaims represents standard claims payload in the token.
type JWTClaims struct {
	Subject     string      `json:"sub,omitempty"`
	Email       string      `json:"email,omitempty"`
	Features    interface{} `json:"features,omitempty"`
	Permissions interface{} `json:"permissions,omitempty"`
	Scope       interface{} `json:"scope,omitempty"`
	Exp         int64       `json:"exp,omitempty"`
	Nbf         int64       `json:"nbf,omitempty"`
	Iat         int64       `json:"iat,omitempty"`
}

// HasFeature returns true ONLY if the claims explicitly contain the required feature.
// Note: Role-based bypasses (like ADMIN or *) are strictly forbidden per security criteria.
func (c *JWTClaims) HasFeature(requiredFeature string) bool {
	if requiredFeature == "" {
		return true
	}

	checkValue := func(val interface{}) bool {
		switch v := val.(type) {
		case string:
			for _, part := range strings.Split(v, " ") {
				if strings.TrimSpace(part) == requiredFeature {
					return true
				}
			}
			for _, part := range strings.Split(v, ",") {
				if strings.TrimSpace(part) == requiredFeature {
					return true
				}
			}
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok {
					if strings.TrimSpace(s) == requiredFeature {
						return true
					}
				}
			}
		case []string:
			for _, s := range v {
				if strings.TrimSpace(s) == requiredFeature {
					return true
				}
			}
		}
		return false
	}

	if checkValue(c.Features) {
		return true
	}
	if checkValue(c.Permissions) {
		return true
	}
	if checkValue(c.Scope) {
		return true
	}

	return false
}

// AuthMiddleware validates JWT authentication and authorization.
type AuthMiddleware struct {
	jwtSecret []byte
}

// NewAuthMiddleware initializes the auth middleware with a JWT secret.
func NewAuthMiddleware(secret string) *AuthMiddleware {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		secret = strings.TrimSpace(os.Getenv("JWT_SECRET"))
	}
	return &AuthMiddleware{
		jwtSecret: []byte(secret),
	}
}

// RequireFeature wraps an http.HandlerFunc ensuring the caller has a valid JWT and the exact required feature.
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
			response.WriteError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "invalid or expired token: "+err.Error(), nil)
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

func (a *AuthMiddleware) validateToken(tokenString string) (*JWTClaims, error) {
	if len(a.jwtSecret) == 0 {
		return nil, errMissingSecret
	}

	tokenParts := strings.Split(tokenString, ".")
	if len(tokenParts) != 3 {
		return nil, errInvalidTokenFormat
	}

	// 1. Decode & Validate Header
	headerJSON, err := decodeBase64URL(tokenParts[0])
	if err != nil {
		return nil, errInvalidHeader
	}
	var header JWTHeader
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, errInvalidHeader
	}
	if !strings.EqualFold(header.Alg, "HS256") {
		return nil, errInvalidHeader
	}

	// 2. Verify HMAC-SHA256 Signature
	signingInput := tokenParts[0] + "." + tokenParts[1]
	h := hmac.New(sha256.New, a.jwtSecret)
	h.Write([]byte(signingInput))
	expectedSig := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	if !hmac.Equal([]byte(expectedSig), []byte(tokenParts[2])) {
		return nil, errSignatureMismatch
	}

	// 3. Decode & Validate Claims Payload
	payloadJSON, err := decodeBase64URL(tokenParts[1])
	if err != nil {
		return nil, errInvalidTokenFormat
	}

	var claims JWTClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, errInvalidTokenFormat
	}

	now := time.Now().Unix()

	// 4. Validate Exp (Expiration Time)
	if claims.Exp > 0 && now > claims.Exp {
		return nil, errTokenExpired
	}

	// 5. Validate Nbf (Not Before)
	if claims.Nbf > 0 && now < claims.Nbf {
		return nil, errTokenNotYetValid
	}

	return &claims, nil
}

func decodeBase64URL(s string) ([]byte, error) {
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err == nil {
		return data, nil
	}
	// Fallback to standard URL encoding with padding if needed
	return base64.URLEncoding.DecodeString(s)
}
