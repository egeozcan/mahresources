package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"mahresources/models/types"
)

// TemplateFieldKind says how a tracked field is compared and diffed.
type TemplateFieldKind int

const (
	// TemplateFieldText is a string compared as it is.
	TemplateFieldText TemplateFieldKind = iota
	// TemplateFieldJSONText is a string that usually holds JSON (MetaSchema,
	// AutoDetectRules): compared as sorted-key pretty-printed JSON when it
	// parses, as raw text when it does not.
	TemplateFieldJSONText
	// TemplateFieldSectionConfig is a types.JSON compared through the section
	// config resolver, so NULL, {}, a sparse config and one restating defaults
	// are equal.
	TemplateFieldSectionConfig
	// TemplateFieldBool is a bool.
	TemplateFieldBool
)

// TemplateField is one tracked column of a Template Carrier.
type TemplateField struct {
	Name string
	Kind TemplateFieldKind
}

// TemplateSlot groups the fields that belong together: a Custom* template and
// its *CSS sibling, or a lone field (MetaSchema, SectionConfig, ...). The
// first field is the slot's primary one.
type TemplateSlot struct {
	Name   string
	Fields []TemplateField
}

func templateSlot(name string) TemplateSlot {
	return TemplateSlot{Name: name, Fields: []TemplateField{
		{Name: name, Kind: TemplateFieldText},
		{Name: name + "CSS", Kind: TemplateFieldText},
	}}
}

func loneSlot(name string, kind TemplateFieldKind) TemplateSlot {
	return TemplateSlot{Name: name, Fields: []TemplateField{{Name: name, Kind: kind}}}
}

// The tracked set per carrier type lives here and nowhere else. Not tracked:
// Name (identifies the carrier, and is unique), Description, and
// MetadataIndexes (builds indexes shared by every carrier of an entity type).
// TestTemplateRevisionTrackedSetCoversEveryTemplateField fails when a Custom*
// field is added to a carrier model and not listed here.
var (
	categoryTemplateSlots = []TemplateSlot{
		templateSlot("CustomHeader"), templateSlot("CustomDetailFooter"), templateSlot("CustomSidebar"),
		templateSlot("CustomSummary"), templateSlot("CustomAvatar"), templateSlot("CustomHoverCard"),
		templateSlot("CustomOwnEntities"), templateSlot("CustomListHeader"), templateSlot("CustomListFooter"),
		templateSlot("CustomMRQLResult"), templateSlot("CustomEntityPickerResult"),
		loneSlot("CustomCSS", TemplateFieldText),
		loneSlot("MetaSchema", TemplateFieldJSONText),
		loneSlot("SectionConfig", TemplateFieldSectionConfig),
	}
	resourceCategoryTemplateSlots = []TemplateSlot{
		templateSlot("CustomHeader"), templateSlot("CustomDetailFooter"), templateSlot("CustomSidebar"),
		templateSlot("CustomPreview"), templateSlot("CustomLightbox"), templateSlot("CustomSummary"),
		templateSlot("CustomAvatar"), templateSlot("CustomHoverCard"), templateSlot("CustomCell"),
		templateSlot("CustomListHeader"), templateSlot("CustomListFooter"),
		templateSlot("CustomMRQLResult"), templateSlot("CustomEntityPickerResult"),
		loneSlot("CustomCSS", TemplateFieldText),
		loneSlot("MetaSchema", TemplateFieldJSONText),
		loneSlot("AutoDetectRules", TemplateFieldJSONText),
		loneSlot("SectionConfig", TemplateFieldSectionConfig),
	}
	noteTypeTemplateSlots = []TemplateSlot{
		templateSlot("CustomHeader"), templateSlot("CustomDetailFooter"), templateSlot("CustomSidebar"),
		templateSlot("CustomSummary"), templateSlot("CustomAvatar"), templateSlot("CustomHoverCard"),
		templateSlot("CustomListHeader"), templateSlot("CustomListFooter"),
		loneSlot("ApplyTemplatesToShares", TemplateFieldBool),
		templateSlot("CustomMRQLResult"), templateSlot("CustomEntityPickerResult"),
		loneSlot("CustomCSS", TemplateFieldText),
		loneSlot("MetaSchema", TemplateFieldJSONText),
		loneSlot("SectionConfig", TemplateFieldSectionConfig),
	}
)

// TemplateSlots returns the tracked slots of a carrier type, in display order,
// or nil for an unknown type.
func TemplateSlots(carrierType string) []TemplateSlot {
	switch carrierType {
	case TemplateCarrierCategory:
		return categoryTemplateSlots
	case TemplateCarrierResourceCategory:
		return resourceCategoryTemplateSlots
	case TemplateCarrierNoteType:
		return noteTypeTemplateSlots
	}
	return nil
}

// TemplateFields returns every tracked field of a carrier type, flattened.
func TemplateFields(carrierType string) []TemplateField {
	var out []TemplateField
	for _, s := range TemplateSlots(carrierType) {
		out = append(out, s.Fields...)
	}
	return out
}

// FindTemplateSlot returns the named slot of a carrier type.
func FindTemplateSlot(carrierType, name string) (TemplateSlot, bool) {
	for _, s := range TemplateSlots(carrierType) {
		if s.Name == name {
			return s, true
		}
	}
	return TemplateSlot{}, false
}

// TemplateSnapshot is the raw tracked state of a carrier: field name to the
// string (or bool) stored in the column, malformed or not. A key a snapshot
// lacks (a slot added after it was taken) reads as the empty value.
type TemplateSnapshot map[string]any

// NewTemplateSnapshot reads the tracked fields off a pointer to a carrier.
func NewTemplateSnapshot(carrierType string, carrier any) TemplateSnapshot {
	v := reflect.ValueOf(carrier).Elem()
	snap := TemplateSnapshot{}
	for _, f := range TemplateFields(carrierType) {
		fv := v.FieldByName(f.Name)
		switch f.Kind {
		case TemplateFieldBool:
			snap[f.Name] = fv.Bool()
		case TemplateFieldSectionConfig:
			snap[f.Name] = string(fv.Interface().(types.JSON))
		default:
			snap[f.Name] = fv.String()
		}
	}
	return snap
}

// ApplyTemplateSnapshot writes every tracked field of snap onto a pointer to a
// carrier. A field the snapshot lacks is cleared, because restoring a
// snapshot reinstates the exact state it recorded.
func ApplyTemplateSnapshot(carrierType string, carrier any, snap TemplateSnapshot) {
	v := reflect.ValueOf(carrier).Elem()
	for _, f := range TemplateFields(carrierType) {
		setSnapshotField(v.FieldByName(f.Name), f, snap)
	}
}

// ApplyTemplateSlot writes one slot of snap onto a carrier, and its CSS
// sibling too when includeCSS is set.
func ApplyTemplateSlot(carrier any, slot TemplateSlot, snap TemplateSnapshot, includeCSS bool) {
	v := reflect.ValueOf(carrier).Elem()
	for i, f := range slot.Fields {
		if i > 0 && !includeCSS {
			break
		}
		setSnapshotField(v.FieldByName(f.Name), f, snap)
	}
}

func setSnapshotField(fv reflect.Value, f TemplateField, snap TemplateSnapshot) {
	switch f.Kind {
	case TemplateFieldBool:
		b, _ := snap[f.Name].(bool)
		fv.SetBool(b)
	case TemplateFieldSectionConfig:
		s, _ := snap[f.Name].(string)
		if s == "" {
			fv.Set(reflect.ValueOf(types.JSON(nil)))
		} else {
			fv.Set(reflect.ValueOf(types.JSON(s)))
		}
	default:
		s, _ := snap[f.Name].(string)
		fv.SetString(s)
	}
}

// ParseTemplateSnapshot decodes a stored Snapshot.
func ParseTemplateSnapshot(raw types.JSON) (TemplateSnapshot, error) {
	snap := TemplateSnapshot{}
	if len(raw) == 0 {
		return snap, nil
	}
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, fmt.Errorf("template revision snapshot is not valid JSON: %w", err)
	}
	if snap == nil {
		snap = TemplateSnapshot{}
	}
	return snap, nil
}

// Marshal encodes the snapshot for storage.
func (s TemplateSnapshot) Marshal() (types.JSON, error) {
	b, err := json.Marshal(map[string]any(s))
	return types.JSON(b), err
}

// CanonicalTemplateField is the view change detection and diffs compare. It
// never fails: input it cannot interpret comes back as raw text.
func CanonicalTemplateField(carrierType string, f TemplateField, snap TemplateSnapshot) string {
	switch f.Kind {
	case TemplateFieldBool:
		b, _ := snap[f.Name].(bool)
		return strconv.FormatBool(b)
	case TemplateFieldJSONText:
		s, _ := snap[f.Name].(string)
		return canonicalJSONText(s)
	case TemplateFieldSectionConfig:
		return canonicalJSONText(ResolvedSectionConfigJSON(carrierType, snap))
	}
	s, _ := snap[f.Name].(string)
	return s
}

// ResolvedSectionConfigJSON is the snapshot's SectionConfig as the Go resolver
// sees it, re-marshalled. It is what the editor must be loaded with: the
// front-end merge in sectionConfigForm.js does not behave like these resolvers.
func ResolvedSectionConfigJSON(carrierType string, snap TemplateSnapshot) string {
	s, _ := snap["SectionConfig"].(string)
	raw := types.JSON(s)
	var resolved any
	switch carrierType {
	case TemplateCarrierCategory:
		resolved = ResolveGroupSectionConfig(&raw)
	case TemplateCarrierResourceCategory:
		resolved = ResolveResourceSectionConfig(&raw)
	case TemplateCarrierNoteType:
		resolved = ResolveNoteSectionConfig(&raw)
	default:
		return s
	}
	b, err := json.Marshal(resolved)
	if err != nil {
		return s
	}
	return string(b)
}

func canonicalJSONText(raw string) string {
	if len(bytes.TrimSpace([]byte(raw))) == 0 {
		return raw
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return raw
	}
	if _, err := dec.Token(); err != io.EOF {
		return raw
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return raw
	}
	return string(bytes.TrimRight(buf.Bytes(), "\n"))
}

// TemplateSnapshotsEqual reports whether two snapshots are the same under the
// canonical view.
func TemplateSnapshotsEqual(carrierType string, a, b TemplateSnapshot) bool {
	return len(ChangedTemplateSlots(carrierType, a, b)) == 0
}

// ChangedTemplateSlots names the slots whose canonical content differs.
func ChangedTemplateSlots(carrierType string, before, after TemplateSnapshot) []string {
	changed := []string{}
	for _, slot := range TemplateSlots(carrierType) {
		for _, f := range slot.Fields {
			if CanonicalTemplateField(carrierType, f, before) != CanonicalTemplateField(carrierType, f, after) {
				changed = append(changed, slot.Name)
				break
			}
		}
	}
	return changed
}

var revisionAppliedPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^preset:[A-Za-z0-9_.-]{1,100}$`),
	regexp.MustCompile(`^generated$`),
	regexp.MustCompile(`^copied:(category|resourceCategory|noteType):[0-9]{1,18}$`),
	regexp.MustCompile(`^loaded:[0-9]{1,9}(:[A-Za-z0-9]{1,64})?$`),
}

// SanitizeRevisionApplied keeps only the revisionApplied entries the server
// recognises, in order and without duplicates. Each submitted value may itself
// be a comma-separated list.
func SanitizeRevisionApplied(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range values {
		for _, part := range splitComma(v) {
			if seen[part] {
				continue
			}
			for _, re := range revisionAppliedPatterns {
				if re.MatchString(part) {
					seen[part] = true
					out = append(out, part)
					break
				}
			}
		}
	}
	return out
}

func splitComma(s string) []string {
	var parts []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}
