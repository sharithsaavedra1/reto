package middleware_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/middleware"
)

func generateRSATestToken(privKey *rsa.PrivateKey, kid string, features []string, expOffset time.Duration, nbfOffset time.Duration) string {
	header := map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	}
	if kid != "" {
		header["kid"] = kid
	}
	headerJSON, _ := json.Marshal(header)
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)

	now := time.Now().Unix()
	claims := map[string]interface{}{
		"sub":      "user-123",
		"features": features,
		"exp":      now + int64(expOffset.Seconds()),
		"iat":      now,
	}
	if nbfOffset != 0 {
		claims["nbf"] = now + int64(nbfOffset.Seconds())
	}

	payloadJSON, _ := json.Marshal(claims)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

	signingInput := headerB64 + "." + payloadB64
	hash := sha256.Sum256([]byte(signingInput))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, privKey, crypto.SHA256, hash[:])
	sigB64 := base64.RawURLEncoding.EncodeToString(sig)

	return signingInput + "." + sigB64
}

func generateHS256TestToken(secret []byte, features []string, expOffset time.Duration) string {
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	headerJSON, _ := json.Marshal(header)
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)

	now := time.Now().Unix()
	claims := map[string]interface{}{
		"sub":      "user-123",
		"features": features,
		"exp":      now + int64(expOffset.Seconds()),
		"iat":      now,
	}
	payloadJSON, _ := json.Marshal(claims)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

	signingInput := headerB64 + "." + payloadB64
	h := sha256.New()
	h.Write([]byte(signingInput + string(secret)))
	sigB64 := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	return signingInput + "." + sigB64
}

func TestAuthMiddleware_RS256_IAMGovernance(t *testing.T) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA test key: %v", err)
	}
	pubKey := &privKey.PublicKey

	auth := middleware.NewAuthMiddlewareWithRSAPublicKey(pubKey)

	handler := auth.RequireFeature("ENV_MANAGE", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	})

	// 1. Valid RS256 token with required feature
	validToken := generateRSATestToken(privKey, "", []string{"ENV_MANAGE"}, 1*time.Hour, 0)
	req := httptest.NewRequest(http.MethodPost, "/training-environments", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid RS256 token, got: %d (%s)", rec.Code, rec.Body.String())
	}

	// 2. RS256 token missing required feature (403 Forbidden)
	viewOnlyToken := generateRSATestToken(privKey, "", []string{"ENV_VIEW"}, 1*time.Hour, 0)
	reqForbidden := httptest.NewRequest(http.MethodPost, "/training-environments", nil)
	reqForbidden.Header.Set("Authorization", "Bearer "+viewOnlyToken)
	recForbidden := httptest.NewRecorder()
	handler(recForbidden, reqForbidden)

	if recForbidden.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for missing feature, got: %d (%s)", recForbidden.Code, recForbidden.Body.String())
	}

	// 3. Expired RS256 token (401 Unauthorized)
	expiredToken := generateRSATestToken(privKey, "", []string{"ENV_MANAGE"}, -1*time.Hour, 0)
	reqExpired := httptest.NewRequest(http.MethodPost, "/training-environments", nil)
	reqExpired.Header.Set("Authorization", "Bearer "+expiredToken)
	recExpired := httptest.NewRecorder()
	handler(recExpired, reqExpired)

	if recExpired.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for expired token, got: %d (%s)", recExpired.Code, recExpired.Body.String())
	}

	// 4. Future Nbf RS256 token (401 Unauthorized)
	futureNbfToken := generateRSATestToken(privKey, "", []string{"ENV_MANAGE"}, 1*time.Hour, 1*time.Hour)
	reqFutureNbf := httptest.NewRequest(http.MethodPost, "/training-environments", nil)
	reqFutureNbf.Header.Set("Authorization", "Bearer "+futureNbfToken)
	recFutureNbf := httptest.NewRecorder()
	handler(recFutureNbf, reqFutureNbf)

	if recFutureNbf.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for future nbf token, got: %d (%s)", recFutureNbf.Code, recFutureNbf.Body.String())
	}
}

func TestAuthMiddleware_JWKS_KeyResolution(t *testing.T) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA test key: %v", err)
	}

	nB64 := base64.RawURLEncoding.EncodeToString(privKey.PublicKey.N.Bytes())
	eBytes := []byte{1, 0, 1} // 65537
	eB64 := base64.RawURLEncoding.EncodeToString(eBytes)

	jwksJSON := fmt.Sprintf(`{
		"keys": [
			{
				"kty": "RSA",
				"kid": "sena-iam-key-1",
				"use": "sig",
				"alg": "RS256",
				"n": "%s",
				"e": "%s"
			}
		]
	}`, nB64, eB64)

	auth := middleware.NewAuthMiddleware("")
	if err := auth.LoadJWKSJSON([]byte(jwksJSON)); err != nil {
		t.Fatalf("failed to load JWKS JSON: %v", err)
	}

	handler := auth.RequireFeature("ENV_VIEW", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	token := generateRSATestToken(privKey, "sena-iam-key-1", []string{"ENV_VIEW"}, 1*time.Hour, 0)
	req := httptest.NewRequest(http.MethodGet, "/training-environments/123", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for JWKS matched token, got: %d (%s)", rec.Code, rec.Body.String())
	}
}
