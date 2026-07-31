package api_tests

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"mahresources/models"
)

// WS14 — the long tail of the 2026-07-29 UI bug hunt.
//
//	61  — REJECTED: "taxonomy entries can never be deleted, no UI and no endpoint"
//	98  — deleting a category in use confirms with a generic prompt
//	104 — the export page prints a raw Go duration and an off-palette link
//	112 — /admin/settings prints raw snake_case config group keys as headings
//	117 — the dashboard's Recent Activity widget has no "View All"
//	129 — edit forms have no Cancel or Back
//	137 — deleting a relation lands on /groups
//	138 — relation cards mirror the badge/name order between the two halves
//	140 — the version delete confirm does not say which version
//	149 — cards advertise "Double-click to edit" where editing is impossible
//
// Everything a browser has to instantiate (Alpine `x-show` branches, the text of
// a window.confirm at submit time, focus) is asserted in
// e2e/tests/regressions/ws14-long-tail.spec.ts instead. A Go test sees the served
// HTML, which contains every `<template>`'s contents.

// ---------------------------------------------------------------------------
// 61 — rejection control.
//
// The report says: "There is no delete affordance anywhere for relation types,
// note types or categories, and the DELETE endpoint does not exist." Phase 1
// already showed the endpoints exist and are POST. The UI half is wrong too, and
// for a reason worth pinning: `partials/form/deleteButton.tpl` renders
// `<input type="submit" value="Delete">`, not a `<button>`. The report's probe
// collected button elements ("btns":["Edit","Edit Tags"]) and so could not see
// the control that was there all along.
//
// This test passes in both directions — that is what a rejection control is for.
// It fails only if someone removes the affordance the finding claimed was absent.
func TestTaxonomyDetailPagesOfferDelete(t *testing.T) {
	tc := SetupTestEnv(t)

	cat := &models.Category{Name: "ws14 category"}
	if err := tc.DB.Create(cat).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
	nt := tc.CreateNoteType(t, "ws14 note type")
	rt := &models.GroupRelationType{Name: "ws14 relation type"}
	if err := tc.DB.Create(rt).Error; err != nil {
		t.Fatalf("create relation type: %v", err)
	}
	rc := &models.ResourceCategory{Name: "ws14 resource category"}
	if err := tc.DB.Create(rc).Error; err != nil {
		t.Fatalf("create resource category: %v", err)
	}
	tag := &models.Tag{Name: "ws14 tag"}
	if err := tc.DB.Create(tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}

	cases := []struct {
		name   string
		url    string
		action string
	}{
		{"category", fmt.Sprintf("/category?id=%d", cat.ID), "/v1/category/delete"},
		{"note type", fmt.Sprintf("/noteType?id=%d", nt.ID), "/v1/note/noteType/delete"},
		{"relation type", fmt.Sprintf("/relationType?id=%d", rt.ID), "/v1/relationType/delete"},
		{"resource category", fmt.Sprintf("/resourceCategory?id=%d", rc.ID), "/v1/resourceCategory/delete"},
		{"tag", fmt.Sprintf("/tag?id=%d", tag.ID), "/v1/tag/delete"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, body := tc.getHTML(t, c.url)
			if code != http.StatusOK {
				t.Fatalf("GET %s = %d, want 200", c.url, code)
			}
			if !strings.Contains(body, `action="`+c.action) {
				t.Errorf("%s renders no delete form pointing at %s", c.url, c.action)
			}
			// The control is an <input type="submit">, which is why the report's
			// button sweep missed it. Assert the shape so a later refactor to a
			// <button> does not quietly invalidate this note.
			form := sectionAfter(body, `action="`+c.action)
			if !strings.Contains(form, `type="submit"`) {
				t.Errorf("%s delete form has no submit control: %.200q", c.url, form)
			}
		})
	}

	// Positive control on the endpoint half of the claim: the route answers, and
	// it answers to POST.
	resp := tc.MakeFormRequest(http.MethodPost, fmt.Sprintf("/v1/relationType/delete?Id=%d", rt.ID), nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /v1/relationType/delete = %d, want 200; body %s", resp.Code, resp.Body.String())
	}
	var remaining int64
	tc.DB.Model(&models.GroupRelationType{}).Where("id = ?", rt.ID).Count(&remaining)
	if remaining != 0 {
		t.Errorf("relation type %d survived its delete", rt.ID)
	}
}

// The default resource category is deliberately undeletable, and that exception
// must not be mistaken for finding 61's claim. Positive control for the case
// above: an id that IS the default renders no delete form.
func TestDefaultResourceCategoryOffersNoDelete(t *testing.T) {
	tc := SetupTestEnv(t)
	code, body := tc.getHTML(t, "/resourceCategory?id=1")
	if code != http.StatusOK {
		t.Fatalf("GET /resourceCategory?id=1 = %d, want 200", code)
	}
	if strings.Contains(body, `action="/v1/resourceCategory/delete`) {
		t.Error("the default resource category offers a delete it must refuse")
	}
}

// ---------------------------------------------------------------------------
// 98 — "Delete category 'X'? N groups will become Uncategorized."
//
// The confirm was the generic "Are you sure you want to delete?", and a category
// in use silently uncategorized its groups. The count has to be a real count, not
// the length of the preloaded association: GetCategory preloads with pageLimit,
// so a category holding more than 50 groups would under-report.
func TestCategoryDeleteConfirmNamesTheCategoryAndItsGroups(t *testing.T) {
	tc := SetupTestEnv(t)

	used := &models.Category{Name: "ws14 used category"}
	if err := tc.DB.Create(used).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
	for i := 0; i < 3; i++ {
		g := &models.Group{Name: fmt.Sprintf("ws14 grp %d", i), CategoryId: &used.ID}
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatalf("create group: %v", err)
		}
	}

	_, body := tc.getHTML(t, fmt.Sprintf("/category?id=%d", used.ID))
	got := confirmMessageFor(t, body, "/v1/category/delete")
	want := "Delete category 'ws14 used category'? 3 groups will become Uncategorized."
	if got != want {
		t.Errorf("category delete confirm does not name the blast radius.\nwant: %q\ngot:  %q", want, got)
	}

	// Positive control: an unused category says so without inventing a count.
	unused := &models.Category{Name: "ws14 unused category"}
	if err := tc.DB.Create(unused).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
	_, body = tc.getHTML(t, fmt.Sprintf("/category?id=%d", unused.ID))
	got = confirmMessageFor(t, body, "/v1/category/delete")
	if got != "Delete category 'ws14 unused category'?" {
		t.Errorf("unused category confirm wrong: %q", got)
	}
	if strings.Contains(got, "will become Uncategorized") {
		t.Error("unused category confirm threatens groups that do not exist")
	}
}

// One group is one group. "1 groups will become Uncategorized" reads as a defect
// in the message that exists to be trusted.
func TestCategoryDeleteConfirmIsSingularForOneGroup(t *testing.T) {
	tc := SetupTestEnv(t)
	cat := &models.Category{Name: "ws14 single"}
	if err := tc.DB.Create(cat).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
	g := &models.Group{Name: "ws14 lone group", CategoryId: &cat.ID}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	_, body := tc.getHTML(t, fmt.Sprintf("/category?id=%d", cat.ID))
	got := confirmMessageFor(t, body, "/v1/category/delete")
	want := "Delete category 'ws14 single'? 1 group will become Uncategorized."
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

// The count must survive past the association preload's page limit. Three rows
// cannot prove a fifty-row cap (docs/lessons.md).
func TestCategoryDeleteConfirmCountsPastThePreloadLimit(t *testing.T) {
	tc := SetupTestEnv(t)
	cat := &models.Category{Name: "ws14 big category"}
	if err := tc.DB.Create(cat).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
	const n = 57
	for i := 0; i < n; i++ {
		g := &models.Group{Name: fmt.Sprintf("ws14 big grp %d", i), CategoryId: &cat.ID}
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatalf("create group: %v", err)
		}
	}
	_, body := tc.getHTML(t, fmt.Sprintf("/category?id=%d", cat.ID))
	got := confirmMessageFor(t, body, "/v1/category/delete")
	if !strings.Contains(got, fmt.Sprintf("? %d groups will become Uncategorized.", n)) {
		t.Errorf("confirm did not report all %d groups: %q", n, got)
	}
	// The meta strip reads the same number, and used to read the preload's cap.
	if !strings.Contains(body, fmt.Sprintf(`<span class="meta-strip-value">%d</span>`, n)) {
		t.Errorf("the Groups meta strip still reports the capped association count, not %d", n)
	}
}

// ---------------------------------------------------------------------------
// 140 — "Delete this version?" was identical on every row.
func TestVersionDeleteConfirmNamesTheVersion(t *testing.T) {
	tc := SetupTestEnv(t)
	res := tc.CreateResourceWithType(t, "ws14 versioned", "image/png")

	versions := []models.ResourceVersion{
		{ResourceID: res.ID, VersionNumber: 1, Comment: "first cut", FileSize: 1024, Hash: "aaa1"},
		{ResourceID: res.ID, VersionNumber: 2, Comment: "Rotated 90 degrees", FileSize: 37 * 1024, Hash: "bbb2"},
	}
	for i := range versions {
		if err := tc.DB.Create(&versions[i]).Error; err != nil {
			t.Fatalf("create version: %v", err)
		}
	}
	res.CurrentVersionID = &versions[1].ID
	if err := tc.DB.Save(res).Error; err != nil {
		t.Fatalf("save resource: %v", err)
	}

	_, body := tc.getHTML(t, fmt.Sprintf("/resource?id=%d", res.ID))
	// v2 is current and so renders no delete; v1 does.
	got := confirmMessageFor(t, body, "version/delete")
	want := "Delete version 1 (first cut, 1024 B)?"
	if got != want {
		t.Errorf("version delete confirm does not identify the row.\nwant: %q\ngot:  %q", want, got)
	}
	if strings.Contains(body, "Delete this version?") {
		t.Error("the generic version confirm is still rendered")
	}
}

// A version with no comment must still be identified, and must not print an
// empty parenthetical.
func TestVersionDeleteConfirmWithoutAComment(t *testing.T) {
	tc := SetupTestEnv(t)
	res := tc.CreateResourceWithType(t, "ws14 uncommented", "image/png")
	v1 := &models.ResourceVersion{ResourceID: res.ID, VersionNumber: 1, FileSize: 2048, Hash: "ccc1"}
	v2 := &models.ResourceVersion{ResourceID: res.ID, VersionNumber: 2, FileSize: 4096, Hash: "ddd2"}
	for _, v := range []*models.ResourceVersion{v1, v2} {
		if err := tc.DB.Create(v).Error; err != nil {
			t.Fatalf("create version: %v", err)
		}
	}
	res.CurrentVersionID = &v2.ID
	if err := tc.DB.Save(res).Error; err != nil {
		t.Fatalf("save resource: %v", err)
	}
	_, body := tc.getHTML(t, fmt.Sprintf("/resource?id=%d", res.ID))
	got := confirmMessageFor(t, body, "version/delete")
	want := "Delete version 1 (2.0 KB)?"
	if got != want {
		t.Errorf("uncommented version confirm wrong.\nwant: %q\ngot:  %q", want, got)
	}
}

// ---------------------------------------------------------------------------
// 104 — the export page printed Go's `time.Duration.String()` where
// /admin/settings shows the same setting as "24h".
func TestExportRetentionIsRenderedTheWaySettingsRendersIt(t *testing.T) {
	tc := SetupTestEnv(t)
	code, body := tc.getHTML(t, "/admin/export")
	if code != http.StatusOK {
		t.Fatalf("GET /admin/export = %d, want 200", code)
	}
	if strings.Contains(body, "24h0m0s") {
		t.Error("/admin/export still prints the raw Go duration 24h0m0s")
	}
	if !strings.Contains(body, ">24h<") {
		t.Errorf("/admin/export does not print the short duration: %.300q", sectionAfter(body, "export-retention-helper"))
	}
}

// 104(b)/(c): the download link was `text-blue-700` on a page whose every other
// link is amber, and the progress bar was a bare native <progress>.
func TestExportPageUsesTheAppPalette(t *testing.T) {
	tc := SetupTestEnv(t)
	_, body := tc.getHTML(t, "/admin/export")
	link := findOpenTag(body, `data-testid="export-download-link"`, "a")
	if link == "" {
		t.Fatal("no export-download-link on /admin/export")
	}
	if strings.Contains(link, "text-blue-") {
		t.Errorf("Download tar is still off-palette blue: %s", link)
	}
	if !strings.Contains(link, "text-amber-") {
		t.Errorf("Download tar is not in the amber link palette: %s", link)
	}
	progress := findOpenTag(body, `class=`, "progress")
	if progress == "" {
		t.Fatal("no <progress> on /admin/export")
	}
	if !strings.Contains(progress, "export-progress-bar") {
		t.Errorf("the progress bar carries no app class to style it: %s", progress)
	}
}

// ---------------------------------------------------------------------------
// 112 — raw snake_case config group keys as section headings.
func TestSettingsSectionHeadingsAreHumanReadable(t *testing.T) {
	tc := SetupTestEnv(t)
	code, body := tc.getHTML(t, "/admin/settings")
	if code != http.StatusOK {
		t.Fatalf("GET /admin/settings = %d, want 200", code)
	}
	for _, heading := range []string{
		">Uploads<", ">Queries<", ">Remote downloads<", ">Sharing<",
		">Docs<", ">Deduplication<", ">Exports<",
	} {
		if !strings.Contains(body, heading) {
			t.Errorf("missing humanised settings heading %s", heading)
		}
	}
	if strings.Contains(body, ">remote_downloads<") {
		t.Error("/admin/settings still prints the raw config group key remote_downloads")
	}
	// The machine key still identifies the section for aria-labelledby, so the
	// humanising must not have moved it.
	if !strings.Contains(body, `id="grp-remote_downloads"`) {
		t.Error("the stable group id was renamed along with the label")
	}
}

// ---------------------------------------------------------------------------
// 117 — Recent Activity was the only dashboard widget with no "View All".
func TestEveryDashboardWidgetLinksToItsFullView(t *testing.T) {
	tc := SetupTestEnv(t)
	code, body := tc.getHTML(t, "/dashboard")
	if code != http.StatusOK {
		t.Fatalf("GET /dashboard = %d, want 200", code)
	}
	for _, w := range []struct{ heading, href string }{
		{"Recent Resources", "/resources"},
		{"Recent Notes", "/notes"},
		{"Recent Groups", "/groups"},
		{"Recent Tags", "/tags"},
		{"Recent Activity", "/logs"},
	} {
		section := sectionAfter(body, ">"+w.heading+"<")
		if section == "" {
			t.Errorf("no %s widget on the dashboard", w.heading)
			continue
		}
		// Look only as far as the next widget heading.
		if cut := strings.Index(section, "dashboard-section-title"); cut > 0 {
			section = section[:cut]
		}
		if !strings.Contains(section, `href="`+w.href+`"`) {
			t.Errorf("%s has no View All link to %s", w.heading, w.href)
		}
	}
}

// ---------------------------------------------------------------------------
// 129 — no Cancel or Back on any create/edit form.
func TestCreateAndEditFormsOfferAWayOut(t *testing.T) {
	tc := SetupTestEnv(t)

	note := tc.CreateDummyNote("ws14 note")
	group := tc.CreateDummyGroup("ws14 group")
	res := tc.CreateResourceWithType(t, "ws14 resource", "image/png")
	tag := &models.Tag{Name: "ws14 form tag"}
	cat := &models.Category{Name: "ws14 form category"}
	rc := &models.ResourceCategory{Name: "ws14 form rescat"}
	nt := tc.CreateNoteType(t, "ws14 form note type")
	rt := &models.GroupRelationType{Name: "ws14 form relation type"}
	q := &models.Query{Name: "ws14 form query", Text: "select 1"}
	tp := &models.TemplatePartial{Name: "ws14-form-partial", Content: "x"}
	for _, m := range []any{tag, cat, rc, rt, q, tp} {
		if err := tc.DB.Create(m).Error; err != nil {
			t.Fatalf("seed %T: %v", m, err)
		}
	}

	cases := []struct {
		name       string
		url        string
		wantCancel string
	}{
		{"note edit", fmt.Sprintf("/note/edit?id=%d", note.ID), fmt.Sprintf("/note?id=%d", note.ID)},
		{"note new", "/note/new", "/notes"},
		{"group edit", fmt.Sprintf("/group/edit?id=%d", group.ID), fmt.Sprintf("/group?id=%d", group.ID)},
		{"group new", "/group/new", "/groups"},
		{"resource edit", fmt.Sprintf("/resource/edit?id=%d", res.ID), fmt.Sprintf("/resource?id=%d", res.ID)},
		{"tag edit", fmt.Sprintf("/tag/edit?id=%d", tag.ID), fmt.Sprintf("/tag?id=%d", tag.ID)},
		{"tag new", "/tag/new", "/tags"},
		{"category edit", fmt.Sprintf("/category/edit?id=%d", cat.ID), fmt.Sprintf("/category?id=%d", cat.ID)},
		{"category new", "/category/new", "/categories"},
		{"resource category edit", fmt.Sprintf("/resourceCategory/edit?id=%d", rc.ID), fmt.Sprintf("/resourceCategory?id=%d", rc.ID)},
		{"note type edit", fmt.Sprintf("/noteType/edit?id=%d", nt.ID), fmt.Sprintf("/noteType?id=%d", nt.ID)},
		{"relation type edit", fmt.Sprintf("/relationType/edit?id=%d", rt.ID), fmt.Sprintf("/relationType?id=%d", rt.ID)},
		{"query edit", fmt.Sprintf("/query/edit?id=%d", q.ID), fmt.Sprintf("/query?id=%d", q.ID)},
		{"template partial edit", fmt.Sprintf("/templatePartial/edit?id=%d", tp.ID), fmt.Sprintf("/templatePartial?id=%d", tp.ID)},
		{"relation new", "/relation/new", "/relations"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, body := tc.getHTML(t, c.url)
			if code != http.StatusOK {
				t.Fatalf("GET %s = %d, want 200", c.url, code)
			}
			cancel := findOpenTag(body, `data-testid="form-cancel"`, "a")
			if cancel == "" {
				t.Fatalf("%s has no Cancel link", c.url)
			}
			if !strings.Contains(cancel, `href="`+c.wantCancel+`"`) {
				t.Errorf("%s Cancel goes to the wrong place: %s (want href=%q)", c.url, cancel, c.wantCancel)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 137 — deleting a relation landed on /groups.
func TestDeletingARelationReturnsToTheRelationsList(t *testing.T) {
	tc := SetupTestEnv(t)
	rel := tc.createNamedRelation(t, "ws14 relation")

	resp := tc.requestWithAccept(http.MethodPost, fmt.Sprintf("/v1/relation/delete?Id=%d", rel.ID), browserAccept, "")
	if resp.Code != http.StatusSeeOther {
		t.Fatalf("POST /v1/relation/delete = %d, want 303; body %s", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Location"); got != "/relations" {
		t.Errorf("relation delete redirected to %q, want /relations", got)
	}
}

// An explicit ?redirect= still wins — the fix must not have replaced one
// hardcoded destination with another that ignores the caller.
func TestRelationDeleteStillHonoursAnExplicitRedirect(t *testing.T) {
	tc := SetupTestEnv(t)
	rel := tc.createNamedRelation(t, "ws14 relation redirect")
	url := fmt.Sprintf("/v1/relation/delete?Id=%d&redirect=%%2Fgroup%%3Fid%%3D%d", rel.ID, *rel.FromGroupId)
	resp := tc.requestWithAccept(http.MethodPost, url, browserAccept, "")
	if resp.Code != http.StatusSeeOther {
		t.Fatalf("POST with redirect = %d, want 303", resp.Code)
	}
	if got := resp.Header().Get("Location"); !strings.HasPrefix(got, "/group?id=") {
		t.Errorf("explicit redirect ignored, went to %q", got)
	}
}

// ---------------------------------------------------------------------------
// 138 — the two halves of a relation pair rendered the badge and the name in
// opposite orders, on both the list and the detail page, and in opposite
// directions from each other.
func TestRelationPairHalvesRenderInTheSameOrder(t *testing.T) {
	tc := SetupTestEnv(t)
	rel := tc.createNamedRelation(t, "ws14 ordered relation")

	for _, page := range []string{"/relations", fmt.Sprintf("/relation?id=%d", rel.ID)} {
		t.Run(page, func(t *testing.T) {
			code, body := tc.getHTML(t, page)
			if code != http.StatusOK {
				t.Fatalf("GET %s = %d, want 200", page, code)
			}
			// Scope to the two group cards. Measuring badge/title positions over
			// the whole document passes vacuously on /relations, where the row's
			// own <h2 class="card-title"> (the relation's name) precedes both
			// halves and shifts every index — the first version of this test did
			// exactly that and reported the list page clean while it was mirrored.
			cards := groupCardsIn(body)
			if len(cards) < 2 {
				t.Fatalf("%s: expected two group cards, got %d", page, len(cards))
			}
			order := make([]bool, 2)
			for i := 0; i < 2; i++ {
				badge := strings.Index(cards[i], "card-badge--relation")
				title := strings.Index(cards[i], `class="card-title"`)
				if badge < 0 || title < 0 {
					t.Fatalf("%s: card %d has no badge/title pair (badge=%d title=%d)", page, i, badge, title)
				}
				order[i] = badge < title
			}
			if order[0] != order[1] {
				t.Errorf("%s: the from-half renders the badge %s the name and the to-half renders it %s",
					page, beforeOrAfter(order[0]), beforeOrAfter(order[1]))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 149 — a card advertised "Double-click to edit" on a description whose editor
// can never open, because the card include passes no descriptionEditUrl.
//
// The plan blamed `@dblclick="editing = !!descriptionEditUrl"`. That markup is
// gone: Batch 4 replaced it with `startEditing()`, which returns early when the
// url is empty. The surviving half is the unconditional title attribute.
func TestDoubleClickToEditIsOnlyAdvertisedWhereItWorks(t *testing.T) {
	tc := SetupTestEnv(t)
	tag := &models.Tag{Name: "ws14 tooltip tag", Description: "a description on a card"}
	if err := tc.DB.Create(tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}

	// A card list: the include passes no descriptionEditUrl, so no editor exists.
	_, list := tc.getHTML(t, "/tags")
	if !strings.Contains(list, `descriptionEditor({ url: '' })`) {
		t.Fatal("precondition failed: /tags renders no read-only description editor scope")
	}
	if strings.Contains(list, `title="Double-click to edit"`) {
		t.Error("/tags advertises an editor its cards do not have")
	}

	// The detail page does pass one, and must keep the affordance.
	_, detail := tc.getHTML(t, fmt.Sprintf("/tag?id=%d", tag.ID))
	if !strings.Contains(detail, `title="Double-click to edit"`) {
		t.Error("/tag lost the affordance that does work there")
	}
}

// ---------------------------------------------------------------------------
// helpers

// sectionAfter returns the markup starting at the first occurrence of marker,
// capped so a failure message stays readable.
func sectionAfter(body, marker string) string {
	i := strings.Index(body, marker)
	if i < 0 {
		return ""
	}
	end := i + 2000
	if end > len(body) {
		end = len(body)
	}
	return body[i:end]
}

// confirmMessageFor returns the decoded confirm text of the first <form> whose
// open tag contains marker. Pongo2's escapejs writes ' and ? as ' / ?,
// so a raw substring assertion would be about the escaping table rather than
// about the sentence a reader is shown.
func confirmMessageFor(t *testing.T, body, marker string) string {
	t.Helper()
	const open = "confirmAction({ message: '"
	for _, tag := range openTagsWithin(body, "form") {
		if !strings.Contains(tag, marker) {
			continue
		}
		i := strings.Index(tag, open)
		if i < 0 {
			t.Fatalf("form matching %q carries no confirm message: %s", marker, tag)
		}
		rest := tag[i+len(open):]
		j := strings.Index(rest, "'")
		if j < 0 {
			t.Fatalf("unterminated confirm message in: %s", tag)
		}
		decoded, err := strconv.Unquote(`"` + rest[:j] + `"`)
		if err != nil {
			t.Fatalf("confirm message %q is not a decodable JS string: %v", rest[:j], err)
		}
		return decoded
	}
	t.Fatalf("no <form> matching %q on the page", marker)
	return ""
}

// groupCardsIn splits the markup at each `card group-card` article so a
// per-card assertion cannot be satisfied by markup belonging to another card.
func groupCardsIn(body string) []string {
	starts := allIndexes(body, `class="card group-card`)
	out := make([]string, 0, len(starts))
	for i, s := range starts {
		end := len(body)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		out = append(out, body[s:end])
	}
	return out
}

func beforeOrAfter(b bool) string {
	if b {
		return "before"
	}
	return "after"
}

func allIndexes(body, marker string) []int {
	var out []int
	for i := 0; ; {
		j := strings.Index(body[i:], marker)
		if j < 0 {
			return out
		}
		out = append(out, i+j)
		i += j + len(marker)
	}
}
