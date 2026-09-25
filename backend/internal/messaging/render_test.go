package messaging

import (
	"errors"
	"testing"
)

func TestRender(t *testing.T) {
	fields := map[string]string{"imie": "Anna", "nazwisko": "Testowa", "klasa": "3B"}
	tests := []struct {
		name    string
		tmpl    string
		want    string
		wantErr error
	}{
		{name: "no placeholders", tmpl: "Jutro zebranie.", want: "Jutro zebranie."},
		{name: "empty body", tmpl: "", want: ""},
		{name: "single placeholder", tmpl: "Dzień dobry {{imie}}!", want: "Dzień dobry Anna!"},
		{name: "several placeholders", tmpl: "{{imie}} {{nazwisko}}, klasa {{klasa}}", want: "Anna Testowa, klasa 3B"},
		{name: "same placeholder twice", tmpl: "{{imie}} i {{imie}}", want: "Anna i Anna"},
		{name: "spaces inside braces", tmpl: "Hej {{ imie }}", want: "Hej Anna"},
		{name: "unclosed braces stay literal", tmpl: "Kod {{abc", want: "Kod {{abc"},
		{name: "single braces stay literal", tmpl: "Zbiór {a, b}", want: "Zbiór {a, b}"},
		{name: "whitespace in text preserved", tmpl: "  {{imie}}\n\nkoniec  ", want: "  Anna\n\nkoniec  "},
		{name: "value is not re-expanded", tmpl: "{{imie}}", want: "Anna"},
		{name: "unmatched open braces before placeholder stay literal", tmpl: "{{abc unrelated {{imie}}", want: "{{abc unrelated Anna"},
		{name: "triple open braces keep one literal brace", tmpl: "{{{imie}}", want: "{Anna"},
		{name: "unknown placeholder", tmpl: "Hej {{imiee}}", wantErr: ErrUnknownPlaceholder},
		{name: "empty placeholder name", tmpl: "Hej {{}}", wantErr: ErrUnknownPlaceholder},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.tmpl, fields)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Render(%q) error = %v, want %v", tt.tmpl, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Render(%q) = %q, want %q", tt.tmpl, got, tt.want)
			}
		})
	}
}

func TestRenderMissingValue(t *testing.T) {
	_, err := Render("Hej {{imie}}", map[string]string{"imie": ""})
	if !errors.Is(err, ErrMissingValue) {
		t.Fatalf("Render() error = %v, want %v", err, ErrMissingValue)
	}
}

func TestRenderDoesNotExpandValues(t *testing.T) {
	got, err := Render("{{imie}}", map[string]string{"imie": "{{nazwisko}}", "nazwisko": "X"})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if got != "{{nazwisko}}" {
		t.Errorf("Render() = %q, want value inserted verbatim", got)
	}
}

func TestPlaceholders(t *testing.T) {
	got := Placeholders("{{x {{imie}} {{ klasa }} {{imie}} {{brak")
	want := []string{"imie", "klasa"}
	if len(got) != len(want) {
		t.Fatalf("Placeholders() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Placeholders()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
