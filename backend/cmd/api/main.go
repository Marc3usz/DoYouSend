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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/delivery"
	"github.com/Marc3usz/DoYouSend/backend/internal/groups"
	"github.com/Marc3usz/DoYouSend/backend/internal/iam"
	"github.com/Marc3usz/DoYouSend/backend/internal/messaging"
	"github.com/Marc3usz/DoYouSend/backend/internal/platform/config"
	"github.com/Marc3usz/DoYouSend/backend/internal/platform/database"
	"github.com/Marc3usz/DoYouSend/backend/internal/platform/httpx"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers/email"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers/setup"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers/sms"
	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// The shell and Make do not load .env, and `go run` is started from backend/.
	envFile, fromExample, err := config.LoadDevEnv()
	if err != nil {
		logger.Error("load env file", "err", err)
		os.Exit(1)
	}
	switch {
	case fromExample:
		logger.Warn("no .env file, using .env.example (test data, DRY_RUN=true); copy it to .env to change settings", "file", envFile)
	case envFile != "":
		logger.Info("loaded env file", "file", envFile)
	}

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
	// Logging in needs the database too, so only then is every route behind
	// the iam middleware.
	var handler http.Handler = mux
	// waitBatches blocks until batches still being dispatched in the background are done.
	waitBatches := func() {}
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	switch {
	case errors.Is(err, database.ErrNoURL):
		logger.Warn("DATABASE_URL is not set: recipient, group, message preview and webhook endpoints are disabled")
		mux.Handle("POST /api/recipients/import/check", recipients.HandleCheckImport(logger))
	case err != nil:
		logger.Error("connect to database", "err", err)
		os.Exit(1)
	default:
		defer pool.Close()
		recipientStore := recipients.NewPGStore(pool)
		recipients.NewHandler(recipients.NewService(recipientStore), logger).Register(mux)
		groupStore := groups.NewPGStore(pool)
		resolver := groups.NewResolver(groupStore, recipientStore)
		groupService := groups.NewService(groupStore, recipientStore)
		groups.NewHandler(groupService, resolver, logger).WithContactAccess(isAdmin).Register(mux)
		smsPrice, err := messaging.ParsePrice(os.Getenv("SMS_PRICE_PER_PART_PLN"))
		if err != nil {
			logger.Error("invalid SMS_PRICE_PER_PART_PLN (see .env.example)", "err", err)
			os.Exit(1)
		}
		mux.Handle("POST /api/messages/preview",
			messaging.HandlePreview(messaging.NewPreviewer(resolver, smsPrice), logger))
		importer := recipients.NewImporter(recipientStore)
		mux.Handle("POST /api/recipients/import/check", recipients.HandleImportFile(importer.Check, logger))
		mux.Handle("POST /api/recipients/import", recipients.HandleImportFile(importer.Import, logger))

		reportConsumer := providers.NewPGDeliveryReportConsumer(pool, logger)
		reportConsumer.StartCleaner(ctx, 24*time.Hour, 7*24*time.Hour)

		smsToken := strings.TrimSpace(os.Getenv("SMSAPI_DLR_TOKEN"))
		if smsToken != "" {
			dlrHandler := sms.HandleSMSAPIDLR(reportConsumer, smsToken, logger)
			mux.Handle("GET /providers/sms/dlr", dlrHandler)
			mux.Handle("POST /providers/sms/dlr", dlrHandler)
			mux.Handle("GET /providers/sms/dlr/{token}", dlrHandler)
			mux.Handle("POST /providers/sms/dlr/{token}", dlrHandler)
		} else {
			logger.Warn("SMSAPI_DLR_TOKEN is not set: SMSAPI DLR webhook routes are disabled")
		}

		sendgridKey := strings.TrimSpace(os.Getenv("SENDGRID_WEBHOOK_PUBLIC_KEY"))
		if sendgridKey != "" {
			mux.Handle("POST /providers/email/events", email.HandleSendGridEvents(reportConsumer, sendgridKey, logger))
		} else {
			logger.Warn("SENDGRID_WEBHOOK_PUBLIC_KEY is not set: SendGrid event webhook route is disabled")
		}

		smsUsageStore := sms.NewPGUsageStore(pool)
		mux.Handle("GET /api/stats/sms", sms.HandleUsageStats(smsUsageStore, smsPrice, logger))

		emailUsageStore := email.NewPGUsageStore(pool)
		mux.Handle("GET /api/stats/email", email.HandleUsageStats(emailUsageStore, logger))

		provCfg := setup.ConfigFromEnv()
		provCfg.DryRun = cfg.DryRun
		provs, err := setup.New(provCfg, logger)
		if err != nil {
			logger.Error("initialize delivery providers", "err", err)
			os.Exit(1)
		}
		logger.Info("delivery providers initialized",
			"email", provCfg.EmailProvider,
			"sms", provCfg.SMSProvider,
			"dryRun", provCfg.DryRun,
		)

		mux.Handle("GET /api/admin/providers/sms/profile", sms.HandleProfile(provs.SMSProfiler(), logger))
		mux.Handle("GET /api/admin/providers/email/profile", email.HandleProfile(provs.EmailProfiler(), logger))

		dispatcher, err := provs.Dispatcher(delivery.DefaultRetryPolicy(), logger)
		if err != nil {
			logger.Error("initialize delivery dispatcher", "err", err)
			os.Exit(1)
		}
		batchStore := delivery.NewPGStore(pool)
		batchService := delivery.NewService(resolver, groupService, dispatcher, batchStore, smsPrice, logger)
		delivery.NewHandler(batchService, batchStore, currentCaller, logger).Register(mux)
		waitBatches = batchService.Wait

		sending := iam.SendingConfig{
			DryRun:               cfg.DryRun,
			EmailProvider:        provCfg.EmailProvider,
			EmailFrom:            provCfg.EmailFrom,
			EmailSandbox:         provCfg.SendGridSandbox,
			EmailCredentialsSet:  provCfg.SendGridAPIKey != "" || provCfg.SMTPPassword != "",
			EmailEventsWebhook:   sendgridKey != "",
			SMSProvider:          provCfg.SMSProvider,
			SMSSenderName:        provCfg.SMSSenderName,
			SMSTestMode:          provCfg.SMSAPITestMode,
			SMSCredentialsSet:    provCfg.SMSAPIKey != "",
			SMSReportsWebhook:    smsToken != "",
			SMSPricePerPartMilli: smsPrice,
		}
		iamSvc := iam.NewService(iam.NewPGStore(pool))
		batchService.SetAuditor(batchAudit{log: iamSvc})
		iam.NewHandler(iamSvc, iam.CookieOptions{Secure: cfg.SessionCookieSecure}, sending, logger).Register(mux)
		handler = iam.Middleware(iamSvc, logger, mux)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.APIPort,
		Handler:           handler,
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
	stop() // a second Ctrl-C now kills the process, even while batches are still being sent
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
	}
	// A confirmed batch is sent to the end, not cut off by a restart (CLAUDE.md rule 5).
	logger.Info("waiting for batches still being sent")
	waitBatches()
}

// batchAudit writes a confirmed batch to the iam audit log: who launched it and
// how big it was. Counts only, no names, addresses or the message body.
type batchAudit struct{ log *iam.Service }

func (a batchAudit) BatchConfirmed(ctx context.Context, b delivery.Batch) {
	a.log.Record(ctx, b.CreatedBy, iam.ActionBatchSent, "batch", b.ID, map[string]string{
		"recipients": strconv.Itoa(b.Counts.Recipients),
		"groups":     strconv.Itoa(len(b.Groups)),
		"smsParts":   strconv.Itoa(b.SMSParts),
	})
}

func (a batchAudit) BatchRetried(ctx context.Context, b delivery.Batch) {
	callerID := b.CreatedBy
	if u, ok := iam.UserFrom(ctx); ok && u.ID != "" {
		callerID = u.ID
	}
	a.log.Record(ctx, callerID, iam.ActionBatchRetried, "batch", b.ID, map[string]string{
		"recipients": strconv.Itoa(b.Counts.Recipients),
		"groups":     strconv.Itoa(len(b.Groups)),
		"smsParts":   strconv.Itoa(b.SMSParts),
	})
// isAdmin reports whether the request comes from a logged-in administrator.
func isAdmin(ctx context.Context) bool {
	u, ok := iam.UserFrom(ctx)
	return ok && u.IsAdmin()
}

// currentCaller adapts the iam session user to what the batch endpoints need.
func currentCaller(ctx context.Context) (delivery.Caller, bool) {
	u, ok := iam.UserFrom(ctx)
	if !ok {
		return delivery.Caller{}, false
	}
	return delivery.Caller{ID: u.ID, FullName: u.FullName, IsAdmin: u.IsAdmin()}, true
}
