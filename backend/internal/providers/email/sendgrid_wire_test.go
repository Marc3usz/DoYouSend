package email

// Test-only mirror of the SendGrid v3 mail/send JSON. Tests decode the request
// the adapter actually put on the wire into these types, independently of the
// SDK types that build it, so a change in the SDK's JSON shows up as a failing test.

type sgMailSend struct {
	Personalizations []sgPersonalization `json:"personalizations"`
	From             sgAddress           `json:"from"`
	Subject          string              `json:"subject"`
	Content          []sgContent         `json:"content"`
	TrackingSettings sgTracking          `json:"tracking_settings"`
	MailSettings     *sgMailSettings     `json:"mail_settings,omitempty"`
	CustomArgs       map[string]string   `json:"custom_args,omitempty"`
}

type sgPersonalization struct {
	To []sgAddress `json:"to"`
}

type sgAddress struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type sgContent struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type sgTracking struct {
	ClickTracking sgToggle `json:"click_tracking"`
	OpenTracking  sgToggle `json:"open_tracking"`
}

type sgToggle struct {
	Enable bool `json:"enable"`
}

type sgMailSettings struct {
	SandboxMode sgToggle `json:"sandbox_mode"`
}
