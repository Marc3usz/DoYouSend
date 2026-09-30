package groups

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// resolveFixture: "Rada rodzicow" = Jan, Lena; "Chor" = Jan, Oskar, plus a
// member (idNobody) that no longer exists in the directory.
func resolveFixture(t *testing.T) (*Resolver, *fakeDirectory, Group, Group) {
	t.Helper()
	store := NewMemStore()
	rada := mustCreate(t, store, "Rada rodziców", idJan, idLena)
	chor := mustCreate(t, store, "Chór", idJan, idOskar, idNobody)
	dir := newFakeDirectory(fixtureRecipients()...)
	return NewResolver(store, dir), dir, rada, chor
}

// summary is a compact view of a Resolution for table comparisons.
type summary struct {
	IDs              []string
	UnknownGroups    []string
	UnknownPeople    []string
	Excluded         []string
	MergedDuplicates int
}

func summarize(res Resolution) summary {
	s := summary{
		UnknownGroups:    res.UnknownGroupIDs,
		UnknownPeople:    res.UnknownRecipientIDs,
		Excluded:         res.ExcludedIDs,
		MergedDuplicates: res.MergedDuplicates,
	}
	s.IDs = ids(res.Recipients)
	return s
}

func ids(rs []Resolved) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Recipient.ID)
	}
	return out
}

func TestResolve(t *testing.T) {
	resolver, _, rada, chor := resolveFixture(t)

	tests := []struct {
		name string
		sel  Selection
		want summary
	}{
		{
			name: "built-in group, sorted by last name",
			sel:  Selection{GroupIDs: []string{AllParentsID}},
			want: summary{IDs: []string{idMaria, idJan, idZofia}},
		},
		{
			name: "overlapping groups and a hand pick give each person once",
			sel:  Selection{GroupIDs: []string{rada.ID, chor.ID, AllParentsID}, RecipientIDs: []string{idJan}},
			// Jan is picked four times (rada, chor, parents, by hand): 3 merged.
			want: summary{IDs: []string{idOskar, idMaria, idJan, idLena, idZofia}, MergedDuplicates: 3},
		},
		{
			name: "both built-in groups cover everyone",
			sel:  Selection{GroupIDs: []string{AllStudentsID, AllParentsID}},
			want: summary{IDs: []string{idOskar, idMaria, idJan, idLena, idZofia}},
		},
		{
			name: "hand picks only",
			sel:  Selection{RecipientIDs: []string{idZofia, idLena}},
			want: summary{IDs: []string{idLena, idZofia}},
		},
		{
			name: "exclusion wins over group and hand pick",
			sel: Selection{
				GroupIDs:             []string{rada.ID},
				RecipientIDs:         []string{idJan, idMaria},
				ExcludedRecipientIDs: []string{idJan, idZofia},
			},
			// Jan was picked twice, but he is excluded: nothing merged is left.
			want: summary{IDs: []string{idMaria, idLena}, Excluded: []string{idJan}},
		},
		{
			name: "only excluded people's repeats are not counted as merged",
			sel: Selection{
				GroupIDs:             []string{rada.ID, chor.ID},
				ExcludedRecipientIDs: []string{idOskar},
			},
			// Jan: rada + chor = 1 merged; Oskar excluded; idNobody gone.
			want: summary{IDs: []string{idJan, idLena}, Excluded: []string{idOskar}, MergedDuplicates: 1},
		},
		{
			name: "excluding everyone leaves an empty list",
			sel:  Selection{RecipientIDs: []string{idJan}, ExcludedRecipientIDs: []string{idJan}},
			want: summary{Excluded: []string{idJan}},
		},
		{
			name: "unknown IDs are reported, not fatal",
			sel:  Selection{GroupIDs: []string{idMissingGroup, rada.ID}, RecipientIDs: []string{idNobody, idZofia}},
			want: summary{
				IDs:           []string{idJan, idLena, idZofia},
				UnknownGroups: []string{idMissingGroup},
				UnknownPeople: []string{idNobody},
			},
		},
		{
			name: "member deleted from the directory is skipped silently",
			sel:  Selection{GroupIDs: []string{chor.ID}},
			want: summary{IDs: []string{idOskar, idJan}},
		},
		{
			name: "IDs differing only in case and spacing are one person",
			sel:  Selection{RecipientIDs: []string{idJan, strings.ToUpper(idJan), " " + idJan + " "}},
			want: summary{IDs: []string{idJan}},
		},
		{
			name: "group ID in upper case",
			sel:  Selection{GroupIDs: []string{strings.ToUpper(rada.ID)}},
			want: summary{IDs: []string{idJan, idLena}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := resolver.Resolve(context.Background(), tt.sel)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got := summarize(res); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Resolve() = %+v\nwant        %+v", got, tt.want)
			}
		})
	}
}

func TestResolveProvenance(t *testing.T) {
	resolver, _, rada, chor := resolveFixture(t)

	res, err := resolver.Resolve(context.Background(), Selection{
		GroupIDs:     []string{chor.ID, AllParentsID, rada.ID},
		RecipientIDs: []string{idJan, idZofia},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	type origin struct {
		Via    []string
		Direct bool
	}
	got := make(map[string]origin)
	for _, r := range res.Recipients {
		got[r.Recipient.ID] = origin{Via: r.ViaGroupIDs, Direct: r.Direct}
	}
	want := map[string]origin{
		idJan:   {Via: []string{chor.ID, AllParentsID, rada.ID}, Direct: true},
		idOskar: {Via: []string{chor.ID}},
		idMaria: {Via: []string{AllParentsID}},
		idZofia: {Via: []string{AllParentsID}, Direct: true},
		idLena:  {Via: []string{rada.ID}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("provenance = %+v\nwant         %+v", got, want)
	}
}

func TestResolveContactIssues(t *testing.T) {
	resolver, _, _, _ := resolveFixture(t)

	res, err := resolver.Resolve(context.Background(), Selection{GroupIDs: []string{AllParentsID, AllStudentsID}})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	tests := []struct {
		id          string
		issues      []ContactIssue
		channels    []Channel
		partial     bool
		unreachable bool
	}{
		{id: idJan, channels: []Channel{ChannelEmail, ChannelSMS}},
		{id: idZofia, channels: []Channel{ChannelEmail, ChannelSMS}},
		{
			id:       idMaria,
			issues:   []ContactIssue{{Channel: ChannelSMS, Reason: IssueMissing}},
			channels: []Channel{ChannelEmail},
			partial:  true,
		},
		{
			id:       idLena,
			issues:   []ContactIssue{{Channel: ChannelEmail, Reason: IssueMissing}},
			channels: []Channel{ChannelSMS},
			partial:  true,
		},
		{
			// Stored values that fail validation are reported, never silently fixed.
			id: idOskar,
			issues: []ContactIssue{
				{Channel: ChannelEmail, Reason: IssueInvalid},
				{Channel: ChannelSMS, Reason: IssueInvalid},
			},
			unreachable: true,
		},
	}
	byID := make(map[string]Resolved)
	for _, r := range res.Recipients {
		byID[r.Recipient.ID] = r
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			r, ok := byID[tt.id]
			if !ok {
				t.Fatalf("recipient %s not resolved", tt.id)
			}
			if !reflect.DeepEqual(r.Issues, tt.issues) {
				t.Errorf("Issues = %+v, want %+v", r.Issues, tt.issues)
			}
			if got := r.Channels(); !reflect.DeepEqual(got, tt.channels) {
				t.Errorf("Channels() = %v, want %v", got, tt.channels)
			}
			if r.Partial() != tt.partial || r.Unreachable() != tt.unreachable {
				t.Errorf("Partial() = %v, Unreachable() = %v, want %v, %v", r.Partial(), r.Unreachable(), tt.partial, tt.unreachable)
			}
		})
	}

	if got := ids(res.WithIssues()); !reflect.DeepEqual(got, []string{idOskar, idMaria, idLena}) {
		t.Errorf("WithIssues() = %v", got)
	}
	if got := ids(res.Partial()); !reflect.DeepEqual(got, []string{idMaria, idLena}) {
		t.Errorf("Partial() = %v", got)
	}
	if got := ids(res.Unreachable()); !reflect.DeepEqual(got, []string{idOskar}) {
		t.Errorf("Unreachable() = %v", got)
	}
}

func TestResolveQueriesOnce(t *testing.T) {
	resolver, dir, rada, chor := resolveFixture(t)

	_, err := resolver.Resolve(context.Background(), Selection{
		GroupIDs:     []string{rada.ID, chor.ID, AllParentsID, AllStudentsID},
		RecipientIDs: []string{idMaria, idZofia},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if dir.byIDCalls != 1 {
		t.Errorf("RecipientsByIDs called %d times, want 1", dir.byIDCalls)
	}
	for typ, n := range dir.byTypeCalls {
		if n != 1 {
			t.Errorf("RecipientsByType(%s) called %d times, want 1", typ, n)
		}
	}
}

func TestResolveRejectsBadSelection(t *testing.T) {
	resolver, _, _, _ := resolveFixture(t)

	tests := []struct {
		name string
		sel  Selection
		want error
	}{
		{name: "nothing picked", sel: Selection{}, want: ErrEmptySelection},
		{name: "only exclusions", sel: Selection{ExcludedRecipientIDs: []string{idJan}}, want: ErrEmptySelection},
		{name: "only blanks", sel: Selection{GroupIDs: []string{" ", ""}}, want: ErrEmptySelection},
		{name: "malformed group ID", sel: Selection{GroupIDs: []string{"3A"}}, want: ErrInvalidInput},
		{name: "malformed exclusion", sel: Selection{RecipientIDs: []string{idJan}, ExcludedRecipientIDs: []string{"jan"}}, want: ErrInvalidInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolver.Resolve(context.Background(), tt.sel)
			if !errors.Is(err, tt.want) {
				t.Errorf("Resolve() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestResolveStorageErrors(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		dirFails bool
		sel      func(custom Group) Selection
	}{
		{name: "find groups", method: "FindGroups", sel: func(g Group) Selection { return Selection{GroupIDs: []string{g.ID}} }},
		{name: "members", method: "Members", sel: func(g Group) Selection { return Selection{GroupIDs: []string{g.ID}} }},
		{name: "directory by type", dirFails: true, sel: func(Group) Selection { return Selection{GroupIDs: []string{AllParentsID}} }},
		{name: "directory by ID", dirFails: true, sel: func(Group) Selection { return Selection{RecipientIDs: []string{idJan}} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mem := NewMemStore()
			g := mustCreate(t, mem, "Rada rodziców", idJan)
			dir := newFakeDirectory(fixtureRecipients()...)
			if tt.dirFails {
				dir.err = errBoom
			}
			resolver := NewResolver(failingStore{Store: mem, method: tt.method, err: errBoom}, dir)

			if _, err := resolver.Resolve(context.Background(), tt.sel(g)); !errors.Is(err, errBoom) {
				t.Errorf("Resolve() error = %v, want wrapped %v", err, errBoom)
			}
		})
	}
}
