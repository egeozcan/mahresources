// Package jobview holds the Job Center's HTTP-facing reading of a Job list
// request and the presentation rules both of its surfaces share: the JSON API in
// server/api_handlers and the server-rendered /jobs page in
// server/template_handlers. The template providers may not import api_handlers,
// so a rule the two must agree on lives here once rather than in each.
package jobview

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mahresources/jobs"
)

// ParseFilter reads the canonical Job filter from a query string. The parameter
// names are a public contract: the legacy /downloads translator, the jobs panel
// and bookmarks all name them.
func ParseFilter(values url.Values) (jobs.Filter, error) {
	var filter jobs.Filter
	filter.States = tokens(values, "states", "state")
	filter.Kinds = tokens(values, "kinds", "kind")
	filter.Origins = tokens(values, "origins", "origin")
	filter.Search = values.Get("search")
	filter.Relationship = values.Get("relationship")
	var err error
	if filter.OwnerID, err = positiveUint(values, "ownerId"); err != nil {
		return jobs.Filter{}, err
	}
	if filter.ActorID, err = positiveUint(values, "actorId"); err != nil {
		return jobs.Filter{}, err
	}
	if filter.AcceptedAfter, err = bound(values, "acceptedAfter", false); err != nil {
		return jobs.Filter{}, err
	}
	if filter.AcceptedBefore, err = bound(values, "acceptedBefore", true); err != nil {
		return jobs.Filter{}, err
	}
	if filter.Pinned, err = optionalBool(values, "pinned"); err != nil {
		return jobs.Filter{}, err
	}
	if filter.Dismissed, err = optionalBool(values, "dismissed"); err != nil {
		return jobs.Filter{}, err
	}
	if commandValues, present := values["command"]; present {
		if len(commandValues) != 1 {
			return jobs.Filter{}, fmt.Errorf("command must be supplied once")
		}
		filter.Command = commandValues[0]
		if strings.TrimSpace(filter.Command) == "" {
			return jobs.Filter{}, fmt.Errorf("command must be a non-empty command key")
		}
		if len(filter.Command) > jobs.MaxCommandKeyBytes {
			return jobs.Filter{}, fmt.Errorf("command key must not exceed %d bytes", jobs.MaxCommandKeyBytes)
		}
	}
	return filter, nil
}

// tokens reads a repeatable parameter, also splitting each value on commas so a
// hand-written `state=failed,blocked` still works.
func tokens(values url.Values, names ...string) []string {
	var out []string
	for _, name := range names {
		for _, value := range values[name] {
			for token := range strings.SplitSeq(value, ",") {
				out = append(out, strings.TrimSpace(token))
			}
		}
	}
	return out
}

func positiveUint(values url.Values, name string) (*uint, error) {
	raw := values.Get(name)
	if raw == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseUint(raw, 10, strconv.IntSize)
	if err != nil || parsed == 0 {
		return nil, fmt.Errorf("%s must be a positive integer", name)
	}
	value := uint(parsed)
	return &value, nil
}

// localBounds are the server-local shapes a bound may take, each with the unit
// it names: a date input submits a day, a datetime input a minute or a second.
var localBounds = []struct {
	layout string
	unit   func(time.Time) time.Time
}{
	{"2006-01-02", func(t time.Time) time.Time { return t.AddDate(0, 0, 1) }},
	{"2006-01-02T15:04", func(t time.Time) time.Time { return t.Add(time.Minute) }},
	{"2006-01-02T15:04:05", func(t time.Time) time.Time { return t.Add(time.Second) }},
}

// bound reads one acceptance bound. An RFC 3339 instant is taken as written. A
// server-local date or datetime names a whole unit at the precision it was
// written in — the day, the minute, the second — the same reading the other
// lists give their date inputs. Both bounds are inclusive, so a start is the
// unit's first instant and an end its last: "before 15:00" includes 15:00.
func bound(values url.Values, name string, end bool) (*time.Time, error) {
	at, err := Bound(values.Get(name), end)
	if err != nil {
		return nil, fmt.Errorf("%s %w", name, err)
	}
	return at, nil
}

// Bound reads one acceptance bound value; bound describes the forms it takes.
// The empty value is no bound.
func Bound(raw string, end bool) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	for _, local := range localBounds {
		at, err := time.ParseInLocation(local.layout, raw, time.Local)
		if err != nil {
			continue
		}
		if end {
			at = local.unit(at).Add(-time.Nanosecond)
		}
		at = at.UTC()
		return &at, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, fmt.Errorf("must be a date, a local date and time, or an RFC3339 timestamp")
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func optionalBool(values url.Values, name string) (*bool, error) {
	raw := values.Get(name)
	if raw == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be true or false", name)
	}
	return &parsed, nil
}

type encodedCursor struct {
	AcceptedAt time.Time `json:"acceptedAt"`
	ID         string    `json:"id"`
}

// EncodeCursor renders a keyset position as the opaque token the list API and
// the /jobs page carry in their `cursor` and `before` parameters.
func EncodeCursor(cursor jobs.Cursor) (string, error) {
	encoded, err := json.Marshal(encodedCursor{AcceptedAt: cursor.AcceptedAt.UTC(), ID: cursor.ID})
	if err != nil {
		return "", err
	}
	return "list-v1." + base64.RawURLEncoding.EncodeToString(encoded), nil
}

// DecodeCursor reads a token EncodeCursor produced. The empty token is the start
// of the listing.
func DecodeCursor(value string) (jobs.Cursor, error) {
	if value == "" {
		return jobs.Cursor{}, nil
	}
	if len(value) > 2048 || !strings.HasPrefix(value, "list-v1.") {
		return jobs.Cursor{}, fmt.Errorf("cursor is invalid")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, "list-v1."))
	if err != nil {
		return jobs.Cursor{}, fmt.Errorf("cursor is invalid")
	}
	var cursor encodedCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.ID == "" || cursor.AcceptedAt.IsZero() {
		return jobs.Cursor{}, fmt.Errorf("cursor is invalid")
	}
	return jobs.Cursor{AcceptedAt: cursor.AcceptedAt.UTC(), ID: cursor.ID}, nil
}
