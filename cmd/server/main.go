package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/S-martin-7/sms/internal/admin"
	"github.com/S-martin-7/sms/internal/config"
	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/events"
	"github.com/S-martin-7/sms/internal/horisen"
	"github.com/S-martin-7/sms/internal/httpapi"
	"github.com/S-martin-7/sms/internal/logger"
	"github.com/S-martin-7/sms/internal/sms"
	"github.com/S-martin-7/sms/internal/tenancy"
	"github.com/S-martin-7/sms/internal/webhooks"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log := logger.New(cfg.LogLevel, cfg.Env)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("db open failed")
	}
	defer pool.Close()

	tenancySvc := tenancy.NewService(pool)
	adminSvc := admin.NewService(pool, cfg.BcryptCost)
	smsSvc := sms.NewService(pool)
	whSvc := webhooks.NewService(pool)
	eventsSvc := events.NewService(pool)

	// Webhook dispatcher polls webhook_deliveries and POSTs to tenant URLs.
	whDispatcher := webhooks.NewDispatcher(webhooks.DispatcherConfig{
		Pool:    pool,
		Workers: 4,
		Logger:  log,
	})
	go whDispatcher.Start(ctx)
	log.Info().Msg("webhook dispatcher enabled")

	// Scheduler — picks up due scheduled_sends and dispatches them through
	// the same outbox path the synchronous /v1/sms uses.
	scheduler := sms.NewScheduler(sms.SchedulerConfig{
		Pool: pool, SMSSvc: smsSvc, Logger: log,
	})
	go scheduler.Start(ctx)
	log.Info().Msg("scheduler enabled")

	// Balance endpoint (OAuth2). Wire only if creds + URLs are configured;
	// otherwise leave the cache nil so the handler 503s with a clear hint.
	var balanceCache *httpapi.BalanceCache
	if cfg.HorisenOAuthClientID != "" && cfg.HorisenOAuthClientSecret != "" &&
		cfg.HorisenOAuthTokenURL != "" && cfg.HorisenBalanceURL != "" {
		tokens := horisen.NewTokenCache(horisen.OAuthConfig{
			ClientID:     cfg.HorisenOAuthClientID,
			ClientSecret: cfg.HorisenOAuthClientSecret,
			TokenURL:     cfg.HorisenOAuthTokenURL,
		})
		balClient, err := horisen.NewBalanceClient(horisen.BalanceClientConfig{
			URL:    cfg.HorisenBalanceURL,
			Tokens: tokens,
		})
		if err != nil {
			log.Fatal().Err(err).Msg("balance client init failed")
		}
		balanceCache = httpapi.NewBalanceCache(balClient, time.Duration(cfg.BalanceCacheSeconds)*time.Second)
		log.Info().Int("cache_seconds", cfg.BalanceCacheSeconds).Msg("balance endpoint enabled")
	} else {
		log.Warn().Msg("balance endpoint disabled — OAuth2 vars not set")
	}

	// Start the Horisen outbox worker (only if creds are configured).
	if cfg.HorisenBaseURL != "" && cfg.HorisenUsername != "" && cfg.HorisenPassword != "" {
		hc, err := horisen.New(horisen.Config{
			BaseURL:       cfg.HorisenBaseURL,
			Username:      cfg.HorisenUsername,
			Password:      cfg.HorisenPassword,
			TLSServerName: cfg.HorisenTLSServerName,
		})
		if err != nil {
			log.Fatal().Err(err).Msg("horisen client init failed")
		}
		dlrURL := ""
		if cfg.PublicBaseURL != "" && cfg.HorisenCallbackSecret != "" {
			dlrURL = cfg.PublicBaseURL + "/v1/horisen/dlr?sig=" + cfg.HorisenCallbackSecret
		}
		ob := sms.NewOutbox(sms.OutboxConfig{
			Pool:    pool,
			Sender:  hc,
			TPS:     cfg.HorisenTPS,
			Workers: cfg.HorisenTPS,
			DLRURL:  dlrURL,
			Logger:  log,
		})
		go ob.Start(ctx)
		log.Info().Str("base_url", cfg.HorisenBaseURL).Int("tps", cfg.HorisenTPS).Msg("horisen outbox enabled")
	} else {
		log.Warn().Msg("horisen not configured — outbox worker NOT started; /v1/sms will queue but never deliver")
	}

	handler := httpapi.NewRouter(httpapi.RouterDeps{
		AdminSvc:              adminSvc,
		TenancySvc:            tenancySvc,
		SMSSvc:                smsSvc,
		WebhooksSvc:           whSvc,
		EventsSvc:             eventsSvc,
		BalanceCache:          balanceCache,
		Pool:                  pool,
		JWTSecret:             []byte(cfg.JWTSecret),
		JWTTTL:                time.Duration(cfg.JWTTTLHours) * time.Hour,
		APIKeyPepper:          cfg.APIKeyPepper,
		HorisenCallbackUser:   cfg.HorisenCallbackUser,
		HorisenCallbackPass:   cfg.HorisenCallbackPass,
		HorisenCallbackSecret: cfg.HorisenCallbackSecret,
		Logger:                &log,
	})

	srv := &http.Server{
		Addr:              cfg.BindAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Info().Str("addr", cfg.BindAddr).Msg("server listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("listen")
		}
	}()

	<-ctx.Done()
	log.Info().Msg("shutdown signal received")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("shutdown")
	}
}
