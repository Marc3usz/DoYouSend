package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/delivery"
	"github.com/Marc3usz/DoYouSend/backend/internal/iam"
	"github.com/Marc3usz/DoYouSend/backend/internal/platform/httpx"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers/email"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers/setup"
	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
	"github.com/Marc3usz/DoYouSend/backend/internal/senders"
	"github.com/Marc3usz/DoYouSend/backend/internal/settings"
)

// senderConfig is what senders needs from the current sending settings.
func senderConfig(settingsSvc *settings.Service, live *setup.Live) senders.Config {
	snap := settingsSvc.Current()
	_, isLive := live.DomainAuthenticator()
	provider := "fake"
	if isLive {
		provider = "sendgrid"
	}
	return senders.Config{
		Provider:  provider,
		Live:      isLive,
		Shared:    senders.ParseAddress(live.Config().EmailFrom),
		ReplyTo:   senders.ParseAddress(snap.ReplyTo()),
		PreferOwn: snap.Value(settings.KeyEmailSenderDefault) != "shared",
		// A real SMTP relay (not the local Mailpit) normally refuses other From
		// addresses than its own account.
		SharedOnly: !snap.DryRun() && snap.Value(settings.KeyEmailProvider) == "mailpit" &&
			!settings.IsLocalHost(snap.Value(settings.KeySMTPHost)),
	}
}

// sendersUser adapts the iam session user to senders.
func sendersUser(ctx context.Context) (senders.User, bool) {
	u, ok := iam.UserFrom(ctx)
	if !ok {
		return senders.User{}, false
	}
	return senders.User{ID: u.ID, Email: u.Email, FullName: u.FullName}, true
}

// senderResolver lets delivery pick a batch's From through senders. The batch
// is confirmed in the author's own request, so the session user is the author;
// a retry may come from an administrator, so the author is looked up by ID.
type senderResolver struct {
	svc   *senders.Service
	users interface {
		GetUser(ctx context.Context, id string) (iam.User, error)
	}
}

func (r senderResolver) CheckSender(ctx context.Context, authorID string, from providers.Address) error {
	u, err := r.users.GetUser(ctx, authorID)
	if err != nil {
		return fmt.Errorf("check sender: author %s: %w", authorID, err)
	}
	err = r.svc.MaySendFrom(ctx, senders.User{ID: u.ID, Email: u.Email, FullName: u.FullName}, from.Email)
	if errors.Is(err, senders.ErrNotAllowed) {
		return fmt.Errorf("%w: %w", delivery.ErrSenderNotAllowed, err)
	}
	return err
}

func (r senderResolver) ResolveSender(ctx context.Context, userID, senderID string) (delivery.EmailSender, error) {
	u, ok := sendersUser(ctx)
	if !ok || u.ID != userID {
		return delivery.EmailSender{}, fmt.Errorf("resolve sender: no signed-in author: %w", delivery.ErrSenderNotAllowed)
	}
	from, replyTo, err := r.svc.ResolveSender(ctx, u, senderID)
	if errors.Is(err, senders.ErrNotAllowed) {
		return delivery.EmailSender{}, fmt.Errorf("%w: %w", delivery.ErrSenderNotAllowed, err)
	}
	if err != nil {
		return delivery.EmailSender{}, err
	}
	return delivery.EmailSender{From: from, ReplyTo: replyTo}, nil
}

// domainAPI adapts the e-mail operator's domain authentication to senders.
type domainAPI struct{ api email.DomainAuthenticator }

func (a domainAPI) CreateDomain(ctx context.Context, domain string) (senders.ProviderDomain, error) {
	d, err := a.api.CreateDomain(ctx, domain)
	return toProviderDomain(d), domainError(err)
}

func (a domainAPI) GetDomain(ctx context.Context, id string) (senders.ProviderDomain, error) {
	d, err := a.api.GetDomain(ctx, id)
	return toProviderDomain(d), domainError(err)
}

func (a domainAPI) ValidateDomain(ctx context.Context, id string) (senders.ProviderDomain, error) {
	d, err := a.api.ValidateDomain(ctx, id)
	return toProviderDomain(d), domainError(err)
}

func (a domainAPI) DeleteDomain(ctx context.Context, id string) error {
	return domainError(a.api.DeleteDomain(ctx, id))
}

func toProviderDomain(d email.AuthDomain) senders.ProviderDomain {
	out := senders.ProviderDomain{ID: d.ID, Valid: d.Valid}
	for _, r := range d.Records {
		out.Records = append(out.Records, senders.DNSRecord{Type: r.Type, Host: r.Host, Data: r.Data, Valid: r.Valid})
	}
	return out
}

// domainError maps operator errors onto the senders ones; the original stays wrapped.
func domainError(err error) error {
	var apiErr *email.SenderAPIError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, email.ErrSenderNotFound):
		return fmt.Errorf("%w: %w", senders.ErrDomainGone, err)
	case errors.As(err, &apiErr) && errors.Is(err, email.ErrSenderRejected) && len(apiErr.Messages) > 0:
		return &senders.RejectedError{Message: strings.Join(apiErr.Messages, "; ")}
	case errors.Is(err, email.ErrSenderRejected):
		return &senders.RejectedError{Message: err.Error()}
	default:
		return err
	}
}

// Test messages from the settings panel. The body is the same on both channels.
const (
	testSubject = "DoYouSend: wiadomość testowa"
	testBody    = "Wiadomość testowa z DoYouSend. Jeśli ją widzisz, wysyłka tym kanałem działa."
	// actionTestMessageSent is the audit action of a test message; entity "settings".
	actionTestMessageSent = "settings_test_sent"
)

// handleTestMessage serves POST /api/admin/settings/test: one message through
// the current provider of a channel, to check the configuration. E-mail goes
// to the administrator's own address unless "to" is given; SMS needs "to".
// It honours DRY_RUN like every send and logs no address.
func handleTestMessage(live *setup.Live, auditor *iam.Service, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Channel string `json:"channel"`
			To      string `json:"to"`
		}
		if err := httpx.DecodeJSON(w, r, &body); err != nil {
			httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_request", Message: "expected a JSON TestMessageRequest"})
			return
		}
		actor, _ := iam.UserFrom(r.Context())
		channel := providers.Channel(body.Channel)
		to := strings.TrimSpace(body.To)
		switch channel {
		case providers.ChannelEmail:
			if to == "" {
				to = actor.Email
			}
			if recipients.ValidateEmail(to) != nil {
				testInvalid(w, "to", "must be an e-mail address")
				return
			}
		case providers.ChannelSMS:
			to = recipients.NormalizePhone(to)
			if recipients.ValidatePhone(to) != nil {
				testInvalid(w, "to", "must be a phone number like +48 500 100 101")
				return
			}
		default:
			testInvalid(w, "channel", "must be email or sms")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		res, err := live.Provider(channel).Send(ctx, providers.Message{
			RecipientID: "test-" + actor.ID, To: to, Subject: testSubject, Body: testBody,
		})
		if err != nil {
			// Not err itself: an SMTP or SendGrid reply may quote the address.
			logger.Warn("test message failed", "channel", channel, "user_id", actor.ID, "reason", providerErrorText(err))
			httpx.Error(w, http.StatusBadGateway, httpx.ErrorBody{Code: "provider_error", Message: providerErrorText(err)})
			return
		}
		dryRun := live.Config().DryRun
		auditor.Record(r.Context(), actor.ID, actionTestMessageSent, "settings", "", map[string]string{
			"channel": string(channel), "dryRun": fmt.Sprint(dryRun),
		})
		httpx.JSON(w, http.StatusOK, map[string]any{"dryRun": dryRun, "providerMessageId": res.ProviderMessageID})
	})
}

// providerErrorText says what kind of failure the operator reported, without
// the raw reply, which may quote the address or the number.
func providerErrorText(err error) string {
	switch {
	case providers.IsRateLimit(err):
		return "the operator is rate limiting; try again in a moment"
	case errors.Is(err, providers.ErrInvalidRecipient):
		return "the operator refused the address or number"
	case providers.IsPermanent(err):
		return "the operator refused the message; check the API key, sender and account settings"
	case providers.IsTransient(err):
		return "the operator could not be reached or is unavailable; check the host, port and network"
	default:
		return "the operator returned an unexpected answer; the message may or may not have been sent"
	}
}

func testInvalid(w http.ResponseWriter, field, msg string) {
	httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{
		Code: "invalid_input", Message: "invalid test message request",
		Fields: []httpx.FieldError{{Field: field, Message: msg}},
	})
}
