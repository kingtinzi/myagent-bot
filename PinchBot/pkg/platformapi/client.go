package platformapi

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
)

type APIError struct {
	StatusCode int
	Message    string
}

func NormalizeUserFacingErrorMessage(message string) string {
	trimmed := strings.TrimSpace(message)
	switch trimmed {
	case "":
		return ""
	case "Invalid login credentials":
		return "邮箱或密码错误"
	case "invalid json":
		return "请求格式错误，请检查后重试"
	case "authentication service unavailable", "auth bridge not configured":
		return "认证服务暂不可用，请稍后重试"
	case "not logged in", "invalid bearer token":
		return "未登录或登录已过期，请重新登录"
	case "login did not return an administrator session", "failed to verify administrator session", "authentication service did not return a valid session":
		return "认证服务未返回有效会话，请稍后重试"
	case "signup succeeded, but agreement sync must be retried before recharge":
		return "注册已成功，但协议确认同步失败，请在充值前重新确认协议"
	case "Supabase signup did not return a session. Disable Confirm email or allow unverified email sign-ins.":
		return "注册成功后未返回会话。请在 Supabase 中关闭“Confirm email”，或允许未验证邮箱直接登录。"
	default:
		lower := strings.ToLower(trimmed)
		switch {
		case strings.Contains(lower, "must be accepted before signup"):
			return "注册前请先阅读并同意当前注册协议"
		case strings.Contains(lower, "must be accepted before recharge"):
			return "充值前请先确认当前充值协议"
		default:
			return trimmed
		}
	}
}

func UserFacingErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode >= http.StatusInternalServerError && looksLikeTransportFailureMessage(apiErr.Message) {
			return "平台服务暂不可用，请稍后重试"
		}
		if msg := NormalizeUserFacingErrorMessage(apiErr.Message); msg != "" {
			return msg
		}
		switch {
		case apiErr.StatusCode == http.StatusUnauthorized:
			return "未登录或登录已过期，请重新登录"
		case apiErr.StatusCode >= http.StatusInternalServerError:
			return "服务暂不可用，请稍后重试"
		case apiErr.StatusCode >= http.StatusBadRequest:
			return "请求失败，请检查后重试"
		}
	}
	if msg := NormalizeUserFacingErrorMessage(err.Error()); msg != "" {
		return msg
	}
	return "服务暂不可用，请稍后重试"
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) != "" {
		return fmt.Sprintf("platform api returned %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("platform api returned %d", e.StatusCode)
}

func looksLikeTransportFailureMessage(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	for _, marker := range []string{
		"dial tcp",
		"connection refused",
		"actively refused",
		"connectex",
		"no connection could be made",
		"lookup ",
		"no such host",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func IsStatusCode(err error, statusCode int) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == statusCode
}

type Client struct {
	baseURL string
	client  *http.Client
}

func NewClient(baseURL string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return &Client{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) BaseURL() string {
	if c == nil {
		return ""
	}
	return c.baseURL
}

// Login returns only the created session.
//
// Deprecated: prefer LoginResponse so callers keep any future auth metadata
// instead of silently discarding it.
func (c *Client) Login(ctx context.Context, req AuthRequest) (Session, error) {
	resp, err := c.LoginResponse(ctx, req)
	if err != nil {
		return Session{}, err
	}
	return resp.Session, nil
}

// LoginResponse returns the complete authentication response.
func (c *Client) LoginResponse(ctx context.Context, req AuthRequest) (AuthResponse, error) {
	var resp AuthResponse
	if err := c.doJSON(ctx, http.MethodPost, "/auth/login", "", req, &resp); err != nil {
		return AuthResponse{}, err
	}
	return resp, nil
}

// SignUp returns only the created session.
//
// Deprecated: prefer SignUpResponse so callers can handle agreement recovery
// metadata such as AgreementSyncRequired and Warning.
func (c *Client) SignUp(ctx context.Context, req AuthRequest) (Session, error) {
	resp, err := c.SignUpResponse(ctx, req)
	if err != nil {
		return Session{}, err
	}
	return resp.Session, nil
}

// SignUpResponse returns the complete signup response, including recovery metadata.
func (c *Client) SignUpResponse(ctx context.Context, req AuthRequest) (AuthResponse, error) {
	var resp AuthResponse
	if err := c.doJSON(ctx, http.MethodPost, "/auth/signup", "", req, &resp); err != nil {
		return AuthResponse{}, err
	}
	return resp, nil
}

func (c *Client) GetMe(ctx context.Context, accessToken string) (SessionView, error) {
	var resp BrowserAuthResponse
	if err := c.doJSON(ctx, http.MethodGet, "/me", accessToken, nil, &resp); err != nil {
		return SessionView{}, err
	}
	return resp.Session, nil
}

func (c *Client) Logout(ctx context.Context, accessToken string) error {
	return c.doJSON(ctx, http.MethodPost, "/auth/logout", accessToken, nil, nil)
}

func (c *Client) GetWallet(ctx context.Context, accessToken string) (WalletSummary, error) {
	var wallet WalletSummary
	if err := c.doJSON(ctx, http.MethodGet, "/wallet", accessToken, nil, &wallet); err != nil {
		return WalletSummary{}, err
	}
	return wallet, nil
}

func (c *Client) GetOfficialAccessState(ctx context.Context, accessToken string) (OfficialAccessState, error) {
	var state OfficialAccessState
	if err := c.doJSON(ctx, http.MethodGet, "/official/access", accessToken, nil, &state); err != nil {
		return OfficialAccessState{}, err
	}
	return state, nil
}

func (c *Client) ListOfficialModels(ctx context.Context, accessToken string) ([]OfficialModel, error) {
	var models []OfficialModel
	if err := c.doJSON(ctx, http.MethodGet, "/official/models", accessToken, nil, &models); err != nil {
		return nil, err
	}
	return models, nil
}

func (c *Client) ListAgreements(ctx context.Context, accessToken string) ([]AgreementDocument, error) {
	var docs []AgreementDocument
	if err := c.doJSON(ctx, http.MethodGet, "/agreements/current", accessToken, nil, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

func (c *Client) AcceptAgreements(ctx context.Context, accessToken string, req AcceptAgreementsRequest) error {
	return c.doJSON(ctx, http.MethodPost, "/agreements/accept", accessToken, req, nil)
}

func (c *Client) ListTransactions(ctx context.Context, accessToken string) ([]WalletTransaction, error) {
	var items []WalletTransaction
	if err := c.doJSON(ctx, http.MethodGet, "/wallet/transactions", accessToken, nil, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (c *Client) CreateOrder(ctx context.Context, accessToken string, req CreateOrderRequest) (RechargeOrder, error) {
	var order RechargeOrder
	if err := c.doJSON(ctx, http.MethodPost, "/wallet/orders", accessToken, req, &order); err != nil {
		return RechargeOrder{}, err
	}
	return order, nil
}

func (c *Client) GetOrder(ctx context.Context, accessToken, orderID string) (RechargeOrder, error) {
	var order RechargeOrder
	if err := c.doJSON(ctx, http.MethodGet, "/wallet/orders/"+strings.TrimSpace(orderID), accessToken, nil, &order); err != nil {
		return RechargeOrder{}, err
	}
	return order, nil
}

func (c *Client) ListRefundRequests(ctx context.Context, accessToken string) ([]RefundRequest, error) {
	var items []RefundRequest
	if err := c.doJSON(ctx, http.MethodGet, "/wallet/refund-requests", accessToken, nil, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (c *Client) CreateRefundRequest(ctx context.Context, accessToken string, req CreateRefundRequest) (RefundRequest, error) {
	var item RefundRequest
	if err := c.doJSON(ctx, http.MethodPost, "/wallet/refund-requests", accessToken, req, &item); err != nil {
		return RefundRequest{}, err
	}
	return item, nil
}

func (c *Client) doJSON(
	ctx context.Context,
	method, path, accessToken string,
	requestBody any,
	responseBody any,
) error {
	var body bytes.Buffer
	if requestBody != nil {
		if err := json.NewEncoder(&body).Encode(requestBody); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, &body)
	if err != nil {
		return err
	}
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(accessToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		message := strings.TrimSpace(string(body))
		return &APIError{StatusCode: resp.StatusCode, Message: message}
	}
	if responseBody == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(responseBody)
}
