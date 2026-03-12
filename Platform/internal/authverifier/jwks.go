package authverifier

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"openclaw/platform/internal/api"
)

type Verifier struct {
	jwksURL  string
	secret   string
	audience string
	client   *http.Client
	mu       sync.RWMutex
	keys     map[string]*rsa.PublicKey
	expires  time.Time
}

func New(jwksURL, secret, audience string) *Verifier {
	return &Verifier{
		jwksURL:  strings.TrimSpace(jwksURL),
		secret:   strings.TrimSpace(secret),
		audience: strings.TrimSpace(audience),
		client:   &http.Client{Timeout: 15 * time.Second},
		keys:     map[string]*rsa.PublicKey{},
	}
}

func (v *Verifier) Verify(ctx context.Context, bearerToken string) (api.AuthUser, error) {
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(bearerToken, claims, v.keyFunc(ctx))
	if err != nil || !parsed.Valid {
		return api.AuthUser{}, fmt.Errorf("invalid jwt: %w", err)
	}
	if v.audience != "" && !hasAudience(claims["aud"], v.audience) {
		return api.AuthUser{}, fmt.Errorf("invalid audience")
	}
	userID, _ := claims["sub"].(string)
	email, _ := claims["email"].(string)
	if userID == "" {
		return api.AuthUser{}, fmt.Errorf("missing sub claim")
	}
	return api.AuthUser{ID: userID, Email: email}, nil
}

func hasAudience(raw any, want string) bool {
	switch aud := raw.(type) {
	case string:
		return aud == want
	case []any:
		for _, item := range aud {
			if s, ok := item.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

func (v *Verifier) keyFunc(ctx context.Context) jwt.Keyfunc {
	return func(token *jwt.Token) (any, error) {
		if v.secret != "" {
			return []byte(v.secret), nil
		}
		kid, _ := token.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("missing kid")
		}
		key, err := v.lookupKey(ctx, kid)
		if err != nil {
			return nil, err
		}
		return key, nil
	}
}

func (v *Verifier) lookupKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	key, ok := v.keys[kid]
	fresh := time.Now().Before(v.expires)
	v.mu.RUnlock()
	if ok && fresh {
		return key, nil
	}
	if err := v.refreshKeys(ctx); err != nil {
		return nil, err
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	key, ok = v.keys[kid]
	if !ok {
		return nil, fmt.Errorf("kid %q not found", kid)
	}
	return key, nil
}

func (v *Verifier) refreshKeys(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var payload struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	keys := make(map[string]*rsa.PublicKey, len(payload.Keys))
	for _, item := range payload.Keys {
		if item.Kty != "RSA" || item.Kid == "" {
			continue
		}
		key, err := parseRSAKey(item.N, item.E)
		if err != nil {
			continue
		}
		keys[item.Kid] = key
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.keys = keys
	v.expires = time.Now().Add(10 * time.Minute)
	return nil
}

func parseRSAKey(nEnc, eEnc string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nEnc)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eEnc)
	if err != nil {
		return nil, err
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: e,
	}, nil
}
