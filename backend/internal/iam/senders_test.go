package iam

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// memSenderStore is an in-memory SenderStore.
type memSenderStore struct {
	mu   sync.Mutex
	byID map[string]SenderIdentity
	fail error // returned by CreateSenderIdentity when set
}

func newMemSenderStore() *memSenderStore {
	return &memSenderStore{byID: map[string]SenderIdentity{}}
}

func (m *memSenderStore) GetSenderIdentity(_ context.Context, userID string) (SenderIdentity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.byID[userID]
	if !ok {
		return SenderIdentity{}, ErrSenderNotConfigured
	}
	return s, nil
}

func (m *memSenderStore) ListSenderIdentities(context.Context) ([]SenderIdentity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]SenderIdentity, 0, len(m.byID))
	for _, s := range m.byID {
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b SenderIdentity) int { return strings.Compare(a.UserID, b.UserID) })
	return out, nil
}

func (m *memSenderStore) CreateSenderIdentity(_ context.Context, s SenderIdentity) (SenderIdentity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return SenderIdentity{}, m.fail
	}
	if _, ok := m.byID[s.UserID]; ok {
		return SenderIdentity{}, ErrSenderExists
	}
	if s.Status == SenderVerified {
		s.VerifiedAt = s.CreatedAt
	}
	m.byID[s.UserID] = s
	return s, nil
}

func (m *memSenderStore) SaveSenderStatus(_ context.Context, userID string, status SenderStatus, at time.Time) (SenderIdentity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.byID[userID]
	if !ok {
		return SenderIdentity{}, ErrSenderNotConfigured
	}
	s.Status, s.CheckedAt = status, at
	switch {
	case status != SenderVerified:
		s.VerifiedAt = time.Time{}
	case s.VerifiedAt.IsZero():
		s.VerifiedAt = at
	}
	m.byID[userID] = s
	return s, nil
}

func (m *memSenderStore) DeleteSenderIdentity(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byID[userID]; !ok {
		return ErrSenderNotConfigured
	}
	delete(m.byID, userID)
	return nil
}

// fakeVerifier records calls and answers from its fields.
type fakeVerifier struct {
	mu        sync.Mutex
	calls     []string
	last      SenderRequest
	verified  map[string]bool
	gone      map[string]bool
	createErr error
	getErr    error
	next      int
}

func newFakeVerifier() *fakeVerifier {
	return &fakeVerifier{verified: map[string]bool{}, gone: map[string]bool{}}
}

func (f *fakeVerifier) CreateSender(_ context.Context, in SenderRequest) (ProviderSender, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "create")
	f.last = in
	if f.createErr != nil {
		return ProviderSender{}, f.createErr
	}
	f.next++
	return ProviderSender{ID: "sg-" + string(rune('0'+f.next))}, nil
}

func (f *fakeVerifier) GetSender(_ context.Context, id string) (ProviderSender, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "get "+id)
	if f.getErr != nil {
		return ProviderSender{}, f.getErr
	}
	if f.gone[id] {
		return ProviderSender{}, ErrSenderGone
	}
	return ProviderSender{ID: id, Verified: f.verified[id]}, nil
}

func (f *fakeVerifier) ResendVerification(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "resend "+id)
	if f.gone[id] {
		return ErrSenderGone
	}
	return nil
}

func (f *fakeVerifier) DeleteSender(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "delete "+id)
	if f.gone[id] {
		return ErrSenderGone
	}
	return nil
}

func testSenders(t *testing.T) (*Senders, *memSenderStore, *fakeVerifier, *memStore, User, User) {
	t.Helper()
	svc, users, _, admin, sender := testService(t)
	store, verifier := newMemSenderStore(), newFakeVerifier()
	return NewSenders(svc, store, verifier, SenderOptions{Provider: "sendgrid", Live: true}), store, verifier, users, admin, sender
}

func TestSendersRequest(t *testing.T) {
	s, store, verifier, users, admin, sender := testSenders(t)
	ctx := context.Background()

	got, err := s.Request(ctx, admin, sender.ID)
	if err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	if got.Status != SenderPending || got.ProviderID != "sg-1" || got.FromEmail != sender.Email || got.Provider != "sendgrid" {
		t.Errorf("Request() = %+v", got)
	}
	want := SenderRequest{
		Nickname: "DoYouSend: Anna Testowa (" + sender.ID[:8] + ")", FromEmail: sender.Email,
		FromName: sender.FullName, ReplyTo: sender.Email,
	}
	if verifier.last != want {
		t.Errorf("provider request = %+v, want %+v", verifier.last, want)
	}
	if last := users.audit[len(users.audit)-1]; last.Action != ActionSenderRequested || last.EntityID != sender.ID || last.UserID != admin.ID {
		t.Errorf("audit = %+v", last)
	}

	if _, err := s.Request(ctx, admin, sender.ID); !errors.Is(err, ErrSenderExists) {
		t.Errorf("second Request() error = %v, want ErrSenderExists", err)
	}
	if _, err := s.Request(ctx, admin, "00000000-0000-4000-8000-999999999999"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Request(unknown user) error = %v, want ErrNotFound", err)
	}
	if _, err := s.Request(ctx, admin, "not-an-id"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Request(bad id) error = %v, want ErrNotFound", err)
	}
	if len(store.byID) != 1 {
		t.Errorf("stored identities = %d, want 1", len(store.byID))
	}
}

func TestSendersRequestRejectsDisabledUsers(t *testing.T) {
	s, _, verifier, users, admin, sender := testSenders(t)
	users.users[users.find(sender.ID)].Disabled = true
	if _, err := s.Request(context.Background(), admin, sender.ID); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("Request(disabled) error = %v, want ErrInvalidInput", err)
	}
	if len(verifier.calls) != 0 {
		t.Errorf("provider was called: %v", verifier.calls)
	}
}

func TestSendersRequestRollsBackProviderIdentity(t *testing.T) {
	s, store, verifier, _, admin, sender := testSenders(t)
	store.fail = errors.New("db down")
	if _, err := s.Request(context.Background(), admin, sender.ID); err == nil {
		t.Fatal("Request() succeeded with a failing store")
	}
	if !slices.Equal(verifier.calls, []string{"create", "delete sg-1"}) {
		t.Errorf("provider calls = %v, want create then delete", verifier.calls)
	}
}

func TestSendersRequestPassesProviderRejection(t *testing.T) {
	s, store, verifier, _, admin, sender := testSenders(t)
	verifier.createErr = &SenderRejectedError{Message: "from_email: already exists"}
	_, err := s.Request(context.Background(), admin, sender.ID)
	var rejected *SenderRejectedError
	if !errors.As(err, &rejected) || rejected.Message != "from_email: already exists" {
		t.Errorf("Request() error = %v, want the provider rejection", err)
	}
	if len(store.byID) != 0 {
		t.Error("a rejected identity was stored")
	}
}

func TestSendersStatus(t *testing.T) {
	s, _, verifier, _, admin, sender := testSenders(t)
	ctx := context.Background()
	if _, err := s.Status(ctx, sender.ID); !errors.Is(err, ErrSenderNotConfigured) {
		t.Fatalf("Status(none) error = %v", err)
	}
	created, _ := s.Request(ctx, admin, sender.ID)

	got, err := s.Status(ctx, sender.ID)
	if err != nil || got.Status != SenderPending || got.CheckedAt.IsZero() {
		t.Errorf("Status(pending) = %+v, %v", got, err)
	}

	verifier.verified[created.ProviderID] = true
	got, err = s.Status(ctx, sender.ID)
	if err != nil || got.Status != SenderVerified || got.VerifiedAt.IsZero() {
		t.Errorf("Status(verified) = %+v, %v", got, err)
	}

	// A verified identity is not checked again.
	n := len(verifier.calls)
	if _, err := s.Status(ctx, sender.ID); err != nil || len(verifier.calls) != n {
		t.Errorf("Status(verified again) called the provider: %v", verifier.calls[n:])
	}
}

func TestSendersStatusMissingAndProviderErrors(t *testing.T) {
	s, _, verifier, _, admin, sender := testSenders(t)
	ctx := context.Background()
	created, _ := s.Request(ctx, admin, sender.ID)

	verifier.getErr = errors.New("sendgrid: HTTP 500")
	if _, err := s.Status(ctx, sender.ID); err == nil {
		t.Error("Status() hid a provider error")
	}
	verifier.getErr = nil
	verifier.gone[created.ProviderID] = true
	if got, err := s.Status(ctx, sender.ID); err != nil || got.Status != SenderMissing {
		t.Errorf("Status(gone) = %+v, %v, want missing", got, err)
	}
}

func TestSendersIgnoreIdentitiesOfAnotherProvider(t *testing.T) {
	s, store, verifier, _, admin, sender := testSenders(t)
	ctx := context.Background()
	store.byID[sender.ID] = SenderIdentity{UserID: sender.ID, Provider: "fake", ProviderID: "fake-1", Status: SenderPending}

	if got, err := s.Status(ctx, sender.ID); err != nil || got.Status != SenderPending {
		t.Errorf("Status() = %+v, %v", got, err)
	}
	var rejected *SenderRejectedError
	if err := s.Resend(ctx, admin, sender.ID); !errors.As(err, &rejected) {
		t.Errorf("Resend() error = %v, want SenderRejectedError", err)
	}
	if err := s.Remove(ctx, admin, sender.ID); err != nil {
		t.Errorf("Remove() error = %v", err)
	}
	if len(verifier.calls) != 0 {
		t.Errorf("the provider was asked about a foreign identity: %v", verifier.calls)
	}
}

func TestSendersResendAndRemove(t *testing.T) {
	s, store, verifier, users, admin, sender := testSenders(t)
	ctx := context.Background()
	if err := s.Resend(ctx, admin, sender.ID); !errors.Is(err, ErrSenderNotConfigured) {
		t.Errorf("Resend(none) error = %v", err)
	}
	created, _ := s.Request(ctx, admin, sender.ID)

	if err := s.Resend(ctx, admin, sender.ID); err != nil {
		t.Errorf("Resend() error = %v", err)
	}
	if last := users.audit[len(users.audit)-1]; last.Action != ActionSenderResent {
		t.Errorf("audit = %+v", last)
	}

	verifier.verified[created.ProviderID] = true
	_, _ = s.Status(ctx, sender.ID)
	if err := s.Resend(ctx, admin, sender.ID); !errors.Is(err, ErrSenderAlreadyVerified) {
		t.Errorf("Resend(verified) error = %v", err)
	}

	// Removing works even when someone already deleted it at the provider.
	verifier.gone[created.ProviderID] = true
	if err := s.Remove(ctx, admin, sender.ID); err != nil {
		t.Errorf("Remove() error = %v", err)
	}
	if len(store.byID) != 0 {
		t.Error("identity still stored after Remove")
	}
	if last := users.audit[len(users.audit)-1]; last.Action != ActionSenderRemoved {
		t.Errorf("audit = %+v", last)
	}
}

func testSenderServer(t *testing.T) (http.Handler, *fakeVerifier, User, User) {
	t.Helper()
	svc, _, _, admin, sender := testService(t)
	verifier := newFakeVerifier()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	NewHandler(svc, CookieOptions{Secure: true}, SendingConfig{}, logger).Register(mux)
	NewSenderHandler(NewSenders(svc, newMemSenderStore(), verifier, SenderOptions{Provider: "fake"}), logger).Register(mux)
	return Middleware(svc, logger, mux), verifier, admin, sender
}

func TestSenderEndpoints(t *testing.T) {
	h, verifier, _, sender := testSenderServer(t)
	adminCookie := login(t, h, "admin@example.test", adminPassword)
	senderCookie := login(t, h, "dyrektor@example.test", senderPassword)
	path := "/api/users/" + sender.ID + "/sender"

	steps := []struct {
		name     string
		method   string
		path     string
		cookie   *http.Cookie
		want     int
		wantCode string
	}{
		{"senders cannot manage identities", http.MethodPost, path, senderCookie, http.StatusForbidden, ""},
		{"senders cannot list identities", http.MethodGet, "/api/admin/senders", senderCookie, http.StatusForbidden, ""},
		{"none yet", http.MethodGet, path, adminCookie, http.StatusNotFound, "sender_not_configured"},
		{"request", http.MethodPost, path, adminCookie, http.StatusCreated, ""},
		{"request twice", http.MethodPost, path, adminCookie, http.StatusConflict, "sender_exists"},
		{"status", http.MethodGet, path, adminCookie, http.StatusOK, ""},
		{"resend", http.MethodPost, path + "/resend", adminCookie, http.StatusNoContent, ""},
		{"list", http.MethodGet, "/api/admin/senders", adminCookie, http.StatusOK, ""},
		{"remove", http.MethodDelete, path, adminCookie, http.StatusNoContent, ""},
		{"remove again", http.MethodDelete, path, adminCookie, http.StatusNotFound, "sender_not_configured"},
		{"unknown user", http.MethodPost, "/api/users/00000000-0000-4000-8000-999999999999/sender", adminCookie, http.StatusNotFound, "not_found"},
	}
	for _, st := range steps {
		rec := request(t, h, st.method, st.path, "", st.cookie)
		if rec.Code != st.want {
			t.Fatalf("%s: %s %s = %d %s, want %d", st.name, st.method, st.path, rec.Code, rec.Body, st.want)
		}
		if st.wantCode != "" {
			if got := decode(t, rec)["code"]; got != st.wantCode {
				t.Errorf("%s: code = %v, want %s", st.name, got, st.wantCode)
			}
		}
		if st.name == "request" {
			body := decode(t, rec)
			if body["status"] != "pending" || body["fromEmail"] != sender.Email || body["checkedAt"] != nil {
				t.Errorf("request body = %v", body)
			}
		}
		if st.name == "list" {
			body := decode(t, rec)
			if body["provider"] != "fake" || body["live"] != false {
				t.Errorf("list body = %v", body)
			}
			if items, _ := body["items"].([]any); len(items) != 1 {
				t.Errorf("list items = %v", body["items"])
			}
		}
	}

	verifier.createErr = &SenderRejectedError{Message: "from_email: already exists"}
	rec := request(t, h, http.MethodPost, path, "", adminCookie)
	if body := decode(t, rec); rec.Code != http.StatusUnprocessableEntity || body["code"] != "sender_rejected" || body["message"] != "from_email: already exists" {
		t.Errorf("rejected = %d %v", rec.Code, body)
	}
	verifier.createErr = errors.New("dial tcp: secret-ish detail")
	rec = request(t, h, http.MethodPost, path, "", adminCookie)
	if rec.Code != http.StatusBadGateway || strings.Contains(rec.Body.String(), "secret-ish") {
		t.Errorf("provider failure = %d %s", rec.Code, rec.Body)
	}
}
