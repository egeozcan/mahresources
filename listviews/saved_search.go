// Package listviews defines the list destinations that can be saved and reopened.
package listviews

import (
	"fmt"
	"net/url"
	"strings"
)

type View struct {
	Path   string
	Family string
	Layout string
}

var views = []View{
	{"/resources", "resources", "Cards"},
	{"/resources/details", "resources", "Details"},
	{"/resources/simple", "resources", "Contact sheet"},
	{"/resources/timeline", "resources", "Timeline"},
	{"/notes", "notes", "Cards"},
	{"/notes/timeline", "notes", "Timeline"},
	{"/groups", "groups", "Cards"},
	{"/groups/text", "groups", "Text"},
	{"/groups/timeline", "groups", "Timeline"},
	{"/tags", "tags", "Cards"},
	{"/tags/timeline", "tags", "Timeline"},
	{"/categories", "categories", "Cards"},
	{"/categories/timeline", "categories", "Timeline"},
	{"/resourceCategories", "resourceCategories", "List"},
	{"/noteTypes", "noteTypes", "List"},
	{"/relations", "relations", "List"},
	{"/relationTypes", "relationTypes", "List"},
	{"/queries", "queries", "List"},
	{"/queries/timeline", "queries", "Timeline"},
	{"/templatePartials", "templatePartials", "List"},
	{"/downloads", "downloads", "List"},
	{"/logs", "logs", "List"},
	{"/reductions", "reductions", "List"},
}

func Lookup(path string) *View {
	for _, view := range views {
		if view.Path == path {
			return &view
		}
	}
	return nil
}

func ValidFamily(family string) bool {
	for _, view := range views {
		if view.Family == family {
			return true
		}
	}
	return false
}

// NormalizeSavedURL keeps the applied search, including ordered repeated values,
// but drops pagination and request-only state. Error is a real Downloads filter.
func NormalizeSavedURL(raw string) (string, *View, error) {
	u, err := url.Parse(raw)
	if err != nil || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") ||
		u.IsAbs() || u.Host != "" || u.User != nil || u.Fragment != "" || u.Opaque != "" || u.RawPath != "" {
		return "", nil, fmt.Errorf("search URL must be a relative list URL")
	}
	view := Lookup(u.Path)
	if view == nil {
		return "", nil, fmt.Errorf("unsupported list URL")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", nil, fmt.Errorf("invalid search parameters")
	}
	for key := range q {
		switch strings.ToLower(key) {
		case "page", "redirect", "csrf", "csrf_token":
			q.Del(key)
		case "error":
			if view.Family != "downloads" {
				q.Del(key)
			}
		}
	}
	u.RawQuery = q.Encode()
	u.ForceQuery = false
	return u.String(), view, nil
}
