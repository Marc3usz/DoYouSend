//go:build integration

package settings

import (
	"context"
	"testing"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/database/dbtest"
)

func TestPGStoreSettings(t *testing.T) {
	ctx := context.Background()
	s := NewPGStore(dbtest.New(t))
	if rows, err := s.ListSettings(ctx); err != nil || len(rows) != 0 {
		t.Fatalf("ListSettings(empty) = %v, %v", rows, err)
	}
	at := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	err := s.SaveSettings(ctx, []Row{
		{Key: KeySMSSenderName, Value: "SZKOLA", UpdatedAt: at},
		{Key: KeySMSAPIKey, Value: "v1:abc", Encrypted: true, UpdatedAt: at},
	}, nil)
	if err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	err = s.SaveSettings(ctx, []Row{{Key: KeySMSSenderName, Value: "SZKOLA-2", UpdatedAt: at.Add(time.Hour)}}, []string{KeySMSAPIKey, "missing"})
	if err != nil {
		t.Fatalf("SaveSettings(update+delete): %v", err)
	}
	rows, err := s.ListSettings(ctx)
	if err != nil || len(rows) != 1 || rows[0].Value != "SZKOLA-2" || rows[0].Encrypted || !rows[0].UpdatedAt.Equal(at.Add(time.Hour)) || rows[0].UpdatedBy != "" {
		t.Errorf("ListSettings = %+v, %v", rows, err)
	}
	// An unknown user ID violates the foreign key and saves nothing.
	if err := s.SaveSettings(ctx, []Row{{Key: KeyDryRun, Value: "false", UpdatedAt: at, UpdatedBy: "00000000-0000-4000-8000-999999999999"}}, nil); err == nil {
		t.Error("SaveSettings accepted an unknown user")
	}
}
