package sms

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
	if stats.TotalParts != 25 {
		t.Errorf("TotalParts = %d, want 25", stats.TotalParts)
	}
	// Planned cost: 25 * 80 = 2000 milli-PLN (2.00 PLN)
	if stats.PlannedCostMilli != 2000 {
		t.Errorf("PlannedCostMilli = %d, want 2000", stats.PlannedCostMilli)
	}
	// Billed cost: (20 + 2) * 80 = 1760 milli-PLN (1.76 PLN)
	if stats.BilledCostMilli != 1760 {
		t.Errorf("BilledCostMilli = %d, want 1760", stats.BilledCostMilli)
	}
	// Delivered cost: 20 * 80 = 1600 milli-PLN (1.60 PLN)
	if stats.DeliveredCostMilli != 1600 {
		t.Errorf("DeliveredCostMilli = %d, want 1600", stats.DeliveredCostMilli)
	}
	if stats.SentMessages != 1 || stats.SentParts != 2 {
		t.Errorf("Sent = (%d, %d), want (1, 2)", stats.SentMessages, stats.SentParts)
	}
	if stats.FailedMessages != 1 || stats.FailedParts != 3 {
		t.Errorf("Failed = (%d, %d), want (1, 3)", stats.FailedMessages, stats.FailedParts)
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
			SentMessages:      1,
			SentParts:         2,
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
	if stats.PlannedCostMilli != 800 {
		t.Errorf("PlannedCostMilli = %d, want 800", stats.PlannedCostMilli)
	}
	if stats.BilledCostMilli != 800 { // (8 + 2) * 80 = 800
		t.Errorf("BilledCostMilli = %d, want 800", stats.BilledCostMilli)
	}
	if stats.DeliveredCostMilli != 640 { // 8 * 80 = 640
		t.Errorf("DeliveredCostMilli = %d, want 640", stats.DeliveredCostMilli)
	}
}

func TestHandleUsageStats_WithFilters(t *testing.T) {
	t.Parallel()

	store := &mockUsageStore{}
	handler := HandleUsageStats(store, 80, slog.Default())

	targetURL := "/api/stats/sms?batch_id=33333333-3333-3333-3333-333333333331&from=2026-10-01T00:00:00Z&to=2026-10-07T23:59:59Z"
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
	handler := HandleUsageStats(store, 80, slog.Default())

	targetURL := "/api/stats/sms?from=2026-10-01&to=2026-10-31"
	req := httptest.NewRequest(http.MethodGet, targetURL, nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rr.Code, http.StatusOK)
	}
	if store.lastFilter.From == nil || store.lastFilter.From.Format("2006-01-02") != "2026-10-01" {
		t.Errorf("lastFilter.From = %v, want 2026-10-01", store.lastFilter.From)
	}
	if store.lastFilter.To == nil || store.lastFilter.To.Format("2006-01-02") != "2026-11-01" {
		t.Errorf("lastFilter.To = %v, want 2026-11-01 (next midnight)", store.lastFilter.To)
	}
	if !store.lastFilter.ToExclusive {
		t.Errorf("lastFilter.ToExclusive = false, want true for date-only parameter")
	}
}

func TestHandleUsageStats_InvalidBatchUUID(t *testing.T) {
	t.Parallel()

	store := &mockUsageStore{}
	handler := HandleUsageStats(store, 80, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/stats/sms?batch_id=not-a-uuid", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusBadRequest)
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

	req2 := httptest.NewRequest(http.MethodGet, "/api/stats/sms?to=not-a-date", nil)
	rr2 := httptest.NewRecorder()

	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", rr2.Code, http.StatusBadRequest)
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
