package recipients

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// classPattern is a normalized class name (ADR-0009): a digit, then upper-case
// letters or digits, 1-6 characters in all, e.g. 3A, 1TI, 2BG. The migration
// 0003_recipient_classes.sql checks the same pattern.
var classPattern = regexp.MustCompile(`^[0-9][0-9A-Z]{0,5}$`)

// NormalizeClass turns a class name as typed (" 3 a ") into its stored form
// (3A): every white space removed, letters upper-cased. The result still
// needs ValidClass.
func NormalizeClass(s string) string {
	return strings.ToUpper(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s))
}

// ValidClass reports whether a normalized class name has the expected form.
func ValidClass(class string) bool {
	return classPattern.MatchString(class)
}

// NormalizeClasses normalizes every name, drops blanks and repeats, and sorts
// the rest with CompareClasses. Invalid names are kept, so Validate can
// report them. No class at all is nil, as in a Recipient without classes.
func NormalizeClasses(classes []string) []string {
	var out []string
	for _, c := range classes {
		if c = NormalizeClass(c); c != "" && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	slices.SortFunc(out, CompareClasses)
	return out
}

// ParseClassList splits the class cell of an import file ("3A, 1B") into
// normalized names, in the order of NormalizeClasses.
func ParseClassList(cell string) []string {
	return NormalizeClasses(strings.Split(cell, ","))
}

// CompareClasses orders class names as a school lists them: by the leading
// year number, compared as a number (2A before 10A), then by the rest of the
// name (1A before 1B). It is the order of the class groups too (ADR-0009).
func CompareClasses(a, b string) int {
	an, arest := splitClass(a)
	bn, brest := splitClass(b)
	if an != bn {
		return an - bn
	}
	return strings.Compare(arest, brest)
}

// splitClass splits a class name into its leading number and the rest. A
// name without a leading number sorts first, which only an invalid name has.
func splitClass(class string) (int, string) {
	i := 0
	for i < len(class) && class[i] >= '0' && class[i] <= '9' {
		i++
	}
	n, err := strconv.Atoi(class[:i])
	if err != nil {
		return -1, class
	}
	return n, class[i:]
}

// validateClasses checks the classes of a recipient of type t: every name in
// its normalized form, and at most one class for a student, whose class is
// their own (a parent has the classes of their children).
func validateClasses(t Type, classes []string) []FieldError {
	var errs []FieldError
	for _, c := range classes {
		if !ValidClass(c) {
			errs = append(errs, FieldError{Field: "classes", Message: "class " + strconv.Quote(c) + " must start with a digit and have at most 6 letters or digits, e.g. 3A"})
		}
	}
	if t == TypeStudent && len(classes) > 1 {
		errs = append(errs, FieldError{Field: "classes", Message: "a student belongs to one class only"})
	}
	return errs
}
