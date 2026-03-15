package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Addr               string
	DatabaseURL        string
	RuntimeConfigPath  string
	SupabaseURL        string
	SupabaseAnonKey    string
	SupabaseJWKSURL    string
	SupabaseJWTSecret  string
	SupabaseAudience   string
	AdminEmails        []string
	PaymentProvider    string
	PublicBaseURL      string
	EasyPayBaseURL     string
	EasyPayPID         string
	EasyPayKey         string
	EasyPayType        string
	AliMPayBaseURL     string
	AliMPayPID         string
	AliMPayKey         string
	AliMPayType        string
	OfficialRoutesJSON string
	OfficialModelsJSON string
	PricingRulesJSON   string
	AgreementsJSON     string
}

func LoadFromEnv() Config {
	cfg := Config{
		Addr:               getEnv("PLATFORM_ADDR", "127.0.0.1:18791"),
		DatabaseURL:        strings.TrimSpace(os.Getenv("PLATFORM_DATABASE_URL")),
		RuntimeConfigPath:  getEnv("PLATFORM_RUNTIME_CONFIG_PATH", "config/platform.runtime.json"),
		SupabaseURL:        strings.TrimRight(strings.TrimSpace(os.Getenv("PLATFORM_SUPABASE_URL")), "/"),
		SupabaseAnonKey:    strings.TrimSpace(os.Getenv("PLATFORM_SUPABASE_ANON_KEY")),
		SupabaseJWKSURL:    strings.TrimSpace(os.Getenv("PLATFORM_SUPABASE_JWKS_URL")),
		SupabaseJWTSecret:  strings.TrimSpace(os.Getenv("PLATFORM_SUPABASE_JWT_SECRET")),
		SupabaseAudience:   getEnv("PLATFORM_SUPABASE_AUDIENCE", "authenticated"),
		AdminEmails:        splitCSV(os.Getenv("PLATFORM_ADMIN_EMAILS")),
		PaymentProvider:    getEnv("PLATFORM_PAYMENT_PROVIDER", "manual"),
		PublicBaseURL:      strings.TrimRight(strings.TrimSpace(os.Getenv("PLATFORM_PUBLIC_BASE_URL")), "/"),
		EasyPayBaseURL:     strings.TrimRight(strings.TrimSpace(os.Getenv("PLATFORM_EASYPAY_BASE_URL")), "/"),
		EasyPayPID:         strings.TrimSpace(os.Getenv("PLATFORM_EASYPAY_PID")),
		EasyPayKey:         strings.TrimSpace(os.Getenv("PLATFORM_EASYPAY_KEY")),
		EasyPayType:        getEnv("PLATFORM_EASYPAY_TYPE", "alipay"),
		AliMPayBaseURL:     strings.TrimRight(strings.TrimSpace(os.Getenv("PLATFORM_ALIMPAY_BASE_URL")), "/"),
		AliMPayPID:         strings.TrimSpace(os.Getenv("PLATFORM_ALIMPAY_PID")),
		AliMPayKey:         strings.TrimSpace(os.Getenv("PLATFORM_ALIMPAY_KEY")),
		AliMPayType:        getEnv("PLATFORM_ALIMPAY_TYPE", "alipay"),
		OfficialRoutesJSON: strings.TrimSpace(os.Getenv("PLATFORM_OFFICIAL_ROUTES_JSON")),
		OfficialModelsJSON: strings.TrimSpace(os.Getenv("PLATFORM_OFFICIAL_MODELS_JSON")),
		PricingRulesJSON:   strings.TrimSpace(os.Getenv("PLATFORM_PRICING_RULES_JSON")),
		AgreementsJSON:     strings.TrimSpace(os.Getenv("PLATFORM_AGREEMENTS_JSON")),
	}
	if cfg.SupabaseJWKSURL == "" && cfg.SupabaseURL != "" {
		cfg.SupabaseJWKSURL = cfg.SupabaseURL + "/auth/v1/.well-known/jwks.json"
	}
	return cfg
}

func getEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func GetEnvInt(key string, fallback int) int {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
