package delivery

import (
	"strings"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/groups"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

func resolved(id, first, email, phone string) groups.Resolved {
	r := recipients.Recipient{
		ID: id, FirstName: first, LastName: "Testowy",
		Email: email, Phone: phone, Type: recipients.TypeParent,
	}
	return groups.Resolved{Recipient: r, Issues: r.ContactIssues()}
}

func TestNewPlan(t *testing.T) {
	const (
		idBoth  = "00000000-0000-4000-8000-000000000001"
		idEmail = "00000000-0000-4000-8000-000000000002"
		idSMS   = "00000000-0000-4000-8000-000000000003"
		idNone  = "00000000-0000-4000-8000-000000000004"
	)
	list := []groups.Resolved{
		resolved(idBoth, "Anna", "anna@example.test", "+48500100101"),
		resolved(idEmail, "Jan", "jan@example.test", ""),
		resolved(idSMS, "Ola", "", "+48500100103"),
		resolved(idNone, "Ewa", "", ""),
		resolved(idBoth, "Anna", "anna@example.test", "+48500100101"),
	}

	plan := NewPlan("Zebranie", "Dzien dobry {{imie}}, zebranie o 18.", list)

	if plan.Subject != "Zebranie" {
		t.Errorf("Subject = %q", plan.Subject)
	}
	if got := len(plan.Recipients); got != 4 {
		t.Fatalf("len(Recipients) = %d, want 4 (duplicate planned once)", got)
	}

	tests := []struct {
		id          string
		body        string
		channels    []providers.Channel
		partial     bool
		unreachable bool
	}{
		{idBoth, "Dzien dobry Anna, zebranie o 18.", []providers.Channel{providers.ChannelEmail, providers.ChannelSMS}, false, false},
		{idEmail, "Dzien dobry Jan, zebranie o 18.", []providers.Channel{providers.ChannelEmail}, true, false},
		{idSMS, "Dzien dobry Ola, zebranie o 18.", []providers.Channel{providers.ChannelSMS}, true, false},
		{idNone, "Dzien dobry Ewa, zebranie o 18.", nil, false, true},
	}
	for i, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := plan.Recipients[i]
			if got.RecipientID != tt.id || got.Body != tt.body {
				t.Errorf("got %s %q, want %s %q", got.RecipientID, got.Body, tt.id, tt.body)
			}
			if got.Partial != tt.partial || got.Unreachable != tt.unreachable {
				t.Errorf("Partial, Unreachable = %v, %v; want %v, %v", got.Partial, got.Unreachable, tt.partial, tt.unreachable)
			}
			if len(got.Deliveries) != len(tt.channels) {
				t.Fatalf("deliveries = %+v, want channels %v", got.Deliveries, tt.channels)
			}
			for j, d := range got.Deliveries {
				if d.Channel != tt.channels[j] || d.Status != StatusPending || d.Parts != 1 {
					t.Errorf("delivery %d = %+v", j, d)
				}
			}
		})
	}
}

func TestNewPlanRenderFailureOnlyFailsThatRecipient(t *testing.T) {
	list := []groups.Resolved{
		resolved("00000000-0000-4000-8000-000000000001", "", "a@example.test", "+48500100101"),
		resolved("00000000-0000-4000-8000-000000000002", "Jan", "jan@example.test", ""),
	}

	plan := NewPlan("S", "Czesc {{imie}}", list)

	broken := plan.Recipients[0]
	if broken.Body != "" || len(broken.Deliveries) != 2 {
		t.Fatalf("broken recipient = %+v", broken)
	}
	for _, d := range broken.Deliveries {
		if d.Status != StatusFailed || !strings.Contains(d.Error, "imie") {
			t.Errorf("delivery = %+v, want failed naming the placeholder", d)
		}
	}
	if ok := plan.Recipients[1].Deliveries[0]; ok.Status != StatusPending {
		t.Errorf("other recipient delivery = %+v, want pending", ok)
	}
}

func TestNewPlanEmptyBodyFails(t *testing.T) {
	plan := NewPlan("S", "", []groups.Resolved{resolved("00000000-0000-4000-8000-000000000001", "Jan", "jan@example.test", "")})
	if d := plan.Recipients[0].Deliveries[0]; d.Status != StatusFailed {
		t.Errorf("delivery = %+v, want failed", d)
	}
}

func TestNewPlanSMSParts(t *testing.T) {
	body := strings.Repeat("a", 161)
	plan := NewPlan("S", body, []groups.Resolved{resolved("00000000-0000-4000-8000-000000000001", "Jan", "jan@example.test", "+48500100101")})
	for _, d := range plan.Recipients[0].Deliveries {
		want := 1
		if d.Channel == providers.ChannelSMS {
			want = 2
		}
		if d.Parts != want {
			t.Errorf("%s parts = %d, want %d", d.Channel, d.Parts, want)
		}
	}
}

func TestPlanStatus(t *testing.T) {
	withStatuses := func(unreachable bool, ss ...Status) Plan {
		r := PlannedRecipient{Unreachable: unreachable}
		for _, s := range ss {
			r.Deliveries = append(r.Deliveries, Delivery{Status: s})
		}
		return Plan{Recipients: []PlannedRecipient{r}}
	}
	tests := []struct {
		name string
		plan Plan
		want BatchStatus
	}{
		{"empty", Plan{}, BatchDone},
		{"all sent", withStatuses(false, StatusSent, StatusDelivered), BatchDone},
		{"one failed", withStatuses(false, StatusSent, StatusFailed), BatchDoneWithErrors},
		{"unreachable", withStatuses(true), BatchDoneWithErrors},
		{"pending wins over failed", withStatuses(false, StatusFailed, StatusPending), BatchRunning},
		{"sending", withStatuses(false, StatusSending), BatchRunning},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.plan.Status(); got != tt.want {
				t.Errorf("Status() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestAdvance(t *testing.T) {
	tests := []struct {
		current, next, want Status
	}{
		{StatusPending, StatusSending, StatusSending},
		{StatusPending, StatusSent, StatusSent},
		{StatusSent, StatusDelivered, StatusDelivered},
		{StatusSent, StatusFailed, StatusFailed},
		{StatusSent, StatusSending, StatusSent},
		{StatusSent, StatusSent, StatusSent},
		{StatusDelivered, StatusFailed, StatusDelivered},
		{StatusFailed, StatusDelivered, StatusFailed},
		{StatusPending, "bogus", StatusPending},
	}
	for _, tt := range tests {
		if got := Advance(tt.current, tt.next); got != tt.want {
			t.Errorf("Advance(%s, %s) = %s, want %s", tt.current, tt.next, got, tt.want)
		}
	}
}
