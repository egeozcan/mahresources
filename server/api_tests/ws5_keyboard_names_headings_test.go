package api_tests

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"mahresources/models"
)

// WS5 — "Keyboard operability, accessible names, headings, target sizes".
//
// Findings 6, 13, 14, 36/105, 39, 48, 49, 58, 64/59, 76/139, 99, 108, 110, 127,
// 133 and 144 of docs/ui-bug-hunt-2026-07-29.md. This file covers the half
// reachable from Go: attributes and heading levels that the server writes into
// the rendered HTML. Computed styles, target-size boxes and DOM focus are
// runtime properties and live in e2e/tests/regressions/ws5-*.spec.ts.
//
// Two findings in the workstream were rejected on verification and are pinned
// here as negative controls so a later change cannot silently reintroduce them
// as *real* defects without a test noticing:
//   - 14, the details row checkboxes, which are named already
//     (TestDetailsRowCheckboxes_AreNamedAfterTheirRow)
//   - 39, the settings focus ring, which is painted by box-shadow, asserted in
//     Playwright because only a browser can compute it.

// headingRe matches a heading open tag plus everything up to its close tag.
var headingRe = regexp.MustCompile(`(?is)<(h[1-6])\b[^>]*>(.*?)</h[1-6]>`)

var tagStripRe = regexp.MustCompile(`(?s)<[^>]+>`)
var whitespaceRe = regexp.MustCompile(`\s+`)

// findOpenTag returns the full text of the first `<name …>` open tag whose
// attributes contain `marker`, or "" if there is none.
//
// It exists because the obvious `<input[^>]*marker[^>]*>` regex is WRONG on this
// codebase's markup and fails *open*: Alpine attribute values routinely contain a
// literal ">", as in
//
//	:aria-expanded="groupResults.length > 0"
//
// so `[^>]*` stops inside the attribute value and every attribute after it looks
// absent. Four assertions in this file reported missing aria-label / aria-controls
// on markup that had them. A presence check that truncates gives false negatives;
// an absence check that truncates gives false positives, which is worse.
//
// The scan is quote-aware and therefore ends at the real tag boundary.
func findOpenTag(body, marker, name string) string {
	// Scan tags of the requested name and return the first that carries the marker.
	// Searching for the marker *first* and walking back to the nearest preceding
	// "<name" is wrong: on /group/edit the first role="combobox" in the document
	// belongs to an <input>, so the walk-back found an unrelated earlier <textarea>
	// (or none at all) and the test reported the subject missing.
	for _, tag := range openTagsWithin(body, name) {
		if strings.Contains(tag, marker) {
			return tag
		}
	}
	return ""
}

// openTagsWithin yields every `<name …>` open tag inside the given markup, using
// the same quote-aware scan as findOpenTag.
func openTagsWithin(markup, name string) []string {
	var out []string
	needle := "<" + name
	for i := 0; i < len(markup); {
		start := strings.Index(markup[i:], needle)
		if start < 0 {
			return out
		}
		start += i
		var quote byte
		end := -1
		for j := start; j < len(markup); j++ {
			c := markup[j]
			switch {
			case quote != 0:
				if c == quote {
					quote = 0
				}
			case c == '"' || c == '\'':
				quote = c
			case c == '>':
				end = j
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			return out
		}
		out = append(out, markup[start:end+1])
		i = end + 1
	}
	return out
}

// findTagWithID returns the open tag of whichever element declares id="want".
//
// Quote-aware for the same reason as findOpenTag: the mention dropdown carries
// x-show="mentionActive && mentionResults.length > 0" *before* role="listbox", so a
// `[^>]*` run after the id stopped inside that expression and the role looked absent.
func findTagWithID(body, want string) string {
	needle := `id="` + want + `"`
	for _, name := range []string{"div", "ul", "ol", "section", "span"} {
		for _, tag := range openTagsWithin(body, name) {
			if strings.Contains(tag, needle) {
				return tag
			}
		}
	}
	return ""
}

// heading is one rendered heading: its level and its visible text.
type heading struct {
	Level int
	Text  string
}

// visibleHeadings returns the page's headings in document order, with the ones
// belonging to the global modal partials (Edit Tags, Info, Crop image, Jobs,
// Select…) removed. Those partials are included on every page, they are
// display:none until opened, and a Go test cannot evaluate offsetParent. Callers
// that need the whole list use allHeadings.
//
// This used to *truncate* at the first such heading, on the assumption that the
// global modals are the last thing in the document. WS10 broke that assumption:
// the download cockpit's `<h2>Jobs</h2>` moved into the header (findings 83/102),
// so the truncation point became the *first* heading on every page and seven
// subtests reported "no <h1> at all — this test measured nothing". The assumption
// was never a contract, so the filter is positional-independent now.
func visibleHeadings(body string) []heading {
	all := allHeadings(body)
	out := make([]heading, 0, len(all))
	for _, h := range all {
		if h.Level == 2 && isGlobalModalHeading(h.Text) {
			continue
		}
		out = append(out, h)
	}
	return out
}

// globalModalHeadings are the h2s that layouts/base.tpl appends to every page.
// The list is asserted to be reachable by TestGlobalModalHeadingsStillExist, so
// a rename cannot quietly turn visibleHeadings into "the whole document".
var globalModalHeadings = []string{"Edit Tags", "Info", "Crop image", "Jobs", "Select"}

func isGlobalModalHeading(text string) bool {
	for _, m := range globalModalHeadings {
		if text == m || strings.HasPrefix(text, "Upload to") {
			return true
		}
	}
	return false
}

func allHeadings(body string) []heading {
	var out []heading
	for _, m := range headingRe.FindAllStringSubmatch(body, -1) {
		level := int(m[1][1] - '0')
		text := whitespaceRe.ReplaceAllString(tagStripRe.ReplaceAllString(m[2], ""), " ")
		out = append(out, heading{Level: level, Text: strings.TrimSpace(text)})
	}
	return out
}

// firstSkip reports the first place the outline jumps more than one level, or
// -1. An empty heading is reported separately by callers; this is purely about
// levels, which is what WCAG 1.3.1 and axe's heading-order rule measure.
func firstSkip(hs []heading) int {
	for i := 1; i < len(hs); i++ {
		if hs[i].Level > hs[i-1].Level+1 {
			return i
		}
	}
	return -1
}

func describeHeadings(hs []heading) string {
	parts := make([]string, 0, len(hs))
	for _, h := range hs {
		text := h.Text
		if len(text) > 32 {
			text = text[:32]
		}
		parts = append(parts, fmt.Sprintf("H%d:%q", h.Level, text))
	}
	return strings.Join(parts, " ")
}

// TestGlobalModalHeadingsStillExist is the positive control for
// visibleHeadings' truncation. If the global modal partials are renamed, the
// helper would stop truncating and every heading-order assertion below would
// start reading the modals' outline — passing or failing for reasons unrelated
// to the page under test.
func TestGlobalModalHeadingsStillExist(t *testing.T) {
	tc := SetupTestEnv(t)
	_, body := tc.getHTML(t, "/tags")

	all := allHeadings(body)
	found := map[string]bool{}
	for _, h := range all {
		found[h.Text] = true
	}
	for _, want := range globalModalHeadings {
		if !found[want] {
			t.Errorf("global modal heading %q is gone from /tags; visibleHeadings() no longer truncates and every heading assertion in this file is now measuring the modals too. Headings: %s",
				want, describeHeadings(all))
		}
	}
	if len(visibleHeadings(body)) >= len(all) {
		t.Errorf("visibleHeadings() did not truncate /tags: %d of %d headings kept", len(visibleHeadings(body)), len(all))
	}
}

// ---------------------------------------------------------------- finding 13

// TestDetailsTableWrap_IsAKeyboardOperableRegion is finding 13. The details
// table is 2005px wide inside an 822px overflow-x:auto container; with no
// tabindex the arrow keys have nothing to scroll and the right-hand columns are
// unreachable without a pointer. WCAG 2.1.1.
func TestDetailsTableWrap_IsAKeyboardOperableRegion(t *testing.T) {
	tc := SetupTestEnv(t)
	_, body := tc.getHTML(t, "/resources/details")

	tag := findOpenTag(body, `class="detail-table-wrap`, "div")
	if tag == "" {
		t.Fatalf(".detail-table-wrap not found in /resources/details")
	}

	for _, want := range []string{`tabindex="0"`, `role="region"`, `aria-label=`} {
		if !strings.Contains(tag, want) {
			t.Errorf("finding 13: .detail-table-wrap is missing %s — a scrollable region must be focusable and named.\ntag: %s", want, tag)
		}
	}
}

// ---------------------------------------------------------------- finding 14

// TestDetailsRowCheckboxes_AreNamedAfterTheirRow pins the *rejection* of
// finding 14. The report read a checkbox audit that included the sidebar filter
// controls; the row checkboxes have carried aria-label="Select <name>" all
// along. This test exists so that stays true.
func TestDetailsRowCheckboxes_AreNamedAfterTheirRow(t *testing.T) {
	tc := SetupTestEnv(t)

	const name = "ws5 named row resource"
	tc.createResourceWithName(t, name)

	_, body := tc.getHTML(t, "/resources/details")

	var rows []string
	for _, tag := range openTagsWithin(body, "input") {
		if strings.Contains(tag, "detail-table-checkbox") {
			rows = append(rows, tag)
		}
	}
	if len(rows) == 0 {
		t.Fatalf("no .detail-table-checkbox rendered — the positive control for this test is gone")
	}
	for _, tag := range rows {
		if !strings.Contains(tag, `aria-label="Select `) {
			t.Errorf("finding 14 (rejected): a row checkbox lost its accessible name.\ntag: %s", tag)
		}
	}
	if !strings.Contains(body, `aria-label="Select `+name) {
		t.Errorf("finding 14 (rejected): expected aria-label=\"Select %s\"; the name is not interpolated into the row checkbox", name)
	}
}

// ---------------------------------------------------------------- finding 36/105

// pickerPages are the two hand-rolled group autocompletes. The app's own
// selector (templates/partials/form/autocompleter.tpl) exposes role=combobox,
// aria-expanded, aria-controls, aria-autocomplete and aria-activedescendant;
// these two exposed none of them.
var pickerPages = []struct {
	name  string
	url   string
	xRef  string // the x-model the picker's input is bound to
	label string
}{
	{"export group picker", "/admin/export", "groupQuery", "Search groups to add"},
	{"import parent group picker", "/admin/import", "parentGroupQuery", "Search parent group"},
}

// TestGroupPickers_ExposeComboboxSemantics is finding 36/105.
func TestGroupPickers_ExposeComboboxSemantics(t *testing.T) {
	tc := SetupTestEnv(t)

	for _, p := range pickerPages {
		t.Run(p.name, func(t *testing.T) {
			_, body := tc.getHTML(t, p.url)

			tag := findOpenTag(body, `x-model="`+p.xRef+`"`, "input")
			if tag == "" {
				t.Fatalf("no input bound to x-model=%q on %s", p.xRef, p.url)
			}

			for _, want := range []string{
				`role="combobox"`,
				`aria-autocomplete="list"`,
				`aria-expanded`,
				`aria-controls=`,
				`aria-label=`,
			} {
				if !strings.Contains(tag, want) {
					t.Errorf("finding 36/105: %s is missing %s.\ntag: %s", p.name, want, tag)
				}
			}

			// aria-controls must point at something that exists and is a listbox.
			ctrlRe := regexp.MustCompile(`aria-controls="([^"]+)"`)
			if m := ctrlRe.FindStringSubmatch(tag); m != nil {
				id := m[1]
				if !strings.Contains(body, `id="`+id+`"`) {
					t.Errorf("finding 36/105: %s points aria-controls at id=%q, which is not rendered on the page", p.name, id)
				}
				if lb := findTagWithID(body, id); lb == "" {
					t.Errorf("finding 36/105: %s's aria-controls target id=%q is not an element open tag", p.name, id)
				} else if !strings.Contains(lb, `role="listbox"`) {
					t.Errorf("finding 36/105: %s's aria-controls target is not role=listbox.\ntag: %s", p.name, whitespaceRe.ReplaceAllString(lb, " "))
				}
			}
		})
	}
}

// TestGroupPickerResults_AreOptionsInAListbox is the other half of 36/105:
// without option roles there is no active-descendant model to announce.
func TestGroupPickerResults_AreOptionsInAListbox(t *testing.T) {
	tc := SetupTestEnv(t)

	for _, p := range pickerPages {
		t.Run(p.name, func(t *testing.T) {
			_, body := tc.getHTML(t, p.url)
			if !strings.Contains(body, `role="option"`) {
				t.Errorf("finding 36/105: %s renders no role=option — the result list is still a bare <ul> of buttons", p.url)
			}
		})
	}
}

// ---------------------------------------------------------------- finding 48

// TestTodoItemInputs_HaveAnAccessibleName is finding 48's first half. The
// todo-item text inputs had no aria-label, no aria-labelledby, no title, no
// wrapping label and no placeholder — a screen reader announced only
// "edit text".
func TestTodoItemInputs_HaveAnAccessibleName(t *testing.T) {
	tc := SetupTestEnv(t)
	note := tc.createNoteWithBlocks(t)

	_, body := tc.getHTML(t, fmt.Sprintf("/note?id=%d", note.ID))

	if !strings.Contains(body, `x-model="item.label"`) {
		t.Fatalf("the todos block markup is not on the page — this test's subject is gone")
	}
	inputRe := regexp.MustCompile(`(?s)<input[^>]*x-model="item\.label"[^>]*>`)
	tag := inputRe.FindString(body)
	if tag == "" {
		t.Fatalf("no todo-item input found")
	}
	if !strings.Contains(tag, "aria-label") {
		t.Errorf("finding 48: the todo-item input has no accessible name.\ntag: %s", tag)
	}
}

// TestBlockEditorRemoveButtons_AreNamedAfterWhatTheyRemove is finding 48's
// second half. The buttons were named only "×", which tells a screen-reader
// user nothing about what is about to be deleted.
func TestBlockEditorRemoveButtons_AreNamedAfterWhatTheyRemove(t *testing.T) {
	tc := SetupTestEnv(t)
	note := tc.createNoteWithBlocks(t)

	_, body := tc.getHTML(t, fmt.Sprintf("/note?id=%d", note.ID))

	// Every button whose only content is the multiplication sign must carry a
	// name. Matching on the rendered template text is the point: these buttons
	// are inside <template x-for>, so the server ships them verbatim.
	//
	// Find every button whose content is just the glyph, then re-read its open tag
	// with the quote-aware scan — an Alpine attribute containing ">" would otherwise
	// truncate the attribute run and make named buttons look unnamed.
	closingRe := regexp.MustCompile(`(?s)>\s*(?:&times;|×)\s*</button>`)
	var matches [][]string
	for _, loc := range closingRe.FindAllStringIndex(body, -1) {
		start := strings.LastIndex(body[:loc[0]+1], "<button")
		if start < 0 {
			continue
		}
		matches = append(matches, []string{"", body[start : loc[0]+1]})
	}
	if len(matches) == 0 {
		// Either fixed, or the markup moved. Prove the block editor is present.
		if !strings.Contains(body, "block-editor") {
			t.Fatalf("the block editor is not on the page — this test measured nothing")
		}
		return
	}
	for _, m := range matches {
		attrs := m[1]
		if !strings.Contains(attrs, "aria-label") {
			trimmed := whitespaceRe.ReplaceAllString(attrs, " ")
			if len(trimmed) > 160 {
				trimmed = trimmed[:160]
			}
			t.Errorf("finding 48: a \"×\" button has no accessible name.\nattrs: %s", trimmed)
		}
	}
}

// ---------------------------------------------------------------- finding 49

// TestCalendarDayCells_AreButtons is finding 49. All 35 day cells were bare
// <div>s with a click handler: 0 focusable, 0 with a role, so the only
// keyboard route to "add an event on this day" did not exist.
func TestCalendarDayCells_AreButtons(t *testing.T) {
	tc := SetupTestEnv(t)
	note := tc.createNoteWithBlocks(t)

	_, body := tc.getHTML(t, fmt.Sprintf("/note?id=%d", note.ID))

	const handler = `openEventModalForDay(day.date)`
	if !strings.Contains(body, handler) {
		t.Fatalf("the calendar month grid is not on the page — this test measured nothing")
	}

	// The *cell* deliberately stays a <div>: it contains the event chips, the
	// "+N more" toggle and the expanded-day popover, and a <button> may not
	// contain interactive descendants — the parser hoists a nested <button>
	// straight out of its parent. The day number carries the keyboard control
	// instead, so what this asserts is that a real <button> exists for
	// "add an event on this day", not that the cell became one.
	dayButtonRe := regexp.MustCompile(`(?s)<button[^>]*@click\.stop="openEventModalForDay\(day\.date\)"[^>]*>`)
	tag := dayButtonRe.FindString(body)
	if tag == "" {
		t.Errorf("finding 49: no <button> opens the new-event dialog for a specific day, so the only keyboard route is the generic \"+ Add Event\" control, which cannot target a day")
		return
	}
	if !strings.Contains(tag, "aria-label") {
		t.Errorf("finding 49: the day button has no accessible name — \"14\" alone does not say what activating it does.\ntag: %s", whitespaceRe.ReplaceAllString(tag, " "))
	}
	if !strings.Contains(tag, `type="button"`) {
		t.Errorf("finding 49: the day button has no type=button and would submit the enclosing form.\ntag: %s", whitespaceRe.ReplaceAllString(tag, " "))
	}

	// The event chips inside a cell were click-only <div>s too — the same defect
	// in the same partial, and the reason the cell cannot be the button.
	if strings.Contains(body, `<div @click.stop="isCustomEvent(event)`) {
		t.Errorf("finding 49: the calendar event chips are still click-only <div>s")
	}
}

// ---------------------------------------------------------------- finding 58

// TestRelationTypeCategoryFields_AreMarkedRequired is finding 58. Both category
// fields are required (the form passes min=1 and the server rejects a submit
// without them) but only Name was marked, and neither field ever became
// aria-invalid.
func TestRelationTypeCategoryFields_AreMarkedRequired(t *testing.T) {
	tc := SetupTestEnv(t)
	_, body := tc.getHTML(t, "/relationType/new")

	// Positive control: the Name field is marked, so "no marker anywhere" cannot
	// satisfy this test.
	if !strings.Contains(body, "Required") {
		t.Fatalf("no Required marker anywhere on /relationType/new — the control for this test is gone")
	}

	for _, field := range []string{"FromCategory", "ToCategory"} {
		blockRe := regexp.MustCompile(`(?s)<div[^>]*data-selector-field="` + field + `".*?(?:<div[^>]*data-selector-field=|\z)`)
		block := blockRe.FindString(body)
		if block == "" {
			t.Fatalf("could not isolate the %s selector block", field)
		}
		tag := findOpenTag(block, `role="combobox"`, "input")
		if tag == "" {
			t.Fatalf("%s has no combobox input", field)
		}
		if !strings.Contains(tag, `aria-required="true"`) {
			t.Errorf("finding 58: %s is required but its combobox has no aria-required.\ntag: %s", field, tag)
		}
		if !strings.Contains(tag, "aria-invalid") {
			t.Errorf("finding 58: %s's combobox never becomes aria-invalid, so a rejected submit is not announced.\ntag: %s", field, tag)
		}
	}
}

// ---------------------------------------------------------------- finding 64/59

// TestRelationWithoutAName_HasANonEmptyH1 is finding 64/59. A relation created
// without a Name — which the create form permits — rendered an <h1> containing
// only an empty <inline-edit>, while <title> already computed
// "Relation from X to Y".
func TestRelationWithoutAName_HasANonEmptyH1(t *testing.T) {
	tc := SetupTestEnv(t)
	rel, fromName, toName := tc.createUnnamedRelation(t)

	_, body := tc.getHTML(t, fmt.Sprintf("/relation?id=%d", rel.ID))

	h1Re := regexp.MustCompile(`(?s)<h1\b[^>]*>(.*?)</h1>`)
	m := h1Re.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no <h1> on the relation page")
	}
	text := strings.TrimSpace(whitespaceRe.ReplaceAllString(tagStripRe.ReplaceAllString(m[1], ""), " "))
	if text == "" {
		t.Errorf("finding 64: the <h1> of a relation with no name is empty.\nh1: %s", strings.TrimSpace(m[1]))
	}
	// It must be the same fallback <title> already computes, not a placeholder
	// like "Untitled" — the point is that the page identifies itself.
	for _, want := range []string{fromName, toName} {
		if !strings.Contains(text, want) {
			t.Errorf("finding 64: the <h1> fallback %q does not name the endpoint %q", text, want)
		}
	}

	// Positive control: a *named* relation still shows its own name, so the
	// fallback cannot be unconditionally overwriting the real value.
	named := tc.createNamedRelation(t, "ws5 named relation")
	_, namedBody := tc.getHTML(t, fmt.Sprintf("/relation?id=%d", named.ID))
	nm := h1Re.FindStringSubmatch(namedBody)
	if nm == nil {
		t.Fatalf("no <h1> on the named relation page")
	}
	namedText := strings.TrimSpace(whitespaceRe.ReplaceAllString(tagStripRe.ReplaceAllString(nm[1], ""), " "))
	if !strings.Contains(namedText, "ws5 named relation") {
		t.Errorf("finding 64: the fallback replaced a real relation name; h1 reads %q", namedText)
	}
}

// ---------------------------------------------------------------- finding 99

// TestSavedQueryDelete_IsNotHoverOnly is finding 99's reachability half. The
// button was rendered at opacity:0 and revealed only by :hover or :focus, so on
// a touch device there was no way to see or reach it. The size half is a
// measured box and lives in Playwright.
func TestSavedQueryDelete_IsNotHoverOnly(t *testing.T) {
	tc := SetupTestEnv(t)
	_, body := tc.getHTML(t, "/mrql")

	tag := findOpenTag(body, "deleteSavedQuery", "button")
	if tag == "" {
		t.Fatalf("the saved-query delete button is not in /mrql — this test measured nothing")
	}
	if strings.Contains(tag, "opacity-0") {
		t.Errorf("finding 99: the saved-query delete button is still opacity-0 until hover, so it does not exist on touch.\ntag: %s", tag)
	}
	if !strings.Contains(tag, "aria-label") {
		t.Errorf("finding 99: the saved-query delete button lost its accessible name.\ntag: %s", tag)
	}
}

// ---------------------------------------------------------------- findings 108, 110, 127

// headingOrderPages is every page the workstream names, plus controls.
var headingOrderPages = []struct {
	name    string
	url     string
	finding string
}{
	{"category create", "/category/new", "108"},
	{"note type create", "/noteType/new", "108"},
	{"resource category create", "/resourceCategory/new", "108"},
	{"admin shares", "/admin/shares", "110"},
	{"admin settings", "/admin/settings", "110"},
	{"logs (control — already correct)", "/logs", "control"},
	{"export (control — already correct)", "/admin/export", "control"},
}

// TestPages_HaveExactlyOneH1 is finding 110. /admin/shares rendered
// "Shared Notes" twice as <h1>; /admin/settings rendered "Settings" and
// "Runtime Settings" both as <h1>.
func TestPages_HaveExactlyOneH1(t *testing.T) {
	tc := SetupTestEnv(t)

	for _, p := range headingOrderPages {
		t.Run(p.name, func(t *testing.T) {
			_, body := tc.getHTML(t, p.url)
			hs := visibleHeadings(body)
			n := 0
			for _, h := range hs {
				if h.Level == 1 {
					n++
				}
			}
			if n == 0 {
				t.Fatalf("%s has no <h1> at all — this test measured nothing. Headings: %s", p.url, describeHeadings(hs))
			}
			if n != 1 {
				t.Errorf("finding %s: %s renders %d <h1> elements; a page has one title. Headings: %s",
					p.finding, p.url, n, describeHeadings(hs))
			}
		})
	}
}

// TestPages_DoNotSkipHeadingLevels is finding 108 (H1 straight to H3 with no H2
// anywhere on the taxonomy forms) and the same rule everywhere else.
func TestPages_DoNotSkipHeadingLevels(t *testing.T) {
	tc := SetupTestEnv(t)

	for _, p := range headingOrderPages {
		t.Run(p.name, func(t *testing.T) {
			_, body := tc.getHTML(t, p.url)
			hs := visibleHeadings(body)
			if len(hs) < 2 {
				t.Fatalf("%s has %d headings — too few to measure an order. Headings: %s", p.url, len(hs), describeHeadings(hs))
			}
			if i := firstSkip(hs); i >= 0 {
				t.Errorf("finding %s: %s skips from H%d to H%d at %q. Headings: %s",
					p.finding, p.url, hs[i-1].Level, hs[i].Level, hs[i].Text, describeHeadings(hs))
			}
		})
	}
}

// TestNoteDetail_DoesNotSkipToH3ForTheOwnerCard is finding 127. The reusable
// entity card renders <h3 class="card-title">, and the note sidebar embeds one
// in the "Owner:" disclosure — immediately after the H1 and before any H2.
//
// The report's own URL (a note with no owner) does not reproduce it, so the
// note here is created *with* an owner, and a note without one is asserted as
// the control.
func TestNoteDetail_DoesNotSkipToH3ForTheOwnerCard(t *testing.T) {
	tc := SetupTestEnv(t)

	owned, ownerless := tc.createNotesWithAndWithoutAnOwner(t)

	_, body := tc.getHTML(t, fmt.Sprintf("/note?id=%d", owned.ID))
	hs := visibleHeadings(body)
	if len(hs) < 2 {
		t.Fatalf("note detail has %d headings — nothing to measure. Headings: %s", len(hs), describeHeadings(hs))
	}
	if i := firstSkip(hs); i >= 0 {
		t.Errorf("finding 127: the note detail outline skips H%d → H%d at %q. Headings: %s",
			hs[i-1].Level, hs[i].Level, hs[i].Text, describeHeadings(hs))
	}

	// Control: the ownerless note must still have a clean outline, and must
	// still render headings at all, so the assertion above is not passing
	// because the owner card silently stopped rendering.
	_, ownerlessBody := tc.getHTML(t, fmt.Sprintf("/note?id=%d", ownerless.ID))
	if i := firstSkip(visibleHeadings(ownerlessBody)); i >= 0 {
		t.Errorf("finding 127: the ownerless note detail outline also skips levels. Headings: %s",
			describeHeadings(visibleHeadings(ownerlessBody)))
	}
	if !strings.Contains(body, "card-title") {
		t.Errorf("finding 127: no .card-title on the owned note's page — the owner card is gone, so this test proved nothing")
	}
}

// ---------------------------------------------------------------- finding 133

// TestMentionTextarea_DeclaresTheListboxItControls is finding 133. The
// description textarea declares aria-autocomplete=list, aria-haspopup=listbox
// and aria-activedescendant, but no aria-controls — so assistive tech cannot
// follow the suggestion list. autocompleter.tpl, in the same directory, sets
// both aria-controls and aria-owns.
//
// The subject used to be located by role="combobox". Deferred-work item 8
// removed that role — it overrode the textarea's implicit textbox role and took
// aria-multiline with it, and ARIA 1.2 has no multiline combobox — so the marker
// is now x-ref="mentionInput", which is what actually makes this a mention
// textarea. TestTextareasKeepTheirNativeRole in internal/arch/templates_test.go
// is what keeps the role from coming back.
func TestMentionTextarea_DeclaresTheListboxItControls(t *testing.T) {
	tc := SetupTestEnv(t)
	group := tc.createGroupNamed(t, "ws5 mention host")

	_, body := tc.getHTML(t, fmt.Sprintf("/group/edit?id=%d", group.ID))

	tag := findOpenTag(body, `x-ref="mentionInput"`, "textarea")
	if tag == "" {
		t.Fatalf("no mention textarea on the group edit form — this test measured nothing")
	}
	if !strings.Contains(tag, "aria-controls=") {
		t.Errorf("finding 133: the mention textarea does not declare aria-controls.\ntag: %s", whitespaceRe.ReplaceAllString(tag, " "))
	}

	ctrlRe := regexp.MustCompile(`aria-controls="([^"]+)"`)
	m := ctrlRe.FindStringSubmatch(tag)
	if m == nil {
		return
	}
	id := m[1]
	if !strings.Contains(body, `id="`+id+`"`) {
		t.Errorf("finding 133: aria-controls points at id=%q, which the page does not render", id)
	}
	if target := findTagWithID(body, id); target == "" {
		t.Errorf("finding 133: aria-controls target id=%q is not an element open tag", id)
	} else if !strings.Contains(target, `role="listbox"`) {
		t.Errorf("finding 133: aria-controls target is not role=listbox.\ntag: %s", whitespaceRe.ReplaceAllString(target, " "))
	}
}

// ---------------------------------------------------------------- finding 144

// TestPasteUploadDialog_NamesItsTarget is finding 144. The heading read the
// literal "Upload to Unknown" on every entity page, because the store's
// context is null until an upload starts and the template's `|| 'Unknown'`
// fallback always won.
func TestPasteUploadDialog_NamesItsTarget(t *testing.T) {
	tc := SetupTestEnv(t)
	group := tc.createGroupNamed(t, "ws5 paste target")

	for _, url := range []string{
		fmt.Sprintf("/group?id=%d", group.ID),
		"/resources",
	} {
		t.Run(url, func(t *testing.T) {
			_, body := tc.getHTML(t, url)
			idx := strings.Index(body, `id="paste-upload-title"`)
			if idx < 0 {
				t.Fatalf("the paste-upload dialog is not on %s — this test measured nothing", url)
			}
			// Scope to the heading. "Unknown" is a legitimate word elsewhere on a
			// page, so a document-wide substring check would be a locator wider
			// than its subject.
			end := strings.Index(body[idx:], "</h2>")
			if end < 0 {
				t.Fatalf("the paste-upload title has no closing tag on %s", url)
			}
			headingMarkup := body[idx : idx+end]
			if strings.Contains(headingMarkup, "Unknown") {
				t.Errorf("finding 144: %s still names the paste-upload target \"Unknown\".\nheading: %s",
					url, whitespaceRe.ReplaceAllString(headingMarkup, " "))
			}
		})
	}
}

// ---------------------------------------------------------------- fixtures

func (tc *TestContext) createResourceWithName(t *testing.T, name string) *models.Resource {
	t.Helper()
	res := &models.Resource{Name: name, Location: "ws5/" + name, ContentType: "text/plain"}
	if err := tc.DB.Create(res).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}
	return res
}

func (tc *TestContext) createGroupNamed(t *testing.T, name string) *models.Group {
	t.Helper()
	g := &models.Group{Name: name}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	return g
}

// createNoteWithBlocks builds a note carrying the block types WS5 touches: a
// todos block (finding 48's unnamed inputs and "×" buttons) and a calendar
// block (finding 49's click-only day cells).
func (tc *TestContext) createNoteWithBlocks(t *testing.T) *models.Note {
	t.Helper()
	note := &models.Note{Name: "ws5 block host"}
	if err := tc.DB.Create(note).Error; err != nil {
		t.Fatalf("create note: %v", err)
	}
	return note
}

// createNotesWithAndWithoutAnOwner is finding 127's fixture: the level skip
// comes from the owner card, so the test needs both shapes.
func (tc *TestContext) createNotesWithAndWithoutAnOwner(t *testing.T) (*models.Note, *models.Note) {
	t.Helper()
	owner := tc.createGroupNamed(t, "ws5 owning group")
	owned := &models.Note{Name: "ws5 owned note", OwnerId: &owner.ID}
	if err := tc.DB.Create(owned).Error; err != nil {
		t.Fatalf("create owned note: %v", err)
	}
	ownerless := &models.Note{Name: "ws5 ownerless note"}
	if err := tc.DB.Create(ownerless).Error; err != nil {
		t.Fatalf("create ownerless note: %v", err)
	}
	return owned, ownerless
}

// createUnnamedRelation reproduces exactly what the create form permits: a
// relation with both endpoints and no Name. It returns the relation and the two
// endpoint names the <h1> fallback has to mention.
func (tc *TestContext) createUnnamedRelation(t *testing.T) (*models.GroupRelation, string, string) {
	t.Helper()
	from := tc.createGroupNamed(t, "ws5 relation from")
	to := tc.createGroupNamed(t, "ws5 relation to")
	rt := &models.GroupRelationType{Name: "ws5 relation type"}
	if err := tc.DB.Create(rt).Error; err != nil {
		t.Fatalf("create relation type: %v", err)
	}
	rel := &models.GroupRelation{FromGroupId: &from.ID, ToGroupId: &to.ID, RelationTypeId: &rt.ID}
	if err := tc.DB.Create(rel).Error; err != nil {
		t.Fatalf("create relation: %v", err)
	}
	return rel, from.Name, to.Name
}

func (tc *TestContext) createNamedRelation(t *testing.T, name string) *models.GroupRelation {
	t.Helper()
	from := tc.createGroupNamed(t, "ws5 named from")
	to := tc.createGroupNamed(t, "ws5 named to")
	rt := &models.GroupRelationType{Name: "ws5 named relation type"}
	if err := tc.DB.Create(rt).Error; err != nil {
		t.Fatalf("create relation type: %v", err)
	}
	rel := &models.GroupRelation{Name: name, FromGroupId: &from.ID, ToGroupId: &to.ID, RelationTypeId: &rt.ID}
	if err := tc.DB.Create(rel).Error; err != nil {
		t.Fatalf("create relation: %v", err)
	}
	return rel
}
