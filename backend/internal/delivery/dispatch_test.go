package delivery

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/groups"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

// scriptedSender returns the queued errors for a recipient in order, then succeeds.
type scriptedSender struct {
	channel providers.Channel
	mu      sync.Mutex
	errs    map[string][]error
	sent    []providers.Message
	onSend  func()
}

func (s *scriptedSender) Channel() providers.Channel { return s.channel }

func (s *scriptedSender) Send(_ context.Context, msg providers.Message) (providers.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, msg)
	if s.onSend != nil {
		s.onSend()
	}
	if q := s.errs[msg.RecipientID]; len(q) > 0 {
		s.errs[msg.RecipientID] = q[1:]
		return providers.Result{}, q[0]
	}
	return providers.Result{ProviderMessageID: string(s.channel) + "-" + msg.RecipientID}, nil
}

func newTestDispatcher(t *testing.T, policy RetryPolicy, senders ...Sender) (*Dispatcher, *[]time.Duration) {
	t.Helper()
	d, err := NewDispatcher(senders, policy, nil)
	if err != nil {
		t.Fatal(err)
	}
	var waits []time.Duration
	d.sleep = func(_ context.Context, w time.Duration) error {
		waits = append(waits, w)
		return nil
	}
	return d, &waits
}

const (
	id1 = "00000000-0000-4000-8000-000000000001"
	id2 = "00000000-0000-4000-8000-000000000002"
	id3 = "00000000-0000-4000-8000-000000000003"
)

var (
	errTransient = providers.TransientError(errors.New("gateway unavailable"))
	errPermanent = providers.PermanentError(providers.ErrInvalidRecipient)
)

func TestDispatchSendsIdenticalBodyOnBothChannels(t *testing.T) {
	email := &scriptedSender{channel: providers.ChannelEmail}
	sms := &scriptedSender{channel: providers.ChannelSMS}
	d, _ := newTestDispatcher(t, DefaultRetryPolicy(), email, sms)
	plan := NewPlan("Zebranie", "Czesc {{imie}}", []groups.Resolved{resolved(id1, "Anna", "anna@example.test", "+48500100101")})

	got, err := d.Dispatch(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}

	if len(email.sent) != 1 || len(sms.sent) != 1 {
		t.Fatalf("sent email=%d sms=%d, want 1 each", len(email.sent), len(sms.sent))
	}
	if email.sent[0].Body != sms.sent[0].Body || email.sent[0].Body != "Czesc Anna" {
		t.Errorf("bodies differ: email %q, sms %q", email.sent[0].Body, sms.sent[0].Body)
	}
	if email.sent[0].To != "anna@example.test" || sms.sent[0].To != "+48500100101" || email.sent[0].Subject != "Zebranie" {
		t.Errorf("messages = %+v %+v", email.sent[0], sms.sent[0])
	}
	for _, del := range got.Recipients[0].Deliveries {
		if del.Status != StatusSent || del.Attempts != 1 || del.ProviderMessageID == "" {
			t.Errorf("delivery = %+v", del)
		}
	}
	if got.Status() != BatchDone {
		t.Errorf("batch = %s, want done", got.Status())
	}
	if plan.Recipients[0].Deliveries[0].Status != StatusPending {
		t.Error("Dispatch changed the caller's plan")
	}
}

func TestDispatchFailuresDoNotAbortBatch(t *testing.T) {
	sms := &scriptedSender{channel: providers.ChannelSMS, errs: map[string][]error{
		id1: {errTransient, errTransient},               // succeeds on 3rd try
		id2: {errPermanent},                             // fails at once
		id3: {errTransient, errTransient, errTransient}, // runs out of tries
	}}
	d, waits := newTestDispatcher(t, RetryPolicy{MaxAttempts: 3, BaseBackoff: time.Second}, sms)
	plan := NewPlan("S", "Tresc", []groups.Resolved{
		resolved(id1, "A", "", "+48500100101"),
		resolved(id2, "B", "", "+48500100102"),
		resolved(id3, "C", "", "+48500100103"),
	})

	got, err := d.Dispatch(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}

	want := []struct {
		status   Status
		attempts int
	}{{StatusSent, 3}, {StatusFailed, 1}, {StatusFailed, 3}}
	for i, w := range want {
		del := got.Recipients[i].Deliveries[0]
		if del.Status != w.status || del.Attempts != w.attempts {
			t.Errorf("recipient %d: status %s attempts %d, want %s %d", i, del.Status, del.Attempts, w.status, w.attempts)
		}
		if del.Status == StatusFailed && del.Error == "" {
			t.Errorf("recipient %d: failed without error text", i)
		}
	}
	if wantWaits := []time.Duration{time.Second, 2 * time.Second, time.Second, 2 * time.Second}; !slices.Equal(*waits, wantWaits) {
		t.Errorf("waits = %v, want %v", *waits, wantWaits)
	}
	if got.Status() != BatchDoneWithErrors {
		t.Errorf("batch = %s, want done_with_errors", got.Status())
	}
}

func TestDispatchDoesNotRetryUnclassifiedErrors(t *testing.T) {
	sms := &scriptedSender{channel: providers.ChannelSMS, errs: map[string][]error{id1: {errors.New("boom")}}}
	d, _ := newTestDispatcher(t, DefaultRetryPolicy(), sms)

	got, _ := d.Dispatch(context.Background(), NewPlan("S", "x", []groups.Resolved{resolved(id1, "A", "", "+48500100101")}))

	if del := got.Recipients[0].Deliveries[0]; del.Status != StatusFailed || del.Attempts != 1 {
		t.Errorf("delivery = %+v, want failed after 1 attempt", del)
	}
}

func TestDispatchSkipsDeliveriesThatAreNotPending(t *testing.T) {
	sms := &scriptedSender{channel: providers.ChannelSMS}
	d, _ := newTestDispatcher(t, DefaultRetryPolicy(), sms)
	// Empty first name: render fails, the delivery is failed before dispatch.
	plan := NewPlan("S", "{{imie}}", []groups.Resolved{resolved(id1, "", "", "+48500100101")})

	if _, err := d.Dispatch(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if len(sms.sent) != 0 {
		t.Errorf("sent %d messages for a body that did not render", len(sms.sent))
	}
}

func TestDispatchMissingSenderFailsOnlyThatChannel(t *testing.T) {
	email := &scriptedSender{channel: providers.ChannelEmail}
	d, _ := newTestDispatcher(t, DefaultRetryPolicy(), email)

	got, err := d.Dispatch(context.Background(), NewPlan("S", "x", []groups.Resolved{resolved(id1, "A", "a@example.test", "+48500100101")}))
	if err != nil {
		t.Fatal(err)
	}
	dels := got.Recipients[0].Deliveries
	if dels[0].Status != StatusSent || dels[1].Status != StatusFailed || dels[1].Attempts != 0 {
		t.Errorf("deliveries = %+v", dels)
	}
}

func TestDispatchStopsOnCancelAndLeavesRestPending(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sms := &scriptedSender{channel: providers.ChannelSMS, onSend: cancel}
	d, _ := newTestDispatcher(t, DefaultRetryPolicy(), sms)
	plan := NewPlan("S", "x", []groups.Resolved{
		resolved(id1, "A", "", "+48500100101"),
		resolved(id2, "B", "", "+48500100102"),
	})

	got, err := d.Dispatch(ctx, plan)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(sms.sent) != 1 {
		t.Errorf("sent %d, want 1", len(sms.sent))
	}
	if del := got.Recipients[1].Deliveries[0]; del.Status != StatusPending || del.Attempts != 0 {
		t.Errorf("second delivery = %+v, want untouched pending", del)
	}
	if got.Status() != BatchRunning {
		t.Errorf("batch = %s, want running", got.Status())
	}
}

func TestDispatchCancelDuringFailedCallLeavesSending(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sms := &scriptedSender{channel: providers.ChannelSMS, onSend: cancel,
		errs: map[string][]error{id1: {providers.TransientError(context.Canceled)}}}
	d, _ := newTestDispatcher(t, DefaultRetryPolicy(), sms)

	got, err := d.Dispatch(ctx, NewPlan("S", "x", []groups.Resolved{resolved(id1, "A", "", "+48500100101")}))

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if del := got.Recipients[0].Deliveries[0]; del.Status != StatusSending || del.Attempts != 1 {
		t.Errorf("delivery = %+v, want sending (outcome unknown), never pending", del)
	}
}

func TestDispatchCancelDuringBackoffLeavesPending(t *testing.T) {
	sms := &scriptedSender{channel: providers.ChannelSMS, errs: map[string][]error{id1: {errTransient}}}
	d, _ := newTestDispatcher(t, DefaultRetryPolicy(), sms)
	d.sleep = func(context.Context, time.Duration) error { return context.Canceled }

	got, _ := d.Dispatch(context.Background(), NewPlan("S", "x", []groups.Resolved{resolved(id1, "A", "", "+48500100101")}))

	if del := got.Recipients[0].Deliveries[0]; del.Status != StatusPending || del.Attempts != 1 {
		t.Errorf("delivery = %+v, want pending after one failed try", del)
	}
}

func TestDescribeDropsGatewayText(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{providers.PermanentError(fmt.Errorf("smsapi error 13: bad number +48500100101: %w", providers.ErrInvalidRecipient)), "permanent: invalid recipient"},
		{providers.TransientError(fmt.Errorf("x: %w", providers.ErrGatewayUnavailable)), "transient: gateway unavailable"},
		{providers.PermanentError(errors.New("rejected jan@example.test")), "permanent provider error"},
		{errors.New("boom +48500100101"), "unclassified provider error"},
	}
	for _, tt := range tests {
		if got := describe(tt.err); got != tt.want {
			t.Errorf("describe(%v) = %q, want %q", tt.err, got, tt.want)
		}
	}
}

func TestNewDispatcherRejectsDuplicateChannel(t *testing.T) {
	a := &scriptedSender{channel: providers.ChannelSMS}
	b := &scriptedSender{channel: providers.ChannelSMS}
	if _, err := NewDispatcher([]Sender{a, b}, DefaultRetryPolicy(), nil); !errors.Is(err, ErrDuplicateChannel) {
		t.Errorf("err = %v, want ErrDuplicateChannel", err)
	}
}

func TestSleepCtx(t *testing.T) {
	if err := sleepCtx(context.Background(), time.Millisecond); err != nil {
		t.Errorf("sleepCtx = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepCtx(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("sleepCtx on cancelled ctx = %v", err)
	}
}
