package notion

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPropertyText(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"title", `{"type":"title","title":[{"plain_text":"Anna"},{"plain_text":" Maria"}]}`, "Anna Maria"},
		{"rich_text", `{"type":"rich_text","rich_text":[{"plain_text":"Testowa"}]}`, "Testowa"},
		{"empty rich_text", `{"type":"rich_text","rich_text":[]}`, ""},
		{"email", `{"type":"email","email":"anna.testowa@example.test"}`, "anna.testowa@example.test"},
		{"null email", `{"type":"email","email":null}`, ""},
		{"phone_number", `{"type":"phone_number","phone_number":"+48 500 100 101"}`, "+48 500 100 101"},
		{"select", `{"type":"select","select":{"name":"rodzic"}}`, "rodzic"},
		{"null select", `{"type":"select","select":null}`, ""},
		{"status", `{"type":"status","status":{"name":"Nowy"}}`, "Nowy"},
		{"multi_select", `{"type":"multi_select","multi_select":[{"name":"1A"},{"name":"2B"}]}`, "1A, 2B"},
		{"number", `{"type":"number","number":48500100101}`, "48500100101"},
		{"decimal", `{"type":"number","number":1.5}`, "1.5"},
		{"checkbox", `{"type":"checkbox","checkbox":true}`, "true"},
		{"formula string", `{"type":"formula","formula":{"type":"string","string":"3A"}}`, "3A"},
		{"formula number", `{"type":"formula","formula":{"type":"number","number":7}}`, "7"},
		{"formula boolean", `{"type":"formula","formula":{"type":"boolean","boolean":false}}`, "false"},
		{"unique_id", `{"type":"unique_id","unique_id":{"prefix":"ODB","number":12}}`, "ODB-12"},
		{"unique_id no prefix", `{"type":"unique_id","unique_id":{"prefix":null,"number":12}}`, "12"},
		{"unsupported", `{"type":"date","date":{"start":"2026-11-03"}}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p Property
			if err := json.Unmarshal([]byte(tt.raw), &p); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := p.Text(); got != tt.want {
				t.Errorf("Text() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseMapping(t *testing.T) {
	tests := []struct {
		in      string
		want    Mapping
		wantErr bool
	}{
		{in: "", want: nil},
		{in: "Name=first_name, Surname = last_name,", want: Mapping{{"Name", "first_name"}, {"Surname", "last_name"}}},
		{in: "Name", wantErr: true},
		{in: "=email", wantErr: true},
		{in: "A=email,A=phone", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseMapping(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseMapping() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseMapping() = %v, want %v", got, tt.want)
			}
		})
	}
}

func page(props map[string]string) Page {
	p := Page{Properties: map[string]Property{}}
	for name, val := range props {
		v := val
		p.Properties[name] = Property{Type: "rich_text", RichText: []richText{{PlainText: v}}}
	}
	return p
}

func TestToTable(t *testing.T) {
	pages := []Page{
		page(map[string]string{"Imię": "Anna", "Nazwisko": "Testowa", "Uwagi": " x "}),
		page(map[string]string{"Imię": "Piotr"}),
	}
	tests := []struct {
		name string
		m    Mapping
		want [][]string
	}{
		{
			name: "identity, sorted, missing property empty",
			want: [][]string{{"Imię", "Nazwisko", "Uwagi"}, {"Anna", "Testowa", "x"}, {"Piotr", "", ""}},
		},
		{
			name: "mapping picks and renames",
			m:    Mapping{{"Nazwisko", "last_name"}, {"Imię", "first_name"}},
			want: [][]string{{"last_name", "first_name"}, {"Testowa", "Anna"}, {"", "Piotr"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToTable(pages, tt.m); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToTable() = %v, want %v", got, tt.want)
			}
		})
	}
	if got := ToTable(nil, nil); !reflect.DeepEqual(got, [][]string{{}}) {
		t.Errorf("ToTable(nil) = %v, want one empty header", got)
	}
}
