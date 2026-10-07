package messaging

import (
	"slices"
	"testing"
)

func TestUnknownPlaceholders(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		want []string
	}{
		{name: "no placeholders", tmpl: "Jutro zebranie.", want: nil},
		{name: "only known", tmpl: "Witaj {{imie}} {{ nazwisko }}", want: nil},
		{name: "unknown listed once in order", tmpl: "{{klasa}} {{imie}} {{data}} {{klasa}}", want: []string{"klasa", "data"}},
		{name: "case matters", tmpl: "{{Imie}}", want: []string{"Imie"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UnknownPlaceholders(tt.tmpl); !slices.Equal(got, tt.want) {
				t.Errorf("UnknownPlaceholders(%q) = %q, want %q", tt.tmpl, got, tt.want)
			}
		})
	}
}

func TestRecipientFieldsRenderEveryKnownPlaceholder(t *testing.T) {
	fields := RecipientFields("Anna", "Nowak")
	got, err := Render("{{imie}} {{nazwisko}}", fields)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got != "Anna Nowak" {
		t.Errorf("Render = %q, want %q", got, "Anna Nowak")
	}
	for _, name := range KnownPlaceholders() {
		if _, ok := fields[name]; !ok {
			t.Errorf("RecipientFields has no value for known placeholder %q", name)
		}
	}
}
