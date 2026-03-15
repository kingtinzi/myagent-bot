package payments

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type AliMPayConfig struct {
	BaseURL   string
	PID       string
	Key       string
	NotifyURL string
	ReturnURL string
	Type      string
	SiteName  string
}

type AliMPayProvider struct {
	cfg    AliMPayConfig
	client *http.Client
}

type aliMPayOrderPayload struct {
	Code        any    `json:"code"`
	Msg         string `json:"msg"`
	PID         string `json:"pid"`
	OutTradeNo  string `json:"out_trade_no"`
	TradeNo     string `json:"trade_no"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	Money       string `json:"money"`
	TradeStatus string `json:"trade_status"`
	Status      any    `json:"status"`
	EndTime     string `json:"endtime"`
	AddTime     string `json:"addtime"`
}

func NewAliMPayProvider(cfg AliMPayConfig) *AliMPayProvider {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.PID = strings.TrimSpace(cfg.PID)
	cfg.Key = strings.TrimSpace(cfg.Key)
	cfg.NotifyURL = strings.TrimSpace(cfg.NotifyURL)
	cfg.ReturnURL = strings.TrimSpace(cfg.ReturnURL)
	cfg.Type = strings.TrimSpace(cfg.Type)
	if cfg.Type == "" {
		cfg.Type = "alipay"
	}
	cfg.SiteName = NormalizeSiteName(cfg.SiteName)
	return &AliMPayProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *AliMPayProvider) Name() string { return "alimpay" }

func (p *AliMPayProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		CanQueryOrder: true,
	}
}

func (p *AliMPayProvider) CreateOrder(ctx context.Context, input CreateOrderInput) (PaymentOrder, error) {
	if p.cfg.BaseURL == "" || p.cfg.PID == "" || p.cfg.Key == "" {
		return PaymentOrder{}, fmt.Errorf("alimpay provider is not configured")
	}
	params := url.Values{}
	params.Set("pid", p.cfg.PID)
	params.Set("type", p.cfg.Type)
	params.Set("out_trade_no", input.OrderID)
	params.Set("notify_url", firstNonEmpty(input.NotifyURL, p.cfg.NotifyURL))
	params.Set("return_url", firstNonEmpty(input.ReturnURL, p.cfg.ReturnURL))
	params.Set("name", RechargeDisplayName(p.cfg.SiteName))
	params.Set("money", fenToYuan(input.AmountFen))
	params.Set("sitename", p.cfg.SiteName)
	params.Set("sign_type", "MD5")
	params.Set("sign", p.sign(params))
	return PaymentOrder{
		OrderID:         input.OrderID,
		ExternalOrderID: input.OrderID,
		Status:          "pending",
		Provider:        p.Name(),
		PayURL:          p.cfg.BaseURL + "/submit.php?" + params.Encode(),
		AmountFen:       input.AmountFen,
	}, nil
}

func (p *AliMPayProvider) VerifyCallback(ctx context.Context, values url.Values) (CallbackResult, error) {
	if p.cfg.PID == "" || p.cfg.Key == "" {
		return CallbackResult{}, fmt.Errorf("alimpay provider is not configured")
	}
	if !strings.EqualFold(values.Get("pid"), p.cfg.PID) {
		return CallbackResult{}, fmt.Errorf("unexpected pid")
	}
	if !strings.EqualFold(values.Get("sign"), p.sign(values)) {
		return CallbackResult{}, fmt.Errorf("invalid alimpay signature")
	}
	amountFen, err := yuanToFen(values.Get("money"))
	if err != nil {
		return CallbackResult{}, fmt.Errorf("invalid alimpay money: %w", err)
	}
	status := strings.TrimSpace(values.Get("trade_status"))
	return CallbackResult{
		OrderID:         strings.TrimSpace(values.Get("out_trade_no")),
		ExternalOrderID: strings.TrimSpace(values.Get("trade_no")),
		AmountFen:       amountFen,
		Paid:            status == "TRADE_SUCCESS" || status == "TRADE_FINISHED",
		Status:          status,
	}, nil
}

func (p *AliMPayProvider) QueryOrder(ctx context.Context, input QueryOrderInput) (OrderStatusResult, error) {
	if p.cfg.PID == "" || p.cfg.Key == "" || p.cfg.BaseURL == "" {
		return OrderStatusResult{}, fmt.Errorf("alimpay provider is not configured")
	}
	orderID := strings.TrimSpace(input.OrderID)
	if orderID == "" {
		return OrderStatusResult{}, fmt.Errorf("alimpay query requires order id")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.BaseURL+"/api.php", nil)
	if err != nil {
		return OrderStatusResult{}, err
	}
	query := req.URL.Query()
	query.Set("action", "order")
	query.Set("pid", p.cfg.PID)
	query.Set("key", p.cfg.Key)
	query.Set("out_trade_no", orderID)
	req.URL.RawQuery = query.Encode()

	resp, err := p.client.Do(req)
	if err != nil {
		return OrderStatusResult{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return OrderStatusResult{}, err
	}
	if resp.StatusCode >= 300 {
		return OrderStatusResult{}, fmt.Errorf("alimpay api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload aliMPayOrderPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return OrderStatusResult{}, fmt.Errorf("decode alimpay response: %w", err)
	}
	if !easyPayCodeOK(payload.Code) {
		message := strings.TrimSpace(payload.Msg)
		if message == "" {
			message = "alimpay api returned a failure response"
		}
		return OrderStatusResult{}, errors.New(message)
	}
	amountFen, _ := yuanToFen(payload.Money)
	status, paid, refunded := inferCodePayOrderState(payload)
	return OrderStatusResult{
		OrderID:         firstNonEmpty(payload.OutTradeNo, input.OrderID),
		ExternalOrderID: firstNonEmpty(payload.TradeNo, input.ExternalOrderID),
		AmountFen:       amountFen,
		Status:          status,
		ProviderStatus:  firstNonEmpty(payload.TradeStatus, stringifyStatus(payload.Status)),
		Paid:            paid,
		Refunded:        refunded,
		LastCheckedUnix: time.Now().Unix(),
	}, nil
}

func (p *AliMPayProvider) Refund(ctx context.Context, input RefundInput) (RefundResult, error) {
	return RefundResult{}, ErrOperationNotSupported
}

func (p *AliMPayProvider) SignForTest(values url.Values) string {
	return p.sign(values)
}

func (p *AliMPayProvider) sign(values url.Values) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key == "sign" || key == "sign_type" {
			continue
		}
		if strings.TrimSpace(values.Get(key)) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values.Get(key))
	}
	sum := md5.Sum([]byte(strings.Join(parts, "&") + p.cfg.Key))
	return strings.ToLower(hex.EncodeToString(sum[:]))
}

func inferCodePayOrderState(payload aliMPayOrderPayload) (status string, paid bool, refunded bool) {
	raw := strings.ToLower(strings.TrimSpace(firstNonEmpty(payload.TradeStatus, stringifyStatus(payload.Status))))
	switch raw {
	case "trade_success", "trade_finished", "paid", "success", "1":
		return "paid", true, false
	case "refunded", "refund_success":
		return "refunded", true, true
	case "closed", "cancelled", "canceled", "failed", "0":
		return "closed", false, false
	default:
		if strings.TrimSpace(payload.EndTime) != "" && strings.TrimSpace(payload.TradeNo) != "" {
			return "paid", true, false
		}
		return "pending", false, false
	}
}
