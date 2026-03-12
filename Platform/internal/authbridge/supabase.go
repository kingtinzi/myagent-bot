package authbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/platformapi"
)

type Client struct {
	baseURL string
	anonKey string
	client  *http.Client
}

func NewClient(baseURL, anonKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		anonKey: strings.TrimSpace(anonKey),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Login(ctx context.Context, req platformapi.AuthRequest) (platformapi.Session, error) {
	return c.exchange(ctx, "/auth/v1/token?grant_type=password", req)
}

func (c *Client) SignUp(ctx context.Context, req platformapi.AuthRequest) (platformapi.Session, error) {
	return c.exchange(ctx, "/auth/v1/signup", req)
}

func (c *Client) exchange(ctx context.Context, path string, req platformapi.AuthRequest) (platformapi.Session, error) {
	if c.baseURL == "" || c.anonKey == "" {
		return platformapi.Session{}, fmt.Errorf("supabase auth bridge is not configured")
	}
	body, err := json.Marshal(map[string]string{
		"email":    req.Email,
		"password": req.Password,
	})
	if err != nil {
		return platformapi.Session{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return platformapi.Session{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("apikey", c.anonKey)
	httpReq.Header.Set("Authorization", "Bearer "+c.anonKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return platformapi.Session{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return platformapi.Session{}, readSupabaseAPIError(resp)
	}
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		User         struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return platformapi.Session{}, err
	}
	return platformapi.Session{
		AccessToken:  payload.AccessToken,
		RefreshToken: payload.RefreshToken,
		UserID:       payload.User.ID,
		Email:        payload.User.Email,
		ExpiresAt:    time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second).Unix(),
	}, nil
}

func readSupabaseAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	message := extractSupabaseErrorMessage(body)
	if message == "" {
		message = fmt.Sprintf("supabase auth returned %d", resp.StatusCode)
	}
	return &platformapi.APIError{
		StatusCode: resp.StatusCode,
		Message:    message,
	}
}

func extractSupabaseErrorMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		for _, key := range []string{"message", "error_description", "msg", "error"} {
			value := strings.TrimSpace(asString(payload[key]))
			if value != "" {
				return value
			}
		}
	}

	return trimmed
}

func asString(value any) string {
	text, _ := value.(string)
	return text
}
