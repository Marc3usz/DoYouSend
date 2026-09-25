package messaging

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrUnknownPlaceholder is returned when the body references a field that does not exist.
	ErrUnknownPlaceholder = errors.New("unknown placeholder")
	// ErrMissingValue is returned when a referenced field is empty for this recipient.
	ErrMissingValue = errors.New("placeholder has no value")
)

const (
	placeholderOpen  = "{{"
	placeholderClose = "}}"
)

// Render substitutes {{name}} placeholders in tmpl with values from fields.
// The result is the single body sent over BOTH e-mail and SMS for one recipient.
// Text outside placeholders is copied byte for byte; inserted values are never
// expanded again. An unmatched "{{" is kept as literal text.
//
// An error concerns this one recipient only: the caller marks that recipient as failed
// and carries on with the rest of the batch; it must never abort the whole send.
func Render(tmpl string, fields map[string]string) (string, error) {
	var b strings.Builder
	b.Grow(len(tmpl))
	rest := tmpl
	for {
		name, before, after, ok := nextPlaceholder(rest)
		if !ok {
			b.WriteString(rest)
			return b.String(), nil
		}
		value, known := fields[name]
		if !known {
			return "", fmt.Errorf("render body: %w: %q", ErrUnknownPlaceholder, name)
		}
		if value == "" {
			return "", fmt.Errorf("render body: %w: %q", ErrMissingValue, name)
		}
		b.WriteString(before)
		b.WriteString(value)
		rest = after
	}
}

// Placeholders lists the distinct placeholder names used in tmpl, in order of first use.
// Use it to validate a draft before any recipient data is loaded.
func Placeholders(tmpl string) []string {
	var names []string
	seen := make(map[string]bool)
	rest := tmpl
	for {
		name, _, after, ok := nextPlaceholder(rest)
		if !ok {
			return names
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
		rest = after
	}
}

// nextPlaceholder finds the first complete {{name}} in s and returns its trimmed name,
// the text before it and the text after it. The "{{" nearest to the closing "}}" wins,
// so in "{{abc {{imie}}" the placeholder is "imie" and "{{abc " stays literal.
func nextPlaceholder(s string) (name, before, after string, ok bool) {
	first := strings.Index(s, placeholderOpen)
	if first < 0 {
		return "", "", "", false
	}
	end := strings.Index(s[first+len(placeholderOpen):], placeholderClose)
	if end < 0 {
		return "", "", "", false
	}
	end += first + len(placeholderOpen)
	start := strings.LastIndex(s[:end], placeholderOpen)
	name = strings.TrimSpace(s[start+len(placeholderOpen) : end])
	return name, s[:start], s[end+len(placeholderClose):], true
}
