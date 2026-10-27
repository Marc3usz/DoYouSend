package delivery

import (
	"context"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/groups"
	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

// directory is a groups.Directory over fixed recipients.
type directory []recipients.Recipient

func (d directory) RecipientsByIDs(_ context.Context, ids []string) ([]recipients.Recipient, error) {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	var out []recipients.Recipient
	for _, r := range d {
		if want[r.ID] {
			out = append(out, r)
		}
	}
	return out, nil
}

func (d directory) RecipientsByType(_ context.Context, t recipients.Type) ([]recipients.Recipient, error) {
	var out []recipients.Recipient
	for _, r := range d {
		if r.Type == t {
			out = append(out, r)
		}
	}
	return out, nil
}

func (d directory) Classes(context.Context) ([]string, error) { return nil, nil }

// TestServiceSendWithRealResolver runs a batch through groups.Resolver instead of a
// fixed Resolution: the ADR-0007 contract between groups and delivery, end to end.
// NewPlan deduplicates again, so this guards the outcome (one message per person),
// not the resolver's own deduplication; groups/resolve_test.go covers that.
func TestServiceSendWithRealResolver(t *testing.T) {
	const idOla = "00000000-0000-4000-8000-000000000004"
	dir := directory{
		{ID: id1, FirstName: "Anna", LastName: "Nowak", Email: "anna@example.test", Phone: "+48500100101", Type: recipients.TypeParent},
		{ID: id2, FirstName: "Jan", LastName: "Kowalski", Phone: "+48500100102", Type: recipients.TypeParent},
		{ID: id3, FirstName: "Piotr", LastName: "Wiśniewski", Email: "piotr@example.test", Type: recipients.TypeParent},
		{ID: idOla, FirstName: "Ola", LastName: "Zielińska", Type: recipients.TypeParent},
	}
	ctx := context.Background()
	store := groups.NewMemStore()
	rada := createGroup(t, store, "Rada rodziców", id1, id2, idOla)
	chor := createGroup(t, store, "Chór", id2, id3)

	email := &scriptedSender{channel: providers.ChannelEmail}
	sms := &scriptedSender{channel: providers.ChannelSMS}
	d, _ := newTestDispatcher(t, DefaultRetryPolicy(), email, sms)
	svc := NewService(groups.NewResolver(store, dir), groups.NewService(store, dir), d, NewMemStore(), 65, nil)

	draft := validDraft()
	// Jan is in both groups and picked by hand; Piotr is excluded although Chór has him.
	draft.Selection = groups.Selection{
		GroupIDs:             []string{rada.ID, chor.ID},
		RecipientIDs:         []string{id2},
		ExcludedRecipientIDs: []string{id3},
	}
	got, err := svc.Send(ctx, draft)
	if err != nil {
		t.Fatal(err)
	}

	byID := make(map[string]PlannedRecipient)
	for _, r := range got.Plan.Recipients {
		if _, dup := byID[r.RecipientID]; dup {
			t.Errorf("recipient %s planned twice", r.RecipientID)
		}
		byID[r.RecipientID] = r
	}
	if len(byID) != 3 {
		t.Errorf("planned %d recipients, want Anna, Jan and Ola", len(byID))
	}
	if _, ok := byID[id3]; ok {
		t.Error("excluded recipient was planned")
	}
	if r := byID[id1]; r.Partial || r.Unreachable || len(r.Deliveries) != 2 {
		t.Errorf("Anna = %+v, want e-mail and SMS", r)
	}
	if r := byID[id2]; !r.Partial || len(r.Deliveries) != 1 || r.Deliveries[0].Channel != providers.ChannelSMS {
		t.Errorf("Jan = %+v, want a partial SMS-only recipient", r)
	}
	if r := byID[idOla]; !r.Unreachable || len(r.Deliveries) != 0 {
		t.Errorf("Ola = %+v, want unreachable with no deliveries", r)
	}

	if len(email.sent) != 1 || len(sms.sent) != 2 {
		t.Fatalf("sent %d e-mails and %d SMS, want 1 and 2", len(email.sent), len(sms.sent))
	}
	for _, m := range sms.sent {
		if m.RecipientID == id1 && m.Body != email.sent[0].Body {
			t.Errorf("Anna's SMS body %q differs from her e-mail body %q", m.Body, email.sent[0].Body)
		}
	}
}

func createGroup(t *testing.T, store *groups.MemStore, name string, members ...string) groups.Group {
	t.Helper()
	g, err := store.CreateGroup(context.Background(), groups.Group{Name: name})
	if err != nil {
		t.Fatalf("CreateGroup(%q): %v", name, err)
	}
	if _, err := store.AddMembers(context.Background(), g.ID, members); err != nil {
		t.Fatalf("AddMembers(%q): %v", name, err)
	}
	return g
}
