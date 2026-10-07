package providers

import "context"

// DeliveryReportConsumer defines the contract for consuming delivery status updates.
// The consumer is responsible for persisting or dispatching reports from providers
// (e.g. SMSAPI DLR callbacks or SendGrid Event Webhooks) to update delivery statuses.
//
// Implementations must ensure that terminal statuses (delivered, failed) are never
// downgraded by out-of-order or late events (forward-only state machine).
type DeliveryReportConsumer interface {
	ConsumeDeliveryReports(ctx context.Context, reports []DeliveryReport) error
}

// DeliveryReportConsumerFunc is an adapter to allow the use of ordinary functions
// as DeliveryReportConsumer.
type DeliveryReportConsumerFunc func(ctx context.Context, reports []DeliveryReport) error

// ConsumeDeliveryReports calls f(ctx, reports).
func (f DeliveryReportConsumerFunc) ConsumeDeliveryReports(ctx context.Context, reports []DeliveryReport) error {
	return f(ctx, reports)
}
