package platformapi

import (
	"time"

	"github.com/sipeed/pinchbot/pkg/providers/protocoltypes"
)

type Session struct {
	AccessToken          string `json:"access_token"`
	RefreshToken         string `json:"refresh_token,omitempty"`
	UserID               string `json:"user_id"`
	Email                string `json:"email,omitempty"`
	ExpiresAt            int64  `json:"expires_at,omitempty"`
	AgreementSyncPending bool   `json:"agreement_sync_pending,omitempty"`
	Warning              string `json:"warning,omitempty"`
}

type AuthRequest struct {
	Email      string              `json:"email"`
	Password   string              `json:"password"`
	Agreements []AgreementDocument `json:"agreements,omitempty"`
}

type AuthResponse struct {
	Session               Session `json:"session"`
	AgreementSyncRequired bool    `json:"agreement_sync_required,omitempty"`
	Warning               string  `json:"warning,omitempty"`
}

type SessionView struct {
	UserID               string `json:"user_id"`
	Email                string `json:"email,omitempty"`
	ExpiresAt            int64  `json:"expires_at,omitempty"`
	AgreementSyncPending bool   `json:"agreement_sync_pending,omitempty"`
	Warning              string `json:"warning,omitempty"`
}

func (s Session) View() SessionView {
	return SessionView{
		UserID:               s.UserID,
		Email:                s.Email,
		ExpiresAt:            s.ExpiresAt,
		AgreementSyncPending: s.AgreementSyncPending,
		Warning:              s.Warning,
	}
}

func (s Session) IsExpired(now time.Time) bool {
	return s.ExpiresAt > 0 && !now.Before(time.Unix(s.ExpiresAt, 0))
}

type BrowserAuthResponse struct {
	Session SessionView `json:"session"`
	Warning string      `json:"warning,omitempty"`
}

type WalletSummary struct {
	UserID      string `json:"user_id"`
	BalanceFen  int64  `json:"balance_fen"`
	Currency    string `json:"currency"`
	UpdatedUnix int64  `json:"updated_unix"`
}

type OfficialAccessState struct {
	Enabled          bool   `json:"enabled"`
	BalanceFen       int64  `json:"balance_fen"`
	Currency         string `json:"currency,omitempty"`
	LowBalance       bool   `json:"low_balance"`
	ModelsConfigured int    `json:"models_configured,omitempty"`
}

type BackendStatus struct {
	GatewayURL      string `json:"gateway_url,omitempty"`
	GatewayHealthy  bool   `json:"gateway_healthy"`
	PlatformURL     string `json:"platform_url,omitempty"`
	PlatformHealthy bool   `json:"platform_healthy"`
	SettingsURL     string `json:"settings_url,omitempty"`
	SettingsHealthy bool   `json:"settings_healthy"`
}

type CreateOrderRequest struct {
	AmountFen int64  `json:"amount_fen"`
	Channel   string `json:"channel"`
}

type RechargeOrder struct {
	ID                string   `json:"id"`
	UserID            string   `json:"user_id"`
	AmountFen         int64    `json:"amount_fen"`
	RefundedFen       int64    `json:"refunded_fen,omitempty"`
	Channel           string   `json:"channel"`
	Provider          string   `json:"provider,omitempty"`
	Status            string   `json:"status"`
	PayURL            string   `json:"pay_url,omitempty"`
	ExternalID        string   `json:"external_id,omitempty"`
	ProviderStatus    string   `json:"provider_status,omitempty"`
	PricingVersion    string   `json:"pricing_version,omitempty"`
	AgreementVersions []string `json:"agreement_versions,omitempty"`
	CreatedUnix       int64    `json:"created_unix"`
	UpdatedUnix       int64    `json:"updated_unix,omitempty"`
	PaidUnix          int64    `json:"paid_unix,omitempty"`
	LastCheckedUnix   int64    `json:"last_checked_unix,omitempty"`
}

type WalletTransaction struct {
	ID             string `json:"id"`
	UserID         string `json:"user_id"`
	Kind           string `json:"kind"`
	AmountFen      int64  `json:"amount_fen"`
	Description    string `json:"description"`
	ReferenceType  string `json:"reference_type,omitempty"`
	ReferenceID    string `json:"reference_id,omitempty"`
	PricingVersion string `json:"pricing_version,omitempty"`
	CreatedUnix    int64  `json:"created_unix"`
}

type OfficialModel struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Enabled        bool   `json:"enabled"`
	PricingVersion string `json:"pricing_version,omitempty"`
}

type AgreementDocument struct {
	Key               string `json:"key"`
	Version           string `json:"version"`
	Title             string `json:"title"`
	Content           string `json:"content,omitempty"`
	URL               string `json:"url,omitempty"`
	EffectiveFromUnix int64  `json:"effective_from_unix,omitempty"`
}

type RefundRequest struct {
	ID               string `json:"id"`
	UserID           string `json:"user_id"`
	OrderID          string `json:"order_id"`
	AmountFen        int64  `json:"amount_fen"`
	Reason           string `json:"reason,omitempty"`
	Status           string `json:"status"`
	ReviewNote       string `json:"review_note,omitempty"`
	ReviewedBy       string `json:"reviewed_by,omitempty"`
	RefundProvider   string `json:"refund_provider,omitempty"`
	ExternalRefundID string `json:"external_refund_id,omitempty"`
	ExternalStatus   string `json:"external_status,omitempty"`
	FailureReason    string `json:"failure_reason,omitempty"`
	CreatedUnix      int64  `json:"created_unix"`
	UpdatedUnix      int64  `json:"updated_unix"`
	SettledUnix      int64  `json:"settled_unix,omitempty"`
}

type CreateRefundRequest struct {
	OrderID   string `json:"order_id"`
	AmountFen int64  `json:"amount_fen"`
	Reason    string `json:"reason,omitempty"`
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
