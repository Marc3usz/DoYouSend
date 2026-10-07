package messaging

import "slices"

// Placeholder names a sender may use in a body, e.g. "Dzien dobry {{imie}}".
// Proposed in ADR-0005: the mapping from a recipient to these names lives in messaging,
// because the names are part of the message, not of the recipient record.
const (
	PlaceholderFirstName = "imie"
	PlaceholderLastName  = "nazwisko"
)

// KnownPlaceholders lists every placeholder name RecipientFields fills.
func KnownPlaceholders() []string {
	return []string{PlaceholderFirstName, PlaceholderLastName}
}

// RecipientFields returns the placeholder values for one recipient, ready for Render.
func RecipientFields(firstName, lastName string) map[string]string {
	return map[string]string{
		PlaceholderFirstName: firstName,
		PlaceholderLastName:  lastName,
	}
}

// UnknownPlaceholders lists the placeholder names used in tmpl that RecipientFields does
// not fill, in order of first use. Such a body would fail for every recipient.
func UnknownPlaceholders(tmpl string) []string {
	known := KnownPlaceholders()
	var unknown []string
	for _, name := range Placeholders(tmpl) {
		if !slices.Contains(known, name) {
			unknown = append(unknown, name)
		}
	}
	return unknown
}
