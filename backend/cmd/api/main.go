// Command api is the HTTP entrypoint of DoYouSend.
//
// The skeleton intentionally uses only the standard library; routing and storage
// are wired in by the owning domain packages as they land (see backend/CLAUDE.md).
package main

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/Marc3usz/DoYouSend/backend/internal/settings"
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

	// dryRunNow reports the DRY_RUN in effect; the settings panel can change it.
	dryRunNow := func() bool { return cfg.DryRun }
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]any{"status": "ok", "dryRun": dryRunNow()})
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
		iamSvc := iam.NewService(iam.NewPGStore(pool))
		recipientStore := recipients.NewPGStore(pool)
		recipients.NewHandler(recipients.NewService(recipientStore), logger).WithAuditor(contactAudit{log: iamSvc}).Register(mux)
		groupStore := groups.NewPGStore(pool)
		resolver := groups.NewResolver(groupStore, recipientStore)
		groupService := groups.NewService(groupStore, recipientStore)
		groups.NewHandler(groupService, resolver, logger).WithContactAccess(isAdmin).Register(mux)
		// Sending configuration (ADR-0018): panel values from the database win over
		// .env, which wins over defaults. Providers are rebuilt on every change.
		settingsSvc, live, err := loadSettings(ctx, pool, logger)
		if err != nil {
			logger.Error("initialize settings", "err", err)
			os.Exit(1)
		}
		dryRunNow = func() bool { return live.Config().DryRun }
		currentPrice := func() int64 { return settingsSvc.Current().SMSPriceMilli() }

		mux.Handle("POST /api/messages/preview", perRequest(func() http.Handler {
			return messaging.HandlePreview(messaging.NewPreviewer(resolver, currentPrice()), logger)
		}))
		importer := recipients.NewImporter(recipientStore)
		mux.Handle("POST /api/recipients/import/check", recipients.HandleImportFile(importer.Check, logger))
		mux.Handle("POST /api/recipients/import", recipients.HandleImportFile(importer.Import, logger))

		reportConsumer := providers.NewPGDeliveryReportConsumer(pool, logger)
		reportConsumer.StartCleaner(ctx, 24*time.Hour, 7*24*time.Hour)

		// Webhook routes are always registered; without a token or key they answer 404,
		// as they did when the routes were left out.
		dlrHandler := cachedBySecret(
			func() string { return settingsSvc.Current().Value(settings.KeySMSAPIDeliveryReports) },
			func(token string) http.Handler { return sms.HandleSMSAPIDLR(reportConsumer, token, logger) })
		mux.Handle("GET /providers/sms/dlr", dlrHandler)
		mux.Handle("POST /providers/sms/dlr", dlrHandler)
		mux.Handle("GET /providers/sms/dlr/{token}", dlrHandler)
		mux.Handle("POST /providers/sms/dlr/{token}", dlrHandler)
		mux.Handle("POST /providers/email/events", cachedBySecret(
			func() string { return settingsSvc.Current().Value(settings.KeySendGridWebhookKey) },
			func(key string) http.Handler { return email.HandleSendGridEvents(reportConsumer, key, logger) }))

		smsUsageStore := sms.NewPGUsageStore(pool)
		mux.Handle("GET /api/stats/sms", perRequest(func() http.Handler {
			return sms.HandleUsageStats(smsUsageStore, currentPrice(), logger)
		}))

		emailUsageStore := email.NewPGUsageStore(pool)
		mux.Handle("GET /api/stats/email", email.HandleUsageStats(emailUsageStore, logger))

		mux.Handle("GET /api/admin/providers/sms/profile", perRequest(func() http.Handler {
			return sms.HandleProfile(live.SMSProfiler(), logger)
		}))
		mux.Handle("GET /api/admin/providers/email/profile", perRequest(func() http.Handler {
			return email.HandleProfile(live.EmailProfiler(), logger)
		}))

		dispatcher, err := live.Dispatcher(delivery.DefaultRetryPolicy(), logger)
		if err != nil {
			logger.Error("initialize delivery dispatcher", "err", err)
			os.Exit(1)
		}
		batchStore := delivery.NewPGStore(pool)
		batchService := delivery.NewService(resolver, groupService, dispatcher, batchStore, currentPrice(), logger)
		delivery.NewHandler(batchService, batchStore, currentCaller, logger).Register(mux)
		waitBatches = batchService.Wait
		batchService.SetAuditor(batchAudit{log: iamSvc})

		iam.NewHandler(iamSvc, iam.CookieOptions{Secure: cfg.SessionCookieSecure}, iam.SendingConfig{}, logger).
			WithConfigSource(func() iam.SendingConfig { return sendingConfig(settingsSvc.Current()) }).
			Register(mux)

		// Sender identities (ADR-0016): fake under DRY_RUN, SendGrid otherwise.
		senders := iam.NewSenders(iamSvc, iam.NewPGStore(pool), senderVerifier{live: live}, senderOptions(live))
		iam.NewSenderHandler(senders, logger).Register(mux)

		settingsSvc.SetApplier(live)
		settingsSvc.SetAuditor(iamSvc)
		settingsSvc.OnChange(func(snap settings.Snapshot) {
			batchService.SetSMSPrice(snap.SMSPriceMilli())
			senders.SetOptions(senderOptions(live))
		})
		settings.NewHandler(settingsSvc, func(ctx context.Context) string {
			u, _ := iam.UserFrom(ctx)
			return u.ID
		}, logger).Register(mux)
		handler = iam.Middleware(iamSvc, logger, mux)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.APIPort,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("api listening", "addr", srv.Addr, "env", cfg.AppEnv, "dryRun", dryRunNow())
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
// contactAudit records a recipient contact fixed by the signed-in user
// (ADR-0013): channel names only, never the values.
type contactAudit struct{ log *iam.Service }

func (a contactAudit) ContactFixed(ctx context.Context, recipientID string, channels []recipients.Channel) {
	u, ok := iam.UserFrom(ctx)
	if !ok {
		return
	}
	names := make([]string, len(channels))
	for i, c := range channels {
		names[i] = string(c)
	}
	a.log.Record(ctx, u.ID, iam.ActionRecipientContactFixed, "recipient", recipientID, map[string]string{
		"channels": strings.Join(names, ","),
	})
}

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
}

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

// senderVerifier adapts the e-mail provider's sender-identity API to iam
// (ADR-0016). It asks live for the current verifier and the school's postal
// address on every call, so settings-panel changes apply at once.
type senderVerifier struct{ live *setup.Live }

func (a senderVerifier) verifier() email.SenderVerifier {
	v, _ := a.live.SenderVerifier()
	return v
}

func (a senderVerifier) CreateSender(ctx context.Context, in iam.SenderRequest) (iam.ProviderSender, error) {
	cfg := a.live.Config()
	s, err := a.verifier().CreateSender(ctx, email.NewSenderIdentity{
		Nickname: in.Nickname, FromEmail: in.FromEmail, FromName: in.FromName,
		ReplyTo: in.ReplyTo, ReplyToName: in.FromName,
		Address: cfg.SenderAddress, City: cfg.SenderCity, Zip: cfg.SenderZip, Country: cfg.SenderCountry,
	})
	if err != nil {
		return iam.ProviderSender{}, senderError(err)
	}
	return iam.ProviderSender{ID: s.ID, Verified: s.Verified}, nil
}

func (a senderVerifier) GetSender(ctx context.Context, id string) (iam.ProviderSender, error) {
	s, err := a.verifier().GetSender(ctx, id)
	if err != nil {
		return iam.ProviderSender{}, senderError(err)
	}
	return iam.ProviderSender{ID: s.ID, Verified: s.Verified}, nil
}

func (a senderVerifier) ResendVerification(ctx context.Context, id string) error {
	return senderError(a.verifier().ResendVerification(ctx, id))
}

func (a senderVerifier) DeleteSender(ctx context.Context, id string) error {
	return senderError(a.verifier().DeleteSender(ctx, id))
}

// senderError maps provider errors onto the iam ones; the original stays wrapped.
func senderError(err error) error {
	var apiErr *email.SenderAPIError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, email.ErrSenderNotFound):
		return fmt.Errorf("%w: %w", iam.ErrSenderGone, err)
	case errors.As(err, &apiErr) && errors.Is(err, email.ErrSenderRejected) && len(apiErr.Messages) > 0:
		return &iam.SenderRejectedError{Message: strings.Join(apiErr.Messages, "; ")}
	case errors.Is(err, email.ErrSenderRejected):
		return &iam.SenderRejectedError{Message: err.Error()}
	default:
		return err
	}
}
