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
	"strconv"
	"strings"
	"time"
)

type EasyPayConfig struct {
	BaseURL   string
	PID       string
	Key       string
	NotifyURL string
	ReturnURL string
	Type      string
	SiteName  string
}

type EasyPayProvider struct {
	cfg    EasyPayConfig
	client *http.Client
}

type easyPayAPIResponse struct {
	Code any             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type easyPayOrderPayload struct {
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

func NewEasyPayProvider(cfg EasyPayConfig) *EasyPayProvider {
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
	return &EasyPayProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *EasyPayProvider) Name() string { return "easypay" }

func (p *EasyPayProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		CanQueryOrder: true,
		CanRefund:     true,
	}
}

func (p *EasyPayProvider) CreateOrder(ctx context.Context, input CreateOrderInput) (PaymentOrder, error) {
	if p.cfg.BaseURL == "" || p.cfg.PID == "" || p.cfg.Key == "" {
		return PaymentOrder{}, fmt.Errorf("easypay provider is not configured")
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

func (p *EasyPayProvider) VerifyCallback(ctx context.Context, values url.Values) (CallbackResult, error) {
	if p.cfg.PID == "" || p.cfg.Key == "" {
		return CallbackResult{}, fmt.Errorf("easypay provider is not configured")
	}
	if !strings.EqualFold(values.Get("pid"), p.cfg.PID) {
		return CallbackResult{}, fmt.Errorf("unexpected pid")
	}
	if !strings.EqualFold(values.Get("sign"), p.sign(values)) {
		return CallbackResult{}, fmt.Errorf("invalid easypay signature")
	}
	amountFen, err := yuanToFen(values.Get("money"))
	if err != nil {
		return CallbackResult{}, fmt.Errorf("invalid easypay money: %w", err)
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

func (p *EasyPayProvider) QueryOrder(ctx context.Context, input QueryOrderInput) (OrderStatusResult, error) {
	if p.cfg.PID == "" || p.cfg.Key == "" || p.cfg.BaseURL == "" {
		return OrderStatusResult{}, fmt.Errorf("easypay provider is not configured")
	}
	form := url.Values{}
	form.Set("act", "order")
	form.Set("pid", p.cfg.PID)
	form.Set("key", p.cfg.Key)
	if strings.TrimSpace(input.OrderID) != "" {
		form.Set("out_trade_no", strings.TrimSpace(input.OrderID))
	}
	if strings.TrimSpace(input.ExternalOrderID) != "" {
		form.Set("trade_no", strings.TrimSpace(input.ExternalOrderID))
	}
	var payload easyPayOrderPayload
	if err := p.callAPI(ctx, form, &payload); err != nil {
		return OrderStatusResult{}, err
	}
	amountFen, _ := yuanToFen(payload.Money)
	status, paid, refunded := inferEasyPayOrderState(payload)
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

func (p *EasyPayProvider) Refund(ctx context.Context, input RefundInput) (RefundResult, error) {
	if p.cfg.PID == "" || p.cfg.Key == "" || p.cfg.BaseURL == "" {
		return RefundResult{}, fmt.Errorf("easypay provider is not configured")
	}
	if strings.TrimSpace(input.OrderID) == "" && strings.TrimSpace(input.ExternalOrderID) == "" {
		return RefundResult{}, fmt.Errorf("refund requires order id or external order id")
	}
	form := url.Values{}
	form.Set("act", "refund")
	form.Set("pid", p.cfg.PID)
	form.Set("key", p.cfg.Key)
	if strings.TrimSpace(input.OrderID) != "" {
		form.Set("out_trade_no", strings.TrimSpace(input.OrderID))
	}
	if strings.TrimSpace(input.ExternalOrderID) != "" {
		form.Set("trade_no", strings.TrimSpace(input.ExternalOrderID))
	}
	if input.AmountFen > 0 {
		form.Set("money", fenToYuan(input.AmountFen))
	}
	if strings.TrimSpace(input.Reason) != "" {
		form.Set("content", strings.TrimSpace(input.Reason))
	}
	var payload struct {
		TradeNo   string `json:"trade_no"`
		OrderNo   string `json:"out_trade_no"`
		RefundNo  string `json:"refund_no"`
		Status    any    `json:"status"`
		Money     string `json:"money"`
		TradeStat string `json:"trade_status"`
	}
	if err := p.callAPI(ctx, form, &payload); err != nil {
		return RefundResult{}, err
	}
	amountFen := input.AmountFen
	if amountFen == 0 {
		amountFen, _ = yuanToFen(payload.Money)
	}
	return RefundResult{
		OrderID:          firstNonEmpty(payload.OrderNo, input.OrderID),
		ExternalOrderID:  firstNonEmpty(payload.TradeNo, input.ExternalOrderID),
		ExternalRefundID: payload.RefundNo,
		AmountFen:        amountFen,
		Status:           "refunded",
		ProviderStatus:   firstNonEmpty(payload.TradeStat, stringifyStatus(payload.Status), "success"),
		Succeeded:        true,
		Message:          "refund accepted by easypay",
	}, nil
}

func (p *EasyPayProvider) SignForTest(values url.Values) string {
	return p.sign(values)
}

func (p *EasyPayProvider) sign(values url.Values) string {
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

func (p *EasyPayProvider) callAPI(ctx context.Context, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+"/api.php", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("easypay api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var envelope easyPayAPIResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode easypay response: %w", err)
	}
	if !easyPayCodeOK(envelope.Code) {
		message := strings.TrimSpace(envelope.Msg)
		if message == "" {
			message = "easypay api returned a failure response"
		}
		return errors.New(message)
	}
	if out == nil || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("decode easypay data: %w", err)
	}
	return nil
}

func easyPayCodeOK(code any) bool {
	switch v := code.(type) {
	case float64:
		return int(v) == 1 || int(v) == 0
	case int:
		return v == 1 || v == 0
	case string:
		v = strings.TrimSpace(strings.ToLower(v))
		return v == "1" || v == "0" || v == "success" || v == "ok"
	default:
		return false
	}
}

func inferEasyPayOrderState(payload easyPayOrderPayload) (status string, paid bool, refunded bool) {
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

func stringifyStatus(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case int:
		return strconv.Itoa(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func fenToYuan(amountFen int64) string {
	return strconv.FormatFloat(float64(amountFen)/100, 'f', 2, 64)
}

func yuanToFen(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("value is required")
	}
	if strings.HasPrefix(raw, "-") {
		return 0, fmt.Errorf("negative values are not allowed")
	}
	whole, frac, found := strings.Cut(raw, ".")
	if !found {
		frac = ""
	}
	if whole == "" {
		whole = "0"
	}
	if len(frac) > 2 {
		return 0, fmt.Errorf("too many decimal places")
	}
	frac += strings.Repeat("0", 2-len(frac))
	wholeFen, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, err
	}
	fracFen, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, err
	}
	return wholeFen*100 + fracFen, nil
}
