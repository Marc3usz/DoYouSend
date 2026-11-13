// Package settings keeps the sending configuration that administrators edit in
// the panel (ADR-0018): operators, sender, SMS price, test modes, DRY_RUN and
// the operators' API keys. A value saved in the panel wins over the
// environment variable of the same setting, which wins over the default, so an
// installation configured through .env keeps working unchanged.
//
// Secrets are stored encrypted (AES-256-GCM, see Cipher) and never leave the
// package in a response: the panel only learns whether a secret is set.
//
// Owner: Marc3usz (domain D). The provider settings mirror
// providers/setup.Config (MichalK252, domain C).
package settings

import (
	"errors"

	"github.com/Marc3usz/DoYouSend/backend/internal/integrations/notion"
)

// Kind is how a setting is edited and validated.
type Kind string

// Setting kinds.
const (
	KindText   Kind = "text"
	KindBool   Kind = "bool"
	KindEnum   Kind = "enum"
	KindSecret Kind = "secret"
	KindPrice  Kind = "price" // PLN with up to 3 decimals, e.g. "0.08"
	KindPort   Kind = "port"
	KindInt    Kind = "int"     // whole number 0..1000
	KindEmail  Kind = "address" // RFC 5322 address, e.g. "Szkola <a@example.test>"
)

// Groups of settings, in the order the panel shows them.
const (
	GroupGeneral = "general"
	GroupEmail   = "email"
	GroupSMS     = "sms"
	// GroupNotion is the Notion database the recipients import reads
	// (ADR-0017). It never affects sending.
	GroupNotion = "notion"
)

// Def describes one setting.
type Def struct {
	Key     string
	Env     string // environment variable used when the panel has no value
	Group   string
	Kind    Kind
	Default string
	Options []string // KindEnum only
	// Normalize, when set, checks a non-empty value further and returns
	// its stored form, e.g. a Notion URL reduced to the ID.
	Normalize func(string) (string, error)
}

// Setting keys. They are part of the HTTP contract (docs/api/openapi.yaml).
const (
	KeyDryRun                = "dry_run"
	KeyEmailProvider         = "email_provider"
	KeyEmailFrom             = "email_from"
	KeyEmailReplyTo          = "email_reply_to"
	KeyEmailSenderDefault    = "email_sender_default"
	KeySMSMaxParts           = "sms_max_parts"
	KeySMTPHost              = "smtp_host"
	KeySMTPPort              = "smtp_port"
	KeySMTPUsername          = "smtp_username"
	KeySMTPPassword          = "smtp_password"
	KeySendGridAPIKey        = "sendgrid_api_key"
	KeySendGridSandbox       = "sendgrid_sandbox"
	KeySendGridRegion        = "sendgrid_region"
	KeySendGridWebhookKey    = "sendgrid_webhook_public_key"
	KeySenderAddress         = "sendgrid_sender_address"
	KeySenderCity            = "sendgrid_sender_city"
	KeySenderZip             = "sendgrid_sender_zip"
	KeySenderCountry         = "sendgrid_sender_country"
	KeySMSProvider           = "sms_provider"
	KeySMSAPIKey             = "sms_api_key"
	KeySMSSenderName         = "sms_sender_name"
	KeySMSTestMode           = "sms_test_mode"
	KeySMSPricePerPart       = "sms_price_per_part_pln"
	KeySMSAPIDeliveryReports = "smsapi_dlr_token"
	KeyNotionToken           = "notion_token"
	KeyNotionDatabaseID      = "notion_database_id"
	KeyNotionDataSourceID    = "notion_data_source_id"
	KeyNotionPropertyMap     = "notion_property_map"
)

// defs lists every setting, in display order. Defaults match .env.example and
// keep the system inert (DRY_RUN, Mailpit, fake SMS).
var defs = []Def{
	{Key: KeyDryRun, Env: "DRY_RUN", Group: GroupGeneral, Kind: KindBool, Default: "true"},

	{Key: KeyEmailProvider, Env: "EMAIL_PROVIDER", Group: GroupEmail, Kind: KindEnum, Default: "mailpit", Options: []string{"mailpit", "sendgrid"}},
	{Key: KeyEmailFrom, Env: "EMAIL_FROM", Group: GroupEmail, Kind: KindEmail, Default: "DoYouSend <no-reply@example.test>"},
	// Where replies to e-mails sent from the shared EMAIL_FROM go (ADR-0021);
	// empty: to EMAIL_FROM itself.
	{Key: KeyEmailReplyTo, Env: "EMAIL_REPLY_TO", Group: GroupEmail, Kind: KindEmail},
	// Which sender the composer preselects (ADR-0021): the user's own address
	// when the operator accepts it, or always the shared EMAIL_FROM.
	{Key: KeyEmailSenderDefault, Env: "EMAIL_SENDER_DEFAULT", Group: GroupEmail, Kind: KindEnum, Default: "own", Options: []string{"own", "shared"}},
	{Key: KeySMTPHost, Env: "SMTP_HOST", Group: GroupEmail, Kind: KindText, Default: "localhost"},
	{Key: KeySMTPPort, Env: "SMTP_PORT", Group: GroupEmail, Kind: KindPort, Default: "1025"},
	{Key: KeySMTPUsername, Env: "SMTP_USERNAME", Group: GroupEmail, Kind: KindText},
	{Key: KeySMTPPassword, Env: "SMTP_PASSWORD", Group: GroupEmail, Kind: KindSecret},
	{Key: KeySendGridAPIKey, Env: "SENDGRID_API_KEY", Group: GroupEmail, Kind: KindSecret},
	{Key: KeySendGridSandbox, Env: "SENDGRID_SANDBOX", Group: GroupEmail, Kind: KindBool, Default: "false"},
	{Key: KeySendGridRegion, Env: "SENDGRID_REGION", Group: GroupEmail, Kind: KindEnum, Default: "global", Options: []string{"global", "eu"}},
	{Key: KeySendGridWebhookKey, Env: "SENDGRID_WEBHOOK_PUBLIC_KEY", Group: GroupEmail, Kind: KindText},
	{Key: KeySenderAddress, Env: "SENDGRID_SENDER_ADDRESS", Group: GroupEmail, Kind: KindText},
	{Key: KeySenderCity, Env: "SENDGRID_SENDER_CITY", Group: GroupEmail, Kind: KindText},
	{Key: KeySenderZip, Env: "SENDGRID_SENDER_ZIP", Group: GroupEmail, Kind: KindText},
	{Key: KeySenderCountry, Env: "SENDGRID_SENDER_COUNTRY", Group: GroupEmail, Kind: KindText},

	{Key: KeySMSProvider, Env: "SMS_PROVIDER", Group: GroupSMS, Kind: KindEnum, Default: "fake", Options: []string{"fake", "smsapi"}},
	{Key: KeySMSAPIKey, Env: "SMS_API_KEY", Group: GroupSMS, Kind: KindSecret},
	{Key: KeySMSSenderName, Env: "SMS_SENDER_NAME", Group: GroupSMS, Kind: KindText, Default: "SZKOLA-TEST"},
	{Key: KeySMSTestMode, Env: "SMS_TEST_MODE", Group: GroupSMS, Kind: KindBool, Default: "false"},
	{Key: KeySMSPricePerPart, Env: "SMS_PRICE_PER_PART_PLN", Group: GroupSMS, Kind: KindPrice, Default: "0.08"},
	// SMSAPI's max_parts: a longer message fails instead of costing more. 0 keeps
	// the account default.
	{Key: KeySMSMaxParts, Env: "SMS_MAX_PARTS", Group: GroupSMS, Kind: KindInt, Default: "0"},
	{Key: KeySMSAPIDeliveryReports, Env: "SMSAPI_DLR_TOKEN", Group: GroupSMS, Kind: KindSecret},

	{Key: KeyNotionToken, Env: "NOTION_TOKEN", Group: GroupNotion, Kind: KindSecret},
	{Key: KeyNotionDatabaseID, Env: "NOTION_DATABASE_ID", Group: GroupNotion, Kind: KindText, Normalize: notionID},
	{Key: KeyNotionDataSourceID, Env: "NOTION_DATA_SOURCE_ID", Group: GroupNotion, Kind: KindText, Normalize: notionID},
	{Key: KeyNotionPropertyMap, Env: "NOTION_PROPERTY_MAP", Group: GroupNotion, Kind: KindText, Normalize: notionMapping},
}

// notionID accepts a Notion ID or a URL copied from Notion and keeps the ID.
func notionID(v string) (string, error) {
	id, err := notion.ParseID(v)
	if err != nil {
		return "", errors.New("must be a Notion ID or a link copied from Notion")
	}
	return id, nil
}

// notionMapping checks a "Notion property=column,..." list.
func notionMapping(v string) (string, error) {
	if _, err := notion.ParseMapping(v); err != nil {
		return "", err
	}
	return v, nil
}

// Defs returns every setting definition in display order.
func Defs() []Def {
	out := make([]Def, len(defs))
	copy(out, defs)
	return out
}

func defByKey(key string) (Def, bool) {
	for _, d := range defs {
		if d.Key == key {
			return d, true
		}
	}
	return Def{}, false
}
