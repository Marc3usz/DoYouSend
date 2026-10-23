package email

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/httpx"
)

// ProfileResponse is the JSON response returned by HandleProfile.
type ProfileResponse struct {
	Provider       string `json:"provider"`
	RemainCredits  *int64 `json:"remainCredits,omitempty"`
	TotalCredits   *int64 `json:"totalCredits,omitempty"`
	UsedCredits    *int64 `json:"usedCredits,omitempty"`
	ResetFrequency string `json:"resetFrequency,omitempty"`
	NextReset      string `json:"nextReset,omitempty"`
}

// HandleProfile returns an HTTP handler serving GET /api/admin/providers/email/profile.
func HandleProfile(profiler Profiler, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if profiler == nil {
			httpx.Error(w, http.StatusNotImplemented, httpx.ErrorBody{
				Code:    "not_supported",
				Message: "email provider profile is not supported",
			})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		profile, err := profiler.GetProfile(ctx)
		if err != nil {
			logger.Error("failed to retrieve email profile", "err", err)
			httpx.Error(w, http.StatusBadGateway, httpx.ErrorBody{
				Code:    "provider_error",
				Message: "failed to retrieve profile from email provider",
			})
			return
		}

		httpx.JSON(w, http.StatusOK, ProfileResponse{
			Provider:       profile.Provider,
			RemainCredits:  profile.RemainCredits,
			TotalCredits:   profile.TotalCredits,
			UsedCredits:    profile.UsedCredits,
			ResetFrequency: profile.ResetFrequency,
			NextReset:      profile.NextReset,
		})
	})
}
