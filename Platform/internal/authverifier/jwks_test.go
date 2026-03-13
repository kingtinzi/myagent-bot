package authverifier

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestVerifierAcceptsSupabaseStyleECDSAJWKS(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	const kid = "supabase-ec-kid"
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{
				{
					"kid": kid,
					"kty": "EC",
					"alg": "ES256",
					"crv": "P-256",
					"x":   base64.RawURLEncoding.EncodeToString(privateKey.X.Bytes()),
					"y":   base64.RawURLEncoding.EncodeToString(privateKey.Y.Bytes()),
				},
			},
		})
	}))
	defer jwksServer.Close()

	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"sub":   "user-1",
		"email": "user@example.com",
		"aud":   "authenticated",
	})
	token.Header["kid"] = kid
	rawToken, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}

	verifier := New(jwksServer.URL, "", "authenticated")
	user, err := verifier.Verify(context.Background(), rawToken)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if user.ID != "user-1" {
		t.Fatalf("user.ID = %q, want %q", user.ID, "user-1")
	}
	if user.Email != "user@example.com" {
		t.Fatalf("user.Email = %q, want %q", user.Email, "user@example.com")
	}
}

func TestVerifierKeepsExistingKeyWhenJWKSRefreshFails(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	const kid = "supabase-ec-kid"
	failRefresh := false
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failRefresh {
			http.Error(w, "boom", http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{
				{
					"kid": kid,
					"kty": "EC",
					"alg": "ES256",
					"crv": "P-256",
					"x":   base64.RawURLEncoding.EncodeToString(privateKey.X.Bytes()),
					"y":   base64.RawURLEncoding.EncodeToString(privateKey.Y.Bytes()),
				},
			},
		})
	}))
	defer jwksServer.Close()

	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"sub":   "user-1",
		"email": "user@example.com",
		"aud":   "authenticated",
	})
	token.Header["kid"] = kid
	rawToken, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}

	verifier := New(jwksServer.URL, "", "authenticated")
	if _, err := verifier.Verify(context.Background(), rawToken); err != nil {
		t.Fatalf("initial Verify() error = %v", err)
	}

	failRefresh = true
	verifier.mu.Lock()
	verifier.expires = time.Now().Add(-time.Minute)
	verifier.mu.Unlock()

	user, err := verifier.Verify(context.Background(), rawToken)
	if err != nil {
		t.Fatalf("Verify() with stale cached key error = %v", err)
	}
	if user.ID != "user-1" {
		t.Fatalf("user.ID = %q, want %q", user.ID, "user-1")
	}
}

func TestVerifierKeepsExistingKeyWhenJWKSRefreshReturnsNoUsableKeys(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	const kid = "supabase-ec-kid"
	returnEmpty := false
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if returnEmpty {
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{
				{
					"kid": kid,
					"kty": "EC",
					"alg": "ES256",
					"crv": "P-256",
					"x":   base64.RawURLEncoding.EncodeToString(privateKey.X.Bytes()),
					"y":   base64.RawURLEncoding.EncodeToString(privateKey.Y.Bytes()),
				},
			},
		})
	}))
	defer jwksServer.Close()

	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"sub":   "user-1",
		"email": "user@example.com",
		"aud":   "authenticated",
	})
	token.Header["kid"] = kid
	rawToken, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}

	verifier := New(jwksServer.URL, "", "authenticated")
	if _, err := verifier.Verify(context.Background(), rawToken); err != nil {
		t.Fatalf("initial Verify() error = %v", err)
	}

	returnEmpty = true
	verifier.mu.Lock()
	verifier.expires = time.Now().Add(-time.Minute)
	verifier.mu.Unlock()

	user, err := verifier.Verify(context.Background(), rawToken)
	if err != nil {
		t.Fatalf("Verify() with empty refreshed JWKS error = %v", err)
	}
	if user.ID != "user-1" {
		t.Fatalf("user.ID = %q, want %q", user.ID, "user-1")
	}
}
