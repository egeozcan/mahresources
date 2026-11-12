package block_types

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

// Issue #89: the reference reader and the validators must agree on which key a
// payload means. Case-variant (or exact) duplicate top-level keys are refused by
// every reference-carrying block type; the reader takes exact keys only.
func TestReferenceBlocksRefuseAmbiguousKeys(t *testing.T) {
	cases := map[string]string{
		"gallery":    `{"resourceIds":[7],"RESOURCEIDS":[]}`,
		"references": `{"groupIds":[7],"GroupIds":[]}`,
		"table":      `{"queryId":7,"QUERYID":null}`,
		"calendar":   `{"calendars":[],"Calendars":[]}`,
	}
	for kind, content := range cases {
		bt := GetBlockType(kind)
		if bt == nil {
			t.Fatalf("no block type %q", kind)
		}
		if err := bt.ValidateContent(json.RawMessage(content)); !errors.Is(err, ErrAmbiguousBlockContent) {
			t.Errorf("%s: ValidateContent(%s) = %v, want ErrAmbiguousBlockContent", kind, content, err)
		}
	}
	if err := RejectAmbiguousKeys(json.RawMessage(`{"resourceIds":[1],"resourceIds":[2]}`)); !errors.Is(err, ErrAmbiguousBlockContent) {
		t.Errorf("exact duplicate key accepted: %v", err)
	}
	if err := RejectAmbiguousKeys(json.RawMessage(`{"resourceIds":[1],"layout":{"a":1,"A":2}}`)); err != nil {
		t.Errorf("nested keys are not ambiguous at the top level: %v", err)
	}
}

func TestExtractBlockReferencesReadsExactKeys(t *testing.T) {
	refs, err := ExtractBlockReferences("gallery", json.RawMessage(`{"RESOURCEIDS":[9],"resourceIds":[1,null,"x",-2,3]}`))
	if err != nil || len(refs.ResourceIDs) != 2 || refs.ResourceIDs[0] != 1 || refs.ResourceIDs[1] != 3 {
		t.Errorf("gallery refs = %+v, %v", refs, err)
	}
	refs, _ = ExtractBlockReferences("table", json.RawMessage(`{"QueryId":4}`))
	if refs.QueryID != nil {
		t.Errorf("a case-variant queryId was read: %d", *refs.QueryID)
	}
	refs, _ = ExtractBlockReferences("table", json.RawMessage(`{"queryId":null}`))
	if refs.QueryID != nil {
		t.Errorf("a null queryId was read as %d", *refs.QueryID)
	}
	refs, _ = ExtractBlockReferences("table", json.RawMessage(`{"queryId":4,"queryParams":{"a":1}}`))
	if refs.QueryID == nil || *refs.QueryID != 4 || refs.QueryParams["a"] != float64(1) {
		t.Errorf("table refs = %+v", refs)
	}
	refs, _ = ExtractBlockReferences("references", json.RawMessage(`{"groupIds":[5]}`))
	if len(refs.GroupIDs) != 1 || refs.GroupIDs[0] != 5 {
		t.Errorf("references refs = %+v", refs)
	}
	refs, _ = ExtractBlockReferences("calendar", json.RawMessage(`{"calendars":[{"id":"c","source":{"type":"resource","resourceId":6}}]}`))
	if len(refs.ResourceIDs) != 1 || refs.ResourceIDs[0] != 6 {
		t.Errorf("calendar refs = %+v", refs)
	}
}

// foldKey must fold exactly as encoding/json matches a key to a struct field,
// including the non-ASCII runes strings.ToLower leaves alone: U+017F (long s)
// folds with s, U+212A (Kelvin sign) with k.
func TestFoldKeyMatchesEncodingJSON(t *testing.T) {
	type probe struct {
		Calendars []int `json:"calendars"`
		Kind      int   `json:"kind"`
	}
	cases := []struct {
		key, field string
		want       bool
	}{
		{"Calendarſ", "calendars", true},
		{"CALENDARS", "calendars", true},
		{"Kind", "kind", true},
		{"calendar", "calendars", false},
		{"calendarß", "calendars", false},
	}
	for _, c := range cases {
		var p probe
		val := `[1]`
		if c.field == "kind" {
			val = `1`
		}
		if err := json.Unmarshal([]byte(`{"`+c.key+`":`+val+`}`), &p); err != nil {
			t.Fatal(err)
		}
		matched := len(p.Calendars) == 1 || p.Kind == 1
		if matched != c.want {
			t.Fatalf("encoding/json matched %q to %q = %v, test case says %v", c.key, c.field, matched, c.want)
		}
		if got := foldKey(c.key) == foldKey(c.field); got != c.want {
			t.Errorf("foldKey(%q) == foldKey(%q) = %v, encoding/json says %v", c.key, c.field, got, c.want)
		}
		if got := bytes.EqualFold([]byte(c.key), []byte(c.field)); got != c.want {
			t.Errorf("bytes.EqualFold(%q, %q) = %v, encoding/json says %v", c.key, c.field, got, c.want)
		}
	}
}

// Calendar content is checked at every level; it has no user-chosen keys.
func TestCalendarRefusesAmbiguousKeysAtAnyLevel(t *testing.T) {
	bt := GetBlockType("calendar")
	refused := map[string]string{
		"top-level long s":  `{"calendars":[],"Calendar` + "ſ" + `":[]}`,
		"nested type/Type":  `{"calendars":[{"id":"a","source":{"type":"resource","resourceId":1,"Type":"url","url":"https://e.com/a.ics"}}]}`,
		"nested long s":     `{"calendars":[{"id":"a","source":{"type":"resource","resourceId":1},"` + "ſ" + `ource":{"type":"url","url":"https://e.com/a.ics"}}]}`,
		"nested resourceId": `{"calendars":[{"id":"a","source":{"type":"resource","resourceId":1,"re` + "ſ" + `ourceId":2}}]}`,
		"nested exact dup":  `{"calendars":[{"id":"a","source":{"type":"resource","resourceId":1,"resourceId":2}}]}`,
		"entry id/ID":       `{"calendars":[{"id":"a","ID":"b","source":{"type":"url","url":"https://e.com/a.ics"}}]}`,
	}
	for name, content := range refused {
		if err := bt.ValidateContent(json.RawMessage(content)); !errors.Is(err, ErrAmbiguousBlockContent) {
			t.Errorf("%s: ValidateContent = %v, want ErrAmbiguousBlockContent", name, err)
		}
		if err := RejectAmbiguousBlockKeys("calendar", json.RawMessage(content)); !errors.Is(err, ErrAmbiguousBlockContent) {
			t.Errorf("%s: RejectAmbiguousBlockKeys = %v", name, err)
		}
	}
	ok := `{"calendars":[{"id":"a","name":"A","color":"#fff","source":{"type":"url","url":"https://e.com/a.ics"}},{"id":"b","source":{"type":"resource","resourceId":2}}]}`
	if err := bt.ValidateContent(json.RawMessage(ok)); err != nil {
		t.Errorf("legitimate calendar refused: %v", err)
	}
}

// Table rows are keyed by column ids, which may differ only in case; the
// top-level-only check for non-calendar types must keep accepting them.
func TestTableRowsWithCaseVariantColumnIDsAreAccepted(t *testing.T) {
	content := `{"columns":[{"id":"a","label":"a"},{"id":"A","label":"A"}],"rows":[{"id":"r1","a":"x","A":"y"}]}`
	if err := GetBlockType("table").ValidateContent(json.RawMessage(content)); err != nil {
		t.Errorf("table with case-variant column ids refused: %v", err)
	}
	if err := RejectAmbiguousBlockKeys("table", json.RawMessage(content)); err != nil {
		t.Errorf("RejectAmbiguousBlockKeys(table) = %v", err)
	}
	if err := RejectAmbiguousKeys(json.RawMessage(`{"resourceIds":[1],"reſourceIds":[]}`)); !errors.Is(err, ErrAmbiguousBlockContent) {
		t.Errorf("top-level long-s variant accepted: %v", err)
	}
}

// ReadCalendarSources reads exact keys only, so a case-variant key is ignored
// in any order and a re-marshal through a map cannot change the result.
func TestReadCalendarSourcesReadsExactKeys(t *testing.T) {
	orders := []string{
		`{"calendars":[{"id":"a","source":{"type":"resource","resourceId":7,"Type":"url","URL":"https://e.com"}}],"Calendar` + "ſ" + `":[]}`,
		`{"Calendar` + "ſ" + `":[],"calendars":[{"id":"a","source":{"Type":"url","URL":"https://e.com","resourceId":7,"type":"resource"}}]}`,
	}
	for _, content := range orders {
		for _, in := range []string{content, remarshal(t, content)} {
			cals, err := ReadCalendarSources(json.RawMessage(in))
			if err != nil {
				t.Fatal(err)
			}
			if len(cals) != 1 || cals[0].ID != "a" || cals[0].Source.Type != "resource" || cals[0].Source.URL != "" ||
				cals[0].Source.ResourceID == nil || *cals[0].Source.ResourceID != 7 {
				t.Errorf("ReadCalendarSources(%s) = %+v", in, cals)
			}
		}
	}
	for _, bad := range []string{`[]`, `{"calendars":{}}`, `{"calendars":[{"source":{"resourceId":-1}}]}`, `{"calendars":[{"id":5}]}`} {
		if _, err := ReadCalendarSources(json.RawMessage(bad)); err == nil {
			t.Errorf("ReadCalendarSources(%s) accepted a wrongly typed value", bad)
		}
	}
	if cals, err := ReadCalendarSources(json.RawMessage(`{"calendars":null}`)); err != nil || len(cals) != 0 {
		t.Errorf("null calendars = %+v, %v", cals, err)
	}
}

func remarshal(t *testing.T, content string) string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(content), &v); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
