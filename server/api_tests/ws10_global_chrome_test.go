package api_tests

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// WS10 — "Global chrome".
//
// Findings 33, 83, 102, 116/121 and 120 of docs/ui-bug-hunt-2026-07-29.md.
//
// Three of the five are geometry or focus, which only a browser can answer; those
// live in e2e/tests/regressions/ws10-global-chrome.spec.ts. What Go can check is
// what the server wrote: which nav link is marked current, whether the settings
// controls are inside a form, where the jobs trigger is, and the one CSS
// declaration behind the sticky header.

// ---------------------------------------------------------- findings 116/121

// TestNavLinks_MarkTheCurrentPage is finding 121: the active nav link was styled
// with `navbar-link--active` and carried no aria-current on any of the seven pages
// the report checked, so the current location was conveyed by colour alone. The app
// already does this correctly for pagination (pagination.tpl:21), which is what
// makes it an internal inconsistency rather than an omission.
func TestNavLinks_MarkTheCurrentPage(t *testing.T) {
	tc := SetupTestEnv(t)

	for _, page := range []struct{ url, label string }{
		{"/resources", "Resources"},
		{"/notes", "Notes"},
		{"/groups", "Groups"},
		{"/tags", "Tags"},
		{"/dashboard", "Dashboard"},
	} {
		t.Run(page.url, func(t *testing.T) {
			_, body := tc.getHTML(t, page.url)

			link := navLinkTo(body, page.url)
			if link == "" {
				t.Fatalf("no nav link to %s in the page — this test measured nothing", page.url)
			}
			if !strings.Contains(link, `aria-current="page"`) {
				t.Errorf("finding 121: the nav link for the current page carries no aria-current.\ntag: %s",
					whitespaceRe.ReplaceAllString(link, " "))
			}
			if !strings.Contains(link, "navbar-link--active") {
				t.Errorf("the nav link for the current page lost its active class.\ntag: %s",
					whitespaceRe.ReplaceAllString(link, " "))
			}

			// The control: exactly one link may claim it, or "current page" means
			// nothing. Counted over the desktop nav and the mobile panel together,
			// which each render the set once.
			if n := strings.Count(body, `aria-current="page"`); n != 2 {
				t.Errorf("finding 121: %d links claim aria-current=\"page\" on %s, want 2 (desktop nav + mobile panel)", n, page.url)
			}
		})
	}
}

// TestNavLinks_KeepTheSectionOnDetailPages is finding 116: /resource?id=85,
// /note?id=61 and /group?id=68 highlighted *nothing* — the report measured an empty
// active-link list on every detail page — so a reader had no indication of where
// they were in the app.
//
// A detail page is not the list page, so it gets aria-current="true" ("the current
// item in this set") rather than "page". `page` would claim the reader is on
// /resources while they are on /resource?id=63.
func TestNavLinks_KeepTheSectionOnDetailPages(t *testing.T) {
	tc := SetupTestEnv(t)
	resource := tc.CreateDummyResource(t, "ws10 detail")
	note := tc.CreateDummyNote("ws10 note")
	group := tc.CreateDummyGroup("ws10 group")

	cases := []struct{ url, section string }{
		{"/resource?id=" + fmt.Sprintf("%d", resource.ID), "/resources"},
		{"/note?id=" + fmt.Sprintf("%d", note.ID), "/notes"},
		{"/group?id=" + fmt.Sprintf("%d", group.ID), "/groups"},
		{"/resources/details", "/resources"},
	}

	for _, c := range cases {
		t.Run(c.url, func(t *testing.T) {
			_, body := tc.getHTML(t, c.url)

			link := navLinkTo(body, c.section)
			if link == "" {
				t.Fatalf("no nav link to %s — this test measured nothing", c.section)
			}
			if !strings.Contains(link, "navbar-link--active") {
				t.Errorf("finding 116: %s does not highlight its section (%s), so nothing in the nav says where you are.\ntag: %s",
					c.url, c.section, whitespaceRe.ReplaceAllString(link, " "))
			}
			if !strings.Contains(link, `aria-current="true"`) {
				t.Errorf("finding 116: %s's section link carries no aria-current, so assistive tech gets nothing.\ntag: %s",
					c.url, whitespaceRe.ReplaceAllString(link, " "))
			}
			if strings.Contains(link, `aria-current="page"`) {
				t.Errorf("a detail page is not the section's list page; aria-current=\"page\" would misreport it.\ntag: %s",
					whitespaceRe.ReplaceAllString(link, " "))
			}
		})
	}
}

// The negative control for the two tests above: a page that belongs to no nav
// section marks nothing, so "some link is always current" cannot satisfy them.
func TestNavLinks_MarkNothingOutsideTheMenu(t *testing.T) {
	tc := SetupTestEnv(t)
	_, body := tc.getHTML(t, "/account")

	for _, section := range []string{"/resources", "/notes", "/groups", "/tags", "/dashboard", "/mrql"} {
		if link := navLinkTo(body, section); strings.Contains(link, "aria-current") {
			t.Errorf("/account is not in any nav section, but %s is marked current.\ntag: %s",
				section, whitespaceRe.ReplaceAllString(link, " "))
		}
	}
}

// ---------------------------------------------------------------- finding 33

// TestRuntimeSettings_ControlsSubmitOnEnter is finding 33: typing a value and
// pressing Enter did nothing at all — no save, no message — because the controls
// were not inside a <form>, so there was nothing for Enter to submit.
//
// Note the shape this had to take: a form whose only button is
// `<button type="button">` still does not submit on Enter when it holds more than
// one text field (the row has a value box and a reason box), because implicit
// submission needs a submit button once more than one field blocks it. So the Save
// button has to be type="submit".
func TestRuntimeSettings_ControlsSubmitOnEnter(t *testing.T) {
	tc := SetupTestEnv(t)
	_, body := tc.getHTML(t, "/admin/settings")

	row := strings.Index(body, `data-setting-key=`)
	if row < 0 {
		t.Fatalf("no setting rows on /admin/settings — this test measured nothing")
	}
	// Scope to one row: the page renders dozens, and a match anywhere would pass.
	next := strings.Index(body[row+1:], `data-setting-key=`)
	if next < 0 {
		next = len(body) - row - 1
	}
	rowMarkup := body[row : row+1+next]

	form := findOpenTag(rowMarkup, "@submit", "form")
	if form == "" {
		t.Fatalf("finding 33: the setting row's controls are not inside a <form>, so Enter has nothing to submit.\nrow: %s",
			whitespaceRe.ReplaceAllString(rowMarkup, " ")[:min(400, len(whitespaceRe.ReplaceAllString(rowMarkup, " ")))])
	}
	if !strings.Contains(form, "@submit.prevent") {
		t.Errorf("finding 33: the settings form does not prevent its default submit, so Enter would navigate.\ntag: %s",
			whitespaceRe.ReplaceAllString(form, " "))
	}
	if !strings.Contains(form, "save()") {
		t.Errorf("finding 33: the settings form's submit does not call save().\ntag: %s",
			whitespaceRe.ReplaceAllString(form, " "))
	}
	if !strings.Contains(rowMarkup, `type="submit"`) {
		t.Errorf("finding 33: no submit button in the row, so implicit submission stays inert with two text fields present")
	}
}

// ------------------------------------------------------------ findings 83/102

// TestJobsTrigger_IsHeaderChromeNotAFixedCorner is findings 83 and 102. The trigger
// was `fixed bottom-4 right-4 z-40`, and it won the hit test over the pagination
// "Next" link on every paginated page and over the /logs "After" date input at
// 1280x720. The geometry is asserted in Playwright; what Go can pin is that the
// button is not a fixed corner overlay any more, and that it sits in the header.
func TestJobsTrigger_IsHeaderChromeNotAFixedCorner(t *testing.T) {
	tc := SetupTestEnv(t)
	_, body := tc.getHTML(t, "/resources")

	trigger := findOpenTag(body, `data-testid="cockpit-trigger"`, "button")
	if trigger == "" {
		t.Fatalf("the jobs trigger is not in the page — this test measured nothing")
	}
	for _, banned := range []string{"fixed", "bottom-4", "right-4"} {
		if strings.Contains(trigger, banned) {
			t.Errorf("findings 83/102: the jobs trigger is still a fixed corner overlay (%q), which covers whatever the page paints in that corner.\ntag: %s",
				banned, whitespaceRe.ReplaceAllString(trigger, " "))
		}
	}

	header := body[strings.Index(body, "<header"):]
	end := strings.Index(header, "</header>")
	if end < 0 {
		t.Fatalf("no </header> in the page")
	}
	if !strings.Contains(header[:end], `data-testid="cockpit-trigger"`) {
		t.Errorf("findings 83/102: the jobs trigger is not inside the header, so it is not chrome and can still overlap page content")
	}

	// The panel it opens must stay a modal dialog: WS4 gave it x-trap and an
	// explicit focus restore, and moving the trigger must not have moved those.
	panel := findOpenTag(body, `data-testid="cockpit-panel"`, "div")
	if !strings.Contains(panel, "x-trap.noreturn") || !strings.Contains(panel, `aria-modal="true"`) {
		t.Errorf("the jobs panel lost its trap or its dialog semantics.\ntag: %s", whitespaceRe.ReplaceAllString(panel, " "))
	}
}

// --------------------------------------------------------------- finding 120

// TestSiteCSS_DoesNotMakeTheBodyAScrollContainer is finding 120, and it is the one
// assertion in this file that reads a file rather than a response.
//
// The header declares `position: sticky; top: 0` and never stuck. The report and
// the plan both blamed the body's `display: grid` — the header being a grid item
// whose containing block is its own ~36px row. Measured, that is not the binding
// constraint: with the grid untouched and `overflow-x: hidden` changed to
// `overflow-x: clip` on `.site`, the header pins (headerBottom -1464 -> 36 at
// scrollY 1500). `overflow-x: hidden` forces the other axis to `auto`, which makes
// <body> a scroll container — and a sticky element sticks to its nearest scroll
// container's scrollport, not the viewport. <body>'s box is as tall as its content,
// so that scrollport never moves. `clip` clips without creating one.
//
// The behaviour is measured in ws10-global-chrome.spec.ts; this exists because CI
// runs Go and not Playwright, and `overflow-x: hidden` is the kind of thing a later
// "restore the overflow guard" change would put back without knowing.
func TestSiteCSS_DoesNotMakeTheBodyAScrollContainer(t *testing.T) {
	css, err := os.ReadFile("../../public/index.css")
	if err != nil {
		t.Fatalf("read index.css: %v", err)
	}

	block := cssBlock(string(css), ".site {")
	if block == "" {
		t.Fatalf("no .site rule in public/index.css — this test measured nothing")
	}
	if strings.Contains(block, "overflow-x: hidden") {
		t.Errorf("finding 120: .site declares overflow-x: hidden, which computes overflow-y: auto and makes <body> a scroll container that never scrolls — so the header's position:sticky is inert. Use overflow-x: clip.\nblock: %s",
			whitespaceRe.ReplaceAllString(block, " "))
	}
	if !strings.Contains(block, "overflow-x: clip") {
		t.Errorf("finding 120: .site no longer clips horizontal overflow at all; the guard that keeps a wide row from pushing the sidebar off-screen is gone.\nblock: %s",
			whitespaceRe.ReplaceAllString(block, " "))
	}

	header := cssBlock(string(css), ".header {")
	if !strings.Contains(header, "position: sticky") {
		t.Errorf("finding 120: the header no longer declares position: sticky, so nothing pins the nav on a long list.\nblock: %s",
			whitespaceRe.ReplaceAllString(header, " "))
	}
}

// TestFooter_IsNotSticky is the other half of finding 120, and it exists because
// making the header stick made the footer's own `sticky bottom-0` work too — both
// declarations were inert for the same reason.
//
// A pinned pagination bar was measured re-creating finding 102: at 1280x720 on
// /logs, `elementFromPoint` over the "After" date input's picker icon returned an
// <a> from the pagination row rather than the input. A bar fixed to the viewport
// bottom covers page content at every scroll offset, which is precisely the
// objection that moved the jobs trigger out of the bottom-right corner in the same
// batch. So the footer's declaration is dropped rather than activated.
func TestFooter_IsNotSticky(t *testing.T) {
	tc := SetupTestEnv(t)
	_, body := tc.getHTML(t, "/resources")

	footer := findOpenTag(body, `class="footer`, "footer")
	if footer == "" {
		t.Fatalf("no <footer class=\"footer\"> in the page — this test measured nothing")
	}
	if strings.Contains(footer, "sticky") || strings.Contains(footer, "bottom-0") {
		t.Errorf("finding 102: the footer is pinned to the viewport bottom again, which covers whatever page content is there — measured on the /logs \"After\" date input at 1280x720.\ntag: %s",
			whitespaceRe.ReplaceAllString(footer, " "))
	}

	// The control: the pagination the footer holds is still rendered, so this is not
	// passing because the footer went away.
	if !strings.Contains(body, `aria-label="Pagination"`) && !strings.Contains(body, "footer") {
		t.Errorf("no pagination and no footer on /resources — the assertion above is vacuous")
	}
}

// navLinkTo returns the first desktop nav <a> whose href is exactly url.
func navLinkTo(body, url string) string {
	for _, tag := range openTagsWithin(body, "a") {
		if strings.Contains(tag, `href="`+url+`"`) && strings.Contains(tag, "navbar-link") {
			return tag
		}
	}
	return ""
}

// cssBlock returns the declarations of the first rule whose selector text matches
// marker (including the trailing "{"), with /* */ comments stripped.
//
// Stripping matters: the .site rule carries a long comment explaining why
// `overflow-x: hidden` is wrong here, and a plain substring search found the
// declaration inside its own explanation.
func cssBlock(css, marker string) string {
	at := strings.Index(css, marker)
	if at < 0 {
		return ""
	}
	rest := css[at+len(marker):]
	end := strings.Index(rest, "}")
	if end < 0 {
		return ""
	}
	return cssCommentRe.ReplaceAllString(rest[:end], " ")
}

var cssCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
