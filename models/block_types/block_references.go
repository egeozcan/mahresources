package block_types

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrAmbiguousBlockContent refuses block content whose top-level object carries
// two keys that differ only in case (or the same key twice).
//
// encoding/json fills a struct case-insensitively with the last matching key
// winning, while a map, a pongo2 lookup and the browser's JSON.parse read exact
// keys. {"resourceIds":[7],"RESOURCEIDS":[]} is therefore an empty gallery to
// every struct-based validator and gallery [7] to every renderer. No editor
// produces such content, so it is refused rather than guessed at.
var ErrAmbiguousBlockContent = errors.New("block content has duplicate keys differing only in case")

// RejectAmbiguousKeys returns ErrAmbiguousBlockContent when the top-level JSON
// object in content holds two keys that are equal under case folding. Content
// that is not an object (or not JSON) is left to the block type's own
// validation.
func RejectAmbiguousKeys(content json.RawMessage) error {
	dec := json.NewDecoder(bytes.NewReader(content))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil
	}
	seen := make(map[string]string)
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil
		}
		key, _ := tok.(string)
		folded := strings.ToLower(key)
		if prev, dup := seen[folded]; dup {
			return fmt.Errorf("%w: %q and %q", ErrAmbiguousBlockContent, prev, key)
		}
		seen[folded] = key
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil
		}
	}
	return nil
}

// BlockReferences are the IDs a block's content points at that the share server
// (and the in-app renderers) dereference on the note's behalf.
type BlockReferences struct {
	ResourceIDs []uint         // gallery resourceIds; calendar resource sources
	GroupIDs    []uint         // references groupIds
	QueryID     *uint          // table queryId
	QueryParams map[string]any // table queryParams
}

// ExtractBlockReferences reads the references out of a block's content. It is
// the one reader for them: the write-time and share-time scope checks and the
// share server all call it, so what is checked is exactly what is resolved
// (issue #89).
//
// gallery, references and table content is read by exact key, as the share
// template, the browser and block_ref_cleanup read it; an element that is not a
// non-negative integer is skipped, as the renderers skip it. Calendar content is
// read with the calendarContent struct, the same decode GetCalendarEvents uses.
// An error means the content does not parse; the caller decides whether that is
// a refusal (scope check) or "no references" (renderer).
func ExtractBlockReferences(blockType string, content json.RawMessage) (BlockReferences, error) {
	var refs BlockReferences
	if len(bytes.TrimSpace(content)) == 0 {
		return refs, nil
	}
	switch blockType {
	case "gallery", "references", "table":
	case "calendar":
		var c calendarContent
		if err := json.Unmarshal(content, &c); err != nil {
			return refs, err
		}
		for _, cal := range c.Calendars {
			if cal.Source.Type == "resource" && cal.Source.ResourceID != nil {
				refs.ResourceIDs = append(refs.ResourceIDs, *cal.Source.ResourceID)
			}
		}
		return refs, nil
	default:
		return refs, nil
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(content, &obj); err != nil {
		return refs, err
	}
	switch blockType {
	case "gallery":
		refs.ResourceIDs = idList(obj["resourceIds"])
	case "references":
		refs.GroupIDs = idList(obj["groupIds"])
	case "table":
		if raw, ok := obj["queryId"]; ok {
			var id *uint
			if json.Unmarshal(raw, &id) == nil {
				refs.QueryID = id
			}
		}
		if raw, ok := obj["queryParams"]; ok {
			var params map[string]any
			if json.Unmarshal(raw, &params) == nil {
				refs.QueryParams = params
			}
		}
	}
	return refs, nil
}

// idList decodes a JSON array of IDs, skipping any element that is not a
// non-negative integer (null included). A missing or non-array value yields no IDs.
func idList(raw json.RawMessage) []uint {
	var elems []json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &elems) != nil {
		return nil
	}
	ids := make([]uint, 0, len(elems))
	for _, e := range elems {
		var id *uint
		if json.Unmarshal(e, &id) == nil && id != nil {
			ids = append(ids, *id)
		}
	}
	return ids
}
