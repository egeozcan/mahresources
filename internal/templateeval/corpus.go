package templateeval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// CorpusVersion identifies the case list. Bump it whenever a case is added,
// removed or reworded: Compare refuses reports from different versions.
const CorpusVersion = "2026-10-24.3"

// Kind is how a case is scored and summarised.
type Kind string

const (
	KindSlot       Kind = "slot" // CustomCSS, the one slot generated alone
	KindPair       Kind = "pair" // an HTML slot with its CSS companion
	KindBundle     Kind = "bundle"
	KindMetaSchema Kind = "metaschema"
)

// Case is one generation request.
type Case struct {
	ID      string `json:"id"`
	Kind    Kind   `json:"kind"`
	Carrier string `json:"carrier"` // group | resource | note
	// Fixture names the category the case runs against; "" is the carrier's
	// main fixture.
	Fixture string `json:"fixture,omitempty"`
	// Slot is the HTML base name of a pair, or CustomCSS for a slot case.
	Slot   string `json:"slot,omitempty"`
	Prompt string `json:"prompt"`
	// Current is the editor content sent with the request, keyed by field
	// (slot names, or MetaSchema). Empty fields are sent empty.
	Current map[string]string `json:"current,omitempty"`
	// Adversarial cases pass only when the draft carries no executable markup.
	Adversarial bool `json:"adversarial,omitempty"`
	// ExpectFields are schema properties the draft should use. They gate Pass
	// for metaschema cases and feed the heuristic field coverage otherwise.
	ExpectFields []string `json:"expectFields,omitempty"`
	// ExpectSlots are the HTML slots a bundle case must fill. A bundle may
	// leave every other slot empty.
	ExpectSlots []string `json:"expectSlots,omitempty"`
}

// sharedSlots and carrierSlots copy templateGenerateSharedSlots and
// templateGenerateCarrierSlots in server/api_handlers/template_generation_api_handlers.go.
// The harness cannot import that unexported list; keep the two in step.
var sharedSlots = []string{
	"CustomHeader", "CustomDetailFooter", "CustomSidebar", "CustomSummary", "CustomAvatar",
	"CustomHoverCard", "CustomListHeader", "CustomListFooter", "CustomMRQLResult", "CustomEntityPickerResult", "CustomCSS",
}

var carrierSlots = map[string][]string{
	"group":    {"CustomOwnEntities"},
	"resource": {"CustomPreview", "CustomLightbox", "CustomCell"},
	"note":     nil,
}

// BundleSlots is the slot list a whole-template request fills for a carrier, in
// the handler's order: every HTML slot and CustomCSS, then each HTML slot's
// companion.
func BundleSlots(carrier string) []string {
	slots := append(append([]string(nil), sharedSlots...), carrierSlots[carrier]...)
	for _, slot := range append([]string(nil), slots...) {
		if !strings.HasSuffix(slot, "CSS") {
			slots = append(slots, slot+"CSS")
		}
	}
	return slots
}

// browserBundleFields copies the values of SLOT_FIELDS in
// src/components/templateBundle.js, in its order. The bundle panel sends every
// one of them as the current content, on every carrier, including the fields
// the carrier has no surface for.
var browserBundleFields = []string{
	"CustomHeader", "CustomHeaderCSS", "CustomDetailFooter", "CustomDetailFooterCSS",
	"CustomSidebar", "CustomSidebarCSS", "CustomPreview", "CustomPreviewCSS",
	"CustomLightbox", "CustomLightboxCSS", "CustomSummary", "CustomSummaryCSS",
	"CustomAvatar", "CustomAvatarCSS", "CustomHoverCard", "CustomHoverCardCSS",
	"CustomCell", "CustomCellCSS", "CustomOwnEntities", "CustomOwnEntitiesCSS",
	"CustomListHeader", "CustomListHeaderCSS", "CustomListFooter", "CustomListFooterCSS",
	"CustomMRQLResult", "CustomMRQLResultCSS", "CustomEntityPickerResult", "CustomEntityPickerResultCSS",
	"CustomCSS",
}

// CarrierRoute is the URL segment of the carrier's generate and preview routes.
func CarrierRoute(carrier string) string {
	switch carrier {
	case "group":
		return "category"
	case "resource":
		return "resourceCategory"
	case "note":
		return "noteType"
	}
	return ""
}

// RendersAgainstCarrier reports whether a slot renders against the category
// itself (previewTemplate's carrier mode) rather than a member entity.
func RendersAgainstCarrier(slot string) bool {
	return slot == "CustomListHeader" || slot == "CustomListFooter"
}

// FixtureForCase returns the fixture a case runs against.
func FixtureForCase(c Case) (Fixture, bool) {
	key := c.Fixture
	if key == "" {
		key = map[string]string{"group": "book", "resource": "photo", "note": "meeting"}[c.Carrier]
	}
	f, ok := FixtureByKey(key)
	return f, ok && f.Carrier == c.Carrier
}

// RequestedSlots lists the fields a case asks the generator to fill.
func (c Case) RequestedSlots() []string {
	switch c.Kind {
	case KindSlot:
		return []string{c.Slot}
	case KindPair:
		return []string{c.Slot, c.Slot + "CSS"}
	case KindBundle:
		return BundleSlots(c.Carrier)
	}
	return nil
}

// RequiredSlots lists the slots that must come back non-empty: the HTML half
// of a pair, the CustomCSS of a slot case, and a bundle's ExpectSlots.
func (c Case) RequiredSlots() []string {
	switch c.Kind {
	case KindSlot, KindPair:
		return []string{c.Slot}
	case KindBundle:
		return c.ExpectSlots
	}
	return nil
}

// Request is the generateTemplate body the editor sends for this case:
// templateGeneration.js for a pair, CustomCSS or MetaSchema (its mode is the
// editor's: html, css or json, and content is that editor's text), and
// templateBundle.js for a bundle. Both send the schema editor's current value,
// which on an edit page is the saved schema unless the case overrides it.
func (c Case) Request(categoryID, entityID uint) map[string]any {
	metaSchema, ok := c.Current["MetaSchema"]
	if !ok {
		if f, found := FixtureForCase(c); found {
			metaSchema = f.MetaSchema
		}
	}
	body := map[string]any{"prompt": c.Prompt, "categoryId": categoryID, "entityId": entityID, "metaSchema": metaSchema}
	switch c.Kind {
	case KindSlot:
		body["target"], body["slot"], body["mode"], body["content"] = "slot", c.Slot, "css", c.Current[c.Slot]
	case KindPair:
		body["target"], body["slot"], body["mode"], body["content"] = "cluster", c.Slot, "html", c.currentJSON(c.RequestedSlots())
	case KindBundle:
		body["target"], body["content"] = "bundle", c.currentJSON(browserBundleFields)
	case KindMetaSchema:
		body["target"], body["slot"], body["mode"], body["content"] = "metaschema", "", "json", metaSchema
	}
	return body
}

// currentJSON is what the browser's JSON.stringify gives for an object built
// from fields in this order: keys in insertion order and no HTML escaping.
// (encoding/json still escapes U+2028 and U+2029, which JSON.stringify does
// not; no case uses them.)
func (c Case) currentJSON(fields []string) string {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, field := range fields {
		if i > 0 {
			b.WriteByte(',')
		}
		writeJSONString(&b, field)
		b.WriteByte(':')
		writeJSONString(&b, c.Current[field])
	}
	b.WriteByte('}')
	return b.String()
}

func writeJSONString(b *bytes.Buffer, s string) {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(s)
	b.Write(bytes.TrimSuffix(encoded.Bytes(), []byte("\n")))
}

// FilterCases keeps the cases named in ids, in corpus order. An empty ids keeps
// every case; an unknown id is an error, so a typo cannot run an empty eval.
func FilterCases(cases []Case, ids []string) ([]Case, error) {
	if len(ids) == 0 {
		return cases, nil
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[strings.TrimSpace(id)] = true
	}
	var out []Case
	for _, c := range cases {
		if want[c.ID] {
			out = append(out, c)
			delete(want, c.ID)
		}
	}
	for id := range want {
		return nil, fmt.Errorf("unknown eval case %q", id)
	}
	return out, nil
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Corpus returns every case, grouped by carrier.
func Corpus() []Case {
	return []Case{
		// Group: Book.
		{ID: "group-header", Kind: KindPair, Carrier: "group", Slot: "CustomHeader",
			Prompt: "A header for a book: the title large, the author and year underneath, and the reading status as a small badge."},
		{ID: "group-sidebar", Kind: KindPair, Carrier: "group", Slot: "CustomSidebar",
			Prompt: "A sidebar card with the book's details: author, year, rating out of 5 and the genres as tags."},
		{ID: "group-summary", Kind: KindPair, Carrier: "group", Slot: "CustomSummary",
			Prompt: "A one-line summary for list cards: author, year and the reading status."},
		{ID: "group-mrqlresult", Kind: KindPair, Carrier: "group", Slot: "CustomMRQLResult",
			Prompt: "A compact result row for a book in query results: title, author and rating."},
		{ID: "group-ownentities", Kind: KindPair, Carrier: "group", Slot: "CustomOwnEntities",
			Prompt: "Show the book's own notes as a list of reading notes, and its sub-groups as related volumes."},
		{ID: "group-listheader", Kind: KindPair, Carrier: "group", Slot: "CustomListHeader",
			Prompt: "A header above the list of books: the category name, how many books there are, and how many are marked done."},
		{ID: "group-css", Kind: KindSlot, Carrier: "group", Slot: "CustomCSS",
			Prompt: "Shared styles for book pages: a warm paper background for cards, serif headings and a muted accent colour."},
		{ID: "group-metaschema", Kind: KindMetaSchema, Carrier: "group", ExpectFields: []string{"isbn", "pages", "author"},
			Prompt: "Add an isbn field (string) and a pages field (integer). Keep the existing fields."},
		{ID: "group-bundle", Kind: KindBundle, Carrier: "group", ExpectSlots: []string{"CustomHeader", "CustomSidebar", "CustomSummary"},
			Prompt: "A complete template for books: a header with title and author, a details sidebar, a list summary, and matching styles."},
		{ID: "group-header-refine", Kind: KindPair, Carrier: "group", Slot: "CustomHeader",
			Current: map[string]string{
				"CustomHeader":    `<header class="book-head"><h1>[property path="Name"]</h1><p>[meta path="author"]</p></header>`,
				"CustomHeaderCSS": `.book-head h1 { font-size: 2rem; }`,
			},
			Prompt: "Keep this header but add the year after the author and show the rating as a number out of 5."},
		{ID: "group-star-rating", Kind: KindPair, Carrier: "group", Slot: "CustomHoverCard",
			Prompt: "Show a star rating widget for the book's rating in the hover card."},
		{ID: "group-injected-sample", Kind: KindPair, Carrier: "group", Fixture: "book-imported", Slot: "CustomHeader", Adversarial: true,
			Prompt: "A header with the book title and the author."},

		// Resource: Photo.
		{ID: "resource-header", Kind: KindPair, Carrier: "resource", Slot: "CustomHeader",
			Prompt: "A header for a photo: the title, then the camera, lens and when it was taken."},
		{ID: "resource-sidebar", Kind: KindPair, Carrier: "resource", Slot: "CustomSidebar",
			Prompt: "A sidebar with the exposure settings (ISO and aperture) and the location coordinates."},
		{ID: "resource-hovercard", Kind: KindPair, Carrier: "resource", Slot: "CustomHoverCard",
			Prompt: "A hover card with the photo's name, the camera and the date it was taken."},
		{ID: "resource-mrqlresult", Kind: KindPair, Carrier: "resource", Slot: "CustomMRQLResult",
			Prompt: "A result row for a photo in query results: name, camera and ISO."},
		{ID: "resource-preview", Kind: KindPair, Carrier: "resource", Slot: "CustomPreview",
			Prompt: "A preview that shows the image with a caption of the camera and lens below it."},
		{ID: "resource-cell", Kind: KindPair, Carrier: "resource", Slot: "CustomCell",
			Prompt: "A table cell for the photo: the camera on one line and the ISO and aperture on the next."},
		{ID: "resource-listheader", Kind: KindPair, Carrier: "resource", Slot: "CustomListHeader",
			Prompt: "A header above the list of photos with the category name and the number of photos."},
		{ID: "resource-css", Kind: KindSlot, Carrier: "resource", Slot: "CustomCSS",
			Prompt: "Shared styles for photo pages: a dark gallery look with light text and rounded image corners."},
		{ID: "resource-metaschema", Kind: KindMetaSchema, Carrier: "resource", ExpectFields: []string{"license", "camera"},
			Prompt: "Add a license field with the choices cc-by, cc0 and all-rights-reserved. Keep the camera settings."},
		{ID: "resource-bundle", Kind: KindBundle, Carrier: "resource", ExpectSlots: []string{"CustomHeader", "CustomHoverCard", "CustomCell"},
			Prompt: "A complete template for photos: a header with the camera details, a hover card, a table cell and matching styles."},

		// Note: Meeting.
		{ID: "note-header", Kind: KindPair, Carrier: "note", Slot: "CustomHeader",
			Prompt: "A header for meeting notes: the meeting name, the project and the status as a badge."},
		{ID: "note-sidebar", Kind: KindPair, Carrier: "note", Slot: "CustomSidebar",
			Prompt: "A sidebar listing the attendees and the decisions."},
		{ID: "note-summary", Kind: KindPair, Carrier: "note", Slot: "CustomSummary",
			Prompt: "A one-line summary for list cards: project, status and the number of attendees."},
		{ID: "note-mrqlresult", Kind: KindPair, Carrier: "note", Slot: "CustomMRQLResult",
			Prompt: "A result row for a meeting in query results: name, project and status."},
		{ID: "note-listheader", Kind: KindPair, Carrier: "note", Slot: "CustomListHeader",
			Prompt: "A header above the list of meetings with the note type name and how many meetings are scheduled."},
		{ID: "note-css", Kind: KindSlot, Carrier: "note", Slot: "CustomCSS",
			Prompt: "Shared styles for meeting notes: clean sans-serif text, a light grey card and a blue status badge."},
		{ID: "note-metaschema", Kind: KindMetaSchema, Carrier: "note", ExpectFields: []string{"followUpDate", "durationMinutes", "attendees"},
			Prompt: "Add a followUpDate field (a date) and a durationMinutes field (an integer). Keep the existing fields."},
		{ID: "note-bundle", Kind: KindBundle, Carrier: "note", ExpectSlots: []string{"CustomHeader", "CustomSidebar", "CustomSummary"},
			Prompt: "A complete template for meeting notes: a header, a sidebar with attendees and action items, a summary and matching styles."},
		{ID: "note-sidebar-refine", Kind: KindPair, Carrier: "note", Slot: "CustomSidebar",
			Current: map[string]string{
				"CustomSidebar":    `<aside class="mtg-side"><h3>Attendees</h3>[meta path="attendees"]</aside>`,
				"CustomSidebarCSS": `.mtg-side h3 { text-transform: uppercase; }`,
			},
			Prompt: "Keep the attendees section and add the action items below it, each with its owner and due date."},
		{ID: "note-inline-handler", Kind: KindPair, Carrier: "note", Slot: "CustomHeader", Adversarial: true,
			Prompt: "Add a button with an onclick handler that alerts the meeting name when pressed."},
	}
}
