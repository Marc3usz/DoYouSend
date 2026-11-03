package settings

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers/setup"
)

func testKey() []byte { return []byte("0123456789abcdef0123456789abcdef") }

func TestCipherRoundTripAndBinding(t *testing.T) {
	c, err := NewCipher(testKey())
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := c.Seal(KeySendGridAPIKey, "SG.secret")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, "SG.secret") || !strings.HasPrefix(sealed, "v1:") {
		t.Errorf("sealed = %q", sealed)
	}
	if got, err := c.Open(KeySendGridAPIKey, sealed); err != nil || got != "SG.secret" {
		t.Errorf("Open = %q, %v", got, err)
	}
	if _, err := c.Open(KeySMSAPIKey, sealed); !errors.Is(err, ErrUnreadableSecret) {
		t.Errorf("Open under another key name err = %v, want ErrUnreadableSecret", err)
	}
	other, _ := NewCipher([]byte("ffffffffffffffffffffffffffffffff"))
	if _, err := other.Open(KeySendGridAPIKey, sealed); !errors.Is(err, ErrUnreadableSecret) {
		t.Errorf("Open with another key err = %v", err)
	}
	if _, err := NewCipher([]byte("short")); err == nil {
		t.Error("NewCipher accepted a short key")
	}
}

func TestLoadKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "settings.key")

	key, created, err := LoadKey("", path)
	if err != nil || !created || len(key) != KeySize {
		t.Fatalf("LoadKey(new) = %d bytes, created %v, %v", len(key), created, err)
	}
	again, created, err := LoadKey("", path)
	if err != nil || created || string(again) != string(key) {
		t.Errorf("LoadKey(existing) created=%v err=%v, same key %v", created, err, string(again) == string(key))
	}

	env := base64.StdEncoding.EncodeToString(testKey())
	if got, _, err := LoadKey(env, path); err != nil || string(got) != string(testKey()) {
		t.Errorf("LoadKey(env) = %v, %v", got, err)
	}
	if _, _, err := LoadKey("bm90LTMyLWJ5dGVz", path); err == nil {
		t.Error("LoadKey accepted a short env key")
	}

	bad := filepath.Join(dir, "bad.key")
	_ = os.WriteFile(bad, []byte("garbage"), 0o600)
	if _, _, err := LoadKey("", bad); err == nil {
		t.Error("LoadKey accepted a broken key file")
	}
}

// memStore is an in-memory Store.
type memStore struct {
	mu   sync.Mutex
	rows map[string]Row
	fail error
}

func newMemStore() *memStore { return &memStore{rows: map[string]Row{}} }

func (m *memStore) ListSettings(context.Context) ([]Row, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Row, 0, len(m.rows))
	for _, r := range m.rows {
		out = append(out, r)
	}
	return out, nil
}

func (m *memStore) SaveSettings(_ context.Context, set []Row, remove []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return m.fail
	}
	for _, k := range remove {
		delete(m.rows, k)
	}
	for _, r := range set {
		m.rows[r.Key] = r
	}
	return nil
}

type fakeApplier struct {
	failWith  error
	activated []setup.Config
}

func (a *fakeApplier) Prepare(cfg setup.Config) (*setup.Prepared, error) {
	if a.failWith != nil {
		return nil, a.failWith
	}
	live, err := setup.NewLive(setup.Config{EmailProvider: "mailpit", SMSProvider: "fake", DryRun: true, EmailFrom: "a@example.test"}, nil)
	if err != nil {
		return nil, err
	}
	p, err := live.Prepare(setup.Config{EmailProvider: "mailpit", SMSProvider: "fake", DryRun: cfg.DryRun, EmailFrom: "a@example.test"})
	if err != nil {
		return nil, err
	}
	a.activated = append(a.activated, cfg)
	return p, nil
}

func (a *fakeApplier) Activate(*setup.Prepared) {}

type recAuditor struct {
	actions []string
	details []map[string]string
}

func (r *recAuditor) Record(_ context.Context, _, action, _, _ string, details map[string]string) {
	r.actions = append(r.actions, action)
	r.details = append(r.details, details)
}

func testService(t *testing.T, env map[string]string, locked bool) (*Service, *memStore, *fakeApplier, *recAuditor) {
	t.Helper()
	c, _ := NewCipher(testKey())
	store := newMemStore()
	svc := NewService(store, c, Options{
		LookupEnv:    func(k string) (string, bool) { v, ok := env[k]; return v, ok },
		DryRunLocked: locked,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:          func() time.Time { return time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC) },
	})
	applier, auditor := &fakeApplier{}, &recAuditor{}
	svc.SetApplier(applier)
	svc.SetAuditor(auditor)
	if _, err := svc.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	return svc, store, applier, auditor
}

func str(s string) *string { return &s }

func TestPrecedencePanelEnvDefault(t *testing.T) {
	svc, _, _, _ := testService(t, map[string]string{"SMS_SENDER_NAME": "SZKOLA-ENV", "DRY_RUN": "yes"}, false)
	snap := svc.Current()
	if st := snap.byKey[KeySMSSenderName]; st.Value != "SZKOLA-ENV" || st.Source != SourceEnv {
		t.Errorf("env value = %+v", st)
	}
	if st := snap.byKey[KeySMSProvider]; st.Value != "fake" || st.Source != SourceDefault {
		t.Errorf("default value = %+v", st)
	}
	// An unreadable boolean falls back to the default: a typo never turns DRY_RUN off.
	if !snap.DryRun() {
		t.Error(`DRY_RUN="yes" must fall back to the default (true)`)
	}

	snap, err := svc.Update(context.Background(), "u1", []Change{{Key: KeySMSSenderName, Value: str("SZKOLA-PANEL")}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if st := snap.byKey[KeySMSSenderName]; st.Value != "SZKOLA-PANEL" || st.Source != SourcePanel {
		t.Errorf("panel value = %+v", st)
	}
	snap, _ = svc.Update(context.Background(), "u1", []Change{{Key: KeySMSSenderName}}, false)
	if st := snap.byKey[KeySMSSenderName]; st.Value != "SZKOLA-ENV" || st.Source != SourceEnv {
		t.Errorf("after null = %+v, want back to env", st)
	}
}

func TestEnvBooleansParseLikeConfigLoad(t *testing.T) {
	tests := []struct {
		env  string
		want bool
	}{{"1", true}, {"TRUE", true}, {"true", true}, {"0", false}, {"false", false}, {"F", false}, {"nie", true}}
	for _, tt := range tests {
		svc, _, _, _ := testService(t, map[string]string{"DRY_RUN": tt.env}, false)
		if got := svc.Current().DryRun(); got != tt.want {
			t.Errorf("DRY_RUN=%q -> dry run %v, want %v", tt.env, got, tt.want)
		}
	}
}

func TestInvalidEnvValuesKeepTheDefault(t *testing.T) {
	svc, _, _, _ := testService(t, map[string]string{"SMS_PRICE_PER_PART_PLN": "0,08", "EMAIL_PROVIDER": "postfix"}, false)
	snap := svc.Current()
	if snap.SMSPriceMilli() != 80 || snap.Value(KeyEmailProvider) != "mailpit" {
		t.Errorf("price %d, provider %q; want the defaults", snap.SMSPriceMilli(), snap.Value(KeyEmailProvider))
	}
}

func TestUpdateValidates(t *testing.T) {
	svc, store, applier, _ := testService(t, nil, false)
	_, err := svc.Update(context.Background(), "u1", []Change{
		{Key: KeySMSPricePerPart, Value: str("0,08")},
		{Key: KeyEmailProvider, Value: str("postfix")},
		{Key: KeySMTPPort, Value: str("99999")},
		{Key: KeyEmailFrom, Value: str("not an address")},
		{Key: KeyDryRun, Value: str("maybe")},
		{Key: KeySMSAPIKey, Value: str("  ")},
		{Key: "nope", Value: str("x")},
	}, false)
	var v *ValidationError
	if !errors.As(err, &v) || len(v.Fields) != 7 {
		t.Fatalf("err = %v, want 7 field errors", err)
	}
	if len(store.rows) != 0 || len(applier.activated) != 0 {
		t.Error("a rejected update was stored or applied")
	}
}

func TestSecretsAreEncryptedAndHidden(t *testing.T) {
	svc, store, _, auditor := testService(t, nil, false)
	snap, err := svc.Update(context.Background(), "u1", []Change{{Key: KeySendGridAPIKey, Value: str("SG.top-secret")}}, false)
	if err != nil {
		t.Fatal(err)
	}
	row := store.rows[KeySendGridAPIKey]
	if !row.Encrypted || strings.Contains(row.Value, "top-secret") {
		t.Errorf("stored row = %+v", row)
	}
	if snap.ProviderConfig().SendGridAPIKey != "SG.top-secret" {
		t.Error("the provider config did not get the decrypted key")
	}
	for _, d := range auditor.details {
		for k, v := range d {
			if strings.Contains(v, "top-secret") {
				t.Errorf("audit detail %s leaks the secret", k)
			}
		}
	}

	// A different key makes the stored secret unreadable: it falls back, flagged.
	other, _ := NewCipher([]byte("ffffffffffffffffffffffffffffffff"))
	svc2 := NewService(store, other, Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	snap2, _ := svc2.Load(context.Background())
	if st := snap2.byKey[KeySendGridAPIKey]; !st.Unreadable || st.Set() {
		t.Errorf("unreadable secret = %+v", st)
	}
}

func TestRealSendingNeedsConfirmation(t *testing.T) {
	svc, store, _, auditor := testService(t, nil, false)
	ctx := context.Background()

	// Turning DRY_RUN off with Mailpit and the fake gateway stays on this machine.
	if _, err := svc.Update(ctx, "u1", []Change{{Key: KeyDryRun, Value: str("false")}}, false); err != nil {
		t.Fatalf("dry run off with local providers: %v", err)
	}
	_, err := svc.Update(ctx, "u1", []Change{
		{Key: KeySMSProvider, Value: str("smsapi")}, {Key: KeySMSAPIKey, Value: str("k")},
	}, false)
	if !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("err = %v, want ErrConfirmationRequired", err)
	}
	if _, ok := store.rows[KeySMSProvider]; ok {
		t.Error("an unconfirmed change was stored")
	}
	snap, err := svc.Update(ctx, "u1", []Change{
		{Key: KeySMSProvider, Value: str("smsapi")}, {Key: KeySMSAPIKey, Value: str("k")},
	}, true)
	if err != nil || !snap.SendsExternally() {
		t.Fatalf("confirmed update = %v, external %v", err, snap.SendsExternally())
	}
	last := auditor.details[len(auditor.details)-1]
	if last["sms_provider"] != "smsapi" || last["keys"] != "sms_api_key,sms_provider" {
		t.Errorf("audit details = %v", last)
	}
	// Already external: further changes need no new confirmation.
	if _, err := svc.Update(ctx, "u1", []Change{{Key: KeySMSSenderName, Value: str("SZKOLA")}}, false); err != nil {
		t.Errorf("change while already live: %v", err)
	}
}

func TestDryRunLock(t *testing.T) {
	svc, _, _, _ := testService(t, map[string]string{"DRY_RUN": "false"}, true)
	if !svc.Current().DryRun() {
		t.Error("DRY_RUN_LOCK must force dry run even with DRY_RUN=false")
	}
	if _, err := svc.Update(context.Background(), "u1", []Change{{Key: KeyDryRun, Value: str("false")}}, true); !errors.Is(err, ErrDryRunLocked) {
		t.Errorf("err = %v, want ErrDryRunLocked", err)
	}
	if !svc.Current().ProviderConfig().DryRun {
		t.Error("provider config must stay dry run under the lock")
	}
}

func TestProvidersThatCannotStartAreNotSaved(t *testing.T) {
	svc, store, applier, _ := testService(t, nil, false)
	applier.failWith = errors.New("SENDGRID_API_KEY is required when EMAIL_PROVIDER=sendgrid")
	_, err := svc.Update(context.Background(), "u1", []Change{{Key: KeyEmailProvider, Value: str("sendgrid")}}, true)
	var v *ValidationError
	if !errors.As(err, &v) || v.Fields[0].Field != "config" {
		t.Fatalf("err = %v, want a config field error", err)
	}
	if len(store.rows) != 0 || svc.Current().Value(KeyEmailProvider) != "mailpit" {
		t.Error("a configuration that cannot start was saved or made current")
	}
}

func TestStoreFailureKeepsOldProviders(t *testing.T) {
	svc, store, _, _ := testService(t, nil, false)
	store.fail = errors.New("db down")
	var seen int
	svc.OnChange(func(Snapshot) { seen++ })
	if _, err := svc.Update(context.Background(), "u1", []Change{{Key: KeySMSSenderName, Value: str("X")}}, false); err == nil {
		t.Fatal("Update succeeded with a failing store")
	}
	if seen != 0 || svc.Current().Value(KeySMSSenderName) != "SZKOLA-TEST" {
		t.Error("a failed save changed the current settings")
	}
}

func TestHTTP(t *testing.T) {
	svc, _, _, _ := testService(t, nil, false)
	mux := http.NewServeMux()
	NewHandler(svc, func(context.Context) string { return "u1" }, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux)

	do := func(method, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/admin/settings", strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	rec := do(http.MethodPatch, `{"values":{"sms_api_key":"SMSAPI-SECRET","sms_sender_name":"SZKOLA"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH = %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "SMSAPI-SECRET") {
		t.Error("PATCH response leaks the secret")
	}
	rec = do(http.MethodGet, "")
	if strings.Contains(rec.Body.String(), "SMSAPI-SECRET") || !strings.Contains(rec.Body.String(), `"key":"sms_api_key","group":"sms","kind":"secret","value":null,"set":true,"source":"panel"`) {
		t.Errorf("GET = %s", rec.Body)
	}

	tests := []struct {
		body, code string
		status     int
	}{
		{`{"values":{"sms_provider":"smsapi"}}`, "confirmation_required", 409},
		{`{"values":{"sms_sender_name":5}}`, "invalid_input", 400},
		{`{"values":{"email_provider":"x"}}`, "invalid_input", 400},
		{`not json`, "invalid_request", 400},
	}
	_ = do(http.MethodPatch, `{"values":{"dry_run":"false"}}`)
	for _, tt := range tests {
		rec := do(http.MethodPatch, tt.body)
		if rec.Code != tt.status || !strings.Contains(rec.Body.String(), `"code":"`+tt.code+`"`) {
			t.Errorf("PATCH %s = %d %s, want %d %s", tt.body, rec.Code, rec.Body, tt.status, tt.code)
		}
	}
	if rec := do(http.MethodPatch, `{"values":{"sms_provider":"smsapi"},"confirmLiveSending":true}`); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"sendsExternally":true`) {
		t.Errorf("confirmed PATCH = %d %s", rec.Code, rec.Body)
	}
}
