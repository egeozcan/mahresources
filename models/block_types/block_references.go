package block_types

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"
)

// ErrAmbiguousBlockContent refuses block content holding, in one JSON object,
// two keys that encoding/json would treat as the same field (or the same key
// twice).
//
// encoding/json fills a struct case-insensitively, under Unicode simple folding
// ("Calendar\u017f" with U+017F LATIN SMALL LETTER LONG S matches "calendars"),
// with the last matching key winning, while a map, a pongo2 lookup and the
// browser's JSON.parse read exact keys. And the stored bytes do not keep their
// order: the block_ref_cleanup scrubs re-marshal through a map (keys sorted
// bytewise) and Postgres jsonb reorders keys on write, so even two readers that
// agree on "last key wins" can disagree once the block is rewritten. No editor
// produces such content, so it is refused rather than guessed at.
var ErrAmbiguousBlockContent = errors.New("block content has duplicate keys differing only in case")

// RejectAmbiguousKeys returns ErrAmbiguousBlockContent when the top-level JSON
// object in content holds two keys that are equal under encoding/json's case
// folding. Content that is not an object (or not JSON) is left to the block
// type's own validation.
func RejectAmbiguousKeys(content json.RawMessage) error {
	return rejectAmbiguousKeys(content, false)
}

// RejectAmbiguousBlockKeys applies the ambiguity rule a block type's readers
// need. Calendar readers consult nested objects (calendars[] and their source),
// and calendar content has no user-chosen keys, so every object in it is
// checked. The other reference-carrying types are read at the top level only,
// and nested objects there can legitimately hold keys differing only in case
// (table rows are keyed by column ids), so only the top level is checked.
func RejectAmbiguousBlockKeys(blockType string, content json.RawMessage) error {
	return rejectAmbiguousKeys(content, blockType == "calendar")
}

func rejectAmbiguousKeys(content json.RawMessage, deep bool) error {
	dec := json.NewDecoder(bytes.NewReader(content))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil
	}
	if !deep {
		return scanObjectKeys(dec, nil)
	}
	return scanObjectKeys(dec, func(dec *json.Decoder) error { return scanValue(dec) })
}

// scanObjectKeys reads the rest of an object whose '{' was consumed, refusing
// two keys equal under foldKey. Values are handed to value when it is non-nil
// (the deep walk) and skipped otherwise. Malformed JSON ends the scan without
// an error; the block type's own validation reports it.
func scanObjectKeys(dec *json.Decoder, value func(*json.Decoder) error) error {
	seen := make(map[string]string)
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil
		}
		key, _ := tok.(string)
		folded := foldKey(key)
		if prev, dup := seen[folded]; dup {
			return fmt.Errorf("%w: %q and %q", ErrAmbiguousBlockContent, prev, key)
		}
		seen[folded] = key
		if value != nil {
			if err := value(dec); err != nil {
				return err
			}
			continue
		}
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil
		}
	}
	_, _ = dec.Token() // the closing '}'
	return nil
}

// scanValue walks one JSON value, checking every object inside it.
func scanValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return nil
	}
	switch tok {
	case json.Delim('{'):
		return scanObjectKeys(dec, scanValue)
	case json.Delim('['):
		for dec.More() {
			if err := scanValue(dec); err != nil {
				return err
			}
		}
		_, _ = dec.Token() // the closing ']'
	}
	return nil
}

// foldKey folds a JSON key exactly as encoding/json does when it matches a key
// to a struct field (foldName in encoding/json/fold.go): ASCII letters to upper
// case, every other rune to the smallest rune of its simple-fold orbit. So
// "\u017f" (long s) folds with "s" and "S", and U+212A KELVIN SIGN with "k".
// foldKey(a) == foldKey(b) exactly when bytes.EqualFold(a, b).
func foldKey(key string) string {
	out := make([]byte, 0, len(key))
	for i := 0; i < len(key); {
		if c := key[i]; c < utf8.RuneSelf {
			if 'a' <= c && c <= 'z' {
				c -= 'a' - 'A'
			}
			out = append(out, c)
			i++
			continue
		}
		r, n := utf8.DecodeRuneInString(key[i:])
		for {
			r2 := unicode.SimpleFold(r)
			if r2 <= r {
				r = r2
				break
			}
			r = r2
		}
		out = utf8.AppendRune(out, r)
		i += n
	}
	return string(out)
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
// read with ReadCalendarSources, the reader GetCalendarEvents uses.
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
		cals, err := ReadCalendarSources(content)
		if err != nil {
			return refs, err
		}
		for _, cal := range cals {
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

// ReadCalendarSources reads calendars[] out of a calendar block's content by
// exact key: calendars, then id, name, color and source on each entry, then
// type, url and resourceId on the source. It is the one reader for calendar
// sources: ValidateContent, the scope checks (through ExtractBlockReferences)
// and GetCalendarEvents all call it.
//
// It replaces a struct decode, which matched keys case-insensitively with the
// last match winning. Which duplicate came last changes when the content is
// re-marshalled (the block_ref_cleanup scrubs, Postgres jsonb), so the struct
// could check one source and later fetch another (issue #89). An exact key is
// the same key in any order. A field of the wrong JSON type is an error, as it
// was for the struct; null and a missing field leave the zero value.
func ReadCalendarSources(content json.RawMessage) ([]CalendarSource, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(content, &top); err != nil {
		return nil, err
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(orNull(top["calendars"]), &entries); err != nil {
		return nil, fmt.Errorf("calendars: %w", err)
	}
	cals := make([]CalendarSource, 0, len(entries))
	for i, e := range entries {
		var cal CalendarSource
		var source map[string]json.RawMessage
		for _, f := range []struct {
			key string
			dst any
		}{{"id", &cal.ID}, {"name", &cal.Name}, {"color", &cal.Color}, {"source", &source}} {
			if err := json.Unmarshal(orNull(e[f.key]), f.dst); err != nil {
				return nil, fmt.Errorf("calendars[%d].%s: %w", i, f.key, err)
			}
		}
		for _, f := range []struct {
			key string
			dst any
		}{{"type", &cal.Source.Type}, {"url", &cal.Source.URL}, {"resourceId", &cal.Source.ResourceID}} {
			if err := json.Unmarshal(orNull(source[f.key]), f.dst); err != nil {
				return nil, fmt.Errorf("calendars[%d].source.%s: %w", i, f.key, err)
			}
		}
		cals = append(cals, cal)
	}
	return cals, nil
}

// orNull stands in JSON null for a missing value, so decoding it leaves the
// destination at its zero value.
func orNull(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("null")
	}
	return raw
}
