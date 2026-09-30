package groups

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestGroupInputNormalize(t *testing.T) {
	tests := []struct {
		in   GroupInput
		want GroupInput
	}{
		{in: GroupInput{Name: "  Rada \t rodziców\n", Description: "  opis \n"}, want: GroupInput{Name: "Rada rodziców", Description: "opis"}},
		{in: GroupInput{Name: "Chór", Description: "linia 1\nlinia 2"}, want: GroupInput{Name: "Chór", Description: "linia 1\nlinia 2"}},
		{in: GroupInput{}, want: GroupInput{}},
	}
	for _, tt := range tests {
		if got := tt.in.Normalize(); got != tt.want {
			t.Errorf("%+v.Normalize() = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestGroupInputValidate(t *testing.T) {
	tests := []struct {
		name   string
		in     GroupInput
		fields []string
	}{
		{name: "valid", in: GroupInput{Name: "Rada rodziców", Description: "Spotkania\nco miesiąc\t(środa)"}},
		{name: "name at the limit, counted in runes", in: GroupInput{Name: strings.Repeat("ż", MaxNameLength)}},
		{name: "empty name", in: GroupInput{}, fields: []string{"name"}},
		{name: "name too long", in: GroupInput{Name: strings.Repeat("a", MaxNameLength+1)}, fields: []string{"name"}},
		{name: "control character in name", in: GroupInput{Name: "Chór\x07"}, fields: []string{"name"}},
		{name: "description too long", in: GroupInput{Name: "Chór", Description: strings.Repeat("ó", MaxDescriptionLength+1)}, fields: []string{"description"}},
		{name: "control character in description", in: GroupInput{Name: "Chór", Description: "a\x1bb"}, fields: []string{"description"}},
		{name: "everything wrong", in: GroupInput{Description: "\x00"}, fields: []string{"name", "description"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, e := range tt.in.Validate() {
				got = append(got, e.Field)
			}
			if !reflect.DeepEqual(got, tt.fields) {
				t.Errorf("Validate() fields = %v, want %v", got, tt.fields)
			}
		})
	}
}

func TestErrorTypesMatchSentinels(t *testing.T) {
	var verr error = &ValidationError{}
	if !errors.Is(verr, ErrInvalidInput) || errors.Is(verr, ErrNotFound) {
		t.Errorf("ValidationError must match ErrInvalidInput only")
	}
	var uerr error = &UnknownRecipientsError{IDs: []string{idNobody}}
	if !errors.Is(uerr, ErrUnknownRecipient) || errors.Is(uerr, ErrInvalidInput) {
		t.Errorf("UnknownRecipientsError must match ErrUnknownRecipient only")
	}
	if !strings.Contains(uerr.Error(), idNobody) {
		t.Errorf("UnknownRecipientsError.Error() = %q, want the ID in it", uerr.Error())
	}
}

func TestIsValidID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{idJan, true},
		{strings.ToUpper(idJan), true},
		{AllParentsID, true},
		{"", false},
		{"3A", false},
		{strings.ReplaceAll(idJan, "-", ""), false},
		{idJan + "0", false},
		{"{" + idJan + "}", false},
		{"zzzzzzzz-1111-4111-8111-000000000001", false},
	}
	for _, tt := range tests {
		if got := IsValidID(tt.id); got != tt.want {
			t.Errorf("IsValidID(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}
