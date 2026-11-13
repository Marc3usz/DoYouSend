package providers

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type mockProvider struct {
	channel    Channel
	sendCalled bool
}

func (m *mockProvider) Send(_ context.Context, _ Message) (Result, error) {
	m.sendCalled = true
	return Result{ProviderMessageID: "real-id"}, nil
}

func (m *mockProvider) Channel() Channel {
	return m.channel
}

func TestWrapDryRun_SendSuppression(t *testing.T) {
	t.Parallel()

	inner := &mockProvider{channel: ChannelEmail}
	wrapped := WrapDryRun(inner, nil) // tests nil logger fallback

	if wrapped.Channel() != ChannelEmail {
		t.Errorf("Channel() = %v, want %v", wrapped.Channel(), ChannelEmail)
	}

	res, err := wrapped.Send(context.Background(), Message{
		RecipientID: "r-dry",
		To:          "test@example.test",
		Subject:     "Subject",
		Body:        "Body",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.sendCalled {
		t.Fatal("inner provider was called despite DRY_RUN wrapper")
	}
	if !strings.HasPrefix(res.ProviderMessageID, "dryrun-email-r-dry-") {
		t.Errorf("got ProviderMessageID = %s, want prefix %s", res.ProviderMessageID, "dryrun-email-r-dry-")
	}
	// Reports and retries match deliveries by this ID, so it must differ per send.
	again, err := wrapped.Send(context.Background(), Message{RecipientID: "r-dry", To: "test@example.test", Body: "Body"})
	if err != nil || again.ProviderMessageID == res.ProviderMessageID {
		t.Errorf("second send ID = %q (err %v), want a new ID", again.ProviderMessageID, err)
	}
}

func TestWrapDryRun_Validation(t *testing.T) {
	t.Parallel()

	inner := &mockProvider{channel: ChannelSMS}
	wrapped := WrapDryRun(inner, nil)

	// Empty To
	_, err := wrapped.Send(context.Background(), Message{
		RecipientID: "r-empty",
		To:          "",
		Body:        "Body",
	})
	if err == nil {
		t.Fatal("expected error for empty To")
	}
	if !IsPermanent(err) {
		t.Errorf("expected permanent error, got: %v", err)
	}

	// Context cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = wrapped.Send(ctx, Message{
		RecipientID: "r-ctx",
		To:          "+48500100101",
		Body:        "Body",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if !IsTransient(err) {
		t.Errorf("expected transient error, got: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled in chain, got: %v", err)
	}
}
