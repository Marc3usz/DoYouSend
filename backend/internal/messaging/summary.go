package messaging

import (
	"errors"
	"fmt"
)

var (
	// ErrNegativePrice is returned when the configured SMS part price is below zero.
	ErrNegativePrice = errors.New("sms part price must not be negative")
	// ErrEmptyBody is returned when a recipient's rendered body is empty: nothing would be sent.
	ErrEmptyBody = errors.New("message body is empty")
)

// RenderedMessage is one recipient's final (already personalised) body together with
// the channels that recipient can be reached on. The same Body goes to both channels.
type RenderedMessage struct {
	Body     string
	HasEmail bool
	HasPhone bool
}

// Summary is what the sender sees before confirming a batch.
type Summary struct {
	Recipients int
	EmailCount int
	SMSCount   int
	// PartialCount is the number of recipients reachable on exactly one channel.
	PartialCount int
	// UnreachableCount is the number of recipients with neither an e-mail nor a phone.
	UnreachableCount int
	// UCS2Count is the number of SMS recipients whose body needs UCS-2 (70 chars per part).
	UCS2Count            int
	MinPartsPerRecipient int
	MaxPartsPerRecipient int
	TotalParts           int
	// CostMilli is the expected SMS cost in thousandths of the currency unit
	// (for PLN: 1000 = 1 zł), so fractional gateway prices such as 0.065 zł stay exact.
	CostMilli int64
}

// Summarize builds the pre-send summary for msgs, pricing every SMS part at
// pricePerPartMilli thousandths of the currency unit (0.08 zł = 80). E-mail is treated as free.
//
// msgs must already be deduplicated: a recipient selected through several groups appears
// exactly once, otherwise counts and cost are overstated.
func Summarize(msgs []RenderedMessage, pricePerPartMilli int64) (Summary, error) {
	if pricePerPartMilli < 0 {
		return Summary{}, fmt.Errorf("summarize batch: %w", ErrNegativePrice)
	}

	s := Summary{Recipients: len(msgs)}
	for i, m := range msgs {
		if m.Body == "" {
			return Summary{}, fmt.Errorf("summarize batch: recipient %d: %w", i, ErrEmptyBody)
		}
		s = addRecipient(s, m)
	}
	s.CostMilli = int64(s.TotalParts) * pricePerPartMilli
	return s, nil
}

func addRecipient(s Summary, m RenderedMessage) Summary {
	switch {
	case !m.HasEmail && !m.HasPhone:
		s.UnreachableCount++
	case m.HasEmail != m.HasPhone:
		s.PartialCount++
	}
	if m.HasEmail {
		s.EmailCount++
	}
	if !m.HasPhone {
		return s
	}

	length := MeasureSMS(m.Body)
	if length.Encoding == EncodingUCS2 {
		s.UCS2Count++
	}
	if s.SMSCount == 0 || length.Parts < s.MinPartsPerRecipient {
		s.MinPartsPerRecipient = length.Parts
	}
	s.MaxPartsPerRecipient = max(s.MaxPartsPerRecipient, length.Parts)
	s.SMSCount++
	s.TotalParts += length.Parts
	return s
}
