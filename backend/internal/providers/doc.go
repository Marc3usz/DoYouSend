// Package providers defines the channel abstractions that delivery depends on, so the
// e-mail service and the SMS gateway can be swapped without touching business logic.
//
// Implementations live in the subpackages:
//   - providers/email: SMTP (Mailpit locally) and the production e-mail service,
//   - providers/sms:   the SMS gateway and a "fake" gateway that records messages.
//   - providers/setup: factory that wires concrete adapters from env config.
//
// Usage (typically in cmd/api/main.go):
//
//	cfg := setup.ConfigFromEnv()            // reads .env variables
//	p, err := setup.New(cfg, logger)        // builds Email + SMS providers
//	// p.Email and p.SMS implement providers.Provider
//
// Dry-run mode is enforced automatically by setup.New when Config.DryRun is true —
// both providers are wrapped with WrapDryRun so no messages leave the system.
//
// Note: SMS part counting is owned and calculated by package messaging (MeasureSMS),
// while delivery per recipient is executed here.
//
// Owner: DEV C (MichalK252). The fake/local implementations must stay usable with DRY_RUN=true.
package providers
