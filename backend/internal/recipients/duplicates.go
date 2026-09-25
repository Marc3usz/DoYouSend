package recipients

import "strings"

// Duplicate groups recipients that share the same normalized e-mail or phone
// number, so a CSV import report (or the manual CRUD UI) can show which rows
// collide before they ever reach the database's unique indexes.
type Duplicate struct {
	Key        string // the normalized email or phone the group collides on
	Recipients []Recipient
}

// FindDuplicates groups recipients by normalized e-mail and by phone number,
// returning one Duplicate per colliding value. A recipient present in both an
// e-mail collision and a phone collision appears in both groups.
func FindDuplicates(candidates []Recipient) []Duplicate {
	var dups []Duplicate
	dups = append(dups, groupBy(candidates, func(r Recipient) string {
		return normalizeEmail(r.Email)
	})...)
	dups = append(dups, groupBy(candidates, func(r Recipient) string {
		return NormalizePhone(r.Phone)
	})...)
	return dups
}

func groupBy(candidates []Recipient, key func(Recipient) string) []Duplicate {
	groups := make(map[string][]Recipient)
	var order []string
	for _, r := range candidates {
		k := key(r)
		if k == "" {
			continue
		}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], r)
	}

	var dups []Duplicate
	for _, k := range order {
		if len(groups[k]) > 1 {
			dups = append(dups, Duplicate{Key: k, Recipients: groups[k]})
		}
	}
	return dups
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
