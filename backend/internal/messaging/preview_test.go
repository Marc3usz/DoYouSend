package messaging

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/groups"
	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

// Fictional recipients (CLAUDE.md rule 2).
const (
	idAnna   = "33333333-3333-4333-8333-000000000001"
	idLukasz = "33333333-3333-4333-8333-000000000002"
	idNoName = "33333333-3333-4333-8333-000000000003"
	idGroup  = "44444444-4444-4444-8444-000000000001"
)

var (
	anna = groups.Resolved{Recipient: recipients.Recipient{
		ID: idAnna, FirstName: "Anna", LastName: "Nowak",
		Email: "anna.nowak@example.test", Phone: "+48500100101",
	}}
	// Łukasz has no phone: e-mail only, a partial send.
	lukasz = groups.Resolved{
		Recipient: recipients.Recipient{ID: idLukasz, FirstName: "Łukasz", LastName: "Wiśniewski", Email: "lukasz.w@example.test"},
		Issues:    []groups.ContactIssue{{Channel: groups.ChannelSMS, Reason: groups.IssueMissing}},
	}
	noName = groups.Resolved{Recipient: recipients.Recipient{ID: idNoName, LastName: "Bezimienna", Phone: "+48500100103"},
		Issues: []groups.ContactIssue{{Channel: groups.ChannelEmail, Reason: groups.IssueMissing}},
	}
)

// fakeResolver returns a fixed resolution and records what it was asked for.
type fakeResolver struct {
	res   groups.Resolution
	err   error
	calls []groups.Selection
}

func (f *fakeResolver) Resolve(_ context.Context, sel groups.Selection) (groups.Resolution, error) {
	f.calls = append(f.calls, sel)
	return f.res, f.err
}

func TestPreview(t *testing.T) {
	picked := groups.Selection{GroupIDs: []string{idGroup}}
	tests := []struct {
		name         string
		body         string
		sel          groups.Selection
		list         []groups.Resolved
		want         Estimate
		wantResolved bool
	}{
		{
			name: "empty selection measures only the template",
			body: "Jutro zebranie.",
			want: Estimate{Template: SMSLength{Encoding: EncodingGSM7, Units: 15, Parts: 1}},
		},
		{
			name: "blank ids count as an empty selection",
			body: "Jutro zebranie.",
			sel:  groups.Selection{GroupIDs: []string{"  "}, ExcludedRecipientIDs: []string{idAnna}},
			want: Estimate{Template: SMSLength{Encoding: EncodingGSM7, Units: 15, Parts: 1}},
		},
		{
			name:         "personalised body is measured per recipient",
			body:         "Witaj {{imie}}",
			sel:          picked,
			list:         []groups.Resolved{anna, lukasz},
			wantResolved: true,
			want: Estimate{
				Template:     SMSLength{Encoding: EncodingGSM7, Units: 18, Parts: 1},
				Placeholders: []string{"imie"},
				Recipients:   2,
				Summary: Summary{
					Recipients: 2, EmailCount: 2, SMSCount: 1, PartialCount: 1,
					MinPartsPerRecipient: 1, MaxPartsPerRecipient: 1, TotalParts: 1, CostMilli: 80,
				},
			},
		},
		{
			name:         "missing value fails only that recipient",
			body:         "Witaj {{imie}}",
			sel:          picked,
			list:         []groups.Resolved{anna, noName},
			wantResolved: true,
			want: Estimate{
				Template:     SMSLength{Encoding: EncodingGSM7, Units: 18, Parts: 1},
				Placeholders: []string{"imie"},
				Recipients:   2,
				Summary: Summary{
					Recipients: 1, EmailCount: 1, SMSCount: 1,
					MinPartsPerRecipient: 1, MaxPartsPerRecipient: 1, TotalParts: 1, CostMilli: 80,
				},
				RenderFailedIDs: []string{idNoName},
			},
		},
		{
			name:         "unknown placeholder renders nothing",
			body:         "{{klasa}}",
			sel:          picked,
			list:         []groups.Resolved{anna},
			wantResolved: true,
			want: Estimate{
				Template:            SMSLength{Encoding: EncodingGSM7, Units: 13, Parts: 1},
				Placeholders:        []string{"klasa"},
				UnknownPlaceholders: []string{"klasa"},
				Recipients:          1,
			},
		},
		{
			name:         "empty body renders nothing",
			sel:          picked,
			list:         []groups.Resolved{anna},
			wantResolved: true,
			want:         Estimate{Template: SMSLength{Encoding: EncodingGSM7}, Recipients: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rv := &fakeResolver{res: groups.Resolution{Recipients: tt.list}}
			got, err := NewPreviewer(rv, 80).Preview(context.Background(), tt.body, tt.sel)
			if err != nil {
				t.Fatalf("Preview: %v", err)
			}
			assertEstimate(t, got, tt.want)
			if resolved := len(rv.calls) > 0; resolved != tt.wantResolved {
				t.Errorf("resolver called = %v, want %v", resolved, tt.wantResolved)
			}
		})
	}
}

func TestPreviewPassesNormalizedSelection(t *testing.T) {
	rv := &fakeResolver{}
	sel := groups.Selection{RecipientIDs: []string{" " + idAnna, idAnna}}
	if _, err := NewPreviewer(rv, 0).Preview(context.Background(), "x", sel); err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if len(rv.calls) != 1 || !slices.Equal(rv.calls[0].RecipientIDs, []string{idAnna}) {
		t.Errorf("resolver got %+v, want one call with [%s]", rv.calls, idAnna)
	}
}

func TestPreviewResolveError(t *testing.T) {
	rv := &fakeResolver{err: groups.ErrInvalidInput}
	_, err := NewPreviewer(rv, 0).Preview(context.Background(), "x", groups.Selection{GroupIDs: []string{"zly"}})
	if !errors.Is(err, groups.ErrInvalidInput) {
		t.Errorf("Preview error = %v, want groups.ErrInvalidInput", err)
	}
}

func assertEstimate(t *testing.T, got, want Estimate) {
	t.Helper()
	if got.Template != want.Template {
		t.Errorf("Template = %+v, want %+v", got.Template, want.Template)
	}
	if !slices.Equal(got.Placeholders, want.Placeholders) {
		t.Errorf("Placeholders = %q, want %q", got.Placeholders, want.Placeholders)
	}
	if !slices.Equal(got.UnknownPlaceholders, want.UnknownPlaceholders) {
		t.Errorf("UnknownPlaceholders = %q, want %q", got.UnknownPlaceholders, want.UnknownPlaceholders)
	}
	if got.Recipients != want.Recipients {
		t.Errorf("Recipients = %d, want %d", got.Recipients, want.Recipients)
	}
	if got.Summary != want.Summary {
		t.Errorf("Summary = %+v, want %+v", got.Summary, want.Summary)
	}
	if !slices.Equal(got.RenderFailedIDs, want.RenderFailedIDs) {
		t.Errorf("RenderFailedIDs = %q, want %q", got.RenderFailedIDs, want.RenderFailedIDs)
	}
}
