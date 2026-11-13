package email

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Domain authentication (SendGrid "Domain Authentication", ADR-0021).
//
// Once the school publishes the DNS records SendGrid gives for its domain and
// SendGrid validates them, every address in that domain may be a From address
// without single-sender verification, and mail is signed (DKIM) for the domain.

// AuthDomain is one authenticated domain as the operator reports it.
type AuthDomain struct {
	ID      string
	Domain  string
	Valid   bool
	Records []AuthDNSRecord
}

// AuthDNSRecord is one DNS record to publish, with the operator's last check.
type AuthDNSRecord struct {
	Type  string
	Host  string
	Data  string
	Valid bool
}

// DomainAuthenticator manages authenticated domains at the e-mail operator.
type DomainAuthenticator interface {
	CreateDomain(ctx context.Context, domain string) (AuthDomain, error)
	GetDomain(ctx context.Context, id string) (AuthDomain, error)
	// ValidateDomain makes the operator check the DNS records now and returns
	// the domain with the outcome.
	ValidateDomain(ctx context.Context, id string) (AuthDomain, error)
	DeleteDomain(ctx context.Context, id string) error
}

// SendGridDomains calls the SendGrid Domain Authentication API
// (/v3/whitelabel/domains). The key needs the "Sender Authentication" scope.
// Errors are *SenderAPIError: 404 matches ErrSenderNotFound, a refused request
// ErrSenderRejected. It is safe for concurrent use.
type SendGridDomains struct{ api *SendGridSenders }

// NewSendGridDomains returns a client using the APIKey, Region, BaseURL and
// HTTPClient of cfg.
func NewSendGridDomains(cfg SendGridConfig, logger *slog.Logger) (*SendGridDomains, error) {
	api, err := NewSendGridSenders(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("sendgrid domains: %w", err)
	}
	return &SendGridDomains{api: api}, nil
}

// sgDomain is the JSON shape of an authenticated domain. dns maps a record
// name (mail_cname, dkim1, dkim2, or mail_server/subdomain_spf/dkim without
// automatic security) to the record.
type sgDomain struct {
	ID     int64  `json:"id"`
	Domain string `json:"domain"`
	Valid  bool   `json:"valid"`
	DNS    map[string]struct {
		Valid bool   `json:"valid"`
		Type  string `json:"type"`
		Host  string `json:"host"`
		Data  string `json:"data"`
	} `json:"dns"`
}

func (d sgDomain) domain() AuthDomain {
	out := AuthDomain{ID: strconv.FormatInt(d.ID, 10), Domain: d.Domain, Valid: d.Valid}
	names := make([]string, 0, len(d.DNS))
	for name := range d.DNS {
		names = append(names, name)
	}
	sort.Strings(names) // stable order for the panel
	for _, name := range names {
		r := d.DNS[name]
		out.Records = append(out.Records, AuthDNSRecord{
			Type: strings.ToLower(r.Type), Host: r.Host, Data: r.Data, Valid: r.Valid,
		})
	}
	return out
}

// CreateDomain implements DomainAuthenticator with POST /v3/whitelabel/domains.
// Automatic security makes SendGrid manage SPF and DKIM through three CNAMEs.
func (c *SendGridDomains) CreateDomain(ctx context.Context, domain string) (AuthDomain, error) {
	body := map[string]any{"domain": domain, "automatic_security": true, "default": false}
	var out sgDomain
	if err := c.api.do(ctx, http.MethodPost, "/v3/whitelabel/domains", body, &out); err != nil {
		return AuthDomain{}, fmt.Errorf("create authenticated domain: %w", err)
	}
	c.api.logger.Info("sendgrid domain authentication created", "domain_id", out.ID, "valid", out.Valid)
	return out.domain(), nil
}

// GetDomain implements DomainAuthenticator with GET /v3/whitelabel/domains/{id}.
func (c *SendGridDomains) GetDomain(ctx context.Context, id string) (AuthDomain, error) {
	n, err := parseSenderID(id)
	if err != nil {
		return AuthDomain{}, err
	}
	var out sgDomain
	if err := c.api.do(ctx, http.MethodGet, "/v3/whitelabel/domains/"+strconv.FormatInt(n, 10), nil, &out); err != nil {
		return AuthDomain{}, fmt.Errorf("get authenticated domain %d: %w", n, err)
	}
	return out.domain(), nil
}

// ValidateDomain implements DomainAuthenticator: POST .../{id}/validate makes
// SendGrid look the records up, then the domain is read again with the
// per-record results.
func (c *SendGridDomains) ValidateDomain(ctx context.Context, id string) (AuthDomain, error) {
	n, err := parseSenderID(id)
	if err != nil {
		return AuthDomain{}, err
	}
	if err := c.api.do(ctx, http.MethodPost, "/v3/whitelabel/domains/"+strconv.FormatInt(n, 10)+"/validate", nil, nil); err != nil {
		return AuthDomain{}, fmt.Errorf("validate authenticated domain %d: %w", n, err)
	}
	return c.GetDomain(ctx, id)
}

// DeleteDomain implements DomainAuthenticator with DELETE /v3/whitelabel/domains/{id}.
func (c *SendGridDomains) DeleteDomain(ctx context.Context, id string) error {
	n, err := parseSenderID(id)
	if err != nil {
		return err
	}
	if err := c.api.do(ctx, http.MethodDelete, "/v3/whitelabel/domains/"+strconv.FormatInt(n, 10), nil, nil); err != nil {
		return fmt.Errorf("delete authenticated domain %d: %w", n, err)
	}
	return nil
}

// fakeDomainPrefix starts every ID FakeDomains hands out.
const fakeDomainPrefix = "fake-domain-"

// FakeDomains is an in-memory DomainAuthenticator for DRY_RUN and local
// development: it calls no API. New domains get realistic-looking CNAME
// records and turn valid on the first validation, so the panel can show both
// states. An unknown "fake-domain-" ID (created before an API restart) counts
// as a valid domain without records.
type FakeDomains struct {
	mu     sync.Mutex
	next   int64
	byID   map[string]AuthDomain
	logger *slog.Logger
}

// NewFakeDomains returns an empty FakeDomains.
func NewFakeDomains(logger *slog.Logger) *FakeDomains {
	if logger == nil {
		logger = slog.Default()
	}
	return &FakeDomains{byID: make(map[string]AuthDomain), logger: logger}
}

// CreateDomain implements DomainAuthenticator.
func (f *FakeDomains) CreateDomain(_ context.Context, domain string) (AuthDomain, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, d := range f.byID {
		if d.Domain == domain {
			return AuthDomain{}, fmt.Errorf("create authenticated domain: %w", &SenderAPIError{
				Status: http.StatusBadRequest, Messages: []string{"domain: already exists"}, kind: ErrSenderRejected,
			})
		}
	}
	f.next++
	n := strconv.FormatInt(f.next, 10)
	d := AuthDomain{ID: fakeDomainPrefix + n, Domain: domain, Records: []AuthDNSRecord{
		{Type: "cname", Host: "em" + n + "." + domain, Data: "u" + n + ".wl.sendgrid.net"},
		{Type: "cname", Host: "s1._domainkey." + domain, Data: "s1.domainkey.u" + n + ".wl.sendgrid.net"},
		{Type: "cname", Host: "s2._domainkey." + domain, Data: "s2.domainkey.u" + n + ".wl.sendgrid.net"},
	}}
	f.byID[d.ID] = d
	f.logger.Info("fake domain authentication created (nothing sent to SendGrid)", "domain_id", d.ID)
	return cloneDomain(d), nil
}

// GetDomain implements DomainAuthenticator.
func (f *FakeDomains) GetDomain(_ context.Context, id string) (AuthDomain, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.get(id)
}

func (f *FakeDomains) get(id string) (AuthDomain, error) {
	d, ok := f.byID[id]
	if ok {
		return cloneDomain(d), nil
	}
	if strings.HasPrefix(id, fakeDomainPrefix) {
		return AuthDomain{ID: id, Valid: true}, nil
	}
	return AuthDomain{}, fmt.Errorf("get authenticated domain %s: %w", id, &SenderAPIError{Status: http.StatusNotFound, kind: ErrSenderNotFound})
}

// ValidateDomain implements DomainAuthenticator: every record checks out.
func (f *FakeDomains) ValidateDomain(_ context.Context, id string) (AuthDomain, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.byID[id]
	if !ok {
		return f.get(id)
	}
	d.Valid = true
	for i := range d.Records {
		d.Records[i].Valid = true
	}
	f.byID[id] = d
	return cloneDomain(d), nil
}

// DeleteDomain implements DomainAuthenticator.
func (f *FakeDomains) DeleteDomain(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byID[id]; !ok && !strings.HasPrefix(id, fakeDomainPrefix) {
		return fmt.Errorf("delete authenticated domain %s: %w", id, &SenderAPIError{Status: http.StatusNotFound, kind: ErrSenderNotFound})
	}
	delete(f.byID, id)
	return nil
}

func cloneDomain(d AuthDomain) AuthDomain {
	d.Records = append([]AuthDNSRecord(nil), d.Records...)
	return d
}
