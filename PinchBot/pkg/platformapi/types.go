package platformapi

import "github.com/sipeed/pinchbot/pkg/providers/protocoltypes"

type Session struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	UserID       string `json:"user_id"`
	Email        string `json:"email,omitempty"`
	ExpiresAt    int64  `json:"expires_at,omitempty"`
}

type AuthRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Session Session `json:"session"`
}

type SessionView struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

func (s Session) View() SessionView {
	return SessionView{
		UserID:    s.UserID,
		Email:     s.Email,
		ExpiresAt: s.ExpiresAt,
	}
}

type BrowserAuthResponse struct {
	Session SessionView `json:"session"`
}

type WalletSummary struct {
	UserID      string `json:"user_id"`
	BalanceFen  int64  `json:"balance_fen"`
	Currency    string `json:"currency"`
	UpdatedUnix int64  `json:"updated_unix"`
}

type CreateOrderRequest struct {
	AmountFen int64  `json:"amount_fen"`
	Channel   string `json:"channel"`
}

type RechargeOrder struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	AmountFen   int64  `json:"amount_fen"`
	Channel     string `json:"channel"`
	Provider    string `json:"provider,omitempty"`
	Status      string `json:"status"`
	PayURL      string `json:"pay_url,omitempty"`
	ExternalID  string `json:"external_id,omitempty"`
	CreatedUnix int64  `json:"created_unix"`
}

type WalletTransaction struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	Kind        string `json:"kind"`
	AmountFen   int64  `json:"amount_fen"`
	Description string `json:"description"`
	CreatedUnix int64  `json:"created_unix"`
}

type OfficialModel struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Enabled        bool   `json:"enabled"`
	PricingVersion string `json:"pricing_version,omitempty"`
}

type AgreementDocument struct {
	Key     string `json:"key"`
	Version string `json:"version"`
	Title   string `json:"title"`
	Content string `json:"content,omitempty"`
	URL     string `json:"url,omitempty"`
}

type AcceptAgreementsRequest struct {
	Agreements []AgreementDocument `json:"agreements"`
}

type ChatProxyRequest struct {
	ModelID  string                         `json:"model_id"`
	Messages []protocoltypes.Message        `json:"messages"`
	Tools    []protocoltypes.ToolDefinition `json:"tools,omitempty"`
	Options  map[string]any                 `json:"options,omitempty"`
}

type ChatProxyResponse struct {
	Response       protocoltypes.LLMResponse `json:"response"`
	ChargedFen     int64                     `json:"charged_fen"`
	PricingVersion string                    `json:"pricing_version,omitempty"`
}
