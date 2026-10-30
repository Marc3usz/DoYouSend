package sms

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/httpx"
)

// ProfileResponse is the JSON response returned by HandleProfile.
type ProfileResponse struct {
	Points   float64 `json:"points"`
	Name     string  `json:"name"`
	UserType string  `json:"userType"`
}

// HandleProfile returns an HTTP handler serving GET /api/admin/providers/sms/profile.
func HandleProfile(profiler Profiler, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.Error(w, http.StatusMethodNotAllowed, httpx.ErrorBody{
				Code:    "invalid_request",
				Message: "method not allowed",
			})
			return
		}

		if profiler == nil {
			httpx.Error(w, http.StatusNotImplemented, httpx.ErrorBody{
				Code:    "not_supported",
				Message: "sms provider profile is not supported",
			})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		profile, err := profiler.GetProfile(ctx)
		if err != nil {
			logger.Error("failed to retrieve sms profile", "err", err)
			httpx.Error(w, http.StatusBadGateway, httpx.ErrorBody{
				Code:    "provider_error",
				Message: "failed to retrieve profile from sms provider",
			})
			return
		}

		httpx.JSON(w, http.StatusOK, ProfileResponse{
			Points:   profile.Points,
			Name:     profile.Name,
			UserType: profile.UserType,
		})
	})
}
