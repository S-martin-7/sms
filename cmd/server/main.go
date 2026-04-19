package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/S-martin-7/sms/internal/config"
	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		// logger not up yet; write straight to stderr via a minimal logger
		l := logger.New("error", "dev")
		l.Fatal().Err(err).Msg("config load failed")
	}

	log := logger.New(cfg.LogLevel, cfg.Env)
	log.Info().
		Str("env", cfg.Env).
		Str("bind_addr", cfg.BindAddr).
		Msg("sms-server starting")

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	openCtx, openCancel := context.WithTimeout(ctx, 10*time.Second)
	pool, err := db.Open(openCtx, cfg.DatabaseURL)
	openCancel()
	if err != nil {
		log.Fatal().Err(err).Msg("db open failed")
	}
	defer pool.Close()
	log.Info().Msg("db ok")

	// Placeholder: the HTTP server and routes come in later plan tasks.
	// For now, wait for a signal and exit cleanly so we can prove the wiring works.
	<-ctx.Done()
	log.Info().Msg("shutting down")
	os.Exit(0)
}
