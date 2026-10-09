package iam

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

// memStore is an in-memory Store for service and HTTP tests.
type memStore struct {
	mu       sync.Mutex
	users    []User
	hashes   map[string]string // user ID -> password hash
	sessions map[string]memSession
	audit    []AuditEntry
	nextID   int
}

type memSession struct {
	userID  string
	expires time.Time
}

func newMemStore() *memStore {
	return &memStore{hashes: map[string]string{}, sessions: map[string]memSession{}}
}

func (m *memStore) ListUsers(context.Context) ([]User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.users), nil
}

func (m *memStore) find(id string) int {
	return slices.IndexFunc(m.users, func(u User) bool { return u.ID == id })
}

func (m *memStore) GetUser(_ context.Context, id string) (User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i := m.find(id); i >= 0 {
		return m.users[i], nil
	}
	return User{}, ErrNotFound
}

func (m *memStore) UserByEmail(_ context.Context, email string) (User, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Email == email {
			return u, m.hashes[u.ID], nil
		}
	}
	return User{}, "", ErrNotFound
}

func (m *memStore) CreateUser(_ context.Context, u User, hash string) (User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, other := range m.users {
		if other.Email == u.Email {
			return User{}, ErrEmailTaken
		}
	}
	m.nextID++
	u.ID = fmt.Sprintf("00000000-0000-4000-8000-%012d", m.nextID)
	u.CreatedAt = time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	m.users = append(m.users, u)
	m.hashes[u.ID] = hash
	return u, nil
}

func (m *memStore) UpdateUser(_ context.Context, u User) (User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.find(u.ID)
	if i < 0 {
		return User{}, ErrNotFound
	}
	m.users[i].FullName, m.users[i].Role, m.users[i].Disabled = u.FullName, u.Role, u.Disabled
	return m.users[i], nil
}

func (m *memStore) SetPassword(_ context.Context, id, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.find(id) < 0 {
		return ErrNotFound
	}
	m.hashes[id] = hash
	return nil
}

func (m *memStore) CountActiveAdmins(context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, u := range m.users {
		if u.IsAdmin() && !u.Disabled {
			n++
		}
	}
	return n, nil
}

func (m *memStore) TouchLogin(_ context.Context, id string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i := m.find(id); i >= 0 {
		m.users[i].LastLoginAt = at
	}
	return nil
}

func (m *memStore) CreateSession(_ context.Context, tokenHash []byte, userID string, expires time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[string(tokenHash)] = memSession{userID: userID, expires: expires}
	return nil
}

func (m *memStore) SessionUser(_ context.Context, tokenHash []byte, now time.Time) (User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[string(tokenHash)]
	if !ok || !s.expires.After(now) {
		return User{}, ErrNotFound
	}
	i := m.find(s.userID)
	if i < 0 || m.users[i].Disabled {
		return User{}, ErrNotFound
	}
	return m.users[i], nil
}

func (m *memStore) DeleteSession(_ context.Context, tokenHash []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, string(tokenHash))
	return nil
}

func (m *memStore) DeleteUserSessions(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, s := range m.sessions {
		if s.userID == userID {
			delete(m.sessions, k)
		}
	}
	return nil
}

func (m *memStore) AddAudit(_ context.Context, e AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e.ID = int64(len(m.audit) + 1)
	m.audit = append(m.audit, e)
	return nil
}

func (m *memStore) ListAudit(_ context.Context, limit, offset int) ([]AuditEntry, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	newest := slices.Clone(m.audit)
	slices.Reverse(newest)
	if offset >= len(newest) {
		return nil, len(newest), nil
	}
	return newest[offset:min(offset+limit, len(newest))], len(newest), nil
}

func (m *memStore) actions() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.audit))
	for i, e := range m.audit {
		out[i] = e.Action
	}
	return out
}

// cheapHash keeps tests fast; VerifyPassword reads the iteration count from
// the hash, so it checks these the same way as real ones.
func cheapHash(password string) (string, error) {
	return encodeHash(password, []byte("sol-testowa-16bt"), 1)
}

// Fictional staff only (CLAUDE.md rule 2).
const (
	adminPassword  = "haslo-admina-123"
	senderPassword = "haslo-dyrektora-1"
)

// testService returns a service with one administrator and one sender, and
// a clock the test can move.
func testService(t *testing.T) (*Service, *memStore, *time.Time, User, User) {
	t.Helper()
	store := newMemStore()
	svc := NewService(store)
	svc.hash = cheapHash
	now := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	h1, _ := cheapHash(adminPassword)
	admin, _ := store.CreateUser(context.Background(), User{Email: "admin@example.test", FullName: "Piotr Przykladowy", Role: RoleAdmin}, h1)
	h2, _ := cheapHash(senderPassword)
	sender, _ := store.CreateUser(context.Background(), User{Email: "dyrektor@example.test", FullName: "Anna Testowa", Role: RoleSender}, h2)
	return svc, store, &now, admin, sender
}

func TestLoginAndAuthenticate(t *testing.T) {
	svc, store, now, admin, _ := testService(t)
	ctx := context.Background()

	session, err := svc.Login(ctx, "  ADMIN@example.test ", adminPassword)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if session.User.ID != admin.ID || len(session.Token) < 40 || !session.ExpiresAt.Equal(now.Add(SessionTTL)) {
		t.Errorf("Login() = %+v", session)
	}
	if _, ok := store.sessions[session.Token]; ok {
		t.Error("the plain token is stored, want only its hash")
	}

	got, err := svc.Authenticate(ctx, session.Token)
	if err != nil || got.ID != admin.ID {
		t.Errorf("Authenticate() = %+v, %v", got, err)
	}
	if _, err := svc.Authenticate(ctx, session.Token+"x"); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("Authenticate(wrong token) error = %v, want ErrUnauthenticated", err)
	}
	if _, err := svc.Authenticate(ctx, ""); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("Authenticate(\"\") error = %v, want ErrUnauthenticated", err)
	}

	*now = now.Add(SessionTTL + time.Second)
	if _, err := svc.Authenticate(ctx, session.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("Authenticate(expired) error = %v, want ErrUnauthenticated", err)
	}

	if want := []string{ActionLogin}; !slices.Equal(store.actions(), want) {
		t.Errorf("audit = %v, want %v", store.actions(), want)
	}
}

func TestLoginFailuresLookAlike(t *testing.T) {
	svc, store, _, _, sender := testService(t)
	ctx := context.Background()

	disabled := true
	if _, err := store.UpdateUser(ctx, User{ID: sender.ID, FullName: sender.FullName, Role: sender.Role, Disabled: disabled}); err != nil {
		t.Fatal(err)
	}
	store.hashes["seed"] = "do-ustawienia-lokalnie"
	store.users = append(store.users, User{ID: "seed", Email: "seed@example.test", Role: RoleAdmin})

	for name, tc := range map[string][2]string{
		"wrong password":       {"admin@example.test", "zle-haslo-zle-haslo"},
		"unknown e-mail":       {"nikt@example.test", adminPassword},
		"disabled account":     {"dyrektor@example.test", senderPassword},
		"placeholder password": {"seed@example.test", "do-ustawienia-lokalnie"},
	} {
		if _, err := svc.Login(ctx, tc[0], tc[1]); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("%s: Login() error = %v, want ErrInvalidCredentials", name, err)
		}
	}
}

func TestLoginThrottling(t *testing.T) {
	svc, _, now, _, _ := testService(t)
	ctx := context.Background()

	for i := 0; i < MaxFailedLogins; i++ {
		if _, err := svc.Login(ctx, "admin@example.test", "zle-haslo-zle-haslo"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d error = %v", i+1, err)
		}
	}
	if _, err := svc.Login(ctx, "admin@example.test", adminPassword); !errors.Is(err, ErrTooManyAttempts) {
		t.Errorf("Login(after %d failures) error = %v, want ErrTooManyAttempts even with the right password", MaxFailedLogins, err)
	}
	if _, err := svc.Login(ctx, "dyrektor@example.test", senderPassword); err != nil {
		t.Errorf("another account is throttled too: %v", err)
	}

	*now = now.Add(FailedLoginWindow + time.Second)
	if _, err := svc.Login(ctx, "admin@example.test", adminPassword); err != nil {
		t.Errorf("Login(after the window) error = %v", err)
	}
}

func TestLogout(t *testing.T) {
	svc, _, _, admin, _ := testService(t)
	ctx := context.Background()
	session, _ := svc.Login(ctx, "admin@example.test", adminPassword)

	if err := svc.Logout(ctx, admin, session.Token); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if _, err := svc.Authenticate(ctx, session.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("Authenticate(after logout) error = %v", err)
	}
}

func TestCreateUser(t *testing.T) {
	svc, store, _, admin, _ := testService(t)
	ctx := context.Background()

	created, err := svc.CreateUser(ctx, admin, NewUser{Email: " Sekretariat@Example.test ", FullName: " Ewa Biurowa ", Role: RoleSender, Password: "haslo-sekretariatu"})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if created.Email != "sekretariat@example.test" || created.FullName != "Ewa Biurowa" {
		t.Errorf("CreateUser() = %+v, want normalized e-mail and trimmed name", created)
	}
	if ok, _ := VerifyPassword("haslo-sekretariatu", store.hashes[created.ID]); !ok {
		t.Error("stored hash does not verify the password")
	}

	_, err = svc.CreateUser(ctx, admin, NewUser{Email: "sekretariat@example.test", FullName: "X", Role: RoleSender, Password: "haslo-sekretariatu"})
	if !errors.Is(err, ErrEmailTaken) {
		t.Errorf("CreateUser(taken e-mail) error = %v, want ErrEmailTaken", err)
	}

	_, err = svc.CreateUser(ctx, admin, NewUser{Email: "zly", Role: "root", Password: "krotkie"})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || len(invalid.Fields) != 4 {
		t.Errorf("CreateUser(bad input) error = %v, want 4 field errors (email, fullName, role, password)", err)
	}
	if last := store.audit[len(store.audit)-1]; last.Action != ActionUserCreated || last.UserID != admin.ID || last.EntityID != created.ID {
		t.Errorf("last audit entry = %+v, want user_created by the admin", last)
	}
}

func TestUpdateUserProtectsAdministrators(t *testing.T) {
	svc, store, _, admin, sender := testService(t)
	ctx := context.Background()
	sender2Role, adminRole := RoleSender, RoleAdmin
	yes := true

	if _, err := svc.UpdateUser(ctx, admin, admin.ID, UserChange{Role: &sender2Role}); !errors.Is(err, ErrSelfLockout) {
		t.Errorf("demote self error = %v, want ErrSelfLockout", err)
	}
	if _, err := svc.UpdateUser(ctx, admin, admin.ID, UserChange{Disabled: &yes}); !errors.Is(err, ErrSelfLockout) {
		t.Errorf("disable self error = %v, want ErrSelfLockout", err)
	}

	// A second admin may demote the first, but not the last one standing.
	promoted, err := svc.UpdateUser(ctx, admin, sender.ID, UserChange{Role: &adminRole})
	if err != nil || !promoted.IsAdmin() {
		t.Fatalf("promote error = %v", err)
	}
	if _, err := svc.UpdateUser(ctx, promoted, admin.ID, UserChange{Disabled: &yes}); err != nil {
		t.Errorf("disable another admin error = %v", err)
	}
	if _, err := svc.UpdateUser(ctx, admin, promoted.ID, UserChange{Role: &sender2Role}); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("demote the last admin error = %v, want ErrLastAdmin", err)
	}
	if want := map[string]string{"role": "admin"}; !mapsEqual(store.audit[len(store.audit)-2].Details, want) {
		t.Errorf("promotion audit details = %v, want %v", store.audit[len(store.audit)-2].Details, want)
	}
}

func TestUpdateUserEndsSessionsOfDisabledUser(t *testing.T) {
	svc, _, _, admin, sender := testService(t)
	ctx := context.Background()
	session, _ := svc.Login(ctx, "dyrektor@example.test", senderPassword)
	yes := true

	if _, err := svc.UpdateUser(ctx, admin, sender.ID, UserChange{Disabled: &yes}); err != nil {
		t.Fatalf("disable error = %v", err)
	}
	if _, err := svc.Authenticate(ctx, session.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("disabled user still authenticated: %v", err)
	}
	blank := "   "
	var invalid *ValidationError
	if _, err := svc.UpdateUser(ctx, admin, sender.ID, UserChange{FullName: &blank}); !errors.As(err, &invalid) {
		t.Errorf("blank name error = %v, want ValidationError", err)
	}
	if _, err := svc.UpdateUser(ctx, admin, "nope", UserChange{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("bad id error = %v, want ErrNotFound", err)
	}
}

func TestResetPassword(t *testing.T) {
	svc, store, _, admin, sender := testService(t)
	ctx := context.Background()
	session, _ := svc.Login(ctx, "dyrektor@example.test", senderPassword)

	if err := svc.ResetPassword(ctx, admin, sender.ID, "nowe-haslo-dyrektora"); err != nil {
		t.Fatalf("ResetPassword() error = %v", err)
	}
	if _, err := svc.Authenticate(ctx, session.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Error("old session survives a password reset")
	}
	if _, err := svc.Login(ctx, "dyrektor@example.test", "nowe-haslo-dyrektora"); err != nil {
		t.Errorf("Login(new password) error = %v", err)
	}
	if err := svc.ResetPassword(ctx, admin, sender.ID, "krotkie"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("ResetPassword(weak) error = %v, want ErrInvalidInput", err)
	}
	if !slices.Contains(store.actions(), ActionPasswordReset) {
		t.Errorf("audit = %v, want password_reset", store.actions())
	}
}

func TestListAuditPaging(t *testing.T) {
	svc, store, _, admin, _ := testService(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_ = store.AddAudit(ctx, AuditEntry{UserID: admin.ID, Action: fmt.Sprintf("a%d", i)})
	}
	page, err := svc.ListAudit(ctx, 2, 0)
	if err != nil || page.Total != 3 || len(page.Items) != 2 || page.Items[0].Action != "a2" {
		t.Errorf("ListAudit(2, 0) = %+v, %v, want newest first", page, err)
	}
	if _, err := svc.ListAudit(ctx, MaxAuditPage+1, 0); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("ListAudit(too big) error = %v", err)
	}
}

func TestTokensAreHashed(t *testing.T) {
	token, hash, err := newToken()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(hash, []byte(token)) || !bytes.Equal(hash, hashToken(token)) {
		t.Error("newToken() hash is not the SHA-256 of the token")
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
