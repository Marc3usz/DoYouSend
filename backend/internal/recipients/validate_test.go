package recipients

import "testing"

func TestValidateEmail(t *testing.T) {
	cases := []struct {
		name    string
		email   string
		wantErr bool
	}{
		{"valid", "jan.kowalski@example.test", false},
		{"valid with plus tag", "jan+szkola@example.test", false},
		{"missing at", "jan.kowalskiexample.test", true},
		{"missing domain dot", "jan@example", true},
		{"contains space", "jan kowalski@example.test", true},
		{"leading dot in domain", "jan@.example.test", true},
		{"double dot in domain", "jan@example..test", true},
		{"trailing dot in domain", "jan@example.test.", true},
		{"empty", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateEmail(tc.email)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateEmail(%q) error = %v, wantErr %v", tc.email, err, tc.wantErr)
			}
		})
	}
}

func TestValidatePhone(t *testing.T) {
	cases := []struct {
		name    string
		phone   string
		wantErr bool
	}{
		{"valid polish", "+48500100101", false},
		{"missing plus", "48500100101", true},
		{"contains spaces", "+48 500 100 101", true},
		{"too short", "+4850", true},
		{"leading zero after plus", "+0500100101", true},
		{"empty", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePhone(tc.phone)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidatePhone(%q) error = %v, wantErr %v", tc.phone, err, tc.wantErr)
			}
		})
	}
}

func TestNormalizePhone(t *testing.T) {
	cases := []struct {
		name  string
		phone string
		want  string
	}{
		{"already E.164", "+48500100101", "+48500100101"},
		{"spaced CLAUDE.md style", "+48 500 100 101", "+48500100101"},
		{"bare 9-digit local number", "500100101", "+48500100101"},
		{"hyphenated", "500-100-101", "+48500100101"},
		{"00 international prefix", "0048500100101", "+48500100101"},
		{"garbage stays garbage", "abc", "abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizePhone(tc.phone); got != tc.want {
				t.Errorf("NormalizePhone(%q) = %q, want %q", tc.phone, got, tc.want)
			}
		})
	}
}

func TestRecipientValidate(t *testing.T) {
	cases := []struct {
		name      string
		recipient Recipient
		wantErrs  int
	}{
		{
			name: "complete and valid",
			recipient: Recipient{
				FirstName: "Jan",
				LastName:  "Kowalski",
				Email:     "jan.kowalski@example.test",
				Phone:     "+48500100101",
				Type:      TypeParent,
			},
			wantErrs: 0,
		},
		{
			name: "only email is enough",
			recipient: Recipient{
				FirstName: "Jan",
				LastName:  "Kowalski",
				Email:     "jan.kowalski@example.test",
				Type:      TypeStudent,
			},
			wantErrs: 0,
		},
		{
			name: "no contact channel",
			recipient: Recipient{
				FirstName: "Jan",
				LastName:  "Kowalski",
				Type:      TypeParent,
			},
			wantErrs: 1,
		},
		{
			name:      "missing everything",
			recipient: Recipient{},
			wantErrs:  4, // first_name, last_name, type, no contact channel
		},
		{
			name: "invalid email and phone",
			recipient: Recipient{
				FirstName: "Jan",
				LastName:  "Kowalski",
				Email:     "not-an-email",
				Phone:     "123",
				Type:      TypeParent,
			},
			wantErrs: 2,
		},
		{
			name: "loose CSV phone format is normalized before validation",
			recipient: Recipient{
				FirstName: "Jan",
				LastName:  "Kowalski",
				Phone:     "500 100 101",
				Type:      TypeParent,
			},
			wantErrs: 0,
		},
		{
			name: "whitespace-only name is not a name",
			recipient: Recipient{
				FirstName: "   ",
				LastName:  "Kowalski",
				Email:     "jan.kowalski@example.test",
				Type:      TypeParent,
			},
			wantErrs: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := tc.recipient.Validate()
			if len(errs) != tc.wantErrs {
				t.Errorf("Validate() = %v, want %d errors", errs, tc.wantErrs)
			}
		})
	}
}
