package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/pinchbot/pkg/platformapi"

	"openclaw/platform/internal/payments"
)

var (
	ErrInvalidAmount          = errors.New("amount_fen must be positive")
	ErrUnknownModel           = errors.New("unknown official model")
	ErrModelDisabled          = errors.New("official model disabled")
	ErrInsufficientFunds      = errors.New("insufficient wallet balance")
	ErrInvalidAgreement       = errors.New("invalid agreement document")
	ErrUnknownAgreement       = errors.New("unknown agreement document")
	ErrCallbackAmount         = errors.New("callback amount does not match recharge order")
	ErrRefundNotAllowed       = errors.New("refund not allowed")
	ErrAdminAccessDenied      = errors.New("admin access required")
	ErrAdminCapabilityDenied  = errors.New("admin capability denied")
	ErrInvalidAdminRole       = errors.New("invalid admin role")
	ErrInvalidAdminCapability = errors.New("invalid admin capability")
	ErrRevisionConflict       = errors.New("revision conflict")
	ErrInvalidRequestID       = errors.New("request_id is required")
	ErrIdempotencyConflict    = errors.New("request_id conflicts with a different admin wallet mutation")
	ErrUserNotFound           = errors.New("admin user overview not found")
)

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type ChatInput struct {
	ModelID string `json:"model_id"`
	Message string `json:"message"`
}

type ChatResult struct {
	Content string `json:"content"`
	Usage   Usage  `json:"usage"`
}

type PricingRule struct {
	ModelID                string `json:"model_id"`
	Version                string `json:"version,omitempty"`
	EffectiveFromUnix      int64  `json:"effective_from_unix,omitempty"`
	InputPriceMicrosPer1K  int64  `json:"input_price_micros_per_1k"`
	OutputPriceMicrosPer1K int64  `json:"output_price_micros_per_1k"`
	FallbackPriceFen       int64  `json:"fallback_price_fen"`
}

type OfficialModel struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
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

type WalletSummary struct {
	UserID      string `json:"user_id"`
	BalanceFen  int64  `json:"balance_fen"`
	Currency    string `json:"currency"`
	UpdatedUnix int64  `json:"updated_unix"`
}

type CreateOrderInput struct {
	AmountFen int64  `json:"amount_fen"`
	Channel   string `json:"channel"`
}

type RechargeOrder struct {
	ID                string   `json:"id"`
	UserID            string   `json:"user_id"`
	UserNo            int64    `json:"user_no,omitempty"`
	Username          string   `json:"username,omitempty"`
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
	UserNo         int64  `json:"user_no,omitempty"`
	Username       string `json:"username,omitempty"`
	Kind           string `json:"kind"`
	AmountFen      int64  `json:"amount_fen"`
	Description    string `json:"description"`
	ReferenceType  string `json:"reference_type,omitempty"`
	ReferenceID    string `json:"reference_id,omitempty"`
	PricingVersion string `json:"pricing_version,omitempty"`
	CreatedUnix    int64  `json:"created_unix"`
}

type ChatClient interface {
	Chat(ctx context.Context, input ChatInput) (ChatResult, error)
}

type OfficialProxyClient interface {
	ProxyChat(ctx context.Context, userID string, request platformapi.ChatProxyRequest) (platformapi.ChatProxyResponse, error)
}

type Store interface {
	GetWallet(ctx context.Context, userID string) (WalletSummary, error)
	SetBalance(userID string, balanceFen int64)
	AppendTransaction(ctx context.Context, tx WalletTransaction) error
	ListTransactions(ctx context.Context, userID string) ([]WalletTransaction, error)
	SaveOrder(ctx context.Context, order RechargeOrder) error
	ListOrders(ctx context.Context) ([]RechargeOrder, error)
	GetOrder(ctx context.Context, userID, orderID string) (RechargeOrder, error)
	FindOrderByID(ctx context.Context, orderID string) (RechargeOrder, error)
	MarkOrderPaid(ctx context.Context, orderID, provider, externalID string) (RechargeOrder, bool, error)
	FinalizeRechargeOrder(ctx context.Context, orderID, provider, externalID, description string) (RechargeOrder, WalletSummary, bool, error)
	Credit(ctx context.Context, userID string, amountFen int64, description string) (WalletSummary, error)
	Debit(ctx context.Context, userID string, amountFen int64, description string) (WalletSummary, error)
	UpsertAdminEmails(ctx context.Context, emails []string) error
	IsAdminUser(ctx context.Context, userID, email string) (bool, error)
	GetAdminOperator(ctx context.Context, userID, email string) (AdminOperator, error)
	ListAdminOperators(ctx context.Context) ([]AdminOperator, error)
	SaveAdminOperator(ctx context.Context, operator AdminOperator) (AdminOperator, error)
	RecordAgreementAcceptance(ctx context.Context, acceptance AgreementAcceptance) error
	HasAgreementAcceptance(ctx context.Context, userID, key, version string) (bool, error)
	ListAgreementAcceptances(ctx context.Context, userID string) ([]AgreementAcceptance, error)
	RecordChatUsage(ctx context.Context, usage ChatUsageRecord) error
	ListChatUsageRecords(ctx context.Context, filter ChatUsageRecordFilter) ([]ChatUsageRecord, error)
	UpsertUserIdentity(ctx context.Context, identity UserIdentity) error
	ApplyWalletAdjustment(ctx context.Context, tx WalletTransaction) (WalletSummary, error)
	ApplyAdminWalletMutation(ctx context.Context, tx WalletTransaction, audit AdminAuditLog) (WalletSummary, bool, error)
	ListUsers(ctx context.Context) ([]UserSummary, error)
	ListWalletAdjustments(ctx context.Context) ([]WalletTransaction, error)
	AppendAuditLog(ctx context.Context, entry AdminAuditLog) error
	ListAuditLogs(ctx context.Context, filter AuditLogFilter) ([]AdminAuditLog, error)
	CreateRefundRequest(ctx context.Context, request RefundRequest) error
	SaveRefundRequest(ctx context.Context, request RefundRequest) error
	GetRefundRequest(ctx context.Context, requestID string) (RefundRequest, error)
	ListRefundRequests(ctx context.Context, userID string) ([]RefundRequest, error)
	ApplyRefundDecision(ctx context.Context, requestID string, input RefundDecisionInput, updatedUnix int64) (RefundRequest, error)
	CreateInfringementReport(ctx context.Context, report InfringementReport) error
	SaveInfringementReport(ctx context.Context, report InfringementReport) error
	GetInfringementReport(ctx context.Context, reportID string) (InfringementReport, error)
	ListInfringementReports(ctx context.Context, userID string) ([]InfringementReport, error)
	ListDataRetentionPolicies(ctx context.Context) ([]DataRetentionPolicy, error)
	SaveDataRetentionPolicies(ctx context.Context, policies []DataRetentionPolicy) error
	ListSystemNotices(ctx context.Context) ([]SystemNotice, error)
	SaveSystemNotices(ctx context.Context, notices []SystemNotice) error
	ListRiskRules(ctx context.Context) ([]RiskRule, error)
	SaveRiskRules(ctx context.Context, rules []RiskRule) error
}

type Service struct {
	store             Store
	chat              ChatClient
	proxy             OfficialProxyClient
	payment           payments.Provider
	pricing           map[string]PricingRule
	pricingCatalog    []PricingRule
	models            []OfficialModel
	currentAgreements []AgreementDocument
	agreementCatalog  []AgreementDocument
	now               func() time.Time
	mu                sync.RWMutex
}

func NewService(store Store, chat ChatClient) *Service {
	return &Service{
		store:   store,
		chat:    chat,
		payment: payments.ManualProvider{},
		pricing: map[string]PricingRule{},
		now:     time.Now,
	}
}

func (s *Service) SetOfficialProxyClient(client OfficialProxyClient) {
	s.proxy = client
}

func (s *Service) SetPaymentProvider(provider payments.Provider) error {
	if provider == nil {
		s.payment = payments.ManualProvider{}
		return nil
	}
	s.payment = provider
	return nil
}

func (s *Service) PaymentProvider() payments.Provider {
	return s.payment
}

func (s *Service) SetOfficialModels(models []OfficialModel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := append([]OfficialModel(nil), models...)
	for i := range cloned {
		if cloned[i].PricingVersion == "" {
			if rule, ok := s.pricing[cloned[i].ID]; ok {
				cloned[i].PricingVersion = normalizedPricingVersion(rule)
			}
		}
	}
	s.models = cloned
}

func (s *Service) ListOfficialModels(ctx context.Context) []OfficialModel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := append([]OfficialModel(nil), s.models...)
	if items == nil {
		return []OfficialModel{}
	}
	return items
}

func (s *Service) ListEnabledOfficialModels(ctx context.Context) []OfficialModel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]OfficialModel, 0, len(s.models))
	for _, model := range s.models {
		rule, ok := s.pricing[model.ID]
		if model.Enabled && ok {
			if model.PricingVersion == "" {
				model.PricingVersion = normalizedPricingVersion(rule)
			}
			items = append(items, model)
		}
	}
	return items
}

func (s *Service) SetAgreement(doc AgreementDocument) {
	s.SetAgreements([]AgreementDocument{doc})
}

func (s *Service) SetAgreements(docs []AgreementDocument) {
	s.mu.Lock()
	defer s.mu.Unlock()
	catalog := normalizeAgreementCatalog(docs)
	s.agreementCatalog = catalog
	s.currentAgreements = selectCurrentAgreements(s.now(), catalog)
}

func (s *Service) ListAgreements(ctx context.Context) []AgreementDocument {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := append([]AgreementDocument(nil), s.currentAgreements...)
	if items == nil {
		return []AgreementDocument{}
	}
	return items
}

func (s *Service) ListAgreementVersions(ctx context.Context) []AgreementDocument {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := append([]AgreementDocument(nil), s.agreementCatalog...)
	if items == nil {
		return []AgreementDocument{}
	}
	return items
}

func (s *Service) SetPricingRules(rules map[string]PricingRule) {
	catalog := make([]PricingRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Version == "" {
			rule.Version = "v1"
		}
		catalog = append(catalog, rule)
	}
	s.SetPricingCatalog(catalog)
}

func (s *Service) SetPricingCatalog(rules []PricingRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pricingCatalog = normalizePricingCatalog(rules)
	s.pricing = make(map[string]PricingRule, len(s.pricingCatalog))
	for _, rule := range s.pricingCatalog {
		active, ok := selectActivePricingRule(s.now(), rule.ModelID, s.pricingCatalog)
		if ok {
			s.pricing[rule.ModelID] = active
		}
	}
	for i := range s.models {
		if active, ok := s.pricing[s.models[i].ID]; ok {
			s.models[i].PricingVersion = normalizedPricingVersion(active)
		}
	}
}

func (s *Service) ListPricingRules() []PricingRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := append([]PricingRule(nil), s.pricingCatalog...)
	if items == nil {
		return []PricingRule{}
	}
	return items
}

func (s *Service) ListModelRoutes(ctx context.Context) []OfficialModel {
	return s.ListOfficialModels(ctx)
}

func (s *Service) GetWallet(ctx context.Context, userID string) (WalletSummary, error) {
	return s.store.GetWallet(ctx, userID)
}

func (s *Service) ListTransactions(ctx context.Context, userID string) ([]WalletTransaction, error) {
	return s.store.ListTransactions(ctx, userID)
}

func (s *Service) CreateRechargeOrder(ctx context.Context, userID string, input CreateOrderInput) (RechargeOrder, error) {
	if input.AmountFen <= 0 {
		return RechargeOrder{}, ErrInvalidAmount
	}
	order := RechargeOrder{
		ID:                fmt.Sprintf("ord_%d", s.now().UnixNano()),
		UserID:            userID,
		AmountFen:         input.AmountFen,
		Channel:           input.Channel,
		Status:            "pending",
		PricingVersion:    s.currentPricingCatalogVersion(),
		AgreementVersions: s.currentAgreementVersionList(),
		CreatedUnix:       s.now().Unix(),
		UpdatedUnix:       s.now().Unix(),
	}
	if s.payment == nil {
		s.payment = payments.ManualProvider{}
	}
	paymentOrder, err := s.payment.CreateOrder(ctx, payments.CreateOrderInput{
		OrderID:   order.ID,
		AmountFen: order.AmountFen,
	})
	if err != nil {
		return RechargeOrder{}, err
	}
	order.Provider = paymentOrder.Provider
	order.Status = paymentOrder.Status
	order.PayURL = paymentOrder.PayURL
	order.ExternalID = paymentOrder.ExternalOrderID
	if err := s.store.SaveOrder(ctx, order); err != nil {
		return RechargeOrder{}, err
	}
	_ = s.appendAuditLog(ctx, AdminAuditLog{
		Action:      "wallet.order.created",
		ActorUserID: userID,
		TargetType:  "recharge_order",
		TargetID:    order.ID,
		Detail:      fmt.Sprintf("amount_fen=%d channel=%s pricing_version=%s", order.AmountFen, order.Channel, order.PricingVersion),
		CreatedUnix: s.now().Unix(),
	})
	return order, nil
}

func (s *Service) GetOrder(ctx context.Context, userID, orderID string) (RechargeOrder, error) {
	return s.store.GetOrder(ctx, userID, orderID)
}

func (s *Service) ProxyOfficialChat(ctx context.Context, userID string, input ChatInput) (ChatResult, error) {
	if err := s.ensureOfficialModelEnabled(input.ModelID); err != nil {
		return ChatResult{}, err
	}
	rule, ok := s.getPricingRule(input.ModelID)
	if !ok {
		return ChatResult{}, ErrUnknownModel
	}
	wallet, err := s.store.GetWallet(ctx, userID)
	if err != nil {
		return ChatResult{}, err
	}
	if wallet.BalanceFen <= 0 {
		return ChatResult{}, ErrInsufficientFunds
	}
	if s.chat == nil {
		return ChatResult{}, errors.New("chat client not configured")
	}
	reservedFen := reserveCharge(rule)
	if wallet.BalanceFen < reservedFen {
		return ChatResult{}, ErrInsufficientFunds
	}
	if _, err := s.store.Debit(ctx, userID, reservedFen, "official model usage reserve"); err != nil {
		return ChatResult{}, err
	}

	result, err := s.chat.Chat(ctx, input)
	if err != nil {
		if _, refundErr := s.store.Credit(ctx, userID, reservedFen, "official model usage reserve refund"); refundErr != nil {
			return ChatResult{}, errors.Join(err, refundErr)
		}
		return ChatResult{}, err
	}

	chargeFen := chargeFromUsage(rule, result.Usage)
	fallbackApplied := false
	if chargeFen == 0 {
		chargeFen = rule.FallbackPriceFen
		fallbackApplied = true
	}
	if chargeFen <= 0 {
		chargeFen = 1
	}
	if err := s.settleReservedCharge(ctx, userID, reservedFen, chargeFen, "official model usage"); err != nil {
		return ChatResult{}, err
	}
	_ = s.store.RecordChatUsage(ctx, ChatUsageRecord{
		ID:                fmt.Sprintf("usage_%d", s.now().UnixNano()),
		UserID:            userID,
		ModelID:           input.ModelID,
		PricingVersion:    normalizedPricingVersion(rule),
		PromptTokens:      result.Usage.PromptTokens,
		CompletionTokens:  result.Usage.CompletionTokens,
		ChargedFen:        chargeFen,
		FallbackApplied:   fallbackApplied,
		RequestKind:       "direct",
		CreatedUnix:       s.now().Unix(),
		AgreementVersions: s.currentAgreementVersionList(),
	})
	return result, nil
}

func (s *Service) ProxyOfficialChatRequest(
	ctx context.Context,
	userID string,
	request platformapi.ChatProxyRequest,
) (platformapi.ChatProxyResponse, error) {
	if s.proxy == nil {
		return platformapi.ChatProxyResponse{}, errors.New("official proxy client not configured")
	}
	if err := s.ensureOfficialModelEnabled(request.ModelID); err != nil {
		return platformapi.ChatProxyResponse{}, err
	}
	rule, ok := s.getPricingRule(request.ModelID)
	if !ok {
		return platformapi.ChatProxyResponse{}, ErrUnknownModel
	}
	wallet, err := s.store.GetWallet(ctx, userID)
	if err != nil {
		return platformapi.ChatProxyResponse{}, err
	}
	if wallet.BalanceFen <= 0 {
		return platformapi.ChatProxyResponse{}, ErrInsufficientFunds
	}
	reservedFen := reserveCharge(rule)
	if wallet.BalanceFen < reservedFen {
		return platformapi.ChatProxyResponse{}, ErrInsufficientFunds
	}
	if _, err := s.store.Debit(ctx, userID, reservedFen, "official platform chat reserve"); err != nil {
		return platformapi.ChatProxyResponse{}, err
	}
	resp, err := s.proxy.ProxyChat(ctx, userID, request)
	if err != nil {
		if _, refundErr := s.store.Credit(ctx, userID, reservedFen, "official platform chat reserve refund"); refundErr != nil {
			return platformapi.ChatProxyResponse{}, errors.Join(err, refundErr)
		}
		return platformapi.ChatProxyResponse{}, err
	}
	chargeFen := int64(0)
	fallbackApplied := false
	if resp.Response.Usage != nil {
		chargeFen = chargeFromUsage(rule, Usage{
			PromptTokens:     resp.Response.Usage.PromptTokens,
			CompletionTokens: resp.Response.Usage.CompletionTokens,
		})
	}
	if chargeFen == 0 {
		chargeFen = rule.FallbackPriceFen
		fallbackApplied = true
	}
	if chargeFen <= 0 {
		chargeFen = 1
	}
	if err := s.settleReservedCharge(ctx, userID, reservedFen, chargeFen, "official platform chat"); err != nil {
		return platformapi.ChatProxyResponse{}, err
	}
	resp.ChargedFen = chargeFen
	resp.PricingVersion = normalizedPricingVersion(rule)
	_ = s.store.RecordChatUsage(ctx, ChatUsageRecord{
		ID:                fmt.Sprintf("usage_%d", s.now().UnixNano()),
		UserID:            userID,
		ModelID:           request.ModelID,
		PricingVersion:    normalizedPricingVersion(rule),
		PromptTokens:      usagePromptTokens(resp),
		CompletionTokens:  usageCompletionTokens(resp),
		ChargedFen:        chargeFen,
		FallbackApplied:   fallbackApplied,
		RequestKind:       "proxy",
		CreatedUnix:       s.now().Unix(),
		AgreementVersions: s.currentAgreementVersionList(),
	})
	return resp, nil
}

func (s *Service) HandleSuccessfulRecharge(
	ctx context.Context,
	orderID, provider, externalID string,
) (bool, error) {
	order, _, changed, err := s.store.FinalizeRechargeOrder(ctx, orderID, provider, externalID, "recharge order paid")
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	_ = s.appendAuditLog(ctx, AdminAuditLog{
		Action:      "wallet.order.paid",
		TargetType:  "recharge_order",
		TargetID:    order.ID,
		RiskLevel:   "medium",
		Detail:      fmt.Sprintf("provider=%s amount_fen=%d", provider, order.AmountFen),
		CreatedUnix: s.now().Unix(),
	})
	return true, nil
}

func (s *Service) HandleSuccessfulRechargeCallback(
	ctx context.Context,
	orderID, provider, externalID string,
	callbackAmountFen int64,
) (bool, error) {
	order, err := s.store.FindOrderByID(ctx, orderID)
	if err != nil {
		return false, err
	}
	if callbackAmountFen <= 0 || callbackAmountFen != order.AmountFen {
		return false, fmt.Errorf(
			"%w: order %s expects %d fen, callback reported %d fen",
			ErrCallbackAmount,
			orderID,
			order.AmountFen,
			callbackAmountFen,
		)
	}
	return s.HandleSuccessfulRecharge(ctx, orderID, provider, externalID)
}

func (s *Service) RecordAgreementAcceptances(
	ctx context.Context,
	userID string,
	docs []AgreementDocument,
	source AgreementAcceptanceSource,
) error {
	required := s.ListAgreementVersions(ctx)
	allowed := make(map[string]AgreementDocument, len(required))
	for _, doc := range required {
		allowed[doc.Key+"::"+doc.Version] = doc
	}
	for _, doc := range docs {
		key := fmt.Sprintf("%s::%s", doc.Key, doc.Version)
		if doc.Key == "" || doc.Version == "" {
			return fmt.Errorf("%w: key and version are required", ErrInvalidAgreement)
		}
		expected, ok := allowed[key]
		if !ok {
			return fmt.Errorf("%w: %s", ErrUnknownAgreement, key)
		}
		if doc.Title != expected.Title || doc.Content != expected.Content || doc.URL != expected.URL {
			return fmt.Errorf("%w: %s does not match current published content", ErrInvalidAgreement, key)
		}
		if err := s.store.RecordAgreementAcceptance(ctx, AgreementAcceptance{
			UserID:          userID,
			AgreementKey:    doc.Key,
			Version:         agreementAcceptanceVersion(expected),
			AcceptedUnix:    s.now().Unix(),
			ClientVersion:   strings.TrimSpace(source.ClientVersion),
			RemoteAddr:      strings.TrimSpace(source.RemoteAddr),
			DeviceSummary:   strings.TrimSpace(source.DeviceSummary),
			ContentChecksum: agreementAcceptanceVersion(expected),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) EnsureRechargeAgreementsAccepted(ctx context.Context, userID string) error {
	required := s.ListAgreements(ctx)
	for _, doc := range required {
		ok, err := s.store.HasAgreementAcceptance(ctx, userID, doc.Key, agreementAcceptanceVersion(doc))
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("agreement %s version %s must be accepted before recharge", doc.Key, doc.Version)
		}
	}
	return nil
}

func agreementAcceptanceVersion(doc AgreementDocument) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(doc.Version),
		strings.TrimSpace(doc.Title),
		strings.TrimSpace(doc.Content),
		strings.TrimSpace(doc.URL),
		fmt.Sprintf("%d", doc.EffectiveFromUnix),
	}, "\n")))
	return fmt.Sprintf("%s:%s", strings.TrimSpace(doc.Version), hex.EncodeToString(sum[:]))
}

func normalizedPricingVersion(rule PricingRule) string {
	if strings.TrimSpace(rule.Version) != "" {
		return strings.TrimSpace(rule.Version)
	}
	return "v1"
}

func (s *Service) getPricingRule(modelID string) (PricingRule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rule, ok := s.pricing[modelID]
	return rule, ok
}

func (s *Service) ensureOfficialModelEnabled(modelID string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.models) == 0 {
		return nil
	}
	for _, model := range s.models {
		if model.ID != modelID {
			continue
		}
		if !model.Enabled {
			return ErrModelDisabled
		}
		return nil
	}
	return ErrUnknownModel
}

func chargeFromUsage(rule PricingRule, usage Usage) int64 {
	if usage.PromptTokens <= 0 && usage.CompletionTokens <= 0 {
		return 0
	}
	totalMicros := ((int64(usage.PromptTokens) * rule.InputPriceMicrosPer1K) +
		(int64(usage.CompletionTokens) * rule.OutputPriceMicrosPer1K)) / 1000
	if totalMicros <= 0 {
		return 1
	}
	return int64(math.Ceil(float64(totalMicros) / 1_000_000.0))
}

func reserveCharge(rule PricingRule) int64 {
	if rule.FallbackPriceFen > 0 {
		return rule.FallbackPriceFen
	}
	return 1
}

func (s *Service) settleReservedCharge(
	ctx context.Context,
	userID string,
	reservedFen, chargeFen int64,
	description string,
) error {
	switch {
	case chargeFen > reservedFen:
		_, err := s.store.Debit(ctx, userID, chargeFen-reservedFen, description+" settlement")
		return err
	case chargeFen < reservedFen:
		_, err := s.store.Credit(ctx, userID, reservedFen-chargeFen, description+" refund")
		return err
	default:
		return nil
	}
}

func normalizePricingCatalog(rules []PricingRule) []PricingRule {
	grouped := make(map[string][]PricingRule)
	for _, rule := range rules {
		rule.ModelID = strings.TrimSpace(rule.ModelID)
		if rule.ModelID == "" {
			continue
		}
		if strings.TrimSpace(rule.Version) == "" {
			rule.Version = "v1"
		}
		grouped[rule.ModelID] = append(grouped[rule.ModelID], rule)
	}
	modelIDs := make([]string, 0, len(grouped))
	for modelID := range grouped {
		modelIDs = append(modelIDs, modelID)
	}
	sort.Strings(modelIDs)
	out := make([]PricingRule, 0, len(rules))
	for _, modelID := range modelIDs {
		sort.Slice(grouped[modelID], func(i, j int) bool {
			if grouped[modelID][i].EffectiveFromUnix == grouped[modelID][j].EffectiveFromUnix {
				return grouped[modelID][i].Version < grouped[modelID][j].Version
			}
			return grouped[modelID][i].EffectiveFromUnix < grouped[modelID][j].EffectiveFromUnix
		})
		out = append(out, grouped[modelID]...)
	}
	return out
}

func normalizeAgreementCatalog(docs []AgreementDocument) []AgreementDocument {
	items := append([]AgreementDocument(nil), docs...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].Key == items[j].Key {
			if items[i].EffectiveFromUnix == items[j].EffectiveFromUnix {
				return items[i].Version < items[j].Version
			}
			return items[i].EffectiveFromUnix < items[j].EffectiveFromUnix
		}
		return items[i].Key < items[j].Key
	})
	return items
}

func (s *Service) currentPricingCatalogVersion() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.pricingCatalog) == 0 {
		return ""
	}
	parts := make([]string, 0, len(s.pricing))
	keys := make([]string, 0, len(s.pricing))
	for key := range s.pricing {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, key+":"+normalizedPricingVersion(s.pricing[key]))
	}
	return strings.Join(parts, ",")
}

func (s *Service) currentAgreementVersionList() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]string, 0, len(s.currentAgreements))
	for _, doc := range s.currentAgreements {
		items = append(items, doc.Key+":"+doc.Version)
	}
	sort.Strings(items)
	return items
}

func usagePromptTokens(resp platformapi.ChatProxyResponse) int {
	if resp.Response.Usage == nil {
		return 0
	}
	return resp.Response.Usage.PromptTokens
}

func usageCompletionTokens(resp platformapi.ChatProxyResponse) int {
	if resp.Response.Usage == nil {
		return 0
	}
	return resp.Response.Usage.CompletionTokens
}
