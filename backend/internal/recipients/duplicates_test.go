package recipients

import "testing"

func TestFindDuplicates(t *testing.T) {
	jan := Recipient{FirstName: "Jan", LastName: "Kowalski", Email: "jan@example.test", Phone: "+48500100101", Type: TypeParent}
	janUpper := Recipient{FirstName: "Jan", LastName: "K.", Email: "JAN@Example.test", Type: TypeParent}
	anna := Recipient{FirstName: "Anna", LastName: "Nowak", Email: "anna@example.test", Phone: "+48500100101", Type: TypeStudent}
	marek := Recipient{FirstName: "Marek", LastName: "Zielinski", Email: "marek@example.test", Phone: "+48500100102", Type: TypeParent}

	dups := FindDuplicates([]Recipient{jan, janUpper, anna, marek})

	if len(dups) != 2 {
		t.Fatalf("got %d duplicate groups, want 2 (email + phone): %+v", len(dups), dups)
	}

	var emailGroup, phoneGroup *Duplicate
	for i := range dups {
		switch dups[i].Key {
		case "jan@example.test":
			emailGroup = &dups[i]
		case "+48500100101":
			phoneGroup = &dups[i]
		}
	}

	if emailGroup == nil || len(emailGroup.Recipients) != 2 {
		t.Errorf("expected email collision on jan@example.test with 2 recipients, got %+v", emailGroup)
	}
	if phoneGroup == nil || len(phoneGroup.Recipients) != 2 {
		t.Errorf("expected phone collision on +48500100101 with 2 recipients, got %+v", phoneGroup)
	}
}

func TestFindDuplicatesNoCollisions(t *testing.T) {
	recipients := []Recipient{
		{FirstName: "Jan", LastName: "Kowalski", Email: "jan@example.test", Type: TypeParent},
		{FirstName: "Anna", LastName: "Nowak", Email: "anna@example.test", Type: TypeStudent},
	}
	if dups := FindDuplicates(recipients); len(dups) != 0 {
		t.Errorf("got %d duplicate groups, want 0: %+v", len(dups), dups)
	}
}

func TestFindDuplicatesIgnoresEmptyFields(t *testing.T) {
	recipients := []Recipient{
		{FirstName: "Jan", LastName: "Kowalski", Phone: "+48500100101", Type: TypeParent},
		{FirstName: "Anna", LastName: "Nowak", Phone: "+48500100102", Type: TypeStudent},
	}
	if dups := FindDuplicates(recipients); len(dups) != 0 {
		t.Errorf("recipients with no e-mail should never collide on empty string, got %+v", dups)
	}
}
