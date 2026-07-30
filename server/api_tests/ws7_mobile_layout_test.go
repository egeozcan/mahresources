package api_tests

import (
	"fmt"
	"strings"
	"testing"

	"mahresources/models"
)

// WS7 — "Mobile and layout".
//
// Findings 3, 8, 19/101, 25/62/63/80, 55, 67, 75, 81, 89, 141, 148 and 150 of
// docs/ui-bug-hunt-2026-07-29.md.
//
// Almost all of WS7 is geometry, and geometry is a runtime property: the measured
// assertions live in e2e/tests/regressions/ws7-mobile-and-layout.spec.ts. What is
// here is the markup and CSS the server ships, which CI can actually check —
// `go test` runs on every PR and the browser suite does not.
//
// Several of these are negative assertions about markup that must NOT come back
// (a `role="dialog"` on the nav panel, a `word-break: break-all` in the metadata
// cards), and each is paired with a positive control in the same test, because an
// empty page satisfies a negative assertion forever.

// ---------------------------------------------------------------- finding 3

// TestMobileNavPanel_CanBeClosed is finding 3. The panel had no Escape handler, no
// close button and no focus trap, and being a descendant of the z-index:40 header
// it painted above the hamburger, so the toggle click was intercepted. The only
// way out was to follow a nav link.
func TestMobileNavPanel_CanBeClosed(t *testing.T) {
	tc := SetupTestEnv(t)
	_, body := tc.getHTML(t, "/dashboard")

	panel := findOpenTag(body, `id="navbar-mobile-panel"`, "div")
	if panel == "" {
		t.Fatalf("the mobile nav panel is not on /dashboard — this test measured nothing")
	}

	for _, want := range []struct{ attr, why string }{
		{"@keydown.escape", "Escape must close the panel"},
		{"x-trap", "focus must be trapped while the panel covers the page"},
		{"aria-label", "the panel is a region and needs a name"},
	} {
		if !strings.Contains(panel, want.attr) {
			t.Errorf("finding 3: the mobile nav panel is missing %s — %s.\ntag: %s",
				want.attr, want.why, whitespaceRe.ReplaceAllString(panel, " "))
		}
	}

	// A close control has to exist inside the panel. The panel contained zero
	// buttons, which is the part of the finding that stranded the user.
	panelStart := strings.Index(body, `id="navbar-mobile-panel"`)
	if panelStart < 0 {
		t.Fatalf("panel id not found")
	}
	if !strings.Contains(body[panelStart:], `aria-label="Close menu"`) {
		t.Errorf("finding 3: the mobile nav panel has no close control")
	}
}

// TestMobileNavPanel_IsNotADialog guards the one thing this fix must not do.
//
// The panel is in every page's DOM. The lightbox is addressed app-wide by a strict
// `[role="dialog"][aria-modal="true"]` locator whose two :not() exclusions name the
// only other always-present modals, so a third would be a strict-mode violation in
// roughly 45 specs rather than a soft failure. x-trap supplies the behaviour; the
// semantics stay a plain labelled region.
func TestMobileNavPanel_IsNotADialog(t *testing.T) {
	tc := SetupTestEnv(t)
	_, body := tc.getHTML(t, "/dashboard")

	panel := findOpenTag(body, `id="navbar-mobile-panel"`, "div")
	if panel == "" {
		t.Fatalf("the mobile nav panel is not on /dashboard — this test measured nothing")
	}
	if strings.Contains(panel, `role="dialog"`) {
		t.Errorf("finding 3: the mobile nav panel declares role=\"dialog\". It is present on every page, and the lightbox's app-wide [role=dialog][aria-modal=true] locator would resolve to two elements.\ntag: %s",
			whitespaceRe.ReplaceAllString(panel, " "))
	}

	// Positive control: some element on the page DOES carry the pair, so
	// "the panel does not" is not passing because the whole convention vanished.
	//
	// The count is deliberately not asserted here. Go sees the served HTML, which
	// includes the contents of every <template x-if> — seven dialogs on this page —
	// while the browser only ever has the two that are not behind a template in the
	// DOM at once. That distinction is only visible at runtime, so the "exactly two
	// always-present dialogs" constraint is asserted in
	// e2e/tests/regressions/ws7-mobile-and-layout.spec.ts instead.
	pairs := 0
	for _, tag := range openTagsWithin(body, "div") {
		if strings.Contains(tag, `role="dialog"`) && strings.Contains(tag, `aria-modal="true"`) {
			pairs++
		}
	}
	if pairs == 0 {
		t.Errorf("finding 3: no element on the page carries role=dialog + aria-modal=true, so this test proved nothing about the panel")
	}
}

// ---------------------------------------------------------------- findings 25/62/63/80

// TestSidebar_IsWrappedInADisclosure is findings 25/62/63/80. Below 900px the
// sidebar stacked above the results at full height, putting the first card at
// y=1745 on /groups and y=2124 on /resources against an 844px viewport.
//
// The geometry is asserted in Playwright. What matters here is the structure the
// decision in docs/todo.md called for, and the three constraints that structure has
// to respect — each of which a spec somewhere depends on.
func TestSidebar_IsWrappedInADisclosure(t *testing.T) {
	tc := SetupTestEnv(t)

	for _, url := range []string{"/groups", "/notes", "/resources", "/tags", "/logs"} {
		t.Run(url, func(t *testing.T) {
			_, body := tc.getHTML(t, url)

			disclosure := findOpenTag(body, `id="sidebar-disclosure"`, "details")
			if disclosure == "" {
				t.Fatalf("finding 25: %s has no sidebar disclosure", url)
			}

			// `open` server-side: there is no pure-CSS way to force a <details> open
			// above a breakpoint, so the desktop default and the no-JS default are
			// both "open", and a parser-blocking script closes it when narrow.
			if !strings.Contains(disclosure, "open") {
				t.Errorf("finding 25: the sidebar disclosure is not `open` by default, so a desktop reader — or anyone with JS disabled — gets a collapsed sidebar.\ntag: %s", disclosure)
			}

			// Constraint 1: the wrapper class must not contain "sidebar".
			// `[class*="sidebar"]` is a live locator in five specs, and the wrapper
			// precedes the <aside> in document order, so `.first()` would resolve to
			// the wrapper instead of the landmark.
			classRe := `class="`
			if i := strings.Index(disclosure, classRe); i >= 0 {
				rest := disclosure[i+len(classRe):]
				if end := strings.Index(rest, `"`); end >= 0 {
					if strings.Contains(rest[:end], "sidebar") {
						t.Errorf("finding 25: the disclosure's class contains \"sidebar\" (%q), which [class*=\"sidebar\"] locators would match before the <aside>", rest[:end])
					}
				}
			}

			// Constraint 2: the <aside class="sidebar"> is wrapped, not replaced —
			// specs address it as `aside, [role="complementary"]` without .first().
			if !strings.Contains(body, `<aside class="sidebar">`) {
				t.Errorf("finding 25: <aside class=\"sidebar\"> is gone from %s; it must be wrapped, not replaced", url)
			}
			if n := strings.Count(body, `<aside`); n != 1 {
				t.Errorf("finding 25: %s renders %d <aside> elements; a second complementary landmark makes every `aside` locator ambiguous", url, n)
			}

			// Constraint 3: the summary text must not collide with the
			// `details.detail-collapsible:has(summary:text-is("…"))` locators used for
			// the section-config panels.
			for _, reserved := range []string{">Relations<", ">Own Entities<", ">Related Entities<"} {
				if strings.Contains(disclosure, reserved) {
					t.Errorf("finding 25: the disclosure summary collides with a reserved section-config summary %q", reserved)
				}
			}
		})
	}
}

// ---------------------------------------------------------------- finding 67

// TestDetailsNameCell_CarriesTheFullNameInATitle is finding 67's markup half. The
// Name column is capped and ellipsised now, which is what stops one 166-character
// name making the table 2005px wide — so the full name has to remain available.
func TestDetailsNameCell_CarriesTheFullNameInATitle(t *testing.T) {
	tc := SetupTestEnv(t)

	const long = "ws7 a resource with a really extremely long descriptive name that ought to test how the interface truncates things in cards, tables and breadcrumbs"
	tc.createResourceWithName(t, long)

	_, body := tc.getHTML(t, "/resources/details")
	if !strings.Contains(body, `title="`+long+`"`) {
		t.Errorf("finding 67: the truncated Name cell does not carry the full name in a title attribute, so a clipped name is unreadable")
	}
}

// ---------------------------------------------------------------- finding 148

// TestResourceMetadataCards_DoNotBreakMidToken is finding 148. `word-break:
// break-all` breaks at any character, so "Jul 29, 2026 12:01" rendered as
// "…2026 1" / "2:01" — a clock time split across lines, which reads as a
// different value.
func TestResourceMetadataCards_DoNotBreakMidToken(t *testing.T) {
	tc := SetupTestEnv(t)
	res := tc.createResourceWithName(t, "ws7 wrap subject")

	_, body := tc.getHTML(t, fmt.Sprintf("/resource?id=%d", res.ID))

	// Positive control: the metadata cards are on the page and do use the wrapping
	// class, so "no break-all" cannot pass because the cards vanished.
	if !strings.Contains(body, "wrap-anywhere") {
		t.Fatalf("finding 148: no .wrap-anywhere on the resource page — the metadata cards are gone and this test measured nothing")
	}

	// Scoped to the metadata <dd>s, not the page. A document-wide search for
	// `break-all` matches the lightbox partial's ten uses, which are unbroken hash
	// and GUID tokens where break-all and anywhere behave identically — a locator
	// wider than its subject, which would fail for a reason unrelated to finding 148.
	for _, dd := range openTagsWithin(body, "dd") {
		if strings.Contains(dd, "break-all") {
			t.Errorf("finding 148: a resource metadata card still uses break-all, which breaks timestamps mid-token.\ntag: %s", dd)
		}
	}
}

// ---------------------------------------------------------------- finding 150

// TestBreadcrumb_DoesNotWrap is finding 150. The arrow separators are full-height
// chevrons drawn to read as one connected trail; when the trail wrapped, each
// arrow starting a new line was stranded at the left margin connecting nothing.
//
// The first fix swapped them for an inline "›" below 900px, which fixed the
// reported viewport and left the defect at 1280px on a seven-crumb trail. So the
// trail does not wrap at all now — asserted here as markup, and measured in
// Playwright at both widths.
func TestBreadcrumb_DoesNotWrap(t *testing.T) {
	tc := SetupTestEnv(t)
	owner := tc.createGroupNamed(t, "ws7 breadcrumb owner")
	res := tc.createResourceOwnedBy(t, "ws7 breadcrumb subject", owner.ID)

	_, body := tc.getHTML(t, fmt.Sprintf("/resource?id=%d", res.ID))

	nav := findOpenTag(body, `aria-label="Breadcrumb"`, "nav")
	if nav == "" {
		t.Fatalf("finding 150: no breadcrumb on the resource page — this test measured nothing")
	}
	if strings.Contains(nav, "flex-wrap") && !strings.Contains(nav, "flex-nowrap") {
		t.Errorf("finding 150: the breadcrumb nav still wraps, which strands the arrow separators at the left margin of every new line.\ntag: %s", nav)
	}
	if !strings.Contains(nav, "flex-nowrap") {
		t.Errorf("finding 150: the breadcrumb nav does not declare flex-nowrap.\ntag: %s", nav)
	}
	// The separators are marked so the stylesheet can address them; losing the
	// class would silently un-fix the CSS half.
	if !strings.Contains(body, "breadcrumb-separator") {
		t.Errorf("finding 150: the arrow separators lost their .breadcrumb-separator class")
	}
}

// ---------------------------------------------------------------- fixtures

func (tc *TestContext) createResourceOwnedBy(t *testing.T, name string, ownerID uint) *models.Resource {
	t.Helper()
	res := &models.Resource{Name: name, Location: "ws7/" + name, ContentType: "text/plain", OwnerId: &ownerID}
	if err := tc.DB.Create(res).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}
	return res
}
