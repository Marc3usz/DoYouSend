package groups

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

func newTestService() (*Service, *MemStore, *fakeDirectory) {
	store := NewMemStore()
	dir := newFakeDirectory(fixtureRecipients()...)
	return NewService(store, dir), store, dir
}

func TestServiceCreate(t *testing.T) {
	tests := []struct {
		name     string
		existing []string
		in       GroupInput
		wantName string
		wantErr  error
	}{
		{name: "normalizes name and description", in: GroupInput{Name: "  Rada   rodziców ", Description: " Spotkania raz w miesiącu \n"}, wantName: "Rada rodziców"},
		{name: "empty name", in: GroupInput{Name: "   "}, wantErr: ErrInvalidInput},
		{name: "name of a built-in group, other case", in: GroupInput{Name: "wszyscy RODZICE"}, wantErr: ErrNameTaken},
		{name: "name of a custom group, other spacing", existing: []string{"Chór"}, in: GroupInput{Name: " chór "}, wantErr: ErrNameTaken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store, _ := newTestService()
			for _, name := range tt.existing {
				mustCreate(t, store, name)
			}

			g, err := svc.Create(context.Background(), tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Create() error = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if g.Name != tt.wantName || g.Kind != KindCustom || !IsValidID(g.ID) || g.CreatedAt.IsZero() {
				t.Errorf("Create() = %+v, want custom group %q with ID and CreatedAt", g, tt.wantName)
			}
			if g.Description != strings.TrimSpace(tt.in.Description) {
				t.Errorf("Description = %q, want trimmed", g.Description)
			}
		})
	}
}

func TestServiceCreateReportsAllFieldErrors(t *testing.T) {
	svc, _, _ := newTestService()

	_, err := svc.Create(context.Background(), GroupInput{Name: "", Description: strings.Repeat("x", MaxDescriptionLength+1)})
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("Create() error = %v, want *ValidationError", err)
	}
	var fields []string
	for _, f := range verr.Fields {
		fields = append(fields, f.Field)
	}
	if want := []string{"name", "description"}; !reflect.DeepEqual(fields, want) {
		t.Errorf("fields = %v, want %v", fields, want)
	}
}

func TestServiceUpdate(t *testing.T) {
	svc, store, _ := newTestService()
	chor := mustCreate(t, store, "Chór")
	mustCreate(t, store, "Rada rodziców")

	tests := []struct {
		name    string
		id      string
		in      GroupInput
		wantErr error
	}{
		{name: "rename", id: chor.ID, in: GroupInput{Name: "Chór szkolny", Description: "Próby we wtorki"}},
		{name: "keep own name in other case", id: chor.ID, in: GroupInput{Name: "CHÓR SZKOLNY"}},
		{name: "name of another group", id: chor.ID, in: GroupInput{Name: "rada rodziców"}, wantErr: ErrNameTaken},
		{name: "built-in group", id: AllParentsID, in: GroupInput{Name: "Rodzice"}, wantErr: ErrSystemGroup},
		{name: "unknown group", id: idMissingGroup, in: GroupInput{Name: "X"}, wantErr: ErrNotFound},
		{name: "malformed ID", id: "chor", in: GroupInput{Name: "X"}, wantErr: ErrInvalidInput},
		{name: "invalid input", id: chor.ID, in: GroupInput{Name: "a\x00b"}, wantErr: ErrInvalidInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := svc.Update(context.Background(), tt.id, tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Update() error = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			stored, err := svc.Get(context.Background(), tt.id)
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if stored.Name != tt.in.Name || stored.Description != tt.in.Description || g.Name != tt.in.Name {
				t.Errorf("stored %+v, returned %+v, want %+v", stored, g, tt.in)
			}
		})
	}
}

func TestServiceDelete(t *testing.T) {
	svc, store, _ := newTestService()
	chor := mustCreate(t, store, "Chór", idJan)

	if err := svc.Delete(context.Background(), AllStudentsID); !errors.Is(err, ErrSystemGroup) {
		t.Errorf("Delete(built-in) error = %v, want ErrSystemGroup", err)
	}
	if err := svc.Delete(context.Background(), strings.ToUpper(chor.ID)); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := svc.Get(context.Background(), chor.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(deleted) error = %v, want ErrNotFound", err)
	}
	if err := svc.Delete(context.Background(), chor.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete(twice) error = %v, want ErrNotFound", err)
	}
}

func TestServiceList(t *testing.T) {
	svc, store, _ := newTestService()
	mustCreate(t, store, "rada rodziców", idJan, idMaria)
	mustCreate(t, store, "Chór", idOskar)
	mustCreate(t, store, "Biblioteka")

	list, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	type row struct {
		Name    string
		Kind    Kind
		Members int
	}
	var got []row
	for _, g := range list {
		got = append(got, row{g.Name, g.Kind, g.MemberCount})
	}
	want := []row{
		{"Wszyscy rodzice", KindSystem, 3},
		{"Wszyscy uczniowie", KindSystem, 2},
		{"Biblioteka", KindCustom, 0},
		{"Chór", KindCustom, 1},
		{"rada rodziców", KindCustom, 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("List() = %+v\nwant     %+v", got, want)
	}
}

func TestServiceMembers(t *testing.T) {
	svc, store, _ := newTestService()
	rada := mustCreate(t, store, "Rada rodziców", idZofia, idJan, idNobody)
	empty := mustCreate(t, store, "Pusta")

	tests := []struct {
		name    string
		id      string
		want    []string
		wantErr error
	}{
		{name: "custom group, deleted member skipped, sorted", id: rada.ID, want: []string{idJan, idZofia}},
		{name: "empty group", id: empty.ID},
		{name: "built-in group", id: AllStudentsID, want: []string{idOskar, idLena}},
		{name: "unknown group", id: idMissingGroup, wantErr: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs, err := svc.Members(context.Background(), tt.id)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Members() error = %v, want %v", err, tt.wantErr)
			}
			if got := recipientIDs(rs); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Members() = %v, want %v", got, tt.want)
			}
		})
	}
}

func recipientIDs(rs []recipients.Recipient) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return out
}

func TestServiceAddMembers(t *testing.T) {
	tests := []struct {
		name        string
		group       string // "custom" or a literal ID
		ids         []string
		want        MembershipChange
		wantErr     error
		wantUnknown []string
		wantMembers []string
	}{
		{
			name:        "adds new, counts repeats as unchanged",
			group:       "custom",
			ids:         []string{idMaria, idJan, strings.ToUpper(idMaria)},
			want:        MembershipChange{Changed: 1, Unchanged: 1},
			wantMembers: []string{idJan, idMaria},
		},
		{
			name:        "unknown recipient: nothing is added",
			group:       "custom",
			ids:         []string{idMaria, idNobody},
			wantErr:     ErrUnknownRecipient,
			wantUnknown: []string{idNobody},
			wantMembers: []string{idJan},
		},
		{name: "no IDs", group: "custom", wantErr: ErrInvalidInput, wantMembers: []string{idJan}},
		{name: "malformed ID", group: "custom", ids: []string{"maria"}, wantErr: ErrInvalidInput, wantMembers: []string{idJan}},
		{name: "built-in group", group: AllParentsID, ids: []string{idMaria}, wantErr: ErrSystemGroup},
		{name: "unknown group", group: idMissingGroup, ids: []string{idMaria}, wantErr: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store, _ := newTestService()
			g := mustCreate(t, store, "Rada rodziców", idJan)
			id := tt.group
			if id == "custom" {
				id = g.ID
			}

			change, err := svc.AddMembers(context.Background(), id, tt.ids)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("AddMembers() error = %v, want %v", err, tt.wantErr)
			}
			if change != tt.want {
				t.Errorf("AddMembers() = %+v, want %+v", change, tt.want)
			}
			var unknown *UnknownRecipientsError
			if errors.As(err, &unknown) && !reflect.DeepEqual(unknown.IDs, tt.wantUnknown) {
				t.Errorf("unknown IDs = %v, want %v", unknown.IDs, tt.wantUnknown)
			}
			if tt.group == "custom" {
				members, _ := store.Members(context.Background(), []string{g.ID})
				if got := members[g.ID]; !reflect.DeepEqual(got, tt.wantMembers) {
					t.Errorf("members = %v, want %v", got, tt.wantMembers)
				}
			}
		})
	}
}

func TestServiceRemoveMembers(t *testing.T) {
	svc, store, _ := newTestService()
	g := mustCreate(t, store, "Rada rodziców", idJan, idMaria)

	// idNobody was never a member (or was deleted since): unchanged, not an error.
	change, err := svc.RemoveMembers(context.Background(), g.ID, []string{idMaria, idNobody})
	if err != nil {
		t.Fatalf("RemoveMembers() error = %v", err)
	}
	if want := (MembershipChange{Changed: 1, Unchanged: 1}); change != want {
		t.Errorf("RemoveMembers() = %+v, want %+v", change, want)
	}
	members, _ := store.Members(context.Background(), []string{g.ID})
	if got := members[g.ID]; !reflect.DeepEqual(got, []string{idJan}) {
		t.Errorf("members = %v, want [%s]", got, idJan)
	}

	if _, err := svc.RemoveMembers(context.Background(), AllParentsID, []string{idJan}); !errors.Is(err, ErrSystemGroup) {
		t.Errorf("RemoveMembers(built-in) error = %v, want ErrSystemGroup", err)
	}
}

func TestServiceStorageErrors(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		dirFails bool
		call     func(svc *Service, g Group) error
	}{
		{name: "list", method: "ListGroups", call: func(s *Service, _ Group) error { _, err := s.List(context.Background()); return err }},
		{name: "list members", method: "Members", call: func(s *Service, _ Group) error { _, err := s.List(context.Background()); return err }},
		{name: "list directory", dirFails: true, call: func(s *Service, _ Group) error { _, err := s.List(context.Background()); return err }},
		{name: "get", method: "FindGroups", call: func(s *Service, g Group) error { _, err := s.Get(context.Background(), g.ID); return err }},
		{name: "add members lookup", dirFails: true, call: func(s *Service, g Group) error {
			_, err := s.AddMembers(context.Background(), g.ID, []string{idJan})
			return err
		}},
		{name: "add members store", method: "AddMembers", call: func(s *Service, g Group) error {
			_, err := s.AddMembers(context.Background(), g.ID, []string{idJan})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mem := NewMemStore()
			g := mustCreate(t, mem, "Chór", idOskar)
			dir := newFakeDirectory(fixtureRecipients()...)
			if tt.dirFails {
				dir.err = errBoom
			}
			svc := NewService(failingStore{Store: mem, method: tt.method, err: errBoom}, dir)

			if err := tt.call(svc, g); !errors.Is(err, errBoom) {
				t.Errorf("error = %v, want wrapped %v", err, errBoom)
			}
		})
	}
}

func TestServiceInfo(t *testing.T) {
	svc, store, _ := newTestService()
	rada := mustCreate(t, store, "Rada rodziców", idJan, idMaria)

	tests := []struct {
		name    string
		id      string
		want    int
		wantErr error
	}{
		{name: "custom group", id: rada.ID, want: 2},
		{name: "built-in group", id: AllStudentsID, want: 2},
		{name: "unknown group", id: idMissingGroup, wantErr: ErrNotFound},
		{name: "malformed id", id: "nope", wantErr: ErrInvalidInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := svc.Info(context.Background(), tt.id)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Info() error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && g.MemberCount != tt.want {
				t.Errorf("MemberCount = %d, want %d", g.MemberCount, tt.want)
			}
		})
	}
}
