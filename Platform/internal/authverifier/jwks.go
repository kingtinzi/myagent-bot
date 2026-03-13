package authverifier

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
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
	keys     map[string]any
	expires  time.Time
}

func New(jwksURL, secret, audience string) *Verifier {
	return &Verifier{
		jwksURL:  strings.TrimSpace(jwksURL),
		secret:   strings.TrimSpace(secret),
		audience: strings.TrimSpace(audience),
		client:   &http.Client{Timeout: 15 * time.Second},
		keys:     map[string]any{},
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

func (v *Verifier) lookupKey(ctx context.Context, kid string) (any, error) {
	v.mu.RLock()
	key, ok := v.keys[kid]
	fresh := time.Now().Before(v.expires)
	v.mu.RUnlock()
	if ok && fresh {
		return key, nil
	}
	if err := v.refreshKeys(ctx); err != nil {
		if ok {
			return key, nil
		}
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
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks endpoint returned %d", resp.StatusCode)
	}
	var payload struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
			Crv string `json:"crv"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	keys := make(map[string]any, len(payload.Keys))
	for _, item := range payload.Keys {
		if item.Kid == "" {
			continue
		}
		switch item.Kty {
		case "RSA":
			key, err := parseRSAKey(item.N, item.E)
			if err != nil {
				continue
			}
			keys[item.Kid] = key
		case "EC":
			key, err := parseECKey(item.Crv, item.X, item.Y)
			if err != nil {
				continue
			}
			keys[item.Kid] = key
		}
	}
	if len(keys) == 0 {
		return fmt.Errorf("jwks endpoint returned no usable keys")
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

func parseECKey(crv, xEnc, yEnc string) (*ecdsa.PublicKey, error) {
	curve, err := namedCurve(crv)
	if err != nil {
		return nil, err
	}
	xBytes, err := base64.RawURLEncoding.DecodeString(xEnc)
	if err != nil {
		return nil, err
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(yEnc)
	if err != nil {
		return nil, err
	}
	key := &ecdsa.PublicKey{
		Curve: curve,
		X:     new(big.Int).SetBytes(xBytes),
		Y:     new(big.Int).SetBytes(yBytes),
	}
	if !curve.IsOnCurve(key.X, key.Y) {
		return nil, fmt.Errorf("ec jwk point is not on curve %s", crv)
	}
	return key, nil
}

func namedCurve(crv string) (elliptic.Curve, error) {
	switch crv {
	case "P-256":
		return elliptic.P256(), nil
	case "P-384":
		return elliptic.P384(), nil
	case "P-521":
		return elliptic.P521(), nil
	default:
		return nil, fmt.Errorf("unsupported ec curve %q", crv)
	}
}
