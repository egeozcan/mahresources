package template_filters

import (
	"strings"
	"sync"
	"testing"

	"github.com/flosch/pongo2/v4"
	"mahresources/models"
	"mahresources/models/types"
)

// countingPageContext is a PageRenderContext whose scope walk counts its calls.
// The embedded interfaces are nil: a template with no [mrql] or [partial] never
// reaches them.
type countingPageContext struct {
	QueryExecutorContext
	PartialResolverContext

	mu            sync.Mutex
	parent, roots map[uint]int
}

func (c *countingPageContext) DeferredSigningKey() []byte { return []byte("k") }
func (c *countingPageContext) MRQLPageQueryBudget() int   { return 0 }

func (c *countingPageContext) ResolveParentScopeID(groupID uint) uint {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.parent[groupID]++
	return 100 + groupID
}

func (c *countingPageContext) ResolveRootScopeID(groupID uint) uint {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.roots[groupID]++
	return 1000 + groupID
}

// A list page renders one shortcode slot per card, and every card's meta context
// carries its owner's parent and root group. Each resolution is a database walk
// up the ownership chain, so a page of 50 cards with one owner made 100 to 150
// sequential queries for an answer that never changed. The walk now runs once
// per owner per page, and every card still gets the same values.
func TestCardsShareTheirOwnersScopeWalkWithinOnePage(t *testing.T) {
	owner, other := uint(7), uint(8)
	resources := []*models.Resource{
		{ID: 1, OwnerId: &owner},
		{ID: 2, OwnerId: &owner},
		{ID: 3, OwnerId: &other},
		{ID: 4, OwnerId: &owner},
	}
	page := &countingPageContext{parent: map[uint]int{}, roots: map[uint]int{}}

	tpl, err := pongo2.FromString(`{% for r in resources %}{% process_shortcodes body r %}|{% endfor %}`)
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}
	out, err := tpl.Execute(pongo2.Context{
		"resources":   resources,
		"body":        `[property path="ID"]`,
		"_appContext": page,
	})
	if err != nil {
		t.Fatalf("execute template: %v", err)
	}
	if got := strings.Count(out, "|"); got != len(resources) {
		t.Fatalf("rendered %d cards, want %d: %q", got, len(resources), out)
	}

	for _, id := range []uint{owner, other} {
		if page.parent[id] != 1 || page.roots[id] != 1 {
			t.Errorf("owner %d: parent walked %d times, root walked %d times; want once each", id, page.parent[id], page.roots[id])
		}
	}

	// The memo is per page: a second render walks again.
	if _, err := tpl.Execute(pongo2.Context{"resources": resources[:1], "body": `[property path="ID"]`, "_appContext": page}); err != nil {
		t.Fatalf("execute again: %v", err)
	}
	if page.parent[owner] != 2 {
		t.Errorf("a new page reused the last page's walk: parent walked %d times, want 2", page.parent[owner])
	}
}

// The memoized values are the resolver's own.
func TestMemoizedScopeKeepsTheResolversValues(t *testing.T) {
	owner := uint(7)
	page := &countingPageContext{parent: map[uint]int{}, roots: map[uint]int{}}
	memo := newMetaScopeMemo(page)
	for range 2 {
		mctx := buildMetaContext(&models.Resource{ID: 1, OwnerId: &owner}, memo)
		if mctx.ScopeGroupID != 7 || mctx.ParentGroupID != 107 || mctx.RootGroupID != 1007 {
			t.Fatalf("scope = (%d,%d,%d), want (7,107,1007)", mctx.ScopeGroupID, mctx.ParentGroupID, mctx.RootGroupID)
		}
	}
	if string(memo.DeferredSigningKey()) != "k" {
		t.Error("the memo must pass the signing key through")
	}
}

// A card's Meta is already JSON from the database. It reaches the shortcode
// context as those bytes, not re-encoded, which on a list of cards with large
// Meta was a validate-and-compact pass over every blob. An empty Meta still
// reads as null, as it did when every Meta was re-encoded.
func TestMetaContextCarriesTheStoredMetaBytes(t *testing.T) {
	raw := types.JSON(`{"a": 1, "b": [true]}`)
	mctx := buildMetaContext(&models.Resource{ID: 1, Meta: raw}, nil)
	if string(mctx.Meta) != string(raw) {
		t.Errorf("Meta = %s, want the stored bytes %s", mctx.Meta, raw)
	}
	if &mctx.Meta[0] != &raw[0] {
		t.Error("Meta was copied or re-encoded instead of passed through")
	}
	if empty := buildMetaContext(&models.Resource{ID: 2}, nil); string(empty.Meta) != "null" {
		t.Errorf("empty Meta = %q, want null", empty.Meta)
	}
}
