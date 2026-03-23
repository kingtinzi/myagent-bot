package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"openclaw/platform/internal/api"
	"openclaw/platform/internal/authbridge"
	"openclaw/platform/internal/authverifier"
	"openclaw/platform/internal/config"
	"openclaw/platform/internal/payments"
	"openclaw/platform/internal/runtimeconfig"
	"openclaw/platform/internal/service"
	"openclaw/platform/internal/store/pg"
	"openclaw/platform/internal/upstream"
)

func main() {
	if err := config.LoadPlatformEnv(); err != nil {
		log.Fatalf("load platform env: %v", err)
	}
	cfg := config.LoadFromEnv()
	ctx := context.Background()

	store := service.NewMemoryStore()
	var pgStore *pg.Store
	if cfg.DatabaseURL != "" {
		var err error
		pgStore, err = pg.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			log.Fatalf("open postgres store: %v", err)
		}
		defer pgStore.Close()
		store = nil
	}

	var svc *service.Service
	if pgStore != nil {
		svc = service.NewService(pgStore, nil)
	} else {
		svc = service.NewService(store, nil)
	}
	if err := configureOfficialPrimaryFailureStore(cfg, svc); err != nil {
		log.Printf("configure official primary failure store: %v", err)
	}
	if err := svc.SetPaymentProvider(buildPaymentProvider(cfg)); err != nil {
		log.Fatalf("configure payment provider: %v", err)
	}
	if err := svc.SyncAdminUsers(ctx, cfg.AdminEmails); err != nil {
		log.Fatalf("sync admin users: %v", err)
	}
	router := upstream.NewRouter(nil)
	svc.SetOfficialProxyClient(router)
	runtimeManager := runtimeconfig.NewManager(cfg.RuntimeConfigPath, svc, router)
	seedState, err := runtimeconfig.BuildStateFromEnv(cfg)
	if err != nil {
		log.Fatalf("load runtime config seed: %v", err)
	}
	if err := runtimeManager.Bootstrap(seedState); err != nil {
		log.Fatalf("bootstrap runtime config: %v", err)
	}

	server := &http.Server{
		Addr: cfg.Addr,
		Handler: api.NewServer(
			svc,
			authverifier.New(cfg.SupabaseJWKSURL, cfg.SupabaseJWTSecret, cfg.SupabaseAudience),
			authbridge.NewClient(cfg.SupabaseURL, cfg.SupabaseAnonKey),
			runtimeManager,
		),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("platform server listening on %s", cfg.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}

func buildPaymentProvider(cfg config.Config) payments.Provider {
	switch cfg.PaymentProvider {
	case "", "manual":
		return payments.ManualProvider{}
	case "easypay":
		return payments.NewEasyPayProvider(payments.EasyPayConfig{
			BaseURL:   cfg.EasyPayBaseURL,
			PID:       cfg.EasyPayPID,
			Key:       cfg.EasyPayKey,
			NotifyURL: cfg.PublicBaseURL + "/payments/easypay/notify",
			ReturnURL: cfg.PublicBaseURL + "/payments/easypay/return",
			Type:      cfg.EasyPayType,
			SiteName:  payments.DefaultSiteName,
		})
	case "alimpay":
		return payments.NewAliMPayProvider(payments.AliMPayConfig{
			BaseURL:   cfg.AliMPayBaseURL,
			PID:       cfg.AliMPayPID,
			Key:       cfg.AliMPayKey,
			NotifyURL: cfg.PublicBaseURL + "/payments/alimpay/notify",
			ReturnURL: cfg.PublicBaseURL + "/payments/alimpay/return",
			Type:      cfg.AliMPayType,
			SiteName:  payments.DefaultSiteName,
		})
	default:
		return payments.ManualProvider{}
	}
}

func configureOfficialPrimaryFailureStore(cfg config.Config, svc *service.Service) error {
	if svc == nil {
		return nil
	}
	redisURL := strings.TrimSpace(cfg.PrimaryFailureRedisURL)
	if redisURL == "" {
		return nil
	}
	store, err := service.NewRedisOfficialPrimaryFailureStore(service.RedisOfficialPrimaryFailureStoreConfig{
		URL:         redisURL,
		KeyPrefix:   cfg.PrimaryFailureRedisKeyPrefix,
		DialTimeout: 2 * time.Second,
		IOTimeout:   2 * time.Second,
	})
	if err != nil {
		return err
	}
	svc.SetOfficialPrimaryFailureStore(store)
	return nil
}
