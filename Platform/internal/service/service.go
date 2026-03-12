package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/platformapi"

	"openclaw/platform/internal/payments"
)

var (
	ErrInvalidAmount     = errors.New("amount_fen must be positive")
	ErrUnknownModel      = errors.New("unknown official model")
	ErrModelDisabled     = errors.New("official model disabled")
	ErrInsufficientFunds = errors.New("insufficient wallet balance")
	ErrInvalidAgreement  = errors.New("invalid agreement document")
	ErrUnknownAgreement  = errors.New("unknown agreement document")
	ErrCallbackAmount    = errors.New("callback amount does not match recharge order")
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
	InputPriceMicrosPer1K  int64  `json:"input_price_micros_per_1k"`
	OutputPriceMicrosPer1K int64  `json:"output_price_micros_per_1k"`
	FallbackPriceFen       int64  `json:"fallback_price_fen"`
}

type OfficialModel struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type AgreementDocument struct {
	Key     string `json:"key"`
	Version string `json:"version"`
	Title   string `json:"title"`
	Content string `json:"content,omitempty"`
	URL     string `json:"url,omitempty"`
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
	GetOrder(ctx context.Context, userID, orderID string) (RechargeOrder, error)
	MarkOrderPaid(ctx context.Context, orderID, provider, externalID string) (RechargeOrder, bool, error)
	FinalizeRechargeOrder(ctx context.Context, orderID, provider, externalID, description string) (RechargeOrder, WalletSummary, bool, error)
	Credit(ctx context.Context, userID string, amountFen int64, description string) (WalletSummary, error)
	Debit(ctx context.Context, userID string, amountFen int64, description string) (WalletSummary, error)
	UpsertAdminEmails(ctx context.Context, emails []string) error
	IsAdminUser(ctx context.Context, userID, email string) (bool, error)
	RecordAgreementAcceptance(ctx context.Context, userID, key, version string) error
	HasAgreementAcceptance(ctx context.Context, userID, key, version string) (bool, error)
}

type Service struct {
	store   Store
	chat    ChatClient
	proxy   OfficialProxyClient
	payment payments.Provider
	pricing map[string]PricingRule
	models  []OfficialModel
	docs    []AgreementDocument
	now     func() time.Time
	mu      sync.RWMutex
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
	s.models = append([]OfficialModel(nil), models...)
}

func (s *Service) ListOfficialModels(ctx context.Context) []OfficialModel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]OfficialModel(nil), s.models...)
}

func (s *Service) ListEnabledOfficialModels(ctx context.Context) []OfficialModel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]OfficialModel, 0, len(s.models))
	for _, model := range s.models {
		if model.Enabled && s.pricing[model.ID].ModelID != "" {
			items = append(items, model)
		}
	}
	return items
}

func (s *Service) SetAgreement(doc AgreementDocument) {
	s.mu.Lock()
	defer s.mu.Unlock()
	replaced := false
	for i := range s.docs {
		if s.docs[i].Key == doc.Key {
			s.docs[i] = doc
			replaced = true
			break
		}
	}
	if !replaced {
		s.docs = append(s.docs, doc)
	}
}

func (s *Service) SetAgreements(docs []AgreementDocument) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs = append([]AgreementDocument(nil), docs...)
}

func (s *Service) ListAgreements(ctx context.Context) []AgreementDocument {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]AgreementDocument(nil), s.docs...)
}

func (s *Service) SetPricingRules(rules map[string]PricingRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pricing = make(map[string]PricingRule, len(rules))
	for k, v := range rules {
		s.pricing[k] = v
	}
}

func (s *Service) ListPricingRules() []PricingRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]PricingRule, 0, len(s.pricing))
	for _, rule := range s.pricing {
		items = append(items, rule)
	}
	return items
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
		ID:          fmt.Sprintf("ord_%d", s.now().UnixNano()),
		UserID:      userID,
		AmountFen:   input.AmountFen,
		Channel:     input.Channel,
		Status:      "pending",
		CreatedUnix: s.now().Unix(),
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
	if chargeFen == 0 {
		chargeFen = rule.FallbackPriceFen
	}
	if chargeFen <= 0 {
		chargeFen = 1
	}
	if err := s.settleReservedCharge(ctx, userID, reservedFen, chargeFen, "official model usage"); err != nil {
		return ChatResult{}, err
	}
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
	if resp.Response.Usage != nil {
		chargeFen = chargeFromUsage(rule, Usage{
			PromptTokens:     resp.Response.Usage.PromptTokens,
			CompletionTokens: resp.Response.Usage.CompletionTokens,
		})
	}
	if chargeFen == 0 {
		chargeFen = rule.FallbackPriceFen
	}
	if chargeFen <= 0 {
		chargeFen = 1
	}
	if err := s.settleReservedCharge(ctx, userID, reservedFen, chargeFen, "official platform chat"); err != nil {
		return platformapi.ChatProxyResponse{}, err
	}
	resp.ChargedFen = chargeFen
	return resp, nil
}

func (s *Service) HandleSuccessfulRecharge(
	ctx context.Context,
	orderID, provider, externalID string,
) (bool, error) {
	_, _, changed, err := s.store.FinalizeRechargeOrder(ctx, orderID, provider, externalID, "recharge order paid")
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	return true, nil
}

type callbackOrderLookupStore interface {
	FindOrderByID(ctx context.Context, orderID string) (RechargeOrder, error)
}

func (s *Service) HandleSuccessfulRechargeCallback(
	ctx context.Context,
	orderID, provider, externalID string,
	callbackAmountFen int64,
) (bool, error) {
	lookup, ok := s.store.(callbackOrderLookupStore)
	if !ok {
		return false, fmt.Errorf("store does not support callback order lookup")
	}
	order, err := lookup.FindOrderByID(ctx, orderID)
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

func (s *Service) SyncAdminUsers(ctx context.Context, emails []string) error {
	return s.store.UpsertAdminEmails(ctx, emails)
}

func (s *Service) IsAdminUser(ctx context.Context, userID, email string) (bool, error) {
	return s.store.IsAdminUser(ctx, userID, email)
}

func (s *Service) RecordAgreementAcceptances(ctx context.Context, userID string, docs []AgreementDocument) error {
	required := s.ListAgreements(ctx)
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
		if err := s.store.RecordAgreementAcceptance(ctx, userID, doc.Key, agreementAcceptanceVersion(expected)); err != nil {
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
	}, "\n")))
	return fmt.Sprintf("%s:%s", strings.TrimSpace(doc.Version), hex.EncodeToString(sum[:]))
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
