package groups

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestMemStoreCreate(t *testing.T) {
	s := NewMemStore()
	ctx := context.Background()

	a, err := s.CreateGroup(ctx, Group{Name: "Chór", Kind: KindSystem, Rule: &SystemRule{}})
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}
	if a.Kind != KindCustom || a.Rule != nil {
		t.Errorf("CreateGroup() = %+v, want a custom group without a rule", a)
	}
	b := mustCreate(t, s, "Biblioteka")
	if a.ID == b.ID {
		t.Errorf("two groups got the same ID %s", a.ID)
	}
	if _, err := s.CreateGroup(ctx, Group{Name: "CHÓR"}); !errors.Is(err, ErrNameTaken) {
		t.Errorf("CreateGroup(duplicate name) error = %v, want ErrNameTaken", err)
	}
}

func TestMemStoreUpdate(t *testing.T) {
	s := NewMemStore()
	ctx := context.Background()
	a := mustCreate(t, s, "Chór")
	mustCreate(t, s, "Biblioteka")

	tests := []struct {
		name    string
		g       Group
		wantErr error
	}{
		{name: "rename", g: Group{ID: a.ID, Name: "Chór szkolny", Description: "Wtorki"}},
		{name: "taken", g: Group{ID: a.ID, Name: "biblioteka"}, wantErr: ErrNameTaken},
		{name: "unknown", g: Group{ID: idMissingGroup, Name: "X"}, wantErr: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := s.UpdateGroup(ctx, tt.g); !errors.Is(err, tt.wantErr) {
				t.Errorf("UpdateGroup() error = %v, want %v", err, tt.wantErr)
			}
		})
	}

	found, _ := s.FindGroups(ctx, []string{a.ID})
	if len(found) != 1 || found[0].Name != "Chór szkolny" || found[0].CreatedAt != a.CreatedAt {
		t.Errorf("stored group = %+v, want renamed with CreatedAt kept", found)
	}
}

func TestMemStoreMembership(t *testing.T) {
	s := NewMemStore()
	ctx := context.Background()
	g := mustCreate(t, s, "Chór")

	if n, err := s.AddMembers(ctx, g.ID, []string{idJan, idMaria, idJan}); err != nil || n != 2 {
		t.Fatalf("AddMembers() = %d, %v, want 2, nil", n, err)
	}
	if n, err := s.AddMembers(ctx, g.ID, []string{idMaria, idLena}); err != nil || n != 1 {
		t.Fatalf("AddMembers(again) = %d, %v, want 1, nil", n, err)
	}
	if n, err := s.RemoveMembers(ctx, g.ID, []string{idJan, idNobody}); err != nil || n != 1 {
		t.Fatalf("RemoveMembers() = %d, %v, want 1, nil", n, err)
	}

	members, _ := s.Members(ctx, []string{g.ID, idMissingGroup})
	want := map[string][]string{g.ID: {idMaria, idLena}}
	if !reflect.DeepEqual(members, want) {
		t.Errorf("Members() = %v, want %v", members, want)
	}

	// The returned slice is a copy: changing it must not change the store.
	members[g.ID][0] = idNobody
	again, _ := s.Members(ctx, []string{g.ID})
	if again[g.ID][0] != idMaria {
		t.Errorf("Members() leaked internal state: %v", again)
	}

	for _, err := range []error{
		func() error { _, err := s.AddMembers(ctx, idMissingGroup, []string{idJan}); return err }(),
		func() error { _, err := s.RemoveMembers(ctx, idMissingGroup, []string{idJan}); return err }(),
	} {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("membership change on unknown group: error = %v, want ErrNotFound", err)
		}
	}
}

func TestMemStoreDeleteDropsMembers(t *testing.T) {
	s := NewMemStore()
	ctx := context.Background()
	g := mustCreate(t, s, "Chór", idJan)

	if err := s.DeleteGroup(ctx, g.ID); err != nil {
		t.Fatalf("DeleteGroup() error = %v", err)
	}
	if members, _ := s.Members(ctx, []string{g.ID}); len(members) != 0 {
		t.Errorf("Members() after delete = %v, want none", members)
	}
	if list, _ := s.ListGroups(ctx); len(list) != 0 {
		t.Errorf("ListGroups() after delete = %v, want none", list)
	}
	// Recreating a group with the name of a deleted one is allowed.
	mustCreate(t, s, "Chór")
}

func TestMemStoreConcurrentUse(t *testing.T) {
	s := NewMemStore()
	ctx := context.Background()
	g := mustCreate(t, s, "Chór")

	var wg sync.WaitGroup
	for _, id := range []string{idJan, idMaria, idLena, idOskar, idZofia} {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = s.AddMembers(ctx, g.ID, []string{id})
		}()
		go func() {
			defer wg.Done()
			_, _ = s.Members(ctx, []string{g.ID})
		}()
	}
	wg.Wait()

	members, _ := s.Members(ctx, []string{g.ID})
	if len(members[g.ID]) != 5 {
		t.Errorf("members = %v, want 5", members[g.ID])
	}
}

func TestNewID(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		id, err := newID()
		if err != nil {
			t.Fatalf("newID() error = %v", err)
		}
		if !IsValidID(id) || id[14] != '4' || !strings.ContainsRune("89ab", rune(id[19])) {
			t.Fatalf("newID() = %q, want a version 4 UUID", id)
		}
		if seen[id] {
			t.Fatalf("newID() repeated %q", id)
		}
		seen[id] = true
	}
}
