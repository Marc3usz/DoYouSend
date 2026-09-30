package sms

import (
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

func TestSMSAPIDLRAckResponse(t *testing.T) {
	t.Parallel()
	if SMSAPIDLRAckResponse != "OK" {
		t.Errorf("SMSAPIDLRAckResponse = %q, want %q", SMSAPIDLRAckResponse, "OK")
	}
}

func TestParseSMSAPIDLR_SingleMessage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		query      string
		wantID     string
		wantStatus providers.DeliveryStatus
		wantError  string
		wantTime   time.Time
	}{
		{
			name:       "404 DELIVERED with status code and donedate",
			query:      "MsgId=613F1B14346335B944450980&status=404&status_name=DELIVERED&donedate=1631525653",
			wantID:     "613F1B14346335B944450980",
			wantStatus: providers.StatusDelivered,
			wantTime:   time.Unix(1631525653, 0).UTC(),
		},
		{
			name:       "403 SENT (in-flight, no donedate)",
			query:      "MsgId=msg-002&status=403&status_name=SENT",
			wantID:     "msg-002",
			wantStatus: providers.StatusSent,
		},
		{
			name:       "409 QUEUE is mapped to StatusSent (never StatusSending)",
			query:      "MsgId=msg-003&status=409&status_name=QUEUE",
			wantID:     "msg-003",
			wantStatus: providers.StatusSent,
		},
		{
			name:       "410 ACCEPTED is mapped to StatusSent",
			query:      "MsgId=msg-004&status=410&status_name=ACCEPTED",
			wantID:     "msg-004",
			wantStatus: providers.StatusSent,
		},
		{
			name:       "411 RENEWAL is mapped to StatusSent",
			query:      "MsgId=msg-005&status=411&status_name=RENEWAL",
			wantID:     "msg-005",
			wantStatus: providers.StatusSent,
		},
		{
			name:       "401 NOT_FOUND is failure",
			query:      "MsgId=msg-006&status=401",
			wantID:     "msg-006",
			wantStatus: providers.StatusFailed,
			wantError:  "not found",
		},
		{
			name:       "402 EXPIRED is failure",
			query:      "MsgId=msg-007&status=402",
			wantID:     "msg-007",
			wantStatus: providers.StatusFailed,
			wantError:  "expired",
		},
		{
			name:       "405 UNDELIVERED is failure",
			query:      "MsgId=msg-008&status=405",
			wantID:     "msg-008",
			wantStatus: providers.StatusFailed,
			wantError:  "could not be delivered",
		},
		{
			name:       "406 FAILED is failure",
			query:      "MsgId=msg-009&status=406",
			wantID:     "msg-009",
			wantStatus: providers.StatusFailed,
			wantError:  "sending failed",
		},
		{
			name:       "407 REJECTED is failure",
			query:      "MsgId=msg-010&status=407",
			wantID:     "msg-010",
			wantStatus: providers.StatusFailed,
			wantError:  "rejected",
		},
		{
			name:       "408 UNKNOWN is failure (not treated as sent)",
			query:      "MsgId=msg-011&status=408",
			wantID:     "msg-011",
			wantStatus: providers.StatusFailed,
			wantError:  "no delivery report available",
		},
		{
			name:       "412 STOP is failure",
			query:      "MsgId=msg-012&status=412",
			wantID:     "msg-012",
			wantStatus: providers.StatusFailed,
			wantError:  "stopped",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			vals, err := url.ParseQuery(tc.query)
			if err != nil {
				t.Fatalf("parse query: %v", err)
			}

			reports, err := ParseSMSAPIDLR(vals)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(reports) != 1 {
				t.Fatalf("got %d reports, want 1", len(reports))
			}

			r := reports[0]
			if r.ProviderMessageID != tc.wantID {
				t.Errorf("ProviderMessageID = %q, want %q", r.ProviderMessageID, tc.wantID)
			}
			if r.Channel != providers.ChannelSMS {
				t.Errorf("Channel = %q, want %q", r.Channel, providers.ChannelSMS)
			}
			if r.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", r.Status, tc.wantStatus)
			}
			if tc.wantError != "" && !containsSubstr(r.ErrorMessage, tc.wantError) {
				t.Errorf("ErrorMessage = %q, want it to contain %q", r.ErrorMessage, tc.wantError)
			}
			if tc.wantError == "" && r.ErrorMessage != "" {
				t.Errorf("ErrorMessage = %q, want empty", r.ErrorMessage)
			}
			if !tc.wantTime.IsZero() && r.Timestamp != tc.wantTime {
				t.Errorf("Timestamp = %v, want %v", r.Timestamp, tc.wantTime)
			}
		})
	}
}

func TestParseSMSAPIDLR_BatchCommaSeparated(t *testing.T) {
	t.Parallel()

	// Based on official SMSAPI batch documentation example:
	// MsgId=id1,id2,id3&status=404,405,403&donedate=1631525653,1631525676,0
	query := "MsgId=id-1,id-2,id-3&status=404,405,403&donedate=1631525653,1631525676,0"
	vals, err := url.ParseQuery(query)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}

	reports, err := ParseSMSAPIDLR(vals)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reports) != 3 {
		t.Fatalf("got %d reports, want 3", len(reports))
	}

	// 1st: delivered
	if reports[0].ProviderMessageID != "id-1" || reports[0].Status != providers.StatusDelivered {
		t.Errorf("report[0] = %+v, want id-1 delivered", reports[0])
	}
	if reports[0].Timestamp != time.Unix(1631525653, 0).UTC() {
		t.Errorf("report[0].Timestamp = %v, want 1631525653", reports[0].Timestamp)
	}

	// 2nd: undelivered (failed)
	if reports[1].ProviderMessageID != "id-2" || reports[1].Status != providers.StatusFailed {
		t.Errorf("report[1] = %+v, want id-2 failed", reports[1])
	}
	if reports[1].ErrorMessage == "" {
		t.Errorf("report[1].ErrorMessage should not be empty")
	}

	// 3rd: sent (in-flight, zero timestamp)
	if reports[2].ProviderMessageID != "id-3" || reports[2].Status != providers.StatusSent {
		t.Errorf("report[2] = %+v, want id-3 sent", reports[2])
	}
	if !reports[2].Timestamp.IsZero() {
		t.Errorf("report[2].Timestamp = %v, want zero", reports[2].Timestamp)
	}
}

func TestParseSMSAPIDLR_Errors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		query string
	}{
		{"empty query", ""},
		{"missing MsgId", "status=404"},
		{"missing status", "MsgId=id-1"},
		{"empty MsgId value", "MsgId=&status=404"},
		{"empty status value", "MsgId=id-1&status="},
		{"unrecognized status code fails rather than assuming sent", "MsgId=id-1&status=999"},
		{"unrecognized status string fails", "MsgId=id-1&status=RANDOM_STATUS"},
		{"mismatched count between MsgId and status", "MsgId=id-1,id-2&status=404"},
		{"mismatched count between MsgId and donedate", "MsgId=id-1,id-2&status=404,403&donedate=1631525653"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			vals, err := url.ParseQuery(tc.query)
			if err != nil {
				t.Fatalf("parse query: %v", err)
			}

			_, err = ParseSMSAPIDLR(vals)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, providers.ErrMalformedReport) {
				t.Errorf("error = %v, want errors.Is ErrMalformedReport", err)
			}
		})
	}
}

func containsSubstr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || searchSubstr(s, substr))
}

func searchSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
