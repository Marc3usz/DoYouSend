package messaging

import (
	"errors"
	"testing"
)

func TestParsePrice(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    int64
		wantErr error
	}{
		{name: "env example value", in: "0.08", want: 80},
		{name: "fractional grosz", in: "0.065", want: 65},
		{name: "whole units", in: "1", want: 1000},
		{name: "one decimal", in: "1.5", want: 1500},
		{name: "zero", in: "0", want: 0},
		{name: "surrounding spaces", in: " 0.08 ", want: 80},
		{name: "empty", in: "", wantErr: ErrInvalidPrice},
		{name: "comma separator", in: "0,08", wantErr: ErrInvalidPrice},
		{name: "more than three decimals", in: "0.0655", wantErr: ErrInvalidPrice},
		{name: "negative", in: "-0.08", wantErr: ErrInvalidPrice},
		{name: "plus sign", in: "+0.08", wantErr: ErrInvalidPrice},
		{name: "trailing dot", in: "1.", wantErr: ErrInvalidPrice},
		{name: "leading dot", in: ".5", wantErr: ErrInvalidPrice},
		{name: "letters", in: "abc", wantErr: ErrInvalidPrice},
		{name: "overflow", in: "99999999999999999999", wantErr: ErrInvalidPrice},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePrice(tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ParsePrice(%q) error = %v, want %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParsePrice(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}
