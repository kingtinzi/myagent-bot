package payments

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
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
	cfg EasyPayConfig
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
	cfg.SiteName = strings.TrimSpace(cfg.SiteName)
	if cfg.SiteName == "" {
		cfg.SiteName = "OpenClaw"
	}
	return &EasyPayProvider{cfg: cfg}
}

func (p *EasyPayProvider) Name() string { return "easypay" }

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
	params.Set("name", p.cfg.SiteName+" Recharge")
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
