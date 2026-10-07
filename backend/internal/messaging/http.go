package messaging

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/Marc3usz/DoYouSend/backend/internal/groups"
	"github.com/Marc3usz/DoYouSend/backend/internal/platform/httpx"
)

// HandlePreview serves POST /api/messages/preview (docs/api/openapi.yaml): the
// SendEstimate the composer shows while the sender types. The body is never logged.
func HandlePreview(p *Previewer, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var draft draftJSON
		if err := httpx.DecodeJSON(w, r, &draft); err != nil {
			httpx.Error(w, http.StatusBadRequest, httpx.ErrorBody{Code: "invalid_request", Message: "expected a JSON message draft"})
			return
		}

		est, err := p.Preview(r.Context(), draft.Body, draft.Selection.toSelection())
		var invalid *groups.ValidationError
		switch {
		case err == nil:
			httpx.JSON(w, http.StatusOK, toEstimateJSON(est))
		case errors.As(err, &invalid):
			out := httpx.ErrorBody{Code: "invalid_input", Message: "invalid recipient selection"}
			for _, f := range invalid.Fields {
				out.Fields = append(out.Fields, httpx.FieldError{Field: f.Field, Message: f.Message})
			}
			httpx.Error(w, http.StatusBadRequest, out)
		case errors.Is(err, context.Canceled):
			// The composer aborts a stale preview on every keystroke; nobody reads the answer.
		default:
			logger.Error("messages preview", "err", err)
			httpx.Error(w, http.StatusInternalServerError, httpx.ErrorBody{Code: "internal", Message: "preview failed"})
		}
	}
}

// JSON shapes from docs/api/openapi.yaml, kept apart from the domain types so the wire
// format does not change when the domain does.
type (
	draftJSON struct {
		Subject   string        `json:"subject"`
		Body      string        `json:"body"`
		Selection selectionJSON `json:"selection"`
	}
	selectionJSON struct {
		GroupIDs             []string `json:"groupIds"`
		RecipientIDs         []string `json:"recipientIds"`
		ExcludedRecipientIDs []string `json:"excludedRecipientIds"`
	}
	smsLengthJSON struct {
		Encoding string `json:"encoding"`
		Units    int    `json:"units"`
		Parts    int    `json:"parts"`
	}
	estimateJSON struct {
		Template             smsLengthJSON `json:"template"`
		Placeholders         []string      `json:"placeholders"`
		UnknownPlaceholders  []string      `json:"unknownPlaceholders"`
		RecipientCount       int           `json:"recipientCount"`
		EmailCount           int           `json:"emailCount"`
		SMSCount             int           `json:"smsCount"`
		PartialCount         int           `json:"partialCount"`
		UnreachableCount     int           `json:"unreachableCount"`
		UCS2Count            int           `json:"ucs2Count"`
		MinPartsPerRecipient int           `json:"minPartsPerRecipient"`
		MaxPartsPerRecipient int           `json:"maxPartsPerRecipient"`
		TotalSMSParts        int           `json:"totalSmsParts"`
		CostMilli            int64         `json:"costMilli"`
		RenderFailedIDs      []string      `json:"renderFailedIds"`
	}
)

func (s selectionJSON) toSelection() groups.Selection {
	return groups.Selection{
		GroupIDs:             s.GroupIDs,
		RecipientIDs:         s.RecipientIDs,
		ExcludedRecipientIDs: s.ExcludedRecipientIDs,
	}
}

// wireEncoding maps an Encoding to the openapi enum, which has no hyphen.
var wireEncoding = map[Encoding]string{EncodingGSM7: "GSM7", EncodingUCS2: "UCS2"}

// toEstimateJSON converts an estimate; empty lists encode as [], not null.
func toEstimateJSON(est Estimate) estimateJSON {
	s := est.Summary
	return estimateJSON{
		Template: smsLengthJSON{
			Encoding: wireEncoding[est.Template.Encoding],
			Units:    est.Template.Units,
			Parts:    est.Template.Parts,
		},
		Placeholders:         nonNil(est.Placeholders),
		UnknownPlaceholders:  nonNil(est.UnknownPlaceholders),
		RecipientCount:       est.Recipients,
		EmailCount:           s.EmailCount,
		SMSCount:             s.SMSCount,
		PartialCount:         s.PartialCount,
		UnreachableCount:     s.UnreachableCount,
		UCS2Count:            s.UCS2Count,
		MinPartsPerRecipient: s.MinPartsPerRecipient,
		MaxPartsPerRecipient: s.MaxPartsPerRecipient,
		TotalSMSParts:        s.TotalParts,
		CostMilli:            s.CostMilli,
		RenderFailedIDs:      nonNil(est.RenderFailedIDs),
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
