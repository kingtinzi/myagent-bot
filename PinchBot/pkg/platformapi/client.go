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

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) != "" {
		return fmt.Sprintf("platform api returned %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("platform api returned %d", e.StatusCode)
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
