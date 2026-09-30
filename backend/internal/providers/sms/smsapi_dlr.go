package sms

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// SMSAPIDLRAckResponse is the plain-text response body that the SMSAPI callback
// endpoint must return to acknowledge receipt. If the endpoint does not return "OK",
// SMSAPI retries sending the report periodically.
const SMSAPIDLRAckResponse = "OK"

// mapSMSAPIStatus maps SMSAPI status code or name to canonical DeliveryStatus
// based on official documentation: https://www.smsapi.pl/docs/#18-lista-statusow-doreczenia
func mapSMSAPIStatus(raw string) (providers.DeliveryStatus, string, bool) {
	switch strings.ToUpper(raw) {
	// Terminal success
	case "404", "DELIVERED":
		return providers.StatusDelivered, "", true

	// In-flight / operator progress (StatusSent, never StatusSending to avoid duplicate delivery)
	case "403", "SENT", "409", "QUEUE", "410", "ACCEPTED", "411", "RENEWAL":
		return providers.StatusSent, "", true

	// Terminal failures
	case "401", "NOT_FOUND":
		return providers.StatusFailed, "message not found or report expired in SMSAPI", true
	case "402", "EXPIRED":
		return providers.StatusFailed, "delivery timed out, message expired before reaching handset", true
	case "405", "UNDELIVERED":
		return providers.StatusFailed, "message could not be delivered to the handset", true
	case "406", "FAILED":
		return providers.StatusFailed, "message sending failed at SMSAPI gateway", true
	case "407", "REJECTED":
		return providers.StatusFailed, "message rejected by carrier or invalid recipient number", true
	case "408", "UNKNOWN":
		return providers.StatusFailed, "no delivery report available from carrier (undeliverable)", true
	case "412", "STOP":
		return providers.StatusFailed, "message delivery stopped", true

	default:
		return "", "", false
	}
}

// ParseSMSAPIDLR parses parameters from an SMSAPI delivery report callback.
//
// According to SMSAPI documentation, reports are sent via HTTP GET (or POST)
// with query/form parameters:
//   - MsgId (or msg_id): message ID(s), comma-separated when batched
//   - status: numerical status code(s) (e.g. "404", "405"), comma-separated
//   - status_name: optional textual status name(s) (e.g. "DELIVERED"), comma-separated
//   - donedate: optional delivery unixtime timestamp(s), comma-separated
//
// Example query string:
//   MsgId=613F1B14346335B944450980&status=404&status_name=DELIVERED&donedate=1631525653
//
// When multiple reports arrive in a single request, values are comma-separated:
//   MsgId=id1,id2&status=404,405&donedate=1631525653,1631525676
//
// Returns a slice of DeliveryReport structs, one per message ID.
// Returns ErrMalformedReport if required fields are missing, counts mismatch,
// or status codes are unrecognised.
func ParseSMSAPIDLR(values url.Values) ([]providers.DeliveryReport, error) {
	msgIDRaw := getFirst(values, "MsgId", "msg_id", "id")
	if strings.TrimSpace(msgIDRaw) == "" {
		return nil, fmt.Errorf("%w: missing required parameter MsgId", providers.ErrMalformedReport)
	}

	statusRaw := getFirst(values, "status", "status_name")
	if strings.TrimSpace(statusRaw) == "" {
		return nil, fmt.Errorf("%w: missing required parameter status", providers.ErrMalformedReport)
	}

	dateRaw := getFirst(values, "donedate", "date")

	ids := splitTrimmed(msgIDRaw)
	statuses := splitTrimmed(statusRaw)
	dates := splitTrimmed(dateRaw)

	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: empty MsgId list", providers.ErrMalformedReport)
	}
	if len(ids) != len(statuses) {
		return nil, fmt.Errorf("%w: mismatched MsgId count (%d) and status count (%d)",
			providers.ErrMalformedReport, len(ids), len(statuses))
	}
	if len(dates) > 0 && len(dates) != len(ids) {
		return nil, fmt.Errorf("%w: mismatched MsgId count (%d) and donedate count (%d)",
			providers.ErrMalformedReport, len(ids), len(dates))
	}

	reports := make([]providers.DeliveryReport, len(ids))
	for i, id := range ids {
		if id == "" {
			return nil, fmt.Errorf("%w: empty MsgId item at index %d", providers.ErrMalformedReport, i)
		}

		rawStatus := statuses[i]
		st, errMsg, ok := mapSMSAPIStatus(rawStatus)
		if !ok {
			return nil, fmt.Errorf("%w: unrecognized SMSAPI status %q for MsgId %s",
				providers.ErrMalformedReport, rawStatus, id)
		}

		var ts time.Time
		if i < len(dates) && dates[i] != "" {
			if sec, err := strconv.ParseInt(dates[i], 10, 64); err == nil && sec > 0 {
				ts = time.Unix(sec, 0).UTC()
			}
		}

		reports[i] = providers.DeliveryReport{
			ProviderMessageID: id,
			Channel: providers.ChannelSMS,
			Status: st,
			ErrorMessage: errMsg,
			Timestamp: ts,
		}
	}

	return reports, nil
}

func getFirst(v url.Values, keys ...string) string {
	for _, k := range keys {
		if val := v.Get(k); val != "" {
			return val
		}
	}
	return ""
}

func splitTrimmed(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	res := make([]string, len(parts))
	for i, p := range parts {
		res[i] = strings.TrimSpace(p)
	}
	return res
}
