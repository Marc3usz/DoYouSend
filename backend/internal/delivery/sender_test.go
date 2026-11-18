package delivery

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// fakeSenders answers like the senders package: one allowed address.
type fakeSenders struct {
	allowed providers.Address
	checked int
}

func (f *fakeSenders) ResolveSender(_ context.Context, _, senderID string) (EmailSender, error) {
	if senderID != "" && senderID != "ok" {
		return EmailSender{}, ErrSenderNotAllowed
	}
	return EmailSender{From: f.allowed}, nil
}

func (f *fakeSenders) CheckSender(_ context.Context, _ string, from providers.Address) error {
	f.checked++
	if from.Email != f.allowed.Email {
		return ErrSenderNotAllowed
	}
	return nil
}

func TestSendUsesTheResolvedSender(t *testing.T) {
	store := NewMemStore()
	svc := newTestService(t, twoRecipients(), store, &scriptedSender{channel: providers.ChannelSMS})
	senders := &fakeSenders{allowed: providers.Address{Email: "sekretariat@example.test", Name: "Sekretariat"}}
	svc.SetSenderResolver(senders)

	d := validDraft()
	d.SenderID = "someone-elses"
	if _, err := svc.Send(context.Background(), d); !errors.Is(err, ErrSenderNotAllowed) {
		t.Fatalf("Send(foreign sender) err = %v, want ErrSenderNotAllowed", err)
	}
	d.SenderID = "ok"
	b, err := svc.Send(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if b.Plan.From != senders.allowed {
		t.Errorf("batch From = %+v, want %+v", b.Plan.From, senders.allowed)
	}
}

// TestRetryRechecksTheSender: a delegation taken away after the batch must not
// be usable through a retry, but a retry that sends no e-mail is not blocked.
func TestRetryRechecksTheSender(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	sender := providers.Address{Email: "sekretariat@example.test"}
	plan := NewPlan("S", "B", twoRecipients().Recipients)
	plan.From = sender
	for i := range plan.Recipients {
		for j := range plan.Recipients[i].Deliveries {
			d := &plan.Recipients[i].Deliveries[j]
			if d.Channel == providers.ChannelEmail {
				d.Status, d.Error = StatusFailed, "transient: gateway timeout"
			} else {
				d.Status = StatusSent
			}
		}
	}
	b, err := store.CreateBatch(ctx, Batch{CreatedBy: userID, Status: BatchDoneWithErrors, Plan: plan})
	if err != nil {
		t.Fatal(err)
	}
	svc := newTestService(t, twoRecipients(), store, &scriptedSender{channel: providers.ChannelSMS})
	revoked := &fakeSenders{allowed: providers.Address{Email: "inny@example.test"}}
	svc.SetSenderResolver(revoked)

	if _, err := svc.RetryBatch(ctx, b.ID, false); !errors.Is(err, ErrSenderNotAllowed) {
		t.Fatalf("RetryBatch with a revoked sender err = %v, want ErrSenderNotAllowed", err)
	}
	if got, _ := store.Batch(ctx, b.ID); got.Status != BatchDoneWithErrors || got.RetryCount != 0 {
		t.Errorf("refused retry changed the batch: status %s, retries %d", got.Status, got.RetryCount)
	}

	svc.SetSenderResolver(&fakeSenders{allowed: sender})
	if _, err := svc.RetryBatch(ctx, b.ID, false); err != nil {
		t.Fatalf("RetryBatch with the sender still allowed: %v", err)
	}
}

func TestSMSOverThePartLimitFailsBeforeSending(t *testing.T) {
	store := NewMemStore()
	sms := &scriptedSender{channel: providers.ChannelSMS}
	svc := newTestService(t, twoRecipients(), store, sms)
	svc.SetSMSMaxParts(1)

	d := validDraft()
	d.Body = strings.Repeat("Zebranie w sali 12. ", 10) // 200 GSM-7 characters: 2 parts
	b, err := svc.Send(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if len(sms.sent) != 0 {
		t.Errorf("%d SMS sent, want none over the limit", len(sms.sent))
	}
	got := smsOf(b.Plan.Recipients[0])
	if got.Status != StatusFailed || !strings.Contains(got.Error, "2 parts, at most 1") || RetryClassOf(got.Error) != RetryNever {
		t.Errorf("SMS delivery = %+v, want failed over the limit and never retried", got)
	}
	if b.SMSParts != 0 || b.CostMilli != 0 {
		t.Errorf("SMSParts = %d, CostMilli = %d; want 0 for SMS that never go out", b.SMSParts, b.CostMilli)
	}
	for _, r := range b.Plan.Recipients {
		for _, del := range r.Deliveries {
			if del.Channel == providers.ChannelEmail && del.Status != StatusSent {
				t.Errorf("e-mail = %+v, want sent despite the SMS limit", del)
			}
		}
	}
}
