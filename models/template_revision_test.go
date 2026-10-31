package models

import (
	"reflect"
	"strings"
	"testing"

	"mahresources/models/types"
)

var templateCarrierModels = map[string]any{
	TemplateCarrierCategory:         &Category{},
	TemplateCarrierResourceCategory: &ResourceCategory{},
	TemplateCarrierNoteType:         &NoteType{},
}

// A new Custom* slot is added to a carrier model often (seven in one release).
// If it is not listed in the tracked set, saves that change only that slot
// record no revision, and the history silently has holes. This fails when a
// template field is added to a model and not tracked.
func TestTemplateRevisionTrackedSetCoversEveryTemplateField(t *testing.T) {
	untracked := map[string]bool{"Name": true, "Description": true, "MetadataIndexes": true}
	special := map[string]bool{"MetaSchema": true, "SectionConfig": true, "AutoDetectRules": true, "ApplyTemplatesToShares": true}

	for carrierType, model := range templateCarrierModels {
		tracked := map[string]bool{}
		for _, f := range TemplateFields(carrierType) {
			tracked[f.Name] = true
		}
		rt := reflect.TypeOf(model).Elem()
		fields := map[string]bool{}
		for i := 0; i < rt.NumField(); i++ {
			name := rt.Field(i).Name
			fields[name] = true
			isTemplateField := strings.HasPrefix(name, "Custom") || special[name]
			if isTemplateField && !tracked[name] {
				t.Errorf("%s.%s is a template field but is not in the tracked set (models/template_revision_fields.go)", carrierType, name)
			}
			if untracked[name] && tracked[name] {
				t.Errorf("%s.%s must not be tracked: it identifies the carrier or reaches beyond it", carrierType, name)
			}
		}
		for name := range tracked {
			if !fields[name] {
				t.Errorf("%s tracks %q, which the model does not have", carrierType, name)
			}
		}
		// Each CSS sibling is part of its slot, so a slot named in the table
		// never needs a separate CSS entry to be caught.
		for _, slot := range TemplateSlots(carrierType) {
			if !tracked[slot.Name] {
				t.Errorf("%s slot %q is not tracked", carrierType, slot.Name)
			}
		}
	}
}

func snapshotWith(carrierType string, mutate func(any)) TemplateSnapshot {
	c := reflect.New(reflect.TypeOf(templateCarrierModels[carrierType]).Elem()).Interface()
	mutate(c)
	return NewTemplateSnapshot(carrierType, c)
}

// NULL scans as "null", and the resolvers fill defaults, so each of these spells
// "all defaults" and must compare equal; a real difference must not.
func TestSectionConfigCanonicalEquality(t *testing.T) {
	equalToDefaults := []string{
		"", "null", "{}", `{"tags":true}`, `{"ownEntities":[],"tags":false}`, `{"ownEntities":null}`, "not json",
	}
	for _, carrierType := range []string{TemplateCarrierCategory, TemplateCarrierResourceCategory, TemplateCarrierNoteType} {
		base := snapshotWith(carrierType, func(any) {})
		for _, raw := range equalToDefaults {
			raw := raw
			// {"ownEntities":[],"tags":false} resolves to defaults only for groups;
			// the other resolvers read "tags" directly. Use the resolver's own
			// verdict as the oracle instead of restating it.
			s := snapshotWith(carrierType, func(c any) {
				reflect.ValueOf(c).Elem().FieldByName("SectionConfig").Set(reflect.ValueOf(types.JSON(raw)))
			})
			want := ResolvedSectionConfigJSON(carrierType, base) == ResolvedSectionConfigJSON(carrierType, s)
			got := TemplateSnapshotsEqual(carrierType, base, s)
			if got != want {
				t.Errorf("%s SectionConfig %q: canonical equality %v, resolver says %v", carrierType, raw, got, want)
			}
		}
		for _, raw := range []string{"", "null", "{}"} {
			s := snapshotWith(carrierType, func(c any) {
				reflect.ValueOf(c).Elem().FieldByName("SectionConfig").Set(reflect.ValueOf(types.JSON(raw)))
			})
			if !TemplateSnapshotsEqual(carrierType, base, s) {
				t.Errorf("%s SectionConfig %q must equal the empty config", carrierType, raw)
			}
		}
	}

	// The two shapes the front-end merge gets wrong, for a Category.
	base := snapshotWith(TemplateCarrierCategory, func(any) {})
	for _, raw := range []string{`{"ownEntities":[],"tags":false}`, `{"ownEntities":null}`} {
		s := snapshotWith(TemplateCarrierCategory, func(c any) { c.(*Category).SectionConfig = types.JSON(raw) })
		if !TemplateSnapshotsEqual(TemplateCarrierCategory, base, s) {
			t.Errorf("Category SectionConfig %q resolves to all defaults in Go and must equal the empty config", raw)
		}
	}
	hidden := snapshotWith(TemplateCarrierCategory, func(c any) { c.(*Category).SectionConfig = types.JSON(`{"tags":false}`) })
	if TemplateSnapshotsEqual(TemplateCarrierCategory, base, hidden) {
		t.Error("hiding a section is a change")
	}
}

func TestJSONFieldsCompareWithoutKeyOrderNoise(t *testing.T) {
	a := snapshotWith(TemplateCarrierCategory, func(c any) { c.(*Category).MetaSchema = `{"type":"object","properties":{"a":{},"b":{}}}` })
	b := snapshotWith(TemplateCarrierCategory, func(c any) {
		c.(*Category).MetaSchema = "{\n  \"properties\": {\"b\": {}, \"a\": {}},\n  \"type\": \"object\"\n}"
	})
	if !TemplateSnapshotsEqual(TemplateCarrierCategory, a, b) {
		t.Error("key order and whitespace must not count as a change")
	}
	if d := DiffTemplateSnapshots(TemplateCarrierCategory, a, b); len(d) != 0 {
		t.Errorf("diff of reordered JSON = %+v, want none", d)
	}
}

func TestMalformedJSONFieldFallsBackToRawText(t *testing.T) {
	bad := snapshotWith(TemplateCarrierCategory, func(c any) { c.(*Category).MetaSchema = `{ not valid json ][` })
	good := snapshotWith(TemplateCarrierCategory, func(c any) { c.(*Category).MetaSchema = `{"type":"object"}` })
	diff := DiffTemplateSnapshots(TemplateCarrierCategory, bad, good)
	if len(diff) != 1 || diff[0].Slot != "MetaSchema" {
		t.Fatalf("diff = %+v, want one MetaSchema slot", diff)
	}
	var deleted []string
	for _, h := range diff[0].Fields[0].Hunks {
		if h.Op == DiffOpDelete {
			deleted = append(deleted, h.Lines...)
		}
	}
	if len(deleted) != 1 || deleted[0] != `{ not valid json ][` {
		t.Errorf("malformed text should be diffed raw, deleted = %q", deleted)
	}
}

func TestDiffLines(t *testing.T) {
	hunks := DiffLines("a\nb\nc\nd", "a\nB\nc\nd\ne")
	var got []string
	for _, h := range hunks {
		got = append(got, h.Op+":"+strings.Join(h.Lines, "|"))
	}
	want := []string{"equal:a", "delete:b", "insert:B", "equal:c|d", "insert:e"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("hunks = %v, want %v", got, want)
	}
	if h := DiffLines("same", "same"); len(h) != 1 || h[0].Op != DiffOpEqual {
		t.Errorf("identical input = %+v", h)
	}
	if h := DiffLines("", "x"); len(h) != 1 || h[0].Op != DiffOpInsert {
		t.Errorf("from empty = %+v", h)
	}
}

func TestChangedTemplateSlotsFoldsCSSIntoItsSlot(t *testing.T) {
	a := snapshotWith(TemplateCarrierNoteType, func(any) {})
	b := snapshotWith(TemplateCarrierNoteType, func(c any) {
		nt := c.(*NoteType)
		nt.CustomHeaderCSS = "x{}"
		nt.ApplyTemplatesToShares = true
	})
	got := ChangedTemplateSlots(TemplateCarrierNoteType, a, b)
	if strings.Join(got, ",") != "CustomHeader,ApplyTemplatesToShares" {
		t.Errorf("changed = %v", got)
	}
}

func TestApplyTemplateSnapshotClearsMissingKeys(t *testing.T) {
	c := &Category{CustomHeader: "h", CustomSidebar: "s", SectionConfig: types.JSON(`{"tags":false}`)}
	ApplyTemplateSnapshot(TemplateCarrierCategory, c, TemplateSnapshot{"CustomHeader": "old"})
	if c.CustomHeader != "old" || c.CustomSidebar != "" || len(c.SectionConfig) != 0 {
		t.Errorf("after apply: %+v", c)
	}
}

func TestSanitizeRevisionApplied(t *testing.T) {
	got := SanitizeRevisionApplied([]string{
		"preset:dashboard, generated", "copied:category:12", "copied:widget:1", "loaded:3", "loaded:3:CustomHeader",
		"loaded:x", "preset:", "<script>", "generated", "preset:../etc",
	})
	want := "preset:dashboard,generated,copied:category:12,loaded:3,loaded:3:CustomHeader"
	if strings.Join(got, ",") != want {
		t.Errorf("sanitized = %v", got)
	}
}
