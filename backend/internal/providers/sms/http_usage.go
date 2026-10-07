package sms

import (
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/httpx"
)

var (
	uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

	warsawLocation = func() *time.Location {
		loc, err := time.LoadLocation("Europe/Warsaw")
		if err != nil {
			return time.FixedZone("Europe/Warsaw", 2*60*60)
		}
		return loc
	}()
)

// parseTimeParam attempts to parse a timestamp query parameter.
// Date-only format (YYYY-MM-DD) is interpreted in the Europe/Warsaw timezone.
// If isEnd is true for date-only format, it advances the timestamp to the next
// midnight in Europe/Warsaw (covering the full day) and returns isExclusive=true.
func parseTimeParam(s string, isEnd bool) (time.Time, bool, error) {
	trimmed := strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return t, false, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return t, false, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", trimmed, warsawLocation); err == nil {
		if isEnd {
			return t.AddDate(0, 0, 1), true, nil
		}
		return t, false, nil
	}
	return time.Time{}, false, fmt.Errorf("unrecognized time format: %q", trimmed)
}

// HandleUsageStats returns an HTTP handler serving aggregated SMS usage metrics and costs.
//
// TODO(iam): admin only - restrict access once IAM middleware lands.
//
// Query parameters:
//   - batch_id: filter by batch UUID
//   - from: filter by batch creation/confirmation timestamp >= from (RFC3339 or YYYY-MM-DD)
//   - to: filter by batch creation/confirmation timestamp <= to (RFC3339 or YYYY-MM-DD)
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
			if !uuidPattern.MatchString(bID) {
				logger.Warn("invalid batch_id parameter", "val", bID)
				http.Error(w, "invalid batch_id parameter, expected UUID", http.StatusBadRequest)
				return
			}
			filter.BatchID = &bID
		}

		if fromStr := strings.TrimSpace(q.Get("from")); fromStr != "" {
			t, _, err := parseTimeParam(fromStr, false)
			if err != nil {
				logger.Warn("invalid from parameter", "val", fromStr, "err", err)
				http.Error(w, "invalid from parameter, expected RFC3339 timestamp or YYYY-MM-DD date", http.StatusBadRequest)
				return
			}
			filter.From = &t
		}

		if toStr := strings.TrimSpace(q.Get("to")); toStr != "" {
			t, exclusive, err := parseTimeParam(toStr, true)
			if err != nil {
				logger.Warn("invalid to parameter", "val", toStr, "err", err)
				http.Error(w, "invalid to parameter, expected RFC3339 timestamp or YYYY-MM-DD date", http.StatusBadRequest)
				return
			}
			filter.To = &t
			filter.ToExclusive = exclusive
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
