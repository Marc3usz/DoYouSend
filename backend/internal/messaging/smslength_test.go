package messaging

import (
	"strings"
	"testing"
)

func TestMeasureSMS(t *testing.T) {
	tests := []struct {
		name string
		body string
		want SMSLength
	}{
		{"empty body", "", SMSLength{Encoding: EncodingGSM7, Units: 0, Parts: 0}},
		{"short ascii", "Jutro zebranie o 17:00.", SMSLength{EncodingGSM7, 23, 1}},
		{"gsm7 single part limit", strings.Repeat("a", 160), SMSLength{EncodingGSM7, 160, 1}},
		{"gsm7 one over single limit", strings.Repeat("a", 161), SMSLength{EncodingGSM7, 161, 2}},
		{"gsm7 two full parts", strings.Repeat("a", 306), SMSLength{EncodingGSM7, 306, 2}},
		{"gsm7 three parts", strings.Repeat("a", 307), SMSLength{EncodingGSM7, 307, 3}},
		{"gsm7 basic non-ascii stays gsm7", "Ñandù è ü @£$¥", SMSLength{EncodingGSM7, 14, 1}},
		{"extension char counts double", "€", SMSLength{EncodingGSM7, 2, 1}},
		{"extension chars at single limit", strings.Repeat("a", 158) + "[", SMSLength{EncodingGSM7, 160, 1}},
		{"extension chars over single limit", strings.Repeat("a", 159) + "]", SMSLength{EncodingGSM7, 161, 2}},
		// 306 septets would fit in 2 parts, but the escape pair at septet 153 cannot be split,
		// so it moves to part 2 and pushes the last septet into part 3.
		{"escape pair not split across parts", strings.Repeat("a", 152) + "{" + strings.Repeat("a", 152), SMSLength{EncodingGSM7, 306, 3}},
		{"polish diacritics force ucs2", "Zażółć gęślą jaźń", SMSLength{EncodingUCS2, 17, 1}},
		{"ucs2 single part limit", strings.Repeat("ą", 70), SMSLength{EncodingUCS2, 70, 1}},
		{"ucs2 one over single limit", strings.Repeat("ą", 71), SMSLength{EncodingUCS2, 71, 2}},
		{"ucs2 two full parts", strings.Repeat("ą", 134), SMSLength{EncodingUCS2, 134, 2}},
		{"ucs2 three parts", strings.Repeat("ą", 135), SMSLength{EncodingUCS2, 135, 3}},
		{"emoji is two utf16 units", "😀", SMSLength{EncodingUCS2, 2, 1}},
		// 134 units would fit in 2 parts, but the surrogate pair at unit 67 cannot be split,
		// so it moves to part 2 and pushes the last unit into part 3.
		{"surrogate pair not split across parts", strings.Repeat("ą", 66) + "😀" + strings.Repeat("ą", 66), SMSLength{EncodingUCS2, 134, 3}},
		{"one non-gsm char switches whole body", strings.Repeat("a", 100) + "ł", SMSLength{EncodingUCS2, 101, 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MeasureSMS(tt.body)
			if got != tt.want {
				t.Errorf("MeasureSMS(%q) = %+v, want %+v", tt.body, got, tt.want)
			}
		})
	}
}
