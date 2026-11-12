package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
)

// listFilterLeakFixture is a subtree (inside-root, category B) under a hidden
// ancestor, and an unrelated hidden group OUTSIDE (category A). Each side has a
// resource carrying the same image DHash, and inside rows are linked to
// OUTSIDE and to a note it owns.
type listFilterLeakFixture struct {
	catA, catB, typeA, typeB      uint
	ancestor, outside, root       uint
	insideRes, outsideRes         uint
	insideNote, outsideNote, tagT uint
}

func buildListFilterLeakFixture(t *testing.T, tc *TestContext) listFilterLeakFixture {
	t.Helper()
	create := func(v any) {
		t.Helper()
		require.NoError(t, tc.DB.Create(v).Error)
	}
	catA := &models.Category{Name: "cat-A"}
	create(catA)
	catB := &models.Category{Name: "cat-B"}
	create(catB)
	typeA := &models.GroupRelationType{Name: "type-A", FromCategoryId: &catA.ID, ToCategoryId: &catA.ID}
	create(typeA)
	typeB := &models.GroupRelationType{Name: "type-B", FromCategoryId: &catB.ID, ToCategoryId: &catB.ID}
	create(typeB)

	ancestor := &models.Group{Name: "ANCESTOR"}
	create(ancestor)
	outside := &models.Group{Name: "OUTSIDE", CategoryId: &catA.ID}
	create(outside)
	root := &models.Group{Name: "inside-root", CategoryId: &catB.ID, OwnerId: &ancestor.ID}
	create(root)

	tag := &models.Tag{Name: "lf-tag"}
	create(tag)
	outsideNote := &models.Note{Name: "OUTSIDE-note", OwnerId: &outside.ID}
	create(outsideNote)
	outsideRes := &models.Resource{Name: "OUTSIDE-res", OwnerId: &outside.ID, Hash: "lf-out", Location: "lf-out"}
	create(outsideRes)
	insideRes := &models.Resource{Name: "inside-res", OwnerId: &root.ID, Hash: "lf-in", Location: "lf-in",
		Tags: []*models.Tag{tag}, Groups: []*models.Group{outside}, Notes: []*models.Note{outsideNote}}
	create(insideRes)
	insideNote := &models.Note{Name: "inside-note", OwnerId: &root.ID, Groups: []*models.Group{outside}}
	create(insideNote)
	create(&models.ImageHash{ResourceId: &insideRes.ID, DHash: "dup"})
	create(&models.ImageHash{ResourceId: &outsideRes.ID, DHash: "dup"})

	return listFilterLeakFixture{
		catA: catA.ID, catB: catB.ID, typeA: typeA.ID, typeB: typeB.ID,
		ancestor: ancestor.ID, outside: outside.ID, root: root.ID,
		insideRes: insideRes.ID, outsideRes: outsideRes.ID,
		insideNote: insideNote.ID, outsideNote: outsideNote.ID, tagT: tag.ID,
	}
}

func listedResourceNames(t *testing.T, ctx *application_context.MahresourcesContext, q *query_models.ResourceSearchQuery) []string {
	t.Helper()
	rows, err := ctx.GetResources(0, 50, q)
	require.NoError(t, err)
	names := []string{}
	for _, r := range rows {
		names = append(names, r.Name)
	}
	sort.Strings(names)
	return names
}

func relationTypeNames(t *testing.T, ctx *application_context.MahresourcesContext, q *query_models.RelationshipTypeQuery) []string {
	t.Helper()
	rows, err := ctx.GetRelationTypes(0, 50, q)
	require.NoError(t, err)
	names := []string{}
	for _, r := range rows {
		names = append(names, r.Name)
	}
	sort.Strings(names)
	return names
}

// assertResourceFilterFindsNothing runs one resource filter through every
// consumer of the resource list builder and asserts each one finds nothing.
// massEditTag 0 skips mass edit, which a guest may not reach.
func assertResourceFilterFindsNothing(t *testing.T, ctx *application_context.MahresourcesContext, label string, q func() *query_models.ResourceSearchQuery, filter string, massEditTag uint) {
	t.Helper()
	assert.Empty(t, listedResourceNames(t, ctx, q()), "%s: GetResources", label)

	n, err := ctx.GetResourceCount(q())
	require.NoError(t, err, label)
	assert.Zero(t, n, "%s: GetResourceCount", label)

	tags, err := ctx.GetPopularResourceTags(q())
	require.NoError(t, err, label)
	assert.Empty(t, tags, "%s: GetPopularResourceTags", label)

	buckets, err := ctx.GetResourceTimelineCounts(q(), application_context.GenerateBucketBoundaries("yearly", time.Now(), 3))
	require.NoError(t, err, label)
	for _, b := range buckets {
		assert.Zero(t, b.Created, "%s: timeline created %s", label, b.Label)
		assert.Zero(t, b.Updated, "%s: timeline updated %s", label, b.Label)
	}

	page, err := ctx.BrowseEntities(&query_models.EntityPickerQuery{Entity: "resource", Filter: filter})
	require.NoError(t, err, label)
	assert.Empty(t, page.Items, "%s: entity picker", label)

	if massEditTag != 0 {
		result, err := ctx.MassEditResources(&query_models.MassEditQuery{
			Target: "filter", Filter: filter, DryRun: true, TagsOp: "add", TagIds: []uint{massEditTag},
		})
		require.NoError(t, err, label)
		assert.Zero(t, result.Matched, "%s: mass-edit filter count", label)
	}
}

func TestListFiltersDoNotReadOutsideTheSubtree(t *testing.T) {
	runListFiltersDoNotReadOutsideTheSubtree(t, SetupTestEnv(t))
}

// runListFiltersDoNotReadOutsideTheSubtree: a resource list filter whose
// predicate reads a row besides the listed one (ShowWithSimilar's duplicate,
// a related group or note) must not read a row the
// caller cannot see; an invisible row behaves as a missing one.
func runListFiltersDoNotReadOutsideTheSubtree(t *testing.T, tc *TestContext) {
	f := buildListFilterLeakFixture(t, tc)
	root := f.root

	similar := func() *query_models.ResourceSearchQuery {
		return &query_models.ResourceSearchQuery{ShowWithSimilar: true}
	}
	leaks := []struct {
		name   string
		query  func() *query_models.ResourceSearchQuery
		filter string
		admin  []string
	}{
		{"show with similar", similar, "ShowWithSimilar=1", []string{"OUTSIDE-res", "inside-res"}},
		{"related group", func() *query_models.ResourceSearchQuery {
			return &query_models.ResourceSearchQuery{Groups: []uint{f.outside}}
		}, fmt.Sprintf("Groups=%d", f.outside), []string{"OUTSIDE-res", "inside-res"}},
		{"related note", func() *query_models.ResourceSearchQuery {
			return &query_models.ResourceSearchQuery{Notes: []uint{f.outsideNote}}
		}, fmt.Sprintf("Notes=%d", f.outsideNote), []string{"inside-res"}},
	}

	principals := map[string]struct {
		ctx         *application_context.MahresourcesContext
		massEditTag uint
	}{
		"scoped guest": {principalCtx(tc, models.RoleGuest, &root), 0},
		"scoped user":  {principalCtx(tc, models.RoleUser, &root), f.tagT},
		"deny-all":     {principalCtx(tc, models.RoleGuest, nil), 0},
	}
	for who, p := range principals {
		for _, l := range leaks {
			assertResourceFilterFindsNothing(t, p.ctx, who+"/"+l.name, l.query, l.filter, p.massEditTag)
		}
	}

	unrestricted := map[string]*application_context.MahresourcesContext{
		"admin (system)": tc.AppCtx,
		"editor":         principalCtx(tc, models.RoleEditor, nil),
		"unscoped user":  principalCtx(tc, models.RoleUser, nil),
	}
	for who, ctx := range unrestricted {
		for _, l := range leaks {
			assert.Equal(t, l.admin, listedResourceNames(t, ctx, l.query()), "%s/%s", who, l.name)
		}
	}

	// A visible duplicate still counts, and the list agrees with MRQL's
	// similarImages for both contexts.
	scoped := principalCtx(tc, models.RoleGuest, &root)
	second := &models.Resource{Name: "inside-res2", OwnerId: &root, Hash: "lf-in2", Location: "lf-in2"}
	require.NoError(t, tc.DB.Create(second).Error)
	require.NoError(t, tc.DB.Create(&models.ImageHash{ResourceId: &second.ID, DHash: "dup"}).Error)
	assert.Equal(t, []string{"inside-res", "inside-res2"}, listedResourceNames(t, scoped, similar()))
	n, err := scoped.GetResourceCount(similar())
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
	mrqlSimilar := &query_models.ResourceSearchQuery{MRQL: "similarImages IS NOT EMPTY"}
	for who, ctx := range map[string]*application_context.MahresourcesContext{"scoped": scoped, "admin": tc.AppCtx} {
		assert.Equal(t, listedResourceNames(t, ctx, mrqlSimilar), listedResourceNames(t, ctx, similar()), "%s: MRQL parity", who)
	}

	// The same rule for notes: a note related to a hidden group is not found
	// by naming that group.
	notes, err := scoped.GetNotes(0, 50, &query_models.NoteQuery{Groups: []uint{f.outside}})
	require.NoError(t, err)
	assert.Empty(t, notes, "scoped note Groups filter")
	count, err := scoped.GetNoteCount(&query_models.NoteQuery{Groups: []uint{f.outside}})
	require.NoError(t, err)
	assert.Zero(t, count, "scoped note Groups count")
	notes, err = tc.AppCtx.GetNotes(0, 50, &query_models.NoteQuery{Groups: []uint{f.outside}})
	require.NoError(t, err)
	names := []string{}
	for _, nt := range notes {
		names = append(names, nt.Name)
	}
	sort.Strings(names)
	assert.Equal(t, []string{"OUTSIDE-note", "inside-note"}, names, "admin note Groups filter")
	page, err := scoped.BrowseEntities(&query_models.EntityPickerQuery{Entity: "note", Filter: fmt.Sprintf("Groups=%d", f.outside)})
	require.NoError(t, err)
	assert.Empty(t, page.Items, "scoped note picker Groups filter")
}

func TestRelationTypeGroupFiltersHideInvisibleGroups(t *testing.T) {
	runRelationTypeGroupFiltersHideInvisibleGroups(t, SetupTestEnv(t))
}

// runRelationTypeGroupFiltersHideInvisibleGroups: ForFromGroup / ForToGroup
// resolve a group to its category. A group the caller cannot see resolves to
// nothing, exactly like a group that does not exist.
func runRelationTypeGroupFiltersHideInvisibleGroups(t *testing.T, tc *TestContext) {
	f := buildListFilterLeakFixture(t, tc)
	root := f.root
	const missing = 999999

	for who, scoped := range map[string]*application_context.MahresourcesContext{
		"scoped guest": principalCtx(tc, models.RoleGuest, &root),
		"scoped user":  principalCtx(tc, models.RoleUser, &root),
	} {
		for _, side := range []string{"from", "to"} {
			q := func(id uint) *query_models.RelationshipTypeQuery {
				if side == "from" {
					return &query_models.RelationshipTypeQuery{ForFromGroup: id}
				}
				return &query_models.RelationshipTypeQuery{ForToGroup: id}
			}
			label := who + "/" + side
			assert.Empty(t, relationTypeNames(t, scoped, q(f.outside)), label)
			n, err := scoped.GetRelationTypesCount(q(f.outside))
			require.NoError(t, err)
			assert.Zero(t, n, label+" count")
			assert.Empty(t, relationTypeNames(t, scoped, q(missing)), label+" missing")
			assert.Equal(t, []string{"type-B"}, relationTypeNames(t, scoped, q(f.root)), label+" visible")
			assert.Equal(t, []string{"type-A"}, relationTypeNames(t, tc.AppCtx, q(f.outside)), label+" admin")
		}

		filter := fmt.Sprintf("ForFromGroup=%d", f.outside)
		page, err := scoped.BrowseEntities(&query_models.EntityPickerQuery{Entity: "relationType", Filter: filter})
		require.NoError(t, err)
		assert.Empty(t, page.Items, who+": picker filter")
		page, err = scoped.BrowseEntities(&query_models.EntityPickerQuery{Entity: "relationType", Constraints: filter})
		require.NoError(t, err)
		assert.Empty(t, page.Items, who+": picker constraints")
		resolved, err := scoped.ResolvePickerEntities(&query_models.EntityPickerResolveQuery{Entity: "relationType", Constraints: filter, IDs: []uint{f.typeA}})
		require.NoError(t, err)
		assert.Empty(t, resolved, who+": picker resolve")
	}

	denied := principalCtx(tc, models.RoleGuest, nil)
	assert.Empty(t, relationTypeNames(t, denied, &query_models.RelationshipTypeQuery{ForFromGroup: f.outside}), "deny-all")

	for who, ctx := range map[string]*application_context.MahresourcesContext{
		"editor":        principalCtx(tc, models.RoleEditor, nil),
		"unscoped user": principalCtx(tc, models.RoleUser, nil),
	} {
		assert.Equal(t, []string{"type-A"}, relationTypeNames(t, ctx, &query_models.RelationshipTypeQuery{ForFromGroup: f.outside, ForToGroup: f.outside}), who)
	}
}

// Over the wire, as a guest confined to inside-root: the JSON API (which was
// served from the unscoped singleton), the HTML list's .json variant and the
// entity picker all answer a hidden group like a missing one.
func TestRelationTypeGroupFiltersOverHTTP(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildListFilterLeakFixture(t, tc)

	guest, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "lf-guest", Password: "password1", Role: models.RoleGuest, ScopeGroupId: &f.root,
	})
	require.NoError(t, err)
	raw, _, err := tc.AppCtx.CreateApiToken(guest.ID, "t", nil)
	require.NoError(t, err)
	guestBearer := "Bearer " + raw
	editor, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "lf-editor", Password: "password1", Role: models.RoleEditor,
	})
	require.NoError(t, err)
	raw, _, err = tc.AppCtx.CreateApiToken(editor.ID, "t", nil)
	require.NoError(t, err)
	editorBearer := "Bearer " + raw
	adminBearer := unscopedAdminBearer(t, tc)

	get := func(bearer, path string) string {
		rr := doReq(tc, http.MethodGet, path, map[string]string{"Accept": "application/json", "Authorization": bearer}, nil, nil)
		require.Equal(t, http.StatusOK, rr.Code, "%s: %s", path, rr.Body.String())
		return strings.TrimSpace(rr.Body.String())
	}

	missing := get(guestBearer, "/v1/relationTypes?ForFromGroup=999999")
	assert.Equal(t, "[]", missing)
	for _, param := range []string{"ForFromGroup", "ForToGroup"} {
		path := fmt.Sprintf("/v1/relationTypes?%s=%d", param, f.outside)
		assert.Equal(t, missing, get(guestBearer, path), "guest %s", path)
		for who, bearer := range map[string]string{"admin": adminBearer, "editor": editorBearer} {
			var types []models.GroupRelationType
			require.NoError(t, json.Unmarshal([]byte(get(bearer, path)), &types))
			require.Len(t, types, 1, "%s %s", who, path)
			assert.Equal(t, "type-A", types[0].Name, "%s %s", who, path)
		}
	}

	page := get(guestBearer, fmt.Sprintf("/relationTypes.json?ForFromGroup=%d", f.outside))
	assert.NotContains(t, page, "type-A", "/relationTypes.json")
	assert.Contains(t, get(adminBearer, fmt.Sprintf("/relationTypes.json?ForFromGroup=%d", f.outside)), "type-A")

	filter := url.QueryEscape(fmt.Sprintf("ForFromGroup=%d", f.outside))
	for _, path := range []string{
		"/v1/entity-picker?entity=relationType&filter=" + filter,
		"/v1/entity-picker?entity=relationType&constraints=" + filter,
		fmt.Sprintf("/v1/entity-picker/resolve?entity=relationType&id=%d&constraints=%s", f.typeA, filter),
	} {
		assert.NotContains(t, get(guestBearer, path), "type-A", "guest %s", path)
		assert.Contains(t, get(adminBearer, path), "type-A", "admin %s", path)
	}
}
