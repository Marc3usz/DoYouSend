package messaging

import (
	"context"
	"errors"
	"fmt"

	"github.com/Marc3usz/DoYouSend/backend/internal/groups"
)

// SelectionResolver expands the sender's choice of groups and people into the final,
// deduplicated recipient list. *groups.Resolver satisfies it.
type SelectionResolver interface {
	Resolve(ctx context.Context, sel groups.Selection) (groups.Resolution, error)
}

// Estimate is everything the composer shows before the sender confirms a batch
// (openapi.yaml SendEstimate).
type Estimate struct {
	// Template measures the body exactly as typed, before placeholders are filled in.
	Template            SMSLength
	Placeholders        []string
	UnknownPlaceholders []string
	// Recipients is the size of the resolved list, whether or not the body renders.
	Recipients int
	// Summary covers the recipients whose body rendered; it is zero while the body is
	// empty or uses an unknown placeholder.
	Summary Summary
	// RenderFailedIDs are recipients lacking a value for a used placeholder.
	RenderFailedIDs []string
}

// Previewer builds the pre-send Estimate for a draft.
type Previewer struct {
	resolver          SelectionResolver
	pricePerPartMilli int64
}

// NewPreviewer returns a Previewer that prices every SMS part at pricePerPartMilli
// thousandths of a złoty (see ParsePrice).
func NewPreviewer(resolver SelectionResolver, pricePerPartMilli int64) *Previewer {
	return &Previewer{resolver: resolver, pricePerPartMilli: pricePerPartMilli}
}

// Preview measures body and, when sel picks anyone, renders it for every resolved
// recipient. The body is never altered. Problems with the content are reported in the
// Estimate; an error means the selection is malformed (groups.ErrInvalidInput) or
// resolving failed.
func (p *Previewer) Preview(ctx context.Context, body string, sel groups.Selection) (Estimate, error) {
	est := Estimate{
		Template:            MeasureSMS(body),
		Placeholders:        Placeholders(body),
		UnknownPlaceholders: UnknownPlaceholders(body),
	}

	sel = sel.Normalize()
	if len(sel.GroupIDs) == 0 && len(sel.RecipientIDs) == 0 {
		return est, nil
	}
	res, err := p.resolver.Resolve(ctx, sel)
	if err != nil {
		return Estimate{}, fmt.Errorf("preview message: %w", err)
	}
	est.Recipients = len(res.Recipients)
	if body == "" || len(est.UnknownPlaceholders) > 0 {
		return est, nil
	}

	msgs, failed, err := renderAll(body, res.Recipients)
	if err != nil {
		return Estimate{}, fmt.Errorf("preview message: %w", err)
	}
	est.RenderFailedIDs = failed
	est.Summary, err = Summarize(msgs, p.pricePerPartMilli)
	if err != nil {
		return Estimate{}, fmt.Errorf("preview message: %w", err)
	}
	return est, nil
}

// renderAll personalises body for each recipient. A recipient with an empty value for a
// used placeholder is reported in failed and left out, never failing the others.
func renderAll(body string, list []groups.Resolved) (msgs []RenderedMessage, failed []string, err error) {
	msgs = make([]RenderedMessage, 0, len(list))
	for _, r := range list {
		text, err := Render(body, RecipientFields(r.Recipient.FirstName, r.Recipient.LastName))
		if errors.Is(err, ErrMissingValue) {
			failed = append(failed, r.Recipient.ID)
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("render for recipient %s: %w", r.Recipient.ID, err)
		}
		msgs = append(msgs, RenderedMessage{
			Body:     text,
			HasEmail: r.CanReach(groups.ChannelEmail),
			HasPhone: r.CanReach(groups.ChannelSMS),
		})
	}
	return msgs, failed, nil
}
