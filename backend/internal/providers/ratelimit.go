package providers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// rateLimitError wraps an error that was caused by a rate limit response (e.g. HTTP 429),
// carrying an optional retry-after duration requested by the provider gateway.
type rateLimitError struct {
	err        error
	retryAfter time.Duration
}

func (e *rateLimitError) Error() string {
	if e.retryAfter > 0 {
		return fmt.Sprintf("rate limited (retry after %s): %s", e.retryAfter, e.err)
	}
	return fmt.Sprintf("rate limited: %s", e.err)
}

func (e *rateLimitError) Unwrap() error { return e.err }

// RetryAfter returns the retry-after duration suggested by the gateway.
func (e *rateLimitError) RetryAfter() time.Duration { return e.retryAfter }

// RateLimitError wraps err as a transient, rate-limited error with an optional retry-after duration.
// The delivery worker uses RetryAfter to delay retries or pause the affected channel.
func RateLimitError(err error, retryAfter time.Duration) error {
	return TransientError(&rateLimitError{err: err, retryAfter: retryAfter})
}

// IsRateLimit reports whether err (or any error in its chain) was caused by rate limiting.
func IsRateLimit(err error) bool {
	var rle interface{ RetryAfter() time.Duration }
	return errors.As(err, &rle)
}

// RetryAfter extracts the suggested retry-after duration from err (or any error in its chain)
// if it implements interface{ RetryAfter() time.Duration }. Returns 0, false if not specified.
func RetryAfter(err error) (time.Duration, bool) {
	var rle interface{ RetryAfter() time.Duration }
	if errors.As(err, &rle) {
		d := rle.RetryAfter()
		if d > 0 {
			return d, true
		}
	}
	return 0, false
}

// ParseRetryAfter parses an HTTP Retry-After header value into a time.Duration relative to now.
// RFC 7231 specifies Retry-After can be either a decimal number of seconds (e.g. "120")
// or an HTTP-date (e.g. "Fri, 31 Dec 1999 23:59:59 GMT").
// If header is empty, invalid, or specifies a time in the past, it returns (0, false).
func ParseRetryAfter(header string, now time.Time) (time.Duration, bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0, false
	}

	// 1. Try parsing as integer seconds.
	if seconds, err := strconv.ParseInt(header, 10, 64); err == nil {
		if seconds > 0 {
			return time.Duration(seconds) * time.Second, true
		}
		return 0, false
	}

	// 2. Try parsing as HTTP-date (RFC 1123, RFC 850, ANSI C).
	for _, format := range []string{http.TimeFormat, time.RFC850, time.ANSIC} {
		if t, err := time.Parse(format, header); err == nil {
			d := t.Sub(now)
			if d > 0 {
				return d, true
			}
			return 0, false
		}
	}

	return 0, false
}
