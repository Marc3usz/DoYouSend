package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Marc3usz/DoYouSend/backend/internal/iam"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers/setup"
	"github.com/Marc3usz/DoYouSend/backend/internal/settings"
)

// loadSettings reads the panel settings (ADR-0018) and builds providers from
// them. When the stored configuration cannot start (an unreadable key, an
// operator removed from .env), the API still starts, from .env alone with
// DRY_RUN forced on, and logs why.
func loadSettings(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) (*settings.Service, *setup.Live, error) {
	keyFile := strings.TrimSpace(os.Getenv("SETTINGS_KEY_FILE"))
	if keyFile == "" {
		keyFile = "settings.key"
	}
	key, created, err := settings.LoadKey(os.Getenv("SETTINGS_ENCRYPTION_KEY"), keyFile)
	if err != nil {
		return nil, nil, err
	}
	if created {
		logger.Warn("generated a new settings encryption key; back this file up, without it the API keys saved in the panel must be entered again",
			"file", keyFile)
	}
	c, err := settings.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}

	locked, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("DRY_RUN_LOCK")))
	svc := settings.NewService(settings.NewPGStore(pool), c, settings.Options{
		LookupEnv: os.LookupEnv, DryRunLocked: locked, Logger: logger,
	})
	snap, err := svc.Load(ctx)
	if err != nil {
		return nil, nil, err
	}

	live, err := setup.NewLive(snap.ProviderConfig(), logger)
	if err == nil {
		return svc, live, nil
	}
	logger.Error("the saved sending settings cannot start; using .env with DRY_RUN on until they are fixed in the panel", "err", err)
	fallback := svc.EnvOnly().ProviderConfig()
	fallback.DryRun = true
	live, ferr := setup.NewLive(fallback, logger)
	if ferr != nil {
		return nil, nil, fmt.Errorf("start providers: %w (fallback: %w)", err, ferr)
	}
	return svc, live, nil
}

// perRequest serves each request with a handler built for it, so handlers that
// take a value at construction (a price, a profiler) see the current settings.
func perRequest(build func() http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		build().ServeHTTP(w, r)
	})
}

// cachedBySecret serves a webhook whose handler needs a token or key from the
// settings. It rebuilds the handler only when the value changes and answers
// 404 while it is empty, as when the route was not registered.
func cachedBySecret(current func() string, build func(string) http.Handler) http.Handler {
	var mu sync.Mutex
	var lastValue string
	var last http.Handler
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := strings.TrimSpace(current())
		if v == "" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		if last == nil || v != lastValue {
			lastValue, last = v, build(v)
		}
		h := last
		mu.Unlock()
		h.ServeHTTP(w, r)
	})
}

// sendingConfig is the read-only summary of GET /api/admin/config.
func sendingConfig(s settings.Snapshot) iam.SendingConfig {
	c := s.ProviderConfig()
	return iam.SendingConfig{
		DryRun:               c.DryRun,
		EmailProvider:        c.EmailProvider,
		EmailFrom:            c.EmailFrom,
		EmailSandbox:         c.SendGridSandbox,
		EmailCredentialsSet:  c.SendGridAPIKey != "" || c.SMTPPassword != "",
		EmailEventsWebhook:   c.SendGridWebhookPublicKey != "",
		SMSProvider:          c.SMSProvider,
		SMSSenderName:        c.SMSSenderName,
		SMSTestMode:          c.SMSAPITestMode,
		SMSCredentialsSet:    c.SMSAPIKey != "",
		SMSReportsWebhook:    s.Value(settings.KeySMSAPIDeliveryReports) != "",
		SMSPricePerPartMilli: s.SMSPriceMilli(),
	}
}

// senderOptions describes the sender verifier live currently uses.
func senderOptions(live *setup.Live) iam.SenderOptions {
	_, real := live.SenderVerifier()
	if real {
		return iam.SenderOptions{Provider: "sendgrid", Live: true}
	}
	return iam.SenderOptions{Provider: "fake", Live: false}
}
