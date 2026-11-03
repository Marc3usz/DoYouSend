package notion

import (
	"fmt"
	"strconv"
	"strings"
)

// Page is one row of a data source.
type Page struct {
	Object     string              `json:"object"`
	ID         string              `json:"id"`
	Properties map[string]Property `json:"properties"`
}

// Property is the value of one column of a page. Only the fields of the
// property types the import can use are decoded; Text renders them.
type Property struct {
	Type        string     `json:"type"`
	Title       []richText `json:"title"`
	RichText    []richText `json:"rich_text"`
	Email       *string    `json:"email"`
	PhoneNumber *string    `json:"phone_number"`
	Select      *option    `json:"select"`
	MultiSelect []option   `json:"multi_select"`
	Status      *option    `json:"status"`
	Number      *float64   `json:"number"`
	Checkbox    bool       `json:"checkbox"`
	Formula     *formula   `json:"formula"`
	UniqueID    *uniqueID  `json:"unique_id"`
}

type richText struct {
	PlainText string `json:"plain_text"`
}

type option struct {
	Name string `json:"name"`
}

type formula struct {
	Type    string   `json:"type"`
	String  *string  `json:"string"`
	Number  *float64 `json:"number"`
	Boolean *bool    `json:"boolean"`
}

type uniqueID struct {
	Prefix *string `json:"prefix"`
	Number *int64  `json:"number"`
}

// Text renders the property as the text a spreadsheet cell would hold.
// Multi-select options are joined with ", " (the class list format of the
// import). Types the import cannot use (dates, people, files, relations...)
// render as "".
func (p Property) Text() string {
	switch p.Type {
	case "title":
		return joinRichText(p.Title)
	case "rich_text":
		return joinRichText(p.RichText)
	case "email":
		return deref(p.Email)
	case "phone_number":
		return deref(p.PhoneNumber)
	case "select":
		return optionName(p.Select)
	case "status":
		return optionName(p.Status)
	case "multi_select":
		names := make([]string, 0, len(p.MultiSelect))
		for _, o := range p.MultiSelect {
			names = append(names, o.Name)
		}
		return strings.Join(names, ", ")
	case "number":
		return formatNumber(p.Number)
	case "checkbox":
		return strconv.FormatBool(p.Checkbox)
	case "formula":
		if p.Formula == nil {
			return ""
		}
		switch p.Formula.Type {
		case "string":
			return deref(p.Formula.String)
		case "number":
			return formatNumber(p.Formula.Number)
		case "boolean":
			if p.Formula.Boolean == nil {
				return ""
			}
			return strconv.FormatBool(*p.Formula.Boolean)
		}
		return ""
	case "unique_id":
		if p.UniqueID == nil || p.UniqueID.Number == nil {
			return ""
		}
		n := strconv.FormatInt(*p.UniqueID.Number, 10)
		if pre := deref(p.UniqueID.Prefix); pre != "" {
			return fmt.Sprintf("%s-%s", pre, n)
		}
		return n
	default:
		return ""
	}
}

func joinRichText(parts []richText) string {
	var b strings.Builder
	for _, t := range parts {
		b.WriteString(t.PlainText)
	}
	return b.String()
}

func optionName(o *option) string {
	if o == nil {
		return ""
	}
	return o.Name
}

func formatNumber(f *float64) string {
	if f == nil {
		return ""
	}
	return strconv.FormatFloat(*f, 'f', -1, 64)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
