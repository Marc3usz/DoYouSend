package setup

import (
	"context"
	"log/slog"
	"sync/atomic"

	"github.com/Marc3usz/DoYouSend/backend/internal/delivery"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers/email"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers/sms"
)

// Live holds the providers built from the current configuration and lets the
// administration panel swap them without restarting the API (ADR-0018).
//
// Everything handed out by Live (senders for the dispatcher, profilers, the
// sender verifier) looks up the current providers on every call, so a swap
// applies to the next message sent. It is safe for concurrent use.
type Live struct {
	current atomic.Pointer[liveState]
	fake    *email.FakeSenders // kept across swaps, so test identities survive them
	domains *email.FakeDomains // likewise for test domains
	logger  *slog.Logger
}

type liveState struct {
	cfg         Config
	provs       *Providers
	verifier    email.SenderVerifier
	liveSenders bool
	domains     email.DomainAuthenticator
}

// Prepared is a configuration whose providers were built successfully but
// are not in use yet. Activate puts it in use.
type Prepared struct{ state *liveState }

// Config returns the configuration the prepared providers were built from.
func (p *Prepared) Config() Config { return p.state.cfg }

// NewLive builds the providers for cfg and returns a Live using them.
func NewLive(cfg Config, logger *slog.Logger) (*Live, error) {
	if logger == nil {
		logger = slog.Default()
	}
	l := &Live{fake: email.NewFakeSenders(logger), domains: email.NewFakeDomains(logger), logger: logger}
	p, err := l.Prepare(cfg)
	if err != nil {
		return nil, err
	}
	l.Activate(p)
	return l, nil
}

// Prepare builds the providers for cfg without using them. An error means cfg
// cannot work (unknown provider, missing API key, bad sender address).
func (l *Live) Prepare(cfg Config) (*Prepared, error) {
	provs, err := New(cfg, l.logger)
	if err != nil {
		return nil, err
	}
	verifier, liveSenders, err := NewSenderVerifier(cfg, l.logger)
	if err != nil {
		return nil, err
	}
	var domains email.DomainAuthenticator = l.domains
	if liveSenders {
		if domains, err = email.NewSendGridDomains(email.SendGridConfig{APIKey: cfg.SendGridAPIKey, Region: cfg.SendGridRegion}, l.logger); err != nil {
			return nil, err
		}
	} else {
		verifier = l.fake
	}
	return &Prepared{state: &liveState{cfg: cfg, provs: provs, verifier: verifier, liveSenders: liveSenders, domains: domains}}, nil
}

// Activate makes p the current configuration. Messages already handed to a
// provider are not affected; the next Send uses the new providers.
func (l *Live) Activate(p *Prepared) {
	l.current.Store(p.state)
	l.logger.Info("delivery providers configured",
		"email", p.state.cfg.EmailProvider, "sms", p.state.cfg.SMSProvider, "dryRun", p.state.cfg.DryRun)
}

func (l *Live) state() *liveState { return l.current.Load() }

// Config returns the configuration in use.
func (l *Live) Config() Config { return l.state().cfg }

// Dispatcher returns a dispatcher whose e-mail and SMS senders always use the
// current providers.
func (l *Live) Dispatcher(policy delivery.RetryPolicy, logger *slog.Logger) (*delivery.Dispatcher, error) {
	return delivery.NewDispatcher([]delivery.Sender{
		liveSender{l: l, channel: providers.ChannelEmail},
		liveSender{l: l, channel: providers.ChannelSMS},
	}, policy, logger)
}

// SMSProfiler returns the current SMS profiler, or nil (see Providers.SMSProfiler).
func (l *Live) SMSProfiler() sms.Profiler { return l.state().provs.SMSProfiler() }

// EmailProfiler returns the current e-mail profiler, or nil (see Providers.EmailProfiler).
func (l *Live) EmailProfiler() email.Profiler { return l.state().provs.EmailProfiler() }

// SenderVerifier returns the current sender-identity API and whether it is the
// real one (false: the in-memory fake, ADR-0016).
func (l *Live) SenderVerifier() (email.SenderVerifier, bool) {
	s := l.state()
	return s.verifier, s.liveSenders
}

// Provider returns the current provider of channel, e.g. for a test message
// from the settings panel. It honours DRY_RUN like every send.
func (l *Live) Provider(channel providers.Channel) providers.Provider {
	if channel == providers.ChannelEmail {
		return l.state().provs.Email
	}
	return l.state().provs.SMS
}

// DomainAuthenticator returns the current domain-authentication API and
// whether it is the real one (false: the in-memory fake, under DRY_RUN or
// without SendGrid, ADR-0021).
func (l *Live) DomainAuthenticator() (email.DomainAuthenticator, bool) {
	s := l.state()
	return s.domains, s.liveSenders
}

// liveSender is a delivery.Sender that forwards to the current provider of
// its channel.
type liveSender struct {
	l       *Live
	channel providers.Channel
}

func (s liveSender) Channel() providers.Channel { return s.channel }

func (s liveSender) Send(ctx context.Context, msg providers.Message) (providers.Result, error) {
	p := s.l.state().provs
	if s.channel == providers.ChannelEmail {
		return p.Email.Send(ctx, msg)
	}
	return p.SMS.Send(ctx, msg)
}
