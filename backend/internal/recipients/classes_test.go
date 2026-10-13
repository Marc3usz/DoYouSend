package recipients

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"testing"
)

func TestNormalizeClasses(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "none", in: nil, want: nil},
		{name: "blanks only", in: []string{"", "  "}, want: nil},
		{name: "spaces removed, upper-cased", in: []string{" 3 a ", "1ti"}, want: []string{"1TI", "3A"}},
		{name: "repeats dropped after normalizing", in: []string{"3A", "3a", " 3 A"}, want: []string{"3A"}},
		{name: "year compared as a number", in: []string{"10A", "2B", "2A", "1C"}, want: []string{"1C", "2A", "2B", "10A"}},
		{name: "invalid names kept for Validate", in: []string{"A3", "3A"}, want: []string{"A3", "3A"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeClasses(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NormalizeClasses(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseClassList(t *testing.T) {
	if got, want := ParseClassList(" 3a, 1 b ,,3A"), []string{"1B", "3A"}; !slices.Equal(got, want) {
		t.Errorf("ParseClassList() = %q, want %q", got, want)
	}
	if got := ParseClassList(""); got != nil {
		t.Errorf("ParseClassList(\"\") = %q, want nil", got)
	}
}

func TestValidClass(t *testing.T) {
	for _, c := range []string{"1", "3A", "1TI", "8ABCDE"} {
		if !ValidClass(c) {
			t.Errorf("ValidClass(%q) = false, want true", c)
		}
	}
	for _, c := range []string{"", "A3", "3a", "3-A", "3ABCDEF", "3Ą"} {
		if ValidClass(c) {
			t.Errorf("ValidClass(%q) = true, want false", c)
		}
	}
}

func TestValidateClasses(t *testing.T) {
	base := Recipient{FirstName: "Lena", LastName: "Nowak", Phone: "+48500100103"}
	tests := []struct {
		name    string
		typ     Type
		classes []string
		want    int // number of "classes" errors
	}{
		{name: "student without a class", typ: TypeStudent, want: 0},
		{name: "student with one class", typ: TypeStudent, classes: []string{"3A"}, want: 0},
		{name: "student with two classes", typ: TypeStudent, classes: []string{"1B", "3A"}, want: 1},
		{name: "parent of children in two classes", typ: TypeParent, classes: []string{"1B", "3A"}, want: 0},
		{name: "malformed name", typ: TypeParent, classes: []string{"A3"}, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := base
			r.Type, r.Classes = tt.typ, tt.classes
			got := 0
			for _, e := range r.Validate() {
				if e.Field == "classes" {
					got++
				} else {
					t.Errorf("unexpected error %v", e)
				}
			}
			if got != tt.want {
				t.Errorf("classes errors = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestServiceClasses(t *testing.T) {
	store := serviceFixture()
	svc := NewService(store)
	ctx := context.Background()
	lena := store.id("Lena")

	classes := []string{" 3 a "}
	got, err := svc.Update(ctx, lena, Input{FirstName: "Lena", LastName: "Nowak", Phone: "+48500100103", Type: TypeStudent, Classes: &classes})
	if err != nil {
		t.Fatalf("Update(classes) error = %v", err)
	}
	if !slices.Equal(got.Classes, []string{"3A"}) {
		t.Errorf("Classes = %q, want [3A]", got.Classes)
	}

	// Omitted classes keep the stored ones.
	got, err = svc.Update(ctx, lena, Input{FirstName: "Lena", LastName: "Nowakowska", Phone: "+48500100103", Type: TypeStudent})
	if err != nil {
		t.Fatalf("Update(no classes) error = %v", err)
	}
	if !slices.Equal(got.Classes, []string{"3A"}) {
		t.Errorf("Classes after update without classes = %q, want [3A] kept", got.Classes)
	}

	// An empty list clears them.
	none := []string{}
	if got, err = svc.Update(ctx, lena, Input{FirstName: "Lena", LastName: "Nowak", Phone: "+48500100103", Type: TypeStudent, Classes: &none}); err != nil || got.Classes != nil {
		t.Errorf("Update(empty classes) = %q, %v, want no classes", got.Classes, err)
	}

	two := []string{"3A", "1B"}
	_, err = svc.Update(ctx, lena, Input{FirstName: "Lena", LastName: "Nowak", Phone: "+48500100103", Type: TypeStudent, Classes: &two})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Fields[0].Field != "classes" {
		t.Errorf("Update(student in two classes) error = %v, want a classes field error", err)
	}

	page, err := svc.List(ctx, ListQuery{Filter: Filter{Class: "3 a"}})
	if err != nil {
		t.Fatalf("List(class) error = %v", err)
	}
	if page.Total != 0 {
		t.Errorf("List(class 3A) total = %d, want 0 after the class was cleared", page.Total)
	}
	if _, err := svc.List(ctx, ListQuery{Filter: Filter{Class: "A3"}}); !errors.Is(err, ErrInvalidParams) {
		t.Errorf("List(bad class) error = %v, want ErrInvalidParams", err)
	}
}

func TestHandlerClasses(t *testing.T) {
	store := serviceFixture()
	mux := crudServer(store)

	rec, created := do(t, mux, http.MethodPost, "/api/recipients",
		`{"firstName":"Ola","lastName":"Wrona","phone":"500 100 130","type":"parent","classes":["3a"," 1 b"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %v", rec.Code, created)
	}
	if got := created["classes"]; !reflect.DeepEqual(got, []any{"1B", "3A"}) {
		t.Errorf("classes = %v, want [1B 3A]", got)
	}

	rec, page := do(t, mux, http.MethodGet, "/api/recipients?class=1b", "")
	if items, _ := page["items"].([]any); rec.Code != http.StatusOK || len(items) != 1 {
		t.Errorf("list?class=1b = %d %v, want Ola only", rec.Code, page)
	}

	rec, listed := do(t, mux, http.MethodGet, "/api/recipients?type=student", "")
	items, _ := listed["items"].([]any)
	if rec.Code != http.StatusOK || len(items) == 0 {
		t.Fatalf("list students = %d %v", rec.Code, listed)
	}
	if first, _ := items[0].(map[string]any); !reflect.DeepEqual(first["classes"], []any{}) {
		t.Errorf("recipient without classes has classes = %v, want []", first["classes"])
	}

	rec, body := do(t, mux, http.MethodGet, "/api/recipients?class=klasa", "")
	if rec.Code != http.StatusBadRequest || body["code"] != "invalid_request" {
		t.Errorf("list?class=klasa = %d %v, want 400 invalid_request", rec.Code, body)
	}
}
