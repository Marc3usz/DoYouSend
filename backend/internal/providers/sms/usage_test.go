package sms

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type mockUsageStore struct {
	counts     RawUsageCounts
	err        error
	lastFilter UsageFilter
}

func (m *mockUsageStore) GetUsage(_ context.Context, filter UsageFilter) (RawUsageCounts, error) {
	m.lastFilter = filter
	if m.err != nil {
		return RawUsageCounts{}, m.err
	}
	return m.counts, nil
}

func TestFormatPLN(t *testing.T) {
	t.Parallel()

	cases := []struct {
		milli int64
		want  string
	}{
		{0, "0.00"},
		{80, "0.08"},
		{100, "0.10"},
		{1000, "1.00"},
		{24000, "24.00"},
		{1234, "1.234"},
		{-80, "-0.08"},
	}

	for _, tc := range cases {
		got := FormatPLN(tc.milli)
		if got != tc.want {
			t.Errorf("FormatPLN(%d) = %q, want %q", tc.milli, got, tc.want)
		}
	}
}

func TestCalculateUsageStats(t *testing.T) {
	t.Parallel()

	raw := RawUsageCounts{
		TotalMessages:     10,
		TotalParts:        25,
		DeliveredMessages: 8,
		DeliveredParts:    20,
		SentMessages:      1,
		SentParts:         2,
		FailedMessages:    1,
		FailedParts:       3,
		InFlightMessages:  0,
		InFlightParts:     0,
	}

	priceMilli := int64(80) // 0.08 PLN
	stats := CalculateUsageStats(raw, priceMilli)

	if stats.PricePerPartMilli != 80 {
		t.Errorf("PricePerPartMilli = %d, want 80", stats.PricePerPartMilli)
	}
	if stats.PricePerPartPLN != "0.08" {
		t.Errorf("PricePerPartPLN = %q, want '0.08'", stats.PricePerPartPLN)
	}
	if stats.TotalParts != 25 {
		t.Errorf("TotalParts = %d, want 25", stats.TotalParts)
	}
	if stats.TotalCostMilli != 2000 { // 25 * 80 = 2000
		t.Errorf("TotalCostMilli = %d, want 2000", stats.TotalCostMilli)
	}
	if stats.TotalCostPLN != "2.00" {
		t.Errorf("TotalCostPLN = %q, want '2.00'", stats.TotalCostPLN)
	}
	if stats.DeliveredCostMilli != 1600 { // 20 * 80 = 1600
		t.Errorf("DeliveredCostMilli = %d, want 1600", stats.DeliveredCostMilli)
	}
	if stats.DeliveredCostPLN != "1.60" {
		t.Errorf("DeliveredCostPLN = %q, want '1.60'", stats.DeliveredCostPLN)
	}
}

func TestHandleUsageStats_Success(t *testing.T) {
	t.Parallel()

	store := &mockUsageStore{
		counts: RawUsageCounts{
			TotalMessages:     5,
			TotalParts:        10,
			DeliveredMessages: 4,
			DeliveredParts:    8,
		},
	}

	handler := HandleUsageStats(store, 80, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/stats/sms", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rr.Code, http.StatusOK)
	}

	var stats UsageStats
	if err := json.NewDecoder(rr.Body).Decode(&stats); err != nil {
		t.Fatalf("decode json response: %v", err)
	}

	if stats.TotalMessages != 5 || stats.TotalParts != 10 {
		t.Errorf("unexpected stats: %+v", stats)
	}
	if stats.TotalCostMilli != 800 {
		t.Errorf("TotalCostMilli = %d, want 800", stats.TotalCostMilli)
	}
}

func TestHandleUsageStats_WithFilters(t *testing.T) {
	t.Parallel()

	store := &mockUsageStore{}
	handler := HandleUsageStats(store, 80, slog.Default())

	targetURL := "/api/stats/sms?batch_id=batch-123&from=2026-10-01T00:00:00Z&to=2026-10-07T23:59:59Z"
	req := httptest.NewRequest(http.MethodGet, targetURL, nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rr.Code, http.StatusOK)
	}
	if store.lastFilter.BatchID == nil || *store.lastFilter.BatchID != "batch-123" {
		t.Errorf("lastFilter.BatchID = %v, want 'batch-123'", store.lastFilter.BatchID)
	}
	if store.lastFilter.From == nil || store.lastFilter.From.IsZero() {
		t.Errorf("lastFilter.From is nil or zero")
	}
	if store.lastFilter.To == nil || store.lastFilter.To.IsZero() {
		t.Errorf("lastFilter.To is nil or zero")
	}
}

func TestHandleUsageStats_InvalidDateParam(t *testing.T) {
	t.Parallel()

	store := &mockUsageStore{}
	handler := HandleUsageStats(store, 80, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/stats/sms?from=not-a-date", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleUsageStats_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	store := &mockUsageStore{}
	handler := HandleUsageStats(store, 80, slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/api/stats/sms", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleUsageStats_Errors(t *testing.T) {
	t.Parallel()

	// Nil store
	handlerNil := HandleUsageStats(nil, 80, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/stats/sms", nil)
	rr := httptest.NewRecorder()
	handlerNil.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("nil store status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	// Store error
	storeErr := &mockUsageStore{err: errors.New("query failed")}
	handlerErr := HandleUsageStats(storeErr, 80, slog.Default())
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
