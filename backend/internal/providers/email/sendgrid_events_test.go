package email

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

func TestParseSendGridEvents_AllEventTypes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		payload      string
		wantCount    int
		wantID       string
		wantStatus   providers.DeliveryStatus
		wantErrorSub string
		wantTime     time.Time
	}{
		{
			name: "delivered event with timestamp and filter suffix",
			payload: `[{
				"email": "user@example.test",
				"timestamp": 1631525653,
				"event": "delivered",
				"sg_message_id": "sg-msg-001.filterdrecv-p3iad2-5b64c7-60",
				"response": "250 2.0.0 OK"
			}]`,
			wantCount:  1,
			wantID:     "sg-msg-001",
			wantStatus: providers.StatusDelivered,
			wantTime:   time.Unix(1631525653, 0).UTC(),
		},
		{
			name: "bounce event with reason and response",
			payload: `[{
				"email": "bounced@example.test",
				"timestamp": 1631525670,
				"event": "bounce",
				"sg_message_id": "sg-msg-002.filter",
				"reason": "550 5.1.1 User unknown",
				"response": "550 5.1.1"
			}]`,
			wantCount:    1,
			wantID:       "sg-msg-002",
			wantStatus:   providers.StatusFailed,
			wantErrorSub: "User unknown",
			wantTime:     time.Unix(1631525670, 0).UTC(),
		},
		{
			name: "dropped event with reason",
			payload: `[{
				"email": "dropped@example.test",
				"timestamp": 1631525680,
				"event": "dropped",
				"sg_message_id": "sg-msg-003",
				"reason": "Unsubscribed Address"
			}]`,
			wantCount:    1,
			wantID:       "sg-msg-003",
			wantStatus:   providers.StatusFailed,
			wantErrorSub: "Unsubscribed Address",
			wantTime:     time.Unix(1631525680, 0).UTC(),
		},
		{
			name: "processed event is StatusSent",
			payload: `[{
				"email": "proc@example.test",
				"timestamp": 1631525600,
				"event": "processed",
				"sg_message_id": "sg-msg-004.filter"
			}]`,
			wantCount:  1,
			wantID:     "sg-msg-004",
			wantStatus: providers.StatusSent,
			wantTime:   time.Unix(1631525600, 0).UTC(),
		},
		{
			name: "deferred event is StatusSent",
			payload: `[{
				"email": "defer@example.test",
				"timestamp": 1631525610,
				"event": "deferred",
				"sg_message_id": "sg-msg-005.filter",
				"response": "451 4.4.0 Try again later"
			}]`,
			wantCount:  1,
			wantID:     "sg-msg-005",
			wantStatus: providers.StatusSent,
			wantTime:   time.Unix(1631525610, 0).UTC(),
		},
		{
			name: "open event is ignored",
			payload: `[{
				"email": "user@example.test",
				"timestamp": 1631525690,
				"event": "open",
				"sg_message_id": "sg-msg-006.filter"
			}]`,
			wantCount: 0,
		},
		{
			name: "click event is ignored",
			payload: `[{
				"email": "user@example.test",
				"timestamp": 1631525695,
				"event": "click",
				"sg_message_id": "sg-msg-007.filter"
			}]`,
			wantCount: 0,
		},
		{
			name: "spamreport event is ignored",
			payload: `[{
				"email": "user@example.test",
				"timestamp": 1631525700,
				"event": "spamreport",
				"sg_message_id": "sg-msg-008.filter"
			}]`,
			wantCount: 0,
		},
		{
			name: "unsubscribe event is ignored",
			payload: `[{
				"email": "user@example.test",
				"timestamp": 1631525705,
				"event": "unsubscribe",
				"sg_message_id": "sg-msg-009.filter"
			}]`,
			wantCount: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			reports, err := ParseSendGridEvents(strings.NewReader(tc.payload))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(reports) != tc.wantCount {
				t.Fatalf("got %d reports, want %d", len(reports), tc.wantCount)
			}

			if tc.wantCount == 0 {
				return
			}

			r := reports[0]
			if r.ProviderMessageID != tc.wantID {
				t.Errorf("ProviderMessageID = %q, want %q", r.ProviderMessageID, tc.wantID)
			}
			if r.Channel != providers.ChannelEmail {
				t.Errorf("Channel = %q, want %q", r.Channel, providers.ChannelEmail)
			}
			if r.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", r.Status, tc.wantStatus)
			}
			if tc.wantErrorSub != "" && !strings.Contains(r.ErrorMessage, tc.wantErrorSub) {
				t.Errorf("ErrorMessage = %q, want it to contain %q", r.ErrorMessage, tc.wantErrorSub)
			}
			if tc.wantErrorSub == "" && r.ErrorMessage != "" {
				t.Errorf("ErrorMessage = %q, want empty", r.ErrorMessage)
			}
			if !tc.wantTime.IsZero() && r.Timestamp != tc.wantTime {
				t.Errorf("Timestamp = %v, want %v", r.Timestamp, tc.wantTime)
			}
		})
	}
}

func TestParseSendGridEvents_MixedBatch(t *testing.T) {
	t.Parallel()

	payload := `[
		{"event": "delivered", "sg_message_id": "msg-001.filter", "timestamp": 1631525650},
		{"event": "open", "sg_message_id": "msg-001.filter", "timestamp": 1631525655},
		{"event": "click", "sg_message_id": "msg-001.filter", "timestamp": 1631525660},
		{"event": "bounce", "sg_message_id": "msg-002", "timestamp": 1631525670, "reason": "Mailbox full"},
		{"event": "deferred", "sg_message_id": "msg-003.filter", "timestamp": 1631525680}
	]`

	reports, err := ParseSendGridEvents(strings.NewReader(payload))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 5 events total, 2 ignored (open, click) -> 3 actionable reports
	if len(reports) != 3 {
		t.Fatalf("got %d reports, want 3", len(reports))
	}

	// 1st: delivered
	if reports[0].ProviderMessageID != "msg-001" || reports[0].Status != providers.StatusDelivered {
		t.Errorf("report[0] = %+v, want msg-001 delivered", reports[0])
	}

	// 2nd: bounce (failed)
	if reports[1].ProviderMessageID != "msg-002" || reports[1].Status != providers.StatusFailed {
		t.Errorf("report[1] = %+v, want msg-002 failed", reports[1])
	}
	if !strings.Contains(reports[1].ErrorMessage, "Mailbox full") {
		t.Errorf("report[1].ErrorMessage = %q, want 'Mailbox full'", reports[1].ErrorMessage)
	}

	// 3rd: deferred (sent)
	if reports[2].ProviderMessageID != "msg-003" || reports[2].Status != providers.StatusSent {
		t.Errorf("report[2] = %+v, want msg-003 sent", reports[2])
	}
}

func TestParseSendGridEvents_Errors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		payload string
	}{
		{
			name:    "empty string / invalid JSON",
			payload: "",
		},
		{
			name:    "JSON object instead of array",
			payload: `{"event":"delivered","sg_message_id":"123"}`,
		},
		{
			name:    "missing sg_message_id on actionable delivered event",
			payload: `[{"event":"delivered","sg_message_id":""}]`,
		},
		{
			name:    "missing sg_message_id on actionable bounce event",
			payload: `[{"event":"bounce"}]`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseSendGridEvents(strings.NewReader(tc.payload))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, providers.ErrMalformedReport) {
				t.Errorf("error = %v, want ErrMalformedReport", err)
			}
		})
	}
}

func TestParseSendGridEvents_EmptyArray(t *testing.T) {
	t.Parallel()

	reports, err := ParseSendGridEvents(strings.NewReader("[]"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("got %d reports, want 0", len(reports))
	}
}
