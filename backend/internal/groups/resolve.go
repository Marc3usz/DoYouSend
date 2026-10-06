package groups

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

// Contact issue types come from package recipients, which owns the contact
// data rules; the aliases keep this package's API (ADR-0007) unchanged.
type (
	Channel      = recipients.Channel
	IssueReason  = recipients.IssueReason
	ContactIssue = recipients.ContactIssue
)

const (
	ChannelEmail = recipients.ChannelEmail
	ChannelSMS   = recipients.ChannelSMS
	IssueMissing = recipients.IssueMissing
	IssueInvalid = recipients.IssueInvalid
)

// Resolved is one recipient of the final list.
type Resolved struct {
	Recipient recipients.Recipient
	// ViaGroupIDs lists the selected groups the recipient belongs to, in
	// selection order. It explains to the sender why someone is on the list.
	ViaGroupIDs []string
	// Direct is true when the recipient was also picked by hand.
	Direct bool
	// Issues lists the channels that cannot be used, in the order e-mail, SMS.
	// It is empty for a recipient reachable on both channels.
	Issues []ContactIssue
}

// CanReach reports whether c can be used for this recipient.
func (r Resolved) CanReach(c Channel) bool {
	for _, is := range r.Issues {
		if is.Channel == c {
			return false
		}
	}
	return true
}

// Channels returns the usable channels, in the order e-mail, SMS.
func (r Resolved) Channels() []Channel {
	var out []Channel
	for _, c := range []Channel{ChannelEmail, ChannelSMS} {
		if r.CanReach(c) {
			out = append(out, c)
		}
	}
	return out
}

// Partial reports whether exactly one channel is usable: such a delivery must
// be marked partial (batch_recipients.is_partial).
func (r Resolved) Partial() bool {
	return len(r.Channels()) == 1
}

// Unreachable reports whether no channel is usable at all.
func (r Resolved) Unreachable() bool {
	return len(r.Channels()) == 0
}

// Resolution is the outcome of expanding a Selection. Recipients is
// deduplicated by recipient ID and sorted by last name, first name, ID.
type Resolution struct {
	Recipients []Resolved
	// UnknownGroupIDs and UnknownRecipientIDs are selected IDs that do not
	// exist (e.g. deleted while the wizard was open). They do not fail the
	// resolution; the caller decides whether to warn or to refuse.
	UnknownGroupIDs     []string
	UnknownRecipientIDs []string
	// ExcludedIDs are the excluded recipients that the selection really
	// contained and that were therefore left out, in exclusion order.
	ExcludedIDs []string
	// MergedDuplicates counts picks collapsed into an earlier one: a person in
	// two selected groups and picked by hand adds 2 here. Excluded people do
	// not count.
	MergedDuplicates int
}

// WithIssues returns the recipients that miss at least one channel.
func (res Resolution) WithIssues() []Resolved {
	return res.filter(func(r Resolved) bool { return len(r.Issues) > 0 })
}

// Partial returns the recipients reachable on exactly one channel.
func (res Resolution) Partial() []Resolved {
	return res.filter(Resolved.Partial)
}

// Unreachable returns the recipients reachable on no channel.
func (res Resolution) Unreachable() []Resolved {
	return res.filter(Resolved.Unreachable)
}

func (res Resolution) filter(keep func(Resolved) bool) []Resolved {
	var out []Resolved
	for _, r := range res.Recipients {
		if keep(r) {
			out = append(out, r)
		}
	}
	return out
}

// Resolver expands a Selection into the final recipient list of a batch.
type Resolver struct {
	store Store
	dir   Directory
}

// NewResolver returns a Resolver reading custom groups from store and
// recipient data from dir.
func NewResolver(store Store, dir Directory) *Resolver {
	return &Resolver{store: store, dir: dir}
}

// Resolve normalizes and validates sel, expands every selected group, adds the
// people picked by hand, removes exclusions and returns each person once
// (CLAUDE.md rule 6). An error means the selection is malformed (ErrEmptySelection,
// ErrInvalidInput) or storage failed; unknown IDs are reported in the result.
func (rv *Resolver) Resolve(ctx context.Context, sel Selection) (Resolution, error) {
	sel = sel.Normalize()
	if err := sel.Validate(); err != nil {
		return Resolution{}, fmt.Errorf("resolve selection: %w", err)
	}

	var res Resolution
	b := newListBuilder()

	groups, unknown, err := rv.findGroups(ctx, sel.GroupIDs)
	if err != nil {
		return Resolution{}, err
	}
	res.UnknownGroupIDs = unknown

	members, err := rv.groupMembers(ctx, groups)
	if err != nil {
		return Resolution{}, err
	}

	// Fetch everyone referenced by ID in one call: custom group members and
	// the people picked by hand. Built-in groups already brought their data.
	var wanted []string
	for _, g := range groups {
		for _, id := range members[g.ID].ids {
			if !members.known(id) {
				wanted = append(wanted, id)
			}
		}
	}
	wanted = append(wanted, sel.RecipientIDs...)
	fetched, err := rv.recipientsByIDs(ctx, uniqueIDs(wanted))
	if err != nil {
		return Resolution{}, err
	}
	people := members.people()
	for id, r := range fetched {
		people[id] = r
	}

	for _, g := range groups {
		for _, id := range members[g.ID].ids {
			// A member missing from the directory was deleted between the two
			// reads; it is simply no longer a member.
			if r, ok := people[id]; ok {
				b.addViaGroup(r, g.ID)
			}
		}
	}
	for _, id := range sel.RecipientIDs {
		r, ok := people[id]
		if !ok {
			res.UnknownRecipientIDs = append(res.UnknownRecipientIDs, id)
			continue
		}
		b.addDirect(r)
	}

	res.ExcludedIDs = b.exclude(sel.ExcludedRecipientIDs)
	res.MergedDuplicates = b.merged
	res.Recipients = b.list()
	return res, nil
}

// findGroups looks up the selected groups, built-in ones from code and custom
// ones from the store, keeping selection order.
func (rv *Resolver) findGroups(ctx context.Context, ids []string) ([]Group, []string, error) {
	var customIDs []string
	for _, id := range ids {
		if _, ok := systemGroup(id); !ok {
			customIDs = append(customIDs, id)
		}
	}

	custom := make(map[string]Group, len(customIDs))
	if len(customIDs) > 0 {
		found, err := rv.store.FindGroups(ctx, customIDs)
		if err != nil {
			return nil, nil, fmt.Errorf("find %d selected groups: %w", len(customIDs), err)
		}
		for _, g := range found {
			custom[canonicalID(g.ID)] = g
		}
	}

	var groups []Group
	var unknown []string
	for _, id := range ids {
		if g, ok := systemGroup(id); ok {
			groups = append(groups, g)
			continue
		}
		g, ok := custom[id]
		if !ok {
			unknown = append(unknown, id)
			continue
		}
		g.ID = id
		groups = append(groups, g)
	}
	return groups, unknown, nil
}

// memberSet is the membership of one group: member IDs in a stable order and,
// for built-in groups, the recipients themselves.
type memberSet struct {
	ids    []string
	people map[string]recipients.Recipient
}

type membership map[string]memberSet

func (m membership) known(id string) bool {
	for _, set := range m {
		if _, ok := set.people[id]; ok {
			return true
		}
	}
	return false
}

func (m membership) people() map[string]recipients.Recipient {
	out := make(map[string]recipients.Recipient)
	for _, set := range m {
		for id, r := range set.people {
			out[id] = r
		}
	}
	return out
}

// groupMembers loads the members of every group: built-in groups through the
// directory (one query per recipient type), custom groups in one store call.
func (rv *Resolver) groupMembers(ctx context.Context, groups []Group) (membership, error) {
	out := make(membership, len(groups))
	byType := make(map[recipients.Type][]recipients.Recipient)

	var customIDs []string
	for _, g := range groups {
		if !g.IsSystem() {
			customIDs = append(customIDs, g.ID)
			continue
		}
		rs, ok := byType[g.Rule.Type]
		if !ok {
			var err error
			rs, err = rv.dir.RecipientsByType(ctx, g.Rule.Type)
			if err != nil {
				return nil, fmt.Errorf("list recipients of type %s for group %s: %w", g.Rule.Type, g.ID, err)
			}
			byType[g.Rule.Type] = rs
		}
		set := memberSet{people: make(map[string]recipients.Recipient, len(rs))}
		for _, r := range rs {
			if !g.Rule.Matches(r) {
				continue
			}
			id := canonicalID(r.ID)
			set.ids = append(set.ids, id)
			set.people[id] = r
		}
		out[g.ID] = set
	}

	if len(customIDs) == 0 {
		return out, nil
	}
	ids, err := rv.store.Members(ctx, customIDs)
	if err != nil {
		return nil, fmt.Errorf("list members of %d groups: %w", len(customIDs), err)
	}
	for groupID, memberIDs := range ids {
		out[canonicalID(groupID)] = memberSet{ids: uniqueIDs(memberIDs)}
	}
	return out, nil
}

func (rv *Resolver) recipientsByIDs(ctx context.Context, ids []string) (map[string]recipients.Recipient, error) {
	out := make(map[string]recipients.Recipient, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rs, err := rv.dir.RecipientsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("look up %d recipients: %w", len(ids), err)
	}
	for _, r := range rs {
		out[canonicalID(r.ID)] = r
	}
	return out, nil
}

// listBuilder collects picks and merges repeats of the same recipient ID.
type listBuilder struct {
	byID   map[string]*Resolved
	picks  map[string]int // how many times each ID was picked
	order  []string
	merged int
}

func newListBuilder() *listBuilder {
	return &listBuilder{byID: make(map[string]*Resolved), picks: make(map[string]int)}
}

func (b *listBuilder) entry(r recipients.Recipient) *Resolved {
	id := canonicalID(r.ID)
	b.picks[id]++
	if e, ok := b.byID[id]; ok {
		b.merged++
		return e
	}
	e := &Resolved{Recipient: r, Issues: r.ContactIssues()}
	b.byID[id] = e
	b.order = append(b.order, id)
	return e
}

func (b *listBuilder) addViaGroup(r recipients.Recipient, groupID string) {
	e := b.entry(r)
	if !slices.Contains(e.ViaGroupIDs, groupID) {
		e.ViaGroupIDs = append(e.ViaGroupIDs, groupID)
	}
}

func (b *listBuilder) addDirect(r recipients.Recipient) {
	b.entry(r).Direct = true
}

// exclude removes ids from the list and returns those that were on it. Picks
// of an excluded person no longer count as merged: nothing of them is left.
func (b *listBuilder) exclude(ids []string) []string {
	var removed []string
	for _, id := range ids {
		if _, ok := b.byID[id]; ok {
			delete(b.byID, id)
			b.merged -= b.picks[id] - 1
			removed = append(removed, id)
		}
	}
	return removed
}

func (b *listBuilder) list() []Resolved {
	out := make([]Resolved, 0, len(b.byID))
	for _, id := range b.order {
		if e, ok := b.byID[id]; ok {
			out = append(out, *e)
		}
	}
	slices.SortStableFunc(out, compareResolved)
	return out
}

// compareResolved orders by last name, first name, ID, ignoring letter case.
// It is byte order, not Polish collation ("Ł" sorts after "Z"); good enough
// for a stable list, and the UI can re-sort for display.
func compareResolved(a, b Resolved) int {
	return cmp.Or(
		strings.Compare(strings.ToLower(a.Recipient.LastName), strings.ToLower(b.Recipient.LastName)),
		strings.Compare(strings.ToLower(a.Recipient.FirstName), strings.ToLower(b.Recipient.FirstName)),
		strings.Compare(canonicalID(a.Recipient.ID), canonicalID(b.Recipient.ID)),
	)
}
