package recipients

import (
	"errors"
	"regexp"
)

// emailPattern is intentionally permissive (RFC 5322 in full is not worth the
// false positives it rejects); it catches the typo/garbage cases an import
// report needs to flag.
var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// phonePattern requires E.164: a leading '+', country code, no spaces or
// separators, e.g. +48500100101. Import/CRUD normalize input before storing.
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
