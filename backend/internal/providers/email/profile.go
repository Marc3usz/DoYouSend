package email

import "context"

// Profile contains account profile details and credit limits from SendGrid or local Mailpit.
type Profile struct {
	Provider       string `json:"provider"`
	RemainCredits  *int64 `json:"remainCredits,omitempty"`
	TotalCredits   *int64 `json:"totalCredits,omitempty"`
	UsedCredits    *int64 `json:"usedCredits,omitempty"`
	ResetFrequency string `json:"resetFrequency,omitempty"`
	NextReset      string `json:"nextReset,omitempty"`
}

// Profiler provides access to email provider account diagnostics and credit limits.
type Profiler interface {
	GetProfile(ctx context.Context) (Profile, error)
}
