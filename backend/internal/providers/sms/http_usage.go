package sms

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/httpx"
)

// parseTimeParam attempts to parse a timestamp query parameter using common formats.
func parseTimeParam(s string) (time.Time, error) {
	trimmed := strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", trimmed)
}

// HandleUsageStats returns an HTTP handler serving aggregated SMS usage metrics and costs.
//
// Query parameters:
//   - batch_id: filter by batch UUID
//   - from: filter by timestamp updated_at >= from (RFC3339 or YYYY-MM-DD)
//   - to: filter by timestamp updated_at <= to (RFC3339 or YYYY-MM-DD)
func HandleUsageStats(store UsageStore, pricePerPartMilli int64, logger *slog.Logger) http.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if store == nil {
			logger.Error("usage store is nil")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		filter := UsageFilter{}
		q := r.URL.Query()

		if bID := strings.TrimSpace(q.Get("batch_id")); bID != "" {
			filter.BatchID = &bID
		}

		if fromStr := strings.TrimSpace(q.Get("from")); fromStr != "" {
			t, err := parseTimeParam(fromStr)
			if err != nil {
				logger.Warn("invalid from parameter", "val", fromStr, "err", err)
				http.Error(w, "invalid from parameter, expected RFC3339 timestamp", http.StatusBadRequest)
				return
			}
			filter.From = &t
		}

		if toStr := strings.TrimSpace(q.Get("to")); toStr != "" {
			t, err := parseTimeParam(toStr)
			if err != nil {
				logger.Warn("invalid to parameter", "val", toStr, "err", err)
				http.Error(w, "invalid to parameter, expected RFC3339 timestamp", http.StatusBadRequest)
				return
			}
			filter.To = &t
		}

		counts, err := store.GetUsage(r.Context(), filter)
		if err != nil {
			logger.Error("get sms usage stats failed", "err", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		stats := CalculateUsageStats(counts, pricePerPartMilli)
		httpx.JSON(w, http.StatusOK, stats)
	}
}
