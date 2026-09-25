package messaging

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ErrInvalidPrice is returned when a configured price is not a plain non-negative decimal.
var ErrInvalidPrice = errors.New("price must be a non-negative decimal with at most 3 fractional digits")

// pricePattern accepts "1", "0.08", "0.065"; a dot is the only separator (as in .env).
var pricePattern = regexp.MustCompile(`^(\d+)(?:\.(\d{1,3}))?$`)

// ParsePrice converts a decimal price in currency units (e.g. SMS_PRICE_PER_PART_PLN="0.08")
// into thousandths of the unit (80), the form Summarize expects.
func ParsePrice(s string) (int64, error) {
	m := pricePattern.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("parse price %q: %w", s, ErrInvalidPrice)
	}
	frac := (m[2] + "000")[:3]
	milli, err := strconv.ParseInt(m[1]+frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse price %q: %w", s, ErrInvalidPrice)
	}
	return milli, nil
}
