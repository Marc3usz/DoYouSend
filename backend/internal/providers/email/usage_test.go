package email

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockUsageStore struct {
	stats      UsageStats
	err        error
	lastFilter UsageFilter
}

func (m *mockUsageStore) GetUsage(_ context.Context, filter UsageFilter) (UsageStats, error) {
	m.lastFilter = filter
	if m.err != nil {
		return UsageStats{}, m.err
	}
	return m.stats, nil
}

func TestHandleUsageStats_Success(t *testing.T) {
	t.Parallel()

	store := &mockUsageStore{
		stats: UsageStats{
			TotalMessages:     15,
			DeliveredMessages: 12,
			SentMessages:      2,
			FailedMessages:    1,
			InFlightMessages:  0,
		},
	}

	handler := HandleUsageStats(store, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/stats/email", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rr.Code, http.StatusOK)
	}

	var stats UsageStats
	if err := json.NewDecoder(rr.Body).Decode(&stats); err != nil {
		t.Fatalf("decode json response: %v", err)
	}

	if stats.TotalMessages != 15 || stats.DeliveredMessages != 12 || stats.SentMessages != 2 || stats.FailedMessages != 1 {
		t.Errorf("unexpected stats: %+v", stats)
	}
}

func TestHandleUsageStats_WithFilters(t *testing.T) {
	t.Parallel()

	store := &mockUsageStore{}
	handler := HandleUsageStats(store, slog.Default())

	targetURL := "/api/stats/email?batch_id=33333333-3333-3333-3333-333333333331&from=2026-10-01T00:00:00Z&to=2026-10-07T23:59:59Z"
	req := httptest.NewRequest(http.MethodGet, targetURL, nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rr.Code, http.StatusOK)
	}
	if store.lastFilter.BatchID == nil || *store.lastFilter.BatchID != "33333333-3333-3333-3333-333333333331" {
		t.Errorf("lastFilter.BatchID = %v, want valid uuid", store.lastFilter.BatchID)
	}
	if store.lastFilter.From == nil || store.lastFilter.From.IsZero() {
		t.Errorf("lastFilter.From is nil or zero")
	}
	if store.lastFilter.To == nil || store.lastFilter.To.IsZero() {
		t.Errorf("lastFilter.To is nil or zero")
	}
	if store.lastFilter.ToExclusive {
		t.Errorf("lastFilter.ToExclusive = true for RFC3339 timestamp, want false")
	}
}

func TestHandleUsageStats_DateOnlyTo_HalfOpenInterval(t *testing.T) {
	t.Parallel()

	store := &mockUsageStore{}
	handler := HandleUsageStats(store, slog.Default())

	targetURL := "/api/stats/email?from=2026-10-01&to=2026-10-31"
	req := httptest.NewRequest(http.MethodGet, targetURL, nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rr.Code, http.StatusOK)
	}
	if store.lastFilter.From == nil || store.lastFilter.From.Format("2006-01-02") != "2026-10-01" {
		t.Errorf("lastFilter.From = %v, want 2026-10-01", store.lastFilter.From)
	}
	if loc := store.lastFilter.From.Location().String(); loc != "Europe/Warsaw" {
		t.Errorf("lastFilter.From.Location() = %s, want Europe/Warsaw", loc)
	}
	if store.lastFilter.To == nil || store.lastFilter.To.Format("2006-01-02") != "2026-11-01" {
		t.Errorf("lastFilter.To = %v, want 2026-11-01 (next midnight)", store.lastFilter.To)
	}
	if loc := store.lastFilter.To.Location().String(); loc != "Europe/Warsaw" {
		t.Errorf("lastFilter.To.Location() = %s, want Europe/Warsaw", loc)
	}
	if !store.lastFilter.ToExclusive {
		t.Errorf("lastFilter.ToExclusive = false, want true for date-only parameter")
	}
}

func TestHandleUsageStats_InvalidBatchUUID(t *testing.T) {
	t.Parallel()

	store := &mockUsageStore{}
	handler := HandleUsageStats(store, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/stats/email?batch_id=not-a-uuid", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleUsageStats_InvalidDateParam(t *testing.T) {
	t.Parallel()

	store := &mockUsageStore{}
	handler := HandleUsageStats(store, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/stats/email?from=not-a-date", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusBadRequest)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/stats/email?to=not-a-date", nil)
	rr2 := httptest.NewRecorder()

	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", rr2.Code, http.StatusBadRequest)
	}
}

func TestHandleUsageStats_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	store := &mockUsageStore{}
	handler := HandleUsageStats(store, slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/api/stats/email", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleUsageStats_Errors(t *testing.T) {
	t.Parallel()

	// Nil store
	handlerNil := HandleUsageStats(nil, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/stats/email", nil)
	rr := httptest.NewRecorder()
	handlerNil.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("nil store status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	// Store error
	storeErr := &mockUsageStore{err: errors.New("query failed")}
	handlerErr := HandleUsageStats(storeErr, slog.Default())
	rr = httptest.NewRecorder()
	handlerErr.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("store error status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestPGUsageStore_NilPool(t *testing.T) {
	t.Parallel()

	store := NewPGUsageStore(nil)
	_, err := store.GetUsage(context.Background(), UsageFilter{})
	if err == nil {
		t.Fatal("expected error for nil pool, got nil")
	}
}
