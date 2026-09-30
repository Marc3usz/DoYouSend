package sms

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

func TestParseSMSAPIDLR(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		body         string
		wantID       string
		wantStatus   providers.DeliveryStatus
		wantError    string
		wantTime     time.Time
		wantParseErr bool
	}{
		{
			name:       "delivered",
			body:       `{"id":"msg-001","status":"DELIVERED","date":1727600000}`,
			wantID:     "msg-001",
			wantStatus: providers.StatusDelivered,
			wantTime:   time.Unix(1727600000, 0).UTC(),
		},
		{
			name:       "undelivered",
			body:       `{"id":"msg-002","status":"UNDELIVERED","date":1727600100}`,
			wantID:     "msg-002",
			wantStatus: providers.StatusFailed,
			wantError:  "message could not be delivered to the handset",
			wantTime:   time.Unix(1727600100, 0).UTC(),
		},
		{
			name:       "expired",
			body:       `{"id":"msg-003","status":"EXPIRED","date":1727600200}`,
			wantID:     "msg-003",
			wantStatus: providers.StatusFailed,
			wantError:  "delivery timed out",
			wantTime:   time.Unix(1727600200, 0).UTC(),
		},
		{
			name:       "rejected",
			body:       `{"id":"msg-004","status":"REJECTED","date":1727600300}`,
			wantID:     "msg-004",
			wantStatus: providers.StatusFailed,
			wantError:  "message rejected",
			wantTime:   time.Unix(1727600300, 0).UTC(),
		},
		{
			name:       "sent (intermediate)",
			body:       `{"id":"msg-005","status":"SENT","date":1727600400}`,
			wantID:     "msg-005",
			wantStatus: providers.StatusSent,
			wantTime:   time.Unix(1727600400, 0).UTC(),
		},
		{
			name:       "queue (intermediate)",
			body:       `{"id":"msg-006","status":"QUEUE"}`,
			wantID:     "msg-006",
			wantStatus: providers.StatusSending,
		},
		{
			name:       "unknown status treated as in-flight",
			body:       `{"id":"msg-007","status":"BUFFERED"}`,
			wantID:     "msg-007",
			wantStatus: providers.StatusSent,
		},
		{
			name:       "zero date gives zero timestamp",
			body:       `{"id":"msg-008","status":"DELIVERED","date":0}`,
			wantID:     "msg-008",
			wantStatus: providers.StatusDelivered,
		},
		{
			name:         "invalid JSON",
			body:         `not json`,
			wantParseErr: true,
		},
		{
			name:         "missing id",
			body:         `{"status":"DELIVERED"}`,
			wantParseErr: true,
		},
		{
			name:         "missing status",
			body:         `{"id":"msg-009"}`,
			wantParseErr: true,
		},
		{
			name:         "empty object",
			body:         `{}`,
			wantParseErr: true,
		},
		{
			name:         "empty body",
			body:         ``,
			wantParseErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			report, err := ParseSMSAPIDLR(strings.NewReader(tc.body))

			if tc.wantParseErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, providers.ErrMalformedReport) {
					t.Errorf("error = %v, want errors.Is ErrMalformedReport", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if report.ProviderMessageID != tc.wantID {
				t.Errorf("ProviderMessageID = %q, want %q", report.ProviderMessageID, tc.wantID)
			}
			if report.Channel != providers.ChannelSMS {
				t.Errorf("Channel = %q, want %q", report.Channel, providers.ChannelSMS)
			}
			if report.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", report.Status, tc.wantStatus)
			}
			if tc.wantError != "" && !strings.Contains(report.ErrorMessage, tc.wantError) {
				t.Errorf("ErrorMessage = %q, want it to contain %q", report.ErrorMessage, tc.wantError)
			}
			if tc.wantError == "" && report.ErrorMessage != "" {
				t.Errorf("ErrorMessage = %q, want empty", report.ErrorMessage)
			}
			if !tc.wantTime.IsZero() && report.Timestamp != tc.wantTime {
				t.Errorf("Timestamp = %v, want %v", report.Timestamp, tc.wantTime)
			}
			if tc.wantTime.IsZero() && !report.Timestamp.IsZero() {
				t.Errorf("Timestamp = %v, want zero", report.Timestamp)
			}
		})
	}
}
