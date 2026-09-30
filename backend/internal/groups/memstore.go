package groups

import (
	"context"
	"crypto/rand"
	"fmt"
	"slices"
	"sync"
	"time"
)

// MemStore is a Store kept in memory. It lets the API and the UI work with
// groups before the Postgres store lands, and backs the tests of this package.
// It is safe for concurrent use. Data is lost when the process exits.
type MemStore struct {
	mu      sync.RWMutex
	groups  map[string]Group
	members map[string][]string // group ID -> member IDs, in insertion order
	now     func() time.Time
}

// NewMemStore returns an empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{
		groups:  make(map[string]Group),
		members: make(map[string][]string),
		now:     time.Now,
	}
}

// ListGroups implements Store.
func (s *MemStore) ListGroups(_ context.Context) ([]Group, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Group, 0, len(s.groups))
	for _, g := range s.groups {
		out = append(out, g)
	}
	return out, nil
}

// FindGroups implements Store.
func (s *MemStore) FindGroups(_ context.Context, ids []string) ([]Group, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []Group
	for _, id := range uniqueIDs(ids) {
		if g, ok := s.groups[id]; ok {
			out = append(out, g)
		}
	}
	return out, nil
}

// CreateGroup implements Store.
func (s *MemStore) CreateGroup(_ context.Context, g Group) (Group, error) {
	id, err := newID()
	if err != nil {
		return Group{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.nameTakenLocked(g.Name, "") {
		return Group{}, fmt.Errorf("group name %q: %w", g.Name, ErrNameTaken)
	}
	g.ID = id
	g.Kind = KindCustom
	g.Rule = nil
	g.CreatedAt = s.now().UTC()
	s.groups[id] = g
	return g, nil
}

// UpdateGroup implements Store.
func (s *MemStore) UpdateGroup(_ context.Context, g Group) error {
	id := canonicalID(g.ID)

	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.groups[id]
	if !ok {
		return fmt.Errorf("group %s: %w", id, ErrNotFound)
	}
	if s.nameTakenLocked(g.Name, id) {
		return fmt.Errorf("group name %q: %w", g.Name, ErrNameTaken)
	}
	stored.Name, stored.Description = g.Name, g.Description
	s.groups[id] = stored
	return nil
}

// DeleteGroup implements Store.
func (s *MemStore) DeleteGroup(_ context.Context, id string) error {
	id = canonicalID(id)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.groups[id]; !ok {
		return fmt.Errorf("group %s: %w", id, ErrNotFound)
	}
	delete(s.groups, id)
	delete(s.members, id)
	return nil
}

// Members implements Store.
func (s *MemStore) Members(_ context.Context, groupIDs []string) (map[string][]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[string][]string)
	for _, id := range uniqueIDs(groupIDs) {
		if ms := s.members[id]; len(ms) > 0 {
			out[id] = slices.Clone(ms)
		}
	}
	return out, nil
}

// AddMembers implements Store.
func (s *MemStore) AddMembers(_ context.Context, groupID string, recipientIDs []string) (int, error) {
	groupID = canonicalID(groupID)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.groups[groupID]; !ok {
		return 0, fmt.Errorf("group %s: %w", groupID, ErrNotFound)
	}
	added := 0
	for _, id := range uniqueIDs(recipientIDs) {
		if !slices.Contains(s.members[groupID], id) {
			s.members[groupID] = append(s.members[groupID], id)
			added++
		}
	}
	return added, nil
}

// RemoveMembers implements Store.
func (s *MemStore) RemoveMembers(_ context.Context, groupID string, recipientIDs []string) (int, error) {
	groupID = canonicalID(groupID)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.groups[groupID]; !ok {
		return 0, fmt.Errorf("group %s: %w", groupID, ErrNotFound)
	}
	remove := make(map[string]bool)
	for _, id := range uniqueIDs(recipientIDs) {
		remove[id] = true
	}
	before := len(s.members[groupID])
	s.members[groupID] = slices.DeleteFunc(s.members[groupID], func(id string) bool { return remove[id] })
	return before - len(s.members[groupID]), nil
}

// nameTakenLocked reports whether another group than exceptID uses name.
// The caller holds s.mu.
func (s *MemStore) nameTakenLocked(name, exceptID string) bool {
	key := nameKey(name)
	for id, g := range s.groups {
		if id != exceptID && nameKey(g.Name) == key {
			return true
		}
	}
	return false
}

// newID returns a random (version 4) UUID, the same kind Postgres
// gen_random_uuid() produces.
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate group id: %w", err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
