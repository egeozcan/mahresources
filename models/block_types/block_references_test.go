package block_types

import (
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
