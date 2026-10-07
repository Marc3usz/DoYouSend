package providers

import (
	"context"
	"testing"
)

func TestDeliveryReportConsumerFunc(t *testing.T) {
	t.Parallel()

	called := false
	var gotReports []DeliveryReport

	fn := DeliveryReportConsumerFunc(func(ctx context.Context, reports []DeliveryReport) error {
		called = true
		gotReports = reports
		return nil
	})

	reports := []DeliveryReport{
		{ProviderMessageID: "msg-1", Channel: ChannelSMS, Status: StatusDelivered},
	}

	err := fn.ConsumeDeliveryReports(context.Background(), reports)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected function to be called")
	}
	if len(gotReports) != 1 || gotReports[0].ProviderMessageID != "msg-1" {
		t.Errorf("got %v, want %v", gotReports, reports)
	}
}

func TestPGDeliveryReportConsumer_NilPool(t *testing.T) {
	t.Parallel()

	var c *PGDeliveryReportConsumer
	err := c.ConsumeDeliveryReports(context.Background(), []DeliveryReport{{ProviderMessageID: "1"}})
	if err == nil {
		t.Fatal("expected error for nil consumer, got nil")
	}

	c2 := NewPGDeliveryReportConsumer(nil)
	err = c2.ConsumeDeliveryReports(context.Background(), []DeliveryReport{{ProviderMessageID: "1"}})
	if err == nil {
		t.Fatal("expected error for nil pool, got nil")
	}
}

func TestPGDeliveryReportConsumer_EmptyReports(t *testing.T) {
	t.Parallel()

	// An empty slice should return nil without touching the pool
	c := NewPGDeliveryReportConsumer(nil)
	if err := c.ConsumeDeliveryReports(context.Background(), nil); err != nil {
		t.Errorf("expected nil for empty slice, got %v", err)
	}
	if err := c.ConsumeDeliveryReports(context.Background(), []DeliveryReport{}); err != nil {
		t.Errorf("expected nil for empty slice, got %v", err)
	}
}
