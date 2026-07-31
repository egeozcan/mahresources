//go:build json1 && fts5

package api_tests

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"mahresources/models"
)

// Phase 3 of the 2026-07-29 UI bug hunt (docs/todo.md).
//
// internal/arch/findings_coverage_test.go audits the ledger against the test
// suite: every finding marked FIXED must be named by a test. Five were not, and
// this file is the answer to that audit rather than a new workstream.
//
//	44/52 — the /group/tree root picker was capped at 50 with no way to reach the rest
//	60/65 — a rejected relation said only "category mismatch"
//
// Finding 71 (tag chips on a detail page linking back to that page) is the third,
// and it is pinned in internal/arch/templates_test.go instead: the defect is
// which arguments a template passes to a card partial, which is decidable from
// the source and needs no fixture. Reproducing it at runtime needs a card whose
// Tags association is preloaded on a detail page, and the two surfaces the
// finding names (series siblings, similar resources) do not preload one.
//
// One of the remaining two was not a coverage gap at all. See the 60/65 case below.

// treeRootListLimit in group_template_context.go. Duplicated rather than exported
// because the number is a page-layout decision, not a contract — what this test
// pins is that whatever it is, the page says so when it bites.
const treeRootPageSize = 50

// 44/52 — GetGroupTreeRoots(50) was hardcoded and unpaginated, so a deployment
// with more than fifty root groups simply could not reach the rest of them from
// /group/tree, and the page said nothing about it.
//
// Note the shape of the fix this pins: not "show them all" — the picker is a
// list of links and a thousand of them is not a picker — but "say the list is
// cut off, and offer the route that is not capped". Raising the constant alone
// would not have worked either: GetGroupTreeRoots clamps limit > 100 back to 50.
func TestGroupTreeRootListSaysWhenItIsCutOff(t *testing.T) {
	tc := SetupTestEnv(t)

	// The threshold has to be crossed for the assertion to mean anything —
	// a fixture of three roots lands on page one and proves nothing.
	for i := range treeRootPageSize + 5 {
		g := &models.Group{Name: fmt.Sprintf("tree root %03d", i)}
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatalf("create root group %d: %v", i, err)
		}
	}

	resp := tc.MakeRequest(http.MethodGet, "/group/tree", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /group/tree = %d, want 200", resp.Code)
	}
	body := resp.Body.String()

	if !strings.Contains(body, "tree root 000") {
		t.Fatal("precondition failed: the root list rendered no roots at all")
	}
	if !strings.Contains(body, fmt.Sprintf("Showing the first %d root groups", treeRootPageSize)) {
		t.Errorf("/group/tree lists a capped set of root groups and does not say so.\n" +
			"GetGroupTreeRoots takes a hardcoded limit and clamps anything over 100 back\n" +
			"to 50, so a deployment with more roots than that had no way to reach the\n" +
			"rest and no indication any were missing.")
	}
	if !strings.Contains(body, `href="/groups"`) {
		t.Error("the truncation notice must offer the uncapped route (/groups); saying " +
			"the list is short without saying what to do instead is only half a fix")
	}
}

// The positive control for the case above, and it is the one that matters: with
// few roots the page must NOT claim to be truncated. Without it, hardcoding the
// notice would satisfy the assertion forever.
func TestGroupTreeRootListIsSilentWhenComplete(t *testing.T) {
	tc := SetupTestEnv(t)

	for i := range 3 {
		g := &models.Group{Name: fmt.Sprintf("short root %d", i)}
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatalf("create root group: %v", err)
		}
	}

	resp := tc.MakeRequest(http.MethodGet, "/group/tree", nil)
	body := resp.Body.String()
	if !strings.Contains(body, "short root 0") {
		t.Fatal("precondition failed: the root list rendered no roots at all")
	}
	if strings.Contains(body, "Showing the first") {
		t.Error("/group/tree claims its root list is truncated with three roots on it")
	}
}

// 60/65 — a relation rejected for category mismatch answered exactly
// "category mismatch": which group, which category, and what the relation type
// wanted were all left to the reader, who has just picked two groups out of a
// list of ninety.
//
// This is not a coverage gap. docs/todo.md records the message as fixed in
// Batch 12 and it was not — application_context/relation_context.go had not been
// touched since the auth merge, and a live POST against a seeded instance still
// answered {"error":"category mismatch"}. The audit found the missing test; the
// missing test was missing because the fix was.
func TestRelationCategoryMismatchNamesBothSides(t *testing.T) {
	tc := SetupTestEnv(t)

	catA := &models.Category{Name: "b13 Person"}
	catB := &models.Category{Name: "b13 Studio"}
	for _, c := range []*models.Category{catA, catB} {
		if err := tc.DB.Create(c).Error; err != nil {
			t.Fatalf("create category: %v", err)
		}
	}

	rt := &models.GroupRelationType{
		Name:           "works at",
		FromCategoryId: &catA.ID,
		ToCategoryId:   &catB.ID,
	}
	if err := tc.DB.Create(rt).Error; err != nil {
		t.Fatalf("create relation type: %v", err)
	}

	// Both groups in the wrong category, so both halves of the message must fire.
	from := &models.Group{Name: "Wrong From", CategoryId: &catB.ID}
	to := &models.Group{Name: "Wrong To", CategoryId: &catA.ID}
	for _, g := range []*models.Group{from, to} {
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatalf("create group: %v", err)
		}
	}

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/relation", url.Values{
		"FromGroupId":         {fmt.Sprint(from.ID)},
		"ToGroupId":           {fmt.Sprint(to.ID)},
		"GroupRelationTypeId": {fmt.Sprint(rt.ID)},
	})
	if resp.Code < 400 {
		t.Fatalf("POST /v1/relation = %d, want a rejection: %s", resp.Code, resp.Body.String())
	}
	msg := resp.Body.String()

	for _, want := range []string{"Wrong From", "Wrong To", "works at", "b13 Person", "b13 Studio"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the rejection does not mention %q.\n"+
				"\"category mismatch\" on its own names neither group, neither category and\n"+
				"not even which of the two picks was wrong.\ngot: %s", want, msg)
		}
	}
}

// Positive control for the message: a relation whose categories DO match is
// still created. A message test that only ever sees rejections cannot tell a
// better message from a broken check.
func TestRelationWithMatchingCategoriesIsStillCreated(t *testing.T) {
	tc := SetupTestEnv(t)

	catA := &models.Category{Name: "b13 Person"}
	catB := &models.Category{Name: "b13 Studio"}
	for _, c := range []*models.Category{catA, catB} {
		if err := tc.DB.Create(c).Error; err != nil {
			t.Fatalf("create category: %v", err)
		}
	}
	rt := &models.GroupRelationType{Name: "works at", FromCategoryId: &catA.ID, ToCategoryId: &catB.ID}
	if err := tc.DB.Create(rt).Error; err != nil {
		t.Fatalf("create relation type: %v", err)
	}
	from := &models.Group{Name: "Right From", CategoryId: &catA.ID}
	to := &models.Group{Name: "Right To", CategoryId: &catB.ID}
	for _, g := range []*models.Group{from, to} {
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatalf("create group: %v", err)
		}
	}

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/relation", url.Values{
		"FromGroupId":         {fmt.Sprint(from.ID)},
		"ToGroupId":           {fmt.Sprint(to.ID)},
		"GroupRelationTypeId": {fmt.Sprint(rt.ID)},
	})
	if resp.Code >= 400 {
		t.Fatalf("a correctly categorised relation was rejected (%d): %s", resp.Code, resp.Body.String())
	}
}
