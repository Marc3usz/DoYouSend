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

// SMSAPI status code mappings based on official documentation:
// https://www.smsapi.pl/docs/#18-lista-statusow-doreczenia
//
// Terminal success:
//   404: DELIVERED (Dostarczona)
//
// In-flight / intermediate (must be StatusSent, never StatusSending to avoid re-triggering):
//   403: SENT (Wysłana do operatora)
//   409: QUEUE (Kolejka u operatora)
//   410: ACCEPTED (Zaakceptowana przez operatora)
//   411: RENEWAL (Ponawianie)
//
// Terminal failures:
//   401: NOT_FOUND (Błędny numer ID lub raport wygasł)
//   402: EXPIRED (Przedawniona — numer niedostępny zbyt długo)
//   405: UNDELIVERED (Niedostarczona — błędny numer lub niedostępny)
//   406: FAILED (Nieudana — błąd bramki)
//   407: REJECTED (Odrzucona przez operatora)
//   408: UNKNOWN (Nieznany — brak możliwości doręczenia)
//   412: STOP (Zatrzymana)

var smsapiStatusCodeMap = map[string]struct {
	status providers.DeliveryStatus
	errMsg string
}{
	// Success
	"404":       {status: providers.StatusDelivered},
	"DELIVERED": {status: providers.StatusDelivered},

	// In-flight / operator progress (StatusSent)
	"403":      {status: providers.StatusSent},
	"SENT":     {status: providers.StatusSent},
	"409":      {status: providers.StatusSent},
	"QUEUE":    {status: providers.StatusSent},
	"410":      {status: providers.StatusSent},
	"ACCEPTED": {status: providers.StatusSent},
	"411":      {status: providers.StatusSent},
	"RENEWAL":  {status: providers.StatusSent},

	// Failures
	"401":         {status: providers.StatusFailed, errMsg: "message not found or report expired in SMSAPI"},
	"NOT_FOUND":   {status: providers.StatusFailed, errMsg: "message not found or report expired in SMSAPI"},
	"402":         {status: providers.StatusFailed, errMsg: "delivery timed out, message expired before reaching handset"},
	"EXPIRED":     {status: providers.StatusFailed, errMsg: "delivery timed out, message expired before reaching handset"},
	"405":         {status: providers.StatusFailed, errMsg: "message could not be delivered to the handset"},
	"UNDELIVERED": {status: providers.StatusFailed, errMsg: "message could not be delivered to the handset"},
	"406":         {status: providers.StatusFailed, errMsg: "message sending failed at SMSAPI gateway"},
	"FAILED":      {status: providers.StatusFailed, errMsg: "message sending failed at SMSAPI gateway"},
	"407":         {status: providers.StatusFailed, errMsg: "message rejected by carrier or invalid recipient number"},
	"REJECTED":    {status: providers.StatusFailed, errMsg: "message rejected by carrier or invalid recipient number"},
	"408":         {status: providers.StatusFailed, errMsg: "no delivery report available from carrier (undeliverable)"},
	"UNKNOWN":     {status: providers.StatusFailed, errMsg: "no delivery report available from carrier (undeliverable)"},
	"412":         {status: providers.StatusFailed, errMsg: "message delivery stopped"},
	"STOP":        {status: providers.StatusFailed, errMsg: "message delivery stopped"},
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
		mapping, ok := smsapiStatusCodeMap[strings.ToUpper(rawStatus)]
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
			Channel:           providers.ChannelSMS,
			Status:            mapping.status,
			ErrorMessage:      mapping.errMsg,
			Timestamp:         ts,
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
