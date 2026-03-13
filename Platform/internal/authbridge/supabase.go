package authbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sipeed/pinchbot/pkg/platformapi"
)

type Client struct {
	baseURL string
	anonKey string
	client  *http.Client
}

type authPayload struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int64        `json:"expires_in"`
	User         authUser     `json:"user"`
	Session      *authSession `json:"session"`
}

type authSession struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresIn    int64    `json:"expires_in"`
	User         authUser `json:"user"`
}

type authUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func NewClient(baseURL, anonKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		anonKey: strings.TrimSpace(anonKey),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Login(ctx context.Context, req platformapi.AuthRequest) (platformapi.Session, error) {
	payload, err := c.exchange(ctx, "/auth/v1/token?grant_type=password", req)
	if err != nil {
		return platformapi.Session{}, err
	}
	session, ok := payload.toSession()
	if ok {
		return session, nil
	}
	return platformapi.Session{}, &platformapi.APIError{
		StatusCode: http.StatusBadGateway,
		Message:    "Supabase login did not return a usable session",
	}
}

func (c *Client) SignUp(ctx context.Context, req platformapi.AuthRequest) (platformapi.Session, error) {
	payload, err := c.exchange(ctx, "/auth/v1/signup", req)
	if err != nil {
		return platformapi.Session{}, err
	}
	if session, ok := payload.toSession(); ok {
		return session, nil
	}

	session, err := c.Login(ctx, req)
	if err == nil {
		return session, nil
	}
	return platformapi.Session{}, missingSignupSessionError(err)
}

func (c *Client) exchange(ctx context.Context, path string, req platformapi.AuthRequest) (authPayload, error) {
	if c.baseURL == "" || c.anonKey == "" {
		return authPayload{}, fmt.Errorf("supabase auth bridge is not configured")
	}
	body, err := json.Marshal(map[string]string{
		"email":    req.Email,
		"password": req.Password,
	})
	if err != nil {
		return authPayload{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return authPayload{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("apikey", c.anonKey)
	httpReq.Header.Set("Authorization", "Bearer "+c.anonKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return authPayload{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return authPayload{}, readSupabaseAPIError(resp)
	}
	var payload authPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return authPayload{}, err
	}
	return payload, nil
}

func (p authPayload) toSession() (platformapi.Session, bool) {
	if strings.TrimSpace(p.AccessToken) == "" && p.Session != nil {
		p.AccessToken = p.Session.AccessToken
		p.RefreshToken = p.Session.RefreshToken
		p.ExpiresIn = p.Session.ExpiresIn
		if strings.TrimSpace(p.User.ID) == "" {
			p.User = p.Session.User
		}
	}
	if strings.TrimSpace(p.AccessToken) == "" || strings.TrimSpace(p.User.ID) == "" {
		return platformapi.Session{}, false
	}
	return platformapi.Session{
		AccessToken:  p.AccessToken,
		RefreshToken: p.RefreshToken,
		UserID:       p.User.ID,
		Email:        p.User.Email,
		ExpiresAt:    time.Now().Add(time.Duration(p.ExpiresIn) * time.Second).Unix(),
	}, true
}

func missingSignupSessionError(fallbackErr error) error {
	const guidance = "Supabase signup did not return a session. Disable Confirm email or allow unverified email sign-ins."

	var apiErr *platformapi.APIError
	if errors.As(fallbackErr, &apiErr) {
		message := guidance
		if detail := strings.TrimSpace(apiErr.Message); detail != "" {
			message += " Upstream login error: " + detail
		}
		status := apiErr.StatusCode
		if status == 0 {
			status = http.StatusBadRequest
		}
		return &platformapi.APIError{
			StatusCode: status,
			Message:    message,
		}
	}

	message := guidance
	if detail := strings.TrimSpace(errString(fallbackErr)); detail != "" {
		message += " Upstream login error: " + detail
	}
	return &platformapi.APIError{
		StatusCode: http.StatusBadRequest,
		Message:    message,
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
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
