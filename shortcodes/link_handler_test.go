package shortcodes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// linkEntity mirrors the category-id fields the [link to="category"] target
// reads via reflection: *uint on group/note, plain uint on resource.
type linkEntity struct {
	ID                 uint
	CategoryId         *uint
	ResourceCategoryId uint
	NoteTypeId         *uint
	Name               string
}

func uintPtr(v uint) *uint { return &v }

func renderLink(sc Shortcode, ctx MetaShortcodeContext) string {
	return RenderLinkShortcode(context.Background(), sc, ctx, nil, nil, 0)
}

func TestLinkInlineSelfGroup(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 7}
	sc := Shortcode{Name: "link", Attrs: map[string]string{}}
	assert.Equal(t, "/group?id=7", renderLink(sc, ctx))
}

func TestLinkInlineSelfResource(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 12}
	sc := Shortcode{Name: "link", Attrs: map[string]string{"to": "self"}}
	assert.Equal(t, "/resource?id=12", renderLink(sc, ctx))
}

func TestLinkInlineSelfNote(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "note", EntityID: 3}
	sc := Shortcode{Name: "link", Attrs: map[string]string{"to": "self"}}
	assert.Equal(t, "/note?id=3", renderLink(sc, ctx))
}

func TestLinkBlockRendersAnchor(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 7, Entity: linkEntity{ID: 7, Name: "Home"}}
	sc := Shortcode{
		Name:         "link",
		Attrs:        map[string]string{"to": "self"},
		InnerContent: `[property path="Name"]`,
		IsBlock:      true,
	}
	assert.Equal(t, `<a href="/group?id=7">Home</a>`, renderLink(sc, ctx))
}

func TestLinkInlineOwnerResource(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 12, ScopeGroupID: 5}
	sc := Shortcode{Name: "link", Attrs: map[string]string{"to": "owner"}}
	assert.Equal(t, "/group?id=5", renderLink(sc, ctx))
}

func TestLinkInlineOwnerGroup(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 12, ParentGroupID: 4}
	sc := Shortcode{Name: "link", Attrs: map[string]string{"to": "owner"}}
	assert.Equal(t, "/group?id=4", renderLink(sc, ctx))
}

func TestLinkOwnerSentinelInlineEmpty(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 12, ScopeGroupID: unresolvedScopeSentinel}
	sc := Shortcode{Name: "link", Attrs: map[string]string{"to": "owner"}}
	assert.Equal(t, "", renderLink(sc, ctx))
}

func TestLinkOwnerSentinelBlockRendersInnerOnly(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 12, ScopeGroupID: unresolvedScopeSentinel}
	sc := Shortcode{Name: "link", Attrs: map[string]string{"to": "owner"}, InnerContent: "Owner", IsBlock: true}
	assert.Equal(t, "Owner", renderLink(sc, ctx))
}

func TestLinkOwnerZeroInlineEmpty(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 12, ScopeGroupID: 0}
	sc := Shortcode{Name: "link", Attrs: map[string]string{"to": "owner"}}
	assert.Equal(t, "", renderLink(sc, ctx))
}

func TestLinkRoot(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 12, RootGroupID: 1}
	sc := Shortcode{Name: "link", Attrs: map[string]string{"to": "root"}}
	assert.Equal(t, "/group?id=1", renderLink(sc, ctx))
}

func TestLinkRootSentinelEmpty(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 12, RootGroupID: unresolvedScopeSentinel}
	sc := Shortcode{Name: "link", Attrs: map[string]string{"to": "root"}}
	assert.Equal(t, "", renderLink(sc, ctx))
}

func TestLinkCategoryGroup(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 7, Entity: linkEntity{ID: 7, CategoryId: uintPtr(9)}}
	sc := Shortcode{Name: "link", Attrs: map[string]string{"to": "category"}}
	assert.Equal(t, "/category?id=9", renderLink(sc, ctx))
}

func TestLinkCategoryResource(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 7, Entity: linkEntity{ID: 7, ResourceCategoryId: 3}}
	sc := Shortcode{Name: "link", Attrs: map[string]string{"to": "category"}}
	assert.Equal(t, "/resourceCategory?id=3", renderLink(sc, ctx))
}

func TestLinkCategoryNote(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "note", EntityID: 7, Entity: linkEntity{ID: 7, NoteTypeId: uintPtr(2)}}
	sc := Shortcode{Name: "link", Attrs: map[string]string{"to": "category"}}
	assert.Equal(t, "/noteType?id=2", renderLink(sc, ctx))
}

func TestLinkCategoryUnsetInlineEmpty(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 7, Entity: linkEntity{ID: 7, CategoryId: nil}}
	sc := Shortcode{Name: "link", Attrs: map[string]string{"to": "category"}}
	assert.Equal(t, "", renderLink(sc, ctx))
}

func TestLinkUnknownTargetInlineEmpty(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 7}
	sc := Shortcode{Name: "link", Attrs: map[string]string{"to": "bogus"}}
	assert.Equal(t, "", renderLink(sc, ctx))
}

func TestLinkInsideHrefAttribute(t *testing.T) {
	// The canonical inline usage: author writes <a href="[link]" class="…">.
	ctx := MetaShortcodeContext{EntityType: "note", EntityID: 8}
	input := `<a href="[link]" class="btn">go</a>`
	result := Process(context.Background(), input, ctx, nil, nil)
	assert.Equal(t, `<a href="/note?id=8" class="btn">go</a>`, result)
}

func TestLinkBlockViaProcess(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 2, Entity: linkEntity{ID: 2, Name: "Team"}}
	input := `[link to="self"][property path="Name"][/link]`
	result := Process(context.Background(), input, ctx, nil, nil)
	assert.Equal(t, `<a href="/group?id=2">Team</a>`, result)
}
