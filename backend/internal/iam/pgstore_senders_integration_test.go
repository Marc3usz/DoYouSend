//go:build integration

package iam

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/database/dbtest"
)

func TestPGStoreSenderIdentities(t *testing.T) {
	ctx := context.Background()
	s := NewPGStore(dbtest.New(t))
	user, err := s.CreateUser(ctx, User{Email: "nauczyciel@example.test", FullName: "Jan Przykladowy", Role: RoleSender}, "h")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSenderIdentity(ctx, user.ID); !errors.Is(err, ErrSenderNotConfigured) {
		t.Fatalf("GetSenderIdentity(none) error = %v", err)
	}

	created := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	in := SenderIdentity{UserID: user.ID, Provider: "sendgrid", ProviderID: "1234",
		FromEmail: user.Email, FromName: user.FullName, Status: SenderPending, CreatedAt: created}
	got, err := s.CreateSenderIdentity(ctx, in)
	if err != nil || got.Status != SenderPending || !got.CreatedAt.Equal(created) || !got.VerifiedAt.IsZero() || !got.CheckedAt.IsZero() {
		t.Fatalf("CreateSenderIdentity() = %+v, %v", got, err)
	}
	if _, err := s.CreateSenderIdentity(ctx, in); !errors.Is(err, ErrSenderExists) {
		t.Errorf("CreateSenderIdentity(twice) error = %v, want ErrSenderExists", err)
	}
	in.UserID = "00000000-0000-4000-8000-999999999999"
	in.ProviderID = "999"
	if _, err := s.CreateSenderIdentity(ctx, in); !errors.Is(err, ErrNotFound) {
		t.Errorf("CreateSenderIdentity(unknown user) error = %v, want ErrNotFound", err)
	}

	first := created.Add(time.Hour)
	got, err = s.SaveSenderStatus(ctx, user.ID, SenderVerified, first)
	if err != nil || got.Status != SenderVerified || !got.VerifiedAt.Equal(first) || !got.CheckedAt.Equal(first) {
		t.Errorf("SaveSenderStatus(verified) = %+v, %v", got, err)
	}
	later := first.Add(time.Hour)
	if got, _ = s.SaveSenderStatus(ctx, user.ID, SenderVerified, later); !got.VerifiedAt.Equal(first) || !got.CheckedAt.Equal(later) {
		t.Errorf("verified_at moved: %+v", got)
	}
	if got, _ = s.SaveSenderStatus(ctx, user.ID, SenderMissing, later); got.Status != SenderMissing || !got.VerifiedAt.IsZero() {
		t.Errorf("SaveSenderStatus(missing) = %+v", got)
	}
	if _, err := s.SaveSenderStatus(ctx, "00000000-0000-4000-8000-999999999999", SenderPending, later); !errors.Is(err, ErrSenderNotConfigured) {
		t.Errorf("SaveSenderStatus(none) error = %v", err)
	}

	if list, err := s.ListSenderIdentities(ctx); err != nil || len(list) != 1 || list[0].UserID != user.ID {
		t.Errorf("ListSenderIdentities() = %+v, %v", list, err)
	}
	if err := s.DeleteSenderIdentity(ctx, user.ID); err != nil {
		t.Errorf("DeleteSenderIdentity() error = %v", err)
	}
	if err := s.DeleteSenderIdentity(ctx, user.ID); !errors.Is(err, ErrSenderNotConfigured) {
		t.Errorf("DeleteSenderIdentity(again) error = %v", err)
	}
}
