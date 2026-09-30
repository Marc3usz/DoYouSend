package groups

import (
	"errors"
	"fmt"

	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

// MaxSelectionIDs bounds each list of a Selection. A school has a few thousand
// recipients at most, so a longer list is a client bug, not a real choice.
const MaxSelectionIDs = 5000

// ErrEmptySelection is returned when a selection picks neither a group nor a
// person, so there would be nobody to send to.
var ErrEmptySelection = errors.New("selection picks no group and no recipient")

// Selection is what the sender picks in the message wizard (openapi.yaml
// RecipientSelection): whole groups, single people, and people to leave out of
// this one batch (description.md "wykluczyc odbiorce z danej wysylki").
type Selection struct {
	GroupIDs             []string
	RecipientIDs         []string
	ExcludedRecipientIDs []string
}

// Normalize returns the selection with IDs trimmed, lower-cased and listed
// once each, keeping the order of first appearance. It does not validate.
func (s Selection) Normalize() Selection {
	return Selection{
		GroupIDs:             uniqueIDs(s.GroupIDs),
		RecipientIDs:         uniqueIDs(s.RecipientIDs),
		ExcludedRecipientIDs: uniqueIDs(s.ExcludedRecipientIDs),
	}
}

// Validate checks a normalized selection: every ID is a UUID, no list is over
// MaxSelectionIDs, and something is actually picked. Excluding everyone is
// not an error here: Resolve simply returns an empty list, which the wizard
// shows as such. Positions in field names ("recipientIds[2]") refer to the
// normalized lists, which a client that sends clean, unique IDs sees as-is.
func (s Selection) Validate() error {
	if len(s.GroupIDs) == 0 && len(s.RecipientIDs) == 0 {
		return ErrEmptySelection
	}

	var errs []recipients.FieldError
	errs = append(errs, validateIDs("groupIds", s.GroupIDs)...)
	errs = append(errs, validateIDs("recipientIds", s.RecipientIDs)...)
	errs = append(errs, validateIDs("excludedRecipientIds", s.ExcludedRecipientIDs)...)
	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	return nil
}

func validateIDs(field string, ids []string) []recipients.FieldError {
	if len(ids) > MaxSelectionIDs {
		return []recipients.FieldError{{
			Field:   field,
			Message: fmt.Sprintf("more than %d IDs", MaxSelectionIDs),
		}}
	}
	var errs []recipients.FieldError
	for i, id := range ids {
		if !IsValidID(id) {
			errs = append(errs, recipients.FieldError{
				Field:   fmt.Sprintf("%s[%d]", field, i),
				Message: "not a valid ID",
			})
		}
	}
	return errs
}

// uniqueIDs canonicalizes ids and drops repeats and blanks.
func uniqueIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = canonicalID(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
