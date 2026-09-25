package messaging

import (
	"errors"
	"strings"
	"testing"
)

func TestSummarize(t *testing.T) {
	long := strings.Repeat("a", 161) // 2 GSM-7 parts
	tests := []struct {
		name  string
		msgs  []RenderedMessage
		price int64
		want  Summary
	}{
		{
			name:  "no recipients",
			msgs:  nil,
			price: 9,
			want:  Summary{},
		},
		{
			name: "both channels, single part",
			msgs: []RenderedMessage{
				{Body: "Jutro zebranie.", HasEmail: true, HasPhone: true},
				{Body: "Jutro zebranie.", HasEmail: true, HasPhone: true},
			},
			price: 9,
			want: Summary{
				Recipients: 2, EmailCount: 2, SMSCount: 2,
				MinPartsPerRecipient: 1, MaxPartsPerRecipient: 1,
				TotalParts: 2, CostMilli: 18,
			},
		},
		{
			name: "email-only recipient adds no sms parts and marks partial",
			msgs: []RenderedMessage{
				{Body: long, HasEmail: true, HasPhone: true},
				{Body: long, HasEmail: true},
			},
			price: 10,
			want: Summary{
				Recipients: 2, EmailCount: 2, SMSCount: 1, PartialCount: 1,
				MinPartsPerRecipient: 2, MaxPartsPerRecipient: 2,
				TotalParts: 2, CostMilli: 20,
			},
		},
		{
			name: "personalised bodies differ in parts and encoding",
			msgs: []RenderedMessage{
				{Body: "Witaj Anno", HasEmail: true, HasPhone: true},
				{Body: "Witaj Łukaszu " + strings.Repeat("a", 60), HasPhone: true},
			},
			price: 7,
			want: Summary{
				Recipients: 2, EmailCount: 1, SMSCount: 2, PartialCount: 1, UCS2Count: 1,
				MinPartsPerRecipient: 1, MaxPartsPerRecipient: 2,
				TotalParts: 3, CostMilli: 21,
			},
		},
		{
			name: "recipient without any contact is counted but not reachable",
			msgs: []RenderedMessage{
				{Body: "Hej"},
			},
			price: 9,
			want:  Summary{Recipients: 1, UnreachableCount: 1},
		},
		{
			name:  "free sms",
			msgs:  []RenderedMessage{{Body: "Hej", HasPhone: true}},
			price: 0,
			want: Summary{
				Recipients: 1, SMSCount: 1, PartialCount: 1,
				MinPartsPerRecipient: 1, MaxPartsPerRecipient: 1, TotalParts: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Summarize(tt.msgs, tt.price)
			if err != nil {
				t.Fatalf("Summarize() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Summarize() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSummarizeRejectsEmptyBody(t *testing.T) {
	tests := []struct {
		name string
		msg  RenderedMessage
	}{
		{name: "sms", msg: RenderedMessage{HasPhone: true}},
		{name: "email", msg: RenderedMessage{HasEmail: true}},
		{name: "both channels", msg: RenderedMessage{HasEmail: true, HasPhone: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgs := []RenderedMessage{{Body: "Hej", HasPhone: true}, tt.msg}
			_, err := Summarize(msgs, 80)
			if !errors.Is(err, ErrEmptyBody) {
				t.Fatalf("Summarize() error = %v, want %v", err, ErrEmptyBody)
			}
			if !strings.Contains(err.Error(), "recipient 1") {
				t.Errorf("Summarize() error = %q, want it to name recipient 1", err)
			}
		})
	}
}

func TestSummarizeFractionalPrice(t *testing.T) {
	// 0.065 zł per part = 65 thousandths of PLN; 3 parts -> 0.195 zł.
	msgs := []RenderedMessage{
		{Body: "Hej", HasPhone: true},
		{Body: strings.Repeat("a", 161), HasPhone: true},
	}
	got, err := Summarize(msgs, 65)
	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}
	if got.CostMilli != 195 {
		t.Errorf("Summarize().CostMilli = %d, want 195", got.CostMilli)
	}
}

func TestSummarizeRejectsNegativePrice(t *testing.T) {
	_, err := Summarize([]RenderedMessage{{Body: "Hej", HasPhone: true}}, -1)
	if !errors.Is(err, ErrNegativePrice) {
		t.Fatalf("Summarize() error = %v, want %v", err, ErrNegativePrice)
	}
}
