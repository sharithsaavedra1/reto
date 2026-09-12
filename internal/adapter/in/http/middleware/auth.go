package middleware

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
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
	errMissingAuthKey     = errors.New("neither JWT secret nor RSA public key/JWKS is configured")
	errKeyNotFound        = errors.New("signing key not found in JWKS / public key set")
)

// JWTHeader represents the header of a JWT.
type JWTHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
	Kid string `json:"kid,omitempty"`
}

// JWK represents a JSON Web Key per RFC 7517.
type JWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JWKS represents a JSON Web Key Set per RFC 7517.
type JWKS struct {
	Keys []JWK `json:"keys"`
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

// AuthMiddleware validates JWT authentication and authorization supporting RS256 (via IAM JWKS / RSA Public Key)
// and HS256 (for local development/testing fallback).
type AuthMiddleware struct {
	mu           sync.RWMutex
	jwtSecret    []byte
	rsaPublicKey *rsa.PublicKey
	jwksKeys     map[string]*rsa.PublicKey
	jwksURL      string
}

// NewAuthMiddleware initializes the auth middleware, automatically inspecting environment variables
// (JWT_PUBLIC_KEY, JWKS_JSON, JWKS_URL, JWT_SECRET).
func NewAuthMiddleware(secret string) *AuthMiddleware {
	m := &AuthMiddleware{
		jwksKeys: make(map[string]*rsa.PublicKey),
	}

	secret = strings.TrimSpace(secret)
	if secret != "" {
		m.jwtSecret = []byte(secret)
	} else if envSecret := strings.TrimSpace(os.Getenv("JWT_SECRET")); envSecret != "" {
		m.jwtSecret = []byte(envSecret)
	}

	// 1. Check for RSA Public Key PEM via environment variable
	if pemStr := strings.TrimSpace(os.Getenv("JWT_PUBLIC_KEY")); pemStr != "" {
		_ = m.LoadRSAPublicKeyPEM([]byte(pemStr))
	}

	// 2. Check for JWKS JSON via environment variable
	if jwksJSON := strings.TrimSpace(os.Getenv("JWKS_JSON")); jwksJSON != "" {
		_ = m.LoadJWKSJSON([]byte(jwksJSON))
	}

	// 3. Check for JWKS URL
	if jwksURL := strings.TrimSpace(os.Getenv("JWKS_URL")); jwksURL != "" {
		m.jwksURL = jwksURL
		_ = m.FetchJWKSFromURL(jwksURL)
	}

	return m
}

// NewAuthMiddlewareWithRSAPublicKey creates an AuthMiddleware with a specific RSA Public Key for RS256.
func NewAuthMiddlewareWithRSAPublicKey(pubKey *rsa.PublicKey) *AuthMiddleware {
	m := &AuthMiddleware{
		rsaPublicKey: pubKey,
		jwksKeys:     make(map[string]*rsa.PublicKey),
	}
	return m
}

// SetRSAPublicKey sets the default RSA Public Key for RS256 verification.
func (a *AuthMiddleware) SetRSAPublicKey(pubKey *rsa.PublicKey) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rsaPublicKey = pubKey
}

// LoadRSAPublicKeyPEM parses a PEM-encoded PKIX or PKCS1 RSA Public Key.
func (a *AuthMiddleware) LoadRSAPublicKeyPEM(pemData []byte) error {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return errors.New("failed to parse PEM block containing public key")
	}

	// Try PKIX (SubjectPublicKeyInfo)
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err == nil {
		if rsaPub, ok := pub.(*rsa.PublicKey); ok {
			a.SetRSAPublicKey(rsaPub)
			return nil
		}
		return errors.New("parsed key is not an RSA public key")
	}

	// Try PKCS#1 RSA Public Key
	rsaPub, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err == nil {
		a.SetRSAPublicKey(rsaPub)
		return nil
	}

	return fmt.Errorf("unable to parse RSA public key: %w", err)
}

// LoadJWKSJSON parses a JWKS JSON byte slice and loads all RSA public keys indexed by kid.
func (a *AuthMiddleware) LoadJWKSJSON(jwksData []byte) error {
	var jwks JWKS
	if err := json.Unmarshal(jwksData, &jwks); err != nil {
		return fmt.Errorf("invalid JWKS JSON: %w", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for _, k := range jwks.Keys {
		if strings.ToUpper(k.Kty) != "RSA" {
			continue
		}
		pubKey, err := parseJWKRSAKey(k)
		if err == nil && pubKey != nil {
			if k.Kid != "" {
				a.jwksKeys[k.Kid] = pubKey
			}
			if a.rsaPublicKey == nil {
				a.rsaPublicKey = pubKey
			}
		}
	}
	return nil
}

// FetchJWKSFromURL fetches JWKS keys from the IAM JWKS endpoint.
func (a *AuthMiddleware) FetchJWKSFromURL(url string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch JWKS from url: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read JWKS body: %w", err)
	}
	return a.LoadJWKSJSON(body)
}

func parseJWKRSAKey(k JWK) (*rsa.PublicKey, error) {
	nBytes, err := decodeBase64URL(k.N)
	if err != nil {
		return nil, fmt.Errorf("invalid modulus n: %w", err)
	}
	eBytes, err := decodeBase64URL(k.E)
	if err != nil {
		return nil, fmt.Errorf("invalid exponent e: %w", err)
	}

	var eInt int
	for _, b := range eBytes {
		eInt = (eInt << 8) | int(b)
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: eInt,
	}, nil
}

// RequireFeature wraps an http.HandlerFunc ensuring the caller has a valid JWT (RS256/HS256) and the required feature.
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

	signingInput := tokenParts[0] + "." + tokenParts[1]
	alg := strings.ToUpper(strings.TrimSpace(header.Alg))

	// 2. Verify Signature based on algorithm
	switch alg {
	case "RS256":
		// IAM Governance standard (Asymmetric RSA signature)
		pubKey := a.resolveRSAPublicKey(header.Kid)
		if pubKey == nil {
			return nil, errKeyNotFound
		}

		sigBytes, err := decodeBase64URL(tokenParts[2])
		if err != nil {
			return nil, errInvalidTokenFormat
		}

		hash := sha256.Sum256([]byte(signingInput))
		if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hash[:], sigBytes); err != nil {
			return nil, errSignatureMismatch
		}

	case "HS256":
		// Local development / testing fallback (Symmetric HMAC-SHA256)
		a.mu.RLock()
		secret := a.jwtSecret
		a.mu.RUnlock()

		if len(secret) == 0 {
			return nil, errMissingAuthKey
		}

		h := hmac.New(sha256.New, secret)
		h.Write([]byte(signingInput))
		expectedSig := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

		if !hmac.Equal([]byte(expectedSig), []byte(tokenParts[2])) {
			return nil, errSignatureMismatch
		}

	default:
		return nil, errInvalidHeader
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

func (a *AuthMiddleware) resolveRSAPublicKey(kid string) *rsa.PublicKey {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if kid != "" {
		if k, ok := a.jwksKeys[kid]; ok {
			return k
		}
	}
	return a.rsaPublicKey
}

func decodeBase64URL(s string) ([]byte, error) {
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err == nil {
		return data, nil
	}
	// Fallback to standard URL encoding with padding if needed
	return base64.URLEncoding.DecodeString(s)
}
