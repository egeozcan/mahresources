package api_tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

// WS6 — "Empty states".
//
// Findings 54, 68/77/146, 122, 126, 29, 31, 32, 18 of
// docs/ui-bug-hunt-2026-07-29.md. This file covers the half reachable from Go:
// the server-rendered list templates, whose zero-result state was a silently
// blank content area, and the out-of-range page, which rendered blank with a
// pagination footer still offering pages that do exist.
//
// templates/listTags.tpl:22-23 is the pattern the fix copies — Pongo2's
// {% empty %} branch rendering a .detail-empty line.

// getHTML issues a browser-shaped GET and returns the rendered body. Every
// assertion in this file is about what a reader sees, so the Accept header has
// to be the browser's or RenderTemplate answers JSON instead.
func (tc *TestContext) getHTML(t *testing.T, url string) (int, string) {
	t.Helper()
	resp := tc.requestWithAccept(http.MethodGet, url, browserAccept, "")
	return resp.Code, resp.Body.String()
}

// emptyStateURLs is every server-rendered list view the app has, paired with a
// filter that cannot match. The five *Timeline views are deliberately absent:
// they share templates/partials/timeline.tpl, which renders its collection in
// the browser from /v1/<entity>/timeline and has no server-side loop to attach
// an {% empty %} branch to. Their empty state is asserted in Playwright.
var emptyStateURLs = []struct {
	name     string
	url      string
	expectIn string
}{
	{"resources grid", "/resources?name=zzzznope", "No resources"},
	{"resources details", "/resources/details?name=zzzznope", "No resources"},
	{"resources simple", "/resources/simple?name=zzzznope", "No resources"},
	{"notes", "/notes?name=zzzznope", "No notes"},
	{"groups", "/groups?name=zzzznope", "No groups"},
	{"groups text", "/groups/text?name=zzzznope", "No groups"},
	{"tags (already correct — positive control)", "/tags?name=zzzznope", "No tags"},
	{"categories (already correct — positive control)", "/categories?name=zzzznope", "No categories"},
}

// TestListPages_RenderAnEmptyState is finding 54/68/77's core assertion: a
// zero-result filter must say so rather than render a blank column.
func TestListPages_RenderAnEmptyState(t *testing.T) {
	tc := SetupTestEnv(t)

	for _, c := range emptyStateURLs {
		t.Run(c.name, func(t *testing.T) {
			code, body := tc.getHTML(t, c.url)
			if code != http.StatusOK {
				t.Fatalf("GET %s: expected 200, got %d", c.url, code)
			}
			if !strings.Contains(body, c.expectIn) {
				t.Errorf("GET %s: no empty state; expected a line containing %q.\nBody:\n%s",
					c.url, c.expectIn, truncate(body))
			}
			// The empty state has to be the house one, so it inherits
			// .detail-empty's styling rather than being a bare sentence.
			if !strings.Contains(body, "detail-empty") {
				t.Errorf("GET %s: empty state should use the .detail-empty class from listTags.tpl", c.url)
			}
		})
	}
}

// TestListPages_EmptyStateOffersClearFilters covers the second half of the
// product default: when a filter is what emptied the list, offer a way out of
// it. An unfiltered empty list gets a "create one" link instead.
func TestListPages_EmptyStateOffersClearFilters(t *testing.T) {
	tc := SetupTestEnv(t)

	t.Run("filtered list offers Clear filters", func(t *testing.T) {
		_, body := tc.getHTML(t, "/resources?name=zzzznope")
		if !strings.Contains(body, "Clear filters") {
			t.Errorf("a filtered zero-result list should offer to clear the filter; got:\n%s", truncate(body))
		}
	})

	t.Run("unfiltered empty list offers Create instead", func(t *testing.T) {
		// A fresh test env has no resources at all, so /resources with no
		// query is genuinely empty and unfiltered.
		_, body := tc.getHTML(t, "/resources")
		if strings.Contains(body, "Clear filters") {
			t.Errorf("an unfiltered empty list must not offer to clear a filter that is not set; got:\n%s", truncate(body))
		}
		// Asserting on "/resource/new" alone would pass trivially — the page's
		// Create action already links there. The wording is what distinguishes
		// the two branches.
		if !strings.Contains(body, "No resources yet") {
			t.Errorf("an unfiltered empty list should say the list is empty and offer to create one; got:\n%s", truncate(body))
		}
	})
}

// TestListPages_SelectAllHiddenWhenNothingToSelect is finding 68's other half.
// selectAllButton.tpl's predicate is
// `selectedIds.length + 1 !== elements.length`, which is 1 !== 0 — true — on an
// empty list, so the control was offered with nothing behind it.
func TestListPages_SelectAllHiddenWhenNothingToSelect(t *testing.T) {
	tc := SetupTestEnv(t)

	_, body := tc.getHTML(t, "/resources?name=zzzznope")

	// The button is rendered by the server and hidden by Alpine, so the
	// assertion is on the x-show predicate, not on the button's absence.
	if !strings.Contains(body, "$store.bulkSelection.elements.length > 0") {
		t.Errorf("Select All should be gated on there being something to select; got:\n%s", truncate(body))
	}
}

// ---------------------------------------------------------------------------
// Findings 68 / 146 — an out-of-range page
// ---------------------------------------------------------------------------

// seedForPagination gives each of the four lists under test at least one row.
// It matters that this is not "notes only": the redirect is deliberately a
// no-op on an empty list (there is no last valid page to send anyone to), so a
// list with no rows would pass TestOutOfRangePage_RedirectsToLastValidPage's
// premise while proving nothing.
func seedForPagination(t *testing.T, tc *TestContext) {
	t.Helper()
	for i := 0; i < 3; i++ {
		tc.CreateDummyNote("note for pagination")
		tc.CreateDummyGroup("group for pagination")
		tc.CreateResourceWithType(t, "resource for pagination", "image/png")
		if err := tc.DB.Create(&models.Tag{Name: fmt.Sprintf("tag-for-pagination-%d", i)}).Error; err != nil {
			t.Fatalf("seedForPagination: %v", err)
		}
	}
}

// TestOutOfRangePage_RedirectsToLastValidPage is the redirect half of 68/146.
// /resources?page=99 rendered a blank column under a pagination footer that
// still listed pages 1-2, and Previous pointed at page 98, which is equally
// blank.
func TestOutOfRangePage_RedirectsToLastValidPage(t *testing.T) {
	tc := SetupTestEnv(t)
	seedForPagination(t, tc)

	cases := []struct {
		name string
		url  string
		want string
	}{
		{"notes", "/notes?page=99", "page=1"},
		{"resources", "/resources?page=99", "page=1"},
		{"groups", "/groups?page=99", "page=1"},
		{"tags", "/tags?page=99", "page=1"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := tc.requestWithAccept(http.MethodGet, c.url, browserAccept, "")
			if resp.Code != http.StatusFound {
				t.Fatalf("GET %s: expected a 302 to the last valid page, got %d", c.url, resp.Code)
			}
			loc := resp.Header().Get("Location")
			if !strings.Contains(loc, c.want) {
				t.Errorf("GET %s: expected the redirect to land on %s, got %q", c.url, c.want, loc)
			}
		})
	}
}

// TestInRangePage_IsNotRedirected is the positive control for the test above.
// Without it, a redirect rule that fires on every request would satisfy every
// assertion in TestOutOfRangePage_RedirectsToLastValidPage.
func TestInRangePage_IsNotRedirected(t *testing.T) {
	tc := SetupTestEnv(t)
	seedForPagination(t, tc)

	for _, url := range []string{"/notes", "/notes?page=1", "/resources", "/groups?page=1"} {
		resp := tc.requestWithAccept(http.MethodGet, url, browserAccept, "")
		if resp.Code != http.StatusOK {
			t.Errorf("GET %s: a valid page must render, got %d (Location: %q)",
				url, resp.Code, resp.Header().Get("Location"))
		}
	}
}

// TestOutOfRangePage_JSONContractUnchanged pins the dual-response contract the
// redirect could silently break: RenderTemplate acts on _redirect before it
// looks at the JSON branch, so a redirect that did not exclude the JSON route
// would turn /resources.json?page=99 from "200 with an empty array" into a 302.
func TestOutOfRangePage_JSONContractUnchanged(t *testing.T) {
	tc := SetupTestEnv(t)
	seedForPagination(t, tc)

	t.Run(".json suffix still answers 200", func(t *testing.T) {
		resp := tc.MakeRequest(http.MethodGet, "/notes.json?page=99", nil)
		if resp.Code != http.StatusOK {
			t.Errorf("expected 200 from the JSON route, got %d to %q",
				resp.Code, resp.Header().Get("Location"))
		}
	})

	t.Run("JSON Accept still answers 200", func(t *testing.T) {
		resp := tc.requestWithAccept(http.MethodGet, "/notes?page=99", "application/json", "")
		if resp.Code != http.StatusOK {
			t.Errorf("expected 200 for a JSON-only Accept, got %d to %q",
				resp.Code, resp.Header().Get("Location"))
		}
	})

	t.Run(".body fragment still answers 200", func(t *testing.T) {
		resp := tc.requestWithAccept(http.MethodGet, "/notes.body?page=99", browserAccept, "")
		if resp.Code != http.StatusOK {
			t.Errorf("expected 200 from the .body morph target, got %d to %q",
				resp.Code, resp.Header().Get("Location"))
		}
	})
}

// TestEmptyResultPage_IsNotRedirected guards the other direction: a filter that
// matches nothing has zero pages, and redirecting it to "the last valid page"
// would either loop or send the reader somewhere that answers the same way.
// The empty state is the correct answer there, not a redirect.
func TestEmptyResultPage_IsNotRedirected(t *testing.T) {
	tc := SetupTestEnv(t)

	for _, url := range []string{"/resources?name=zzzznope&page=2", "/notes?name=zzzznope&page=5"} {
		resp := tc.requestWithAccept(http.MethodGet, url, browserAccept, "")
		if resp.Code != http.StatusOK {
			t.Errorf("GET %s: a zero-result filter should render its empty state, not redirect; got %d to %q",
				url, resp.Code, resp.Header().Get("Location"))
		}
		if !strings.Contains(resp.Body.String(), "detail-empty") {
			t.Errorf("GET %s: expected the empty state", url)
		}
	}
}
