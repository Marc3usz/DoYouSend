// Package providers defines the channel abstractions that delivery depends on, so the
// e-mail service and the SMS gateway can be swapped without touching business logic.
//
// Implementations live in the subpackages:
//   - providers/email: SMTP (Mailpit locally) and the production e-mail service,
//   - providers/sms:   the SMS gateway and a "fake" gateway that records messages.
//
// Dry-run mode can be enforced per provider or via the WrapDryRun decorator.
// Note: SMS part counting is owned and calculated by package messaging (MeasureSMS),
// while delivery per recipient is executed here.
//
// Owner: DEV C (MichalK252). The fake/local implementations must stay usable with DRY_RUN=true.
package providers
