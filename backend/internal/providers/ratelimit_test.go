package providers_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		header   string
		wantOK   bool
		wantDiff time.Duration
	}{
		{
			name:     "positive integer seconds",
			header:   "120",
			wantOK:   true,
			wantDiff: 120 * time.Second,
		},
		{
			name:     "integer with surrounding whitespace",
			header:   "  45  ",
			wantOK:   true,
			wantDiff: 45 * time.Second,
		},
		{
			name:   "zero seconds",
			header: "0",
			wantOK: false,
		},
		{
			name:   "negative seconds",
			header: "-10",
			wantOK: false,
		},
		{
			name:   "empty header",
			header: "",
			wantOK: false,
		},
		{
			name:   "non-numeric text",
			header: "soon",
			wantOK: false,
		},
		{
			name:     "valid RFC1123 date in the future",
			header:   now.Add(90 * time.Second).Format(http.TimeFormat),
			wantOK:   true,
			wantDiff: 90 * time.Second,
		},
		{
			name:   "valid RFC1123 date in the past",
			header: now.Add(-30 * time.Second).Format(http.TimeFormat),
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := providers.ParseRetryAfter(tt.header, now)
			if ok != tt.wantOK {
				t.Fatalf("ParseRetryAfter(%q) ok = %v, want %v", tt.header, ok, tt.wantOK)
			}
			if ok && got != tt.wantDiff {
				t.Errorf("ParseRetryAfter(%q) = %v, want %v", tt.header, got, tt.wantDiff)
			}
		})
	}
}

func TestRateLimitError(t *testing.T) {
	t.Parallel()

	baseErr := errors.New("upstream 429 too many requests")
	retryAfter := 15 * time.Second

	err := providers.RateLimitError(baseErr, retryAfter)

	if !providers.IsTransient(err) {
		t.Errorf("IsTransient(err) = false, want true")
	}
	if providers.IsPermanent(err) {
		t.Errorf("IsPermanent(err) = true, want false")
	}

	if !errors.Is(err, baseErr) {
		t.Errorf("errors.Is(err, baseErr) = false, want true")
	}

	gotAfter, ok := providers.RetryAfter(err)
	if !ok || gotAfter != retryAfter {
		t.Errorf("RetryAfter(err) = (%v, %v), want (%v, true)", gotAfter, ok, retryAfter)
	}

	wrapped := fmt.Errorf("dispatcher call: %w", err)
	gotAfterWrapped, okWrapped := providers.RetryAfter(wrapped)
	if !okWrapped || gotAfterWrapped != retryAfter {
		t.Errorf("RetryAfter(wrapped) = (%v, %v), want (%v, true)", gotAfterWrapped, okWrapped, retryAfter)
	}

	unrelatedErr := errors.New("regular network error")
	if _, ok := providers.RetryAfter(unrelatedErr); ok {
		t.Errorf("RetryAfter(unrelatedErr) ok = true, want false")
	}
}
