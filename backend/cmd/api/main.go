// Command api is the HTTP entrypoint of DoYouSend.
//
// The skeleton intentionally uses only the standard library; routing and storage
// are wired in by the owning domain packages as they land (see backend/CLAUDE.md).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/groups"
	"github.com/Marc3usz/DoYouSend/backend/internal/platform/config"
	"github.com/Marc3usz/DoYouSend/backend/internal/platform/database"
	"github.com/Marc3usz/DoYouSend/backend/internal/platform/httpx"
	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]any{"status": "ok", "dryRun": cfg.DryRun})
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Without a database the API still serves what needs none (health, import
	// check), so frontend work on those screens does not require Postgres.
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	switch {
	case errors.Is(err, database.ErrNoURL):
		logger.Warn("DATABASE_URL is not set: recipient and group endpoints are disabled")
		mux.Handle("POST /api/recipients/import/check", recipients.HandleCheckImport(logger))
	case err != nil:
		logger.Error("connect to database", "err", err)
		os.Exit(1)
	default:
		defer pool.Close()
		recipientStore := recipients.NewPGStore(pool)
		recipients.NewHandler(recipients.NewService(recipientStore), logger).Register(mux)
		groupStore := groups.NewPGStore(pool)
		groups.NewHandler(
			groups.NewService(groupStore, recipientStore),
			groups.NewResolver(groupStore, recipientStore),
			logger,
		).Register(mux)
		importer := recipients.NewImporter(recipientStore)
		mux.Handle("POST /api/recipients/import/check", recipients.HandleImportFile(importer.Check, logger))
		mux.Handle("POST /api/recipients/import", recipients.HandleImportFile(importer.Import, logger))
	}

	srv := &http.Server{
		Addr:              ":" + cfg.APIPort,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("api listening", "addr", srv.Addr, "env", cfg.AppEnv, "dryRun", cfg.DryRun)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
	}
}
