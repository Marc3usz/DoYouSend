//go:build integration

package iam

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/database/dbtest"
)

func TestPGStoreUsersAndSessions(t *testing.T) {
	ctx := context.Background()
	s := NewPGStore(dbtest.New(t))

	admin, err := s.CreateUser(ctx, User{Email: "admin@example.test", FullName: "Piotr Przykladowy", Role: RoleAdmin}, "hash-a")
	if err != nil || admin.ID == "" || admin.CreatedAt.IsZero() || admin.Disabled || !admin.LastLoginAt.IsZero() {
		t.Fatalf("CreateUser() = %+v, %v", admin, err)
	}
	if _, err := s.CreateUser(ctx, User{Email: "ADMIN@example.test", FullName: "X", Role: RoleSender}, "h"); !errors.Is(err, ErrEmailTaken) {
		t.Errorf("CreateUser(same e-mail, other case) error = %v, want ErrEmailTaken", err)
	}
	sender, err := s.CreateUser(ctx, User{Email: "dyrektor@example.test", FullName: "Anna Testowa", Role: RoleSender}, "hash-s")
	if err != nil {
		t.Fatal(err)
	}

	got, hash, err := s.UserByEmail(ctx, "admin@example.test")
	if err != nil || got.ID != admin.ID || hash != "hash-a" {
		t.Errorf("UserByEmail() = %+v, %q, %v", got, hash, err)
	}
	if _, _, err := s.UserByEmail(ctx, "nikt@example.test"); !errors.Is(err, ErrNotFound) {
		t.Errorf("UserByEmail(unknown) error = %v", err)
	}

	sender.Role, sender.Disabled, sender.FullName = RoleAdmin, true, "Anna Testowa-Nowak"
	updated, err := s.UpdateUser(ctx, sender)
	if err != nil || !updated.Disabled || updated.Role != RoleAdmin || updated.FullName != "Anna Testowa-Nowak" {
		t.Errorf("UpdateUser() = %+v, %v", updated, err)
	}
	if n, err := s.CountActiveAdmins(ctx); err != nil || n != 1 {
		t.Errorf("CountActiveAdmins() = %d, %v, want 1 (the disabled one does not count)", n, err)
	}
	users, err := s.ListUsers(ctx)
	if err != nil || len(users) != 2 || users[0].ID != admin.ID {
		t.Errorf("ListUsers() = %+v, %v, want the enabled admin first", users, err)
	}
	if err := s.SetPassword(ctx, admin.ID, "hash-b"); err != nil {
		t.Errorf("SetPassword() error = %v", err)
	}
	if _, hash, _ := s.UserByEmail(ctx, "admin@example.test"); hash != "hash-b" {
		t.Errorf("hash after SetPassword = %q", hash)
	}
	at := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	if err := s.TouchLogin(ctx, admin.ID, at); err != nil {
		t.Fatal(err)
	}
	if u, _ := s.GetUser(ctx, admin.ID); !u.LastLoginAt.Equal(at) {
		t.Errorf("LastLoginAt = %v, want %v", u.LastLoginAt, at)
	}

	now := time.Now()
	token := []byte("skrot-tokenu-32-bajty-xxxxxxxxxx")
	if err := s.CreateSession(ctx, token, admin.ID, now.Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if u, err := s.SessionUser(ctx, token, now); err != nil || u.ID != admin.ID {
		t.Errorf("SessionUser() = %+v, %v", u, err)
	}
	if _, err := s.SessionUser(ctx, token, now.Add(2*time.Hour)); !errors.Is(err, ErrNotFound) {
		t.Errorf("SessionUser(expired) error = %v, want ErrNotFound", err)
	}
	disabledToken := []byte("token-wylaczonego-konta-xxxxxxxx")
	_ = s.CreateSession(ctx, disabledToken, sender.ID, now.Add(time.Hour))
	if _, err := s.SessionUser(ctx, disabledToken, now); !errors.Is(err, ErrNotFound) {
		t.Errorf("SessionUser(disabled user) error = %v, want ErrNotFound", err)
	}
	if err := s.DeleteUserSessions(ctx, admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionUser(ctx, token, now); !errors.Is(err, ErrNotFound) {
		t.Errorf("SessionUser(after DeleteUserSessions) error = %v", err)
	}
}

func TestPGStoreAudit(t *testing.T) {
	ctx := context.Background()
	s := NewPGStore(dbtest.New(t))
	admin, err := s.CreateUser(ctx, User{Email: "admin@example.test", FullName: "Piotr Przykladowy", Role: RoleAdmin}, "h")
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range []AuditEntry{
		{Action: ActionLoginFailed, Entity: "user"},
		{UserID: admin.ID, Action: ActionUserCreated, Entity: "user", EntityID: admin.ID, Details: map[string]string{"role": "admin"}},
	} {
		if err := s.AddAudit(ctx, e); err != nil {
			t.Fatalf("AddAudit(%s) error = %v", e.Action, err)
		}
	}
	// Another domain may write non-string details.
	if _, err := s.pool.Exec(ctx, `INSERT INTO audit_log (user_id, action, details) VALUES ($1, 'batch_confirmed', '{"recipients": 12}')`, admin.ID); err != nil {
		t.Fatal(err)
	}

	items, total, err := s.ListAudit(ctx, 10, 0)
	if err != nil || total != 3 || len(items) != 3 {
		t.Fatalf("ListAudit() = %d items, total %d, %v", len(items), total, err)
	}
	if items[0].Action != "batch_confirmed" || items[0].Details["recipients"] != "12" || items[0].UserName != "Piotr Przykladowy" {
		t.Errorf("newest entry = %+v, want batch_confirmed with details as text and the user's name", items[0])
	}
	if items[1].Details["role"] != "admin" {
		t.Errorf("details = %v", items[1].Details)
	}
	if last := items[2]; last.UserID != "" || last.UserName != "" || last.EntityID != "" {
		t.Errorf("entry without a user = %+v, want empty user fields", last)
	}
	if page, _, _ := s.ListAudit(ctx, 1, 2); len(page) != 1 || page[0].Action != ActionLoginFailed {
		t.Errorf("ListAudit(1, 2) = %+v", page)
	}
}

func TestServiceWithPGStore(t *testing.T) {
	ctx := context.Background()
	store := NewPGStore(dbtest.New(t))
	svc := NewService(store)
	svc.hash = cheapHash
	hash, _ := cheapHash(adminPassword)
	if _, err := store.CreateUser(ctx, User{Email: "admin@example.test", FullName: "Piotr Przykladowy", Role: RoleAdmin}, hash); err != nil {
		t.Fatal(err)
	}

	session, err := svc.Login(ctx, "Admin@Example.test", adminPassword)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	u, err := svc.Authenticate(ctx, session.Token)
	if err != nil || u.LastLoginAt.IsZero() {
		t.Errorf("Authenticate() = %+v, %v, want the last login recorded", u, err)
	}
	if err := svc.Logout(ctx, u, session.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, session.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("Authenticate(after logout) error = %v", err)
	}
	page, err := svc.ListAudit(ctx, 0, 0)
	if err != nil || page.Total != 2 || page.Items[0].Action != ActionLogout {
		t.Errorf("audit = %+v, %v, want login then logout", page, err)
	}
}
