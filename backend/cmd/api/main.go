// Command api is the HTTP entrypoint of DoYouSend: it loads the configuration,
// wires the domain packages to Postgres and the providers, and serves the API
// on the standard library mux. Business logic lives in internal/ (see
// backend/CLAUDE.md); this package only adapts packages to each other.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
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
	"github.com/Marc3usz/DoYouSend/backend/internal/senders"
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
	// recoverBatches resumes batches a stopped process left running; releaseLock
	// gives back the dispatch lock it takes.
	recoverBatches, releaseLock := func() {}, func() {}
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

		// Sender addresses (ADR-0021): who may send from which address, and SendGrid
		// domain authentication. Settings and providers are read on every call.
		senderSvc := senders.NewService(senders.NewPGStore(pool), senders.Options{
			API: func() senders.DomainAPI {
				d, _ := live.DomainAuthenticator()
				return domainAPI{api: d}
			},
			Config:  func() senders.Config { return senderConfig(settingsSvc, live) },
			Auditor: iamSvc,
		})
		senders.NewHandler(senderSvc, sendersUser, logger).Register(mux)
		batchService.SetSenderResolver(senderResolver{svc: senderSvc})
		mux.Handle("POST /api/admin/settings/test", handleTestMessage(live, iamSvc, logger))

		// Batches a stopped process left running are finished once this process owns
		// the port (a second `make dev-api` exits before touching anything) and holds
		// the dispatch lock (no other process on this database is sending them).
		recoverBatches = func() {
			release, ok, err := batchStore.AcquireDispatchLock(ctx)
			switch {
			case err != nil:
				logger.Error("recover batches left running: no dispatch lock", "err", err)
				return
			case !ok:
				logger.Warn("another API process holds the dispatch lock; batches left running are not resumed here")
				return
			}
			releaseLock = release
			if n, err := batchService.Recover(ctx); err != nil {
				logger.Error("recover batches left running", "err", err)
			} else if n > 0 {
				logger.Warn("resumed batches left running by a stopped process", "batches", n)
			}
		}

		iam.NewHandler(iamSvc, iam.CookieOptions{Secure: cfg.SessionCookieSecure}, iam.SendingConfig{}, logger).
			WithConfigSource(func() iam.SendingConfig { return sendingConfig(settingsSvc.Current()) }).
			Register(mux)

		// Sender identities (ADR-0016): fake under DRY_RUN, SendGrid otherwise.
		senders := iam.NewSenders(iamSvc, iam.NewPGStore(pool), senderVerifier{live: live}, senderOptions(live))
		iam.NewSenderHandler(senders, logger).Register(mux)

		settingsSvc.SetApplier(live)
		settingsSvc.SetAuditor(iamSvc)
		settingsSvc.SetBusy(batchService.Dispatching)
		settingsSvc.OnChange(func(snap settings.Snapshot) {
			batchService.SetSMSPrice(snap.SMSPriceMilli())
			senders.SetOptions(senderOptions(live))
		})
		settings.NewHandler(settingsSvc, func(ctx context.Context) string {
			u, _ := iam.UserFrom(ctx)
			return u.ID
		}, logger).Register(mux)
		// Notion import (ADR-0017): the database and token come from the
		// settings panel, read on every request.
		notionConfig := func() recipients.NotionConfig {
			snap := settingsSvc.Current()
			return recipients.NotionConfig{
				Token:        snap.Value(settings.KeyNotionToken),
				DatabaseID:   snap.Value(settings.KeyNotionDatabaseID),
				DataSourceID: snap.Value(settings.KeyNotionDataSourceID),
				PropertyMap:  snap.Value(settings.KeyNotionPropertyMap),
			}
		}
		mux.Handle("POST /api/recipients/import/notion/check", recipients.NotionImport{
			Config: notionConfig, Process: importer.CheckTable, Logger: logger,
		})
		mux.Handle("POST /api/recipients/import/notion", recipients.NotionImport{
			Config: notionConfig, Process: importer.ImportTable, Imported: importAudit{log: iamSvc}.NotionImported, Logger: logger,
		})

		handler = iam.Middleware(iamSvc, logger, mux)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.APIPort,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Listen before recovering: a second API on the same port fails here, before
	// it could mistake the first one's batches for abandoned ones.
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		logger.Error("listen", "addr", srv.Addr, "err", err)
		os.Exit(1)
	}
	recoverBatches()
	defer releaseLock()

	go func() {
		logger.Info("api listening", "addr", srv.Addr, "env", cfg.AppEnv, "dryRun", dryRunNow())
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

// importAudit records recipients stored from Notion: counts only, never
// names or contacts.
type importAudit struct{ log *iam.Service }

func (a importAudit) NotionImported(ctx context.Context, rep recipients.ImportReport, pages int) {
	u, ok := iam.UserFrom(ctx)
	if !ok {
		return
	}
	a.log.Record(ctx, u.ID, iam.ActionRecipientsImported, "recipients", "", map[string]string{
		"source":     "notion",
		"pages":      strconv.Itoa(pages),
		"imported":   strconv.Itoa(len(rep.Valid)),
		"invalid":    strconv.Itoa(len(rep.Invalid)),
		"duplicates": strconv.Itoa(len(rep.Duplicates)),
	})
}

// batchAudit writes a confirmed or retried batch to the iam audit log: who
// launched it and how big it was. Counts only, no names, addresses or the body.
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
