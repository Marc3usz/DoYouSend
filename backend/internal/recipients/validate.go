package recipients

import (
	"errors"
	"regexp"
	"strings"
)

// emailPattern is intentionally permissive on the local part (RFC 5322 in
// full is not worth the false positives it rejects), but the domain must be
// a proper dot-separated list of labels — no leading/trailing/double dots —
// so obvious typos like "a@b..c" or "a@b.c." are still caught.
var emailPattern = regexp.MustCompile(`^[^\s@]+@[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?)+$`)

// phonePattern requires E.164: a leading '+', country code, no spaces or
// separators, e.g. +48500100101. Callers should run NormalizePhone first —
// ValidatePhone itself does not accept spaced or local-format input.
var phonePattern = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)

var (
	ErrInvalidEmail = errors.New("not a valid e-mail address")
	ErrInvalidPhone = errors.New("not a valid E.164 phone number, expected e.g. +48500100101")
)

// ValidateEmail reports whether email is well-formed enough to attempt delivery.
func ValidateEmail(email string) error {
	if !emailPattern.MatchString(email) {
		return ErrInvalidEmail
	}
	return nil
}

// ValidatePhone reports whether phone is a well-formed E.164 number.
func ValidatePhone(phone string) error {
	if !phonePattern.MatchString(phone) {
		return ErrInvalidPhone
	}
	return nil
}

// localPhonePattern matches a bare Polish mobile/landline number without a
// country code, e.g. "500100101" as it typically appears in a school's CSV
// export.
var localPhonePattern = regexp.MustCompile(`^\d{9}$`)

// NormalizePhone turns loose human/CSV input into an E.164 candidate:
// it strips spaces, hyphens and parentheses, rewrites a leading international
// "00" prefix to "+", and assumes a bare 9-digit number is a Polish number.
// The result still needs ValidatePhone — normalization never invents digits
// that were not there, so garbage input stays garbage, just untidy garbage.
func NormalizePhone(phone string) string {
	s := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(strings.TrimSpace(phone))
	switch {
	case strings.HasPrefix(s, "+"):
		return s
	case strings.HasPrefix(s, "00"):
		return "+" + s[2:]
	case localPhonePattern.MatchString(s):
		return "+48" + s
	default:
		return s
	}
}
