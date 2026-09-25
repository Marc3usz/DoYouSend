package messaging

import (
	"strings"
	"unicode/utf16"
)

// Encoding is the SMS alphabet a body has to be sent in.
type Encoding string

const (
	// EncodingGSM7 is the GSM 03.38 default alphabet, 7 bits per character.
	EncodingGSM7 Encoding = "GSM-7"
	// EncodingUCS2 is used as soon as any character is outside GSM-7 (e.g. Polish diacritics).
	EncodingUCS2 Encoding = "UCS-2"
)

// Part capacities in encoding units. A multipart SMS loses room to the UDH concatenation header.
const (
	gsm7SinglePart = 160
	gsm7MultiPart  = 153
	ucs2SinglePart = 70
	ucs2MultiPart  = 67
)

// gsm7Basic holds the characters of the GSM 03.38 basic table (one septet each).
const gsm7Basic = "@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?" +
	"¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà"

// gsm7Extension holds the characters reached via the escape code (two septets each).
const gsm7Extension = "\f^{}\\[~]|€"

// SMSLength describes how a body is carried over SMS.
type SMSLength struct {
	Encoding Encoding
	// Units is the body length in septets (GSM-7) or UTF-16 code units (UCS-2).
	Units int
	// Parts is the number of SMS messages one recipient receives.
	Parts int
}

// MeasureSMS reports the encoding, length and number of SMS parts for body.
// It never alters the text: the caller sends body as is, split into Parts messages.
func MeasureSMS(body string) SMSLength {
	encoding := EncodingGSM7
	if !isGSM7(body) {
		encoding = EncodingUCS2
	}

	costs := unitCosts(body, encoding)
	units := 0
	for _, c := range costs {
		units += c
	}

	single, multi := gsm7SinglePart, gsm7MultiPart
	if encoding == EncodingUCS2 {
		single, multi = ucs2SinglePart, ucs2MultiPart
	}

	parts := 0
	switch {
	case units == 0:
	case units <= single:
		parts = 1
	default:
		parts = countParts(costs, multi)
	}
	return SMSLength{Encoding: encoding, Units: units, Parts: parts}
}

func isGSM7(body string) bool {
	for _, r := range body {
		if !strings.ContainsRune(gsm7Basic, r) && !isGSM7Extension(r) {
			return false
		}
	}
	return true
}

// unitCosts returns the per-character cost in encoding units. Characters costing 2
// (GSM-7 escape sequences, UTF-16 surrogate pairs) must not be split between parts.
func unitCosts(body string, encoding Encoding) []int {
	costs := make([]int, 0, len(body))
	for _, r := range body {
		cost := 1
		switch {
		case encoding == EncodingGSM7 && isGSM7Extension(r):
			cost = 2
		case encoding == EncodingUCS2 && utf16.RuneLen(r) == 2:
			cost = 2
		}
		costs = append(costs, cost)
	}
	return costs
}

// countParts packs characters greedily into parts of the given capacity,
// starting a new part when a character does not fit whole.
func countParts(costs []int, capacity int) int {
	parts, used := 1, 0
	for _, c := range costs {
		if used+c > capacity {
			parts++
			used = 0
		}
		used += c
	}
	return parts
}

func isGSM7Extension(r rune) bool {
	return strings.ContainsRune(gsm7Extension, r)
}
