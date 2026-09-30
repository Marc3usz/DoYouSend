package groups

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSelectionNormalize(t *testing.T) {
	sel := Selection{
		GroupIDs:             []string{" " + strings.ToUpper(AllParentsID), AllParentsID, "", AllStudentsID},
		RecipientIDs:         []string{idJan, idMaria, strings.ToUpper(idJan)},
		ExcludedRecipientIDs: []string{"  "},
	}
	want := Selection{
		GroupIDs:     []string{AllParentsID, AllStudentsID},
		RecipientIDs: []string{idJan, idMaria},
	}
	if got := sel.Normalize(); !reflect.DeepEqual(got, want) {
		t.Errorf("Normalize() = %+v, want %+v", got, want)
	}
}

func TestSelectionValidate(t *testing.T) {
	tooMany := make([]string, MaxSelectionIDs+1)
	for i := range tooMany {
		tooMany[i] = idJan
	}

	tests := []struct {
		name    string
		sel     Selection
		wantErr error
		fields  []string
	}{
		{name: "groups only", sel: Selection{GroupIDs: []string{AllParentsID}}},
		{name: "people only", sel: Selection{RecipientIDs: []string{idJan}}},
		{name: "everyone excluded is still valid", sel: Selection{RecipientIDs: []string{idJan}, ExcludedRecipientIDs: []string{idJan}}},
		{name: "empty", sel: Selection{}, wantErr: ErrEmptySelection},
		{
			name:    "every bad ID is reported with its position",
			sel:     Selection{GroupIDs: []string{AllParentsID, "3A"}, RecipientIDs: []string{"x"}, ExcludedRecipientIDs: []string{idJan, "y"}},
			wantErr: ErrInvalidInput,
			fields:  []string{"groupIds[1]", "recipientIds[0]", "excludedRecipientIds[1]"},
		},
		{name: "too many IDs", sel: Selection{RecipientIDs: tooMany}, wantErr: ErrInvalidInput, fields: []string{"recipientIds"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.sel.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.wantErr)
			}
			var verr *ValidationError
			if !errors.As(err, &verr) {
				return
			}
			var got []string
			for _, f := range verr.Fields {
				got = append(got, f.Field)
			}
			if !reflect.DeepEqual(got, tt.fields) {
				t.Errorf("fields = %v, want %v", got, tt.fields)
			}
		})
	}
}
