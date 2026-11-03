package notion

import (
	"fmt"
	"sort"
	"strings"
)

// Column maps one Notion property to one import column. Column is a header
// the recipients import understands, e.g. "first_name", "nazwisko" or "klasa"
// (see recipients.columnAliases).
type Column struct {
	Property string
	Column   string
}

// Mapping lists the Notion properties to import, in output order. An empty
// Mapping imports every property under its own name, so a database whose
// columns are named like the CSV template ("Imię", "Nazwisko", "E-mail",
// "Telefon", "Typ", "Klasa") needs no mapping at all.
type Mapping []Column

// ParseMapping reads "Notion property=column" pairs separated by commas,
// e.g. "Name=first_name,Surname=last_name,Parent e-mail=email". An empty
// string is the empty (identity) Mapping.
func ParseMapping(s string) (Mapping, error) {
	var m Mapping
	seen := make(map[string]bool)
	for _, pair := range strings.Split(s, ",") {
		if strings.TrimSpace(pair) == "" {
			continue
		}
		prop, col, ok := strings.Cut(pair, "=")
		prop, col = strings.TrimSpace(prop), strings.TrimSpace(col)
		if !ok || prop == "" || col == "" {
			return nil, fmt.Errorf("notion property map: %q is not \"property=column\"", strings.TrimSpace(pair))
		}
		if seen[prop] {
			return nil, fmt.Errorf("notion property map: property %q is mapped twice", prop)
		}
		seen[prop] = true
		m = append(m, Column{Property: prop, Column: col})
	}
	return m, nil
}

// ToTable turns pages into an import table: a header row and one row per
// page, in page order, so table row i+1 is pages[i] (row 1 is the header).
// A property missing from a page reads as an empty cell.
func ToTable(pages []Page, m Mapping) [][]string {
	if len(m) == 0 {
		m = identity(pages)
	}
	table := make([][]string, 0, len(pages)+1)
	header := make([]string, len(m))
	for i, c := range m {
		header[i] = c.Column
	}
	table = append(table, header)
	for _, p := range pages {
		row := make([]string, len(m))
		for i, c := range m {
			row[i] = strings.TrimSpace(p.Properties[c.Property].Text())
		}
		table = append(table, row)
	}
	return table
}

// identity maps every property found on any page to a column of the same
// name, sorted by name so the output does not depend on map order.
func identity(pages []Page) Mapping {
	seen := make(map[string]bool)
	var names []string
	for _, p := range pages {
		for name := range p.Properties {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	m := make(Mapping, len(names))
	for i, n := range names {
		m[i] = Column{Property: n, Column: n}
	}
	return m
}
