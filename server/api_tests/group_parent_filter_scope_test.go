package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
)

// parentFilterFixture is a hidden parent above a scope root, the scope root, a
// child inside it, and a hidden sibling of the parent holding a note and a
// resource related to the scope root.
//
//	Acme Legal hold (hidden; budget 200000; tag secret)
//	└── Mine (scope root; region north; tag mine)
//	    └── Kid (x 1)
//	Hidden (unrelated to Mine's tree; related-group edge Hidden → Mine,
//	        owns a note and a resource related to Mine)
type parentFilterFixture struct {
	secretTag, mineTag             uint
	parent, mine, kid, hidden      uint
	hiddenNote, hiddenRes, kidNote uint
}

func buildParentFilterFixture(t *testing.T, tc *TestContext) parentFilterFixture {
	t.Helper()
	ctx := tc.AppCtx
	secret, err := ctx.CreateTag(&query_models.TagCreator{Name: "secret-tag"})
	require.NoError(t, err)
	mineTag, err := ctx.CreateTag(&query_models.TagCreator{Name: "mine-tag"})
	require.NoError(t, err)
	parent, err := ctx.CreateGroup(&query_models.GroupCreator{
		Name: "Acme Legal hold", Meta: `{"budget": 200000}`, Tags: []uint{secret.ID}})
	require.NoError(t, err)
	mine, err := ctx.CreateGroup(&query_models.GroupCreator{
		Name: "Mine", OwnerId: parent.ID, Meta: `{"region": "north"}`, Tags: []uint{mineTag.ID}})
	require.NoError(t, err)
	kid, err := ctx.CreateGroup(&query_models.GroupCreator{Name: "Kid", OwnerId: mine.ID, Meta: `{"x": 1}`})
	require.NoError(t, err)
	hidden, err := ctx.CreateGroup(&query_models.GroupCreator{Name: "Hidden"})
	require.NoError(t, err)

	hiddenNote := &models.Note{Name: "hidden-note", OwnerId: &hidden.ID}
	require.NoError(t, tc.DB.Create(hiddenNote).Error)
	hiddenRes := &models.Resource{Name: "hidden-res", OwnerId: &hidden.ID}
	require.NoError(t, tc.DB.Create(hiddenRes).Error)
	kidNote := &models.Note{Name: "kid-note", OwnerId: &kid.ID}
	require.NoError(t, tc.DB.Create(kidNote).Error)

	require.NoError(t, tc.DB.Exec("INSERT INTO group_related_groups (group_id, related_group_id) VALUES (?, ?)", hidden.ID, mine.ID).Error)
	require.NoError(t, tc.DB.Exec("INSERT INTO groups_related_notes (group_id, note_id) VALUES (?, ?)", mine.ID, hiddenNote.ID).Error)
	require.NoError(t, tc.DB.Exec("INSERT INTO groups_related_notes (group_id, note_id) VALUES (?, ?)", mine.ID, kidNote.ID).Error)
	require.NoError(t, tc.DB.Exec("INSERT INTO groups_related_resources (group_id, resource_id) VALUES (?, ?)", mine.ID, hiddenRes.ID).Error)

	return parentFilterFixture{
		secretTag: secret.ID, mineTag: mineTag.ID,
		parent: parent.ID, mine: mine.ID, kid: kid.ID, hidden: hidden.ID,
		hiddenNote: hiddenNote.ID, hiddenRes: hiddenRes.ID, kidNote: kidNote.ID,
	}
}

// parentLeakQuery is one way of testing the hidden parent, both as a decoded
// query and as the raw filter string the picker and mass edit decode.
type parentLeakQuery struct {
	name   string
	query  func() *query_models.GroupQuery
	filter string
	// visible is what a caller that can see the parent gets: the parent
	// matches its own name and tag, and Mine matches through its parent.
	visible []string
}

func parentLeakQueries(f parentFilterFixture) []parentLeakQuery {
	return []parentLeakQuery{
		{"name", func() *query_models.GroupQuery {
			return &query_models.GroupQuery{Name: "Acme Legal", SearchParentsForName: true}
		}, "Name=Acme+Legal&SearchParentsForName=1", []string{"Acme Legal hold", "Mine"}},
		{"tags", func() *query_models.GroupQuery {
			return &query_models.GroupQuery{Tags: []uint{f.secretTag}, SearchParentsForTags: true}
		}, fmt.Sprintf("Tags=%d&SearchParentsForTags=1", f.secretTag), []string{"Acme Legal hold", "Mine"}},
		{"meta", func() *query_models.GroupQuery {
			return &query_models.GroupQuery{MetaQuery: []query_models.ColumnMeta{{Key: "parent.budget", Operation: "GT", Value: 100000}}}
		}, "MetaQuery=" + url.QueryEscape("parent.budget:GT:100000"), []string{"Mine"}},
	}
}

func groupNames(t *testing.T, ctx *application_context.MahresourcesContext, q *query_models.GroupQuery) []string {
	t.Helper()
	groups, err := ctx.GetGroups(0, 50, q)
	require.NoError(t, err)
	names := make([]string, 0, len(groups))
	for _, g := range groups {
		names = append(names, g.Name)
	}
	sort.Strings(names)
	return names
}

func principalCtx(tc *TestContext, role models.Role, scope *uint) *application_context.MahresourcesContext {
	return tc.AppCtx.WithPrincipal(&auth.Principal{UserID: 77, Role: role, ScopeGroupID: scope})
}

// accountCtx is principalCtx backed by a stored account. A containment write
// (re-parent, re-own, merge) re-reads the account and refuses a principal whose
// account is missing or no longer matches it.
func accountCtx(t *testing.T, tc *TestContext, role models.Role, scope *uint) *application_context.MahresourcesContext {
	t.Helper()
	var n int64
	require.NoError(t, tc.DB.Model(&models.User{}).Count(&n).Error)
	u := &models.User{Username: fmt.Sprintf("account-%d", n+1), Role: role, ScopeGroupId: scope}
	require.NoError(t, tc.DB.Create(u).Error)
	return tc.AppCtx.WithPrincipal(auth.FromUser(u))
}

// assertGroupFilterFindsNothing runs one group filter through every consumer
// of the group list builder and asserts each one finds nothing.
func assertGroupFilterFindsNothing(t *testing.T, ctx *application_context.MahresourcesContext, label string, q func() *query_models.GroupQuery, filter string, massEditTag uint) {
	t.Helper()
	assert.Empty(t, groupNames(t, ctx, q()), "%s: GetGroups", label)

	n, err := ctx.GetGroupsCount(q())
	require.NoError(t, err, label)
	assert.Zero(t, n, "%s: GetGroupsCount", label)

	tags, err := ctx.GetPopularGroupTags(q())
	require.NoError(t, err, label)
	assert.Empty(t, tags, "%s: GetPopularGroupTags", label)

	buckets, err := ctx.GetGroupTimelineCounts(q(), application_context.GenerateBucketBoundaries("yearly", time.Now(), 3))
	require.NoError(t, err, label)
	for _, b := range buckets {
		assert.Zero(t, b.Created, "%s: timeline created %s", label, b.Label)
		assert.Zero(t, b.Updated, "%s: timeline updated %s", label, b.Label)
	}

	page, err := ctx.BrowseEntities(&query_models.EntityPickerQuery{Entity: "group", Filter: filter})
	require.NoError(t, err, label)
	assert.Empty(t, page.Items, "%s: entity picker filter", label)
	page, err = ctx.BrowseEntities(&query_models.EntityPickerQuery{Entity: "group", Constraints: filter})
	require.NoError(t, err, label)
	assert.Empty(t, page.Items, "%s: entity picker constraints", label)

	// Mass edit is a write, so only a principal that may write reaches it; 0
	// skips it.
	if massEditTag != 0 {
		result, err := ctx.MassEditGroups(&query_models.MassEditQuery{
			Target: "filter", Filter: filter, DryRun: true, TagsOp: "add", TagIds: []uint{massEditTag},
		})
		require.NoError(t, err, label)
		assert.Zero(t, result.Matched, "%s: mass-edit filter count", label)
	}
}

// A group-limited principal cannot read the name, tags or Meta of the group
// above its scope root through the parent branches of the group list filters:
// that parent behaves as if it did not exist, as it does in MRQL.
func TestParentFiltersDoNotReadTheScopeRootsHiddenParent(t *testing.T) {
	tc := SetupTestEnv(t)
	f := buildParentFilterFixture(t, tc)

	principals := map[string]*application_context.MahresourcesContext{
		"scoped user":  scopedTo(t, tc, f.mine),
		"scoped guest": principalCtx(tc, models.RoleGuest, &f.mine),
	}
	for who, ctx := range principals {
		massEditTag := uint(0)
		if who == "scoped user" {
			massEditTag = f.mineTag
		}
		for _, lq := range parentLeakQueries(f) {
			assertGroupFilterFindsNothing(t, ctx, who+"/"+lq.name, lq.query, lq.filter, massEditTag)
		}
		// A miss reads the same as a hit.
		miss := func() *query_models.GroupQuery {
			return &query_models.GroupQuery{Name: "Acme Lexal", SearchParentsForName: true}
		}
		assertGroupFilterFindsNothing(t, ctx, who+"/miss", miss, "Name=Acme+Lexal&SearchParentsForName=1", massEditTag)
	}

	// Deny-all: a guest with no scope group gets nothing, and no error.
	denied := principalCtx(tc, models.RoleGuest, nil)
	for _, lq := range parentLeakQueries(f) {
		assertGroupFilterFindsNothing(t, denied, "deny-all/"+lq.name, lq.query, lq.filter, 0)
	}

	// Unrestricted roles still find the scope root through its parent.
	unrestricted := map[string]*application_context.MahresourcesContext{
		"admin (system)": tc.AppCtx,
		"editor":         principalCtx(tc, models.RoleEditor, nil),
		"unscoped user":  principalCtx(tc, models.RoleUser, nil),
	}
	for who, ctx := range unrestricted {
		for _, lq := range parentLeakQueries(f) {
			assert.Equal(t, lq.visible, groupNames(t, ctx, lq.query()), "%s/%s", who, lq.name)
			n, err := ctx.GetGroupsCount(lq.query())
			require.NoError(t, err)
			assert.Equal(t, int64(len(lq.visible)), n, "%s/%s count", who, lq.name)
		}
	}
}

// The guard sits on the parent branch only: inside the subtree, parent search
// still works, and the scope root still matches its own name and tags.
func TestParentFiltersStillWorkInsideTheSubtree(t *testing.T) {
	tc := SetupTestEnv(t)
	f := buildParentFilterFixture(t, tc)
	scoped := scopedTo(t, tc, f.mine)

	assert.Equal(t, []string{"Kid", "Mine"},
		groupNames(t, scoped, &query_models.GroupQuery{Name: "Mine", SearchParentsForName: true}), "parent name")
	assert.Equal(t, []string{"Kid", "Mine"},
		groupNames(t, scoped, &query_models.GroupQuery{Tags: []uint{f.mineTag}, SearchParentsForTags: true}), "parent tags")
	assert.Equal(t, []string{"Kid"},
		groupNames(t, scoped, &query_models.GroupQuery{MetaQuery: []query_models.ColumnMeta{{Key: "parent.region", Operation: "EQ", Value: "north"}}}), "parent meta")
	assert.Equal(t, []string{"Kid", "Mine"},
		groupNames(t, scoped, &query_models.GroupQuery{Name: "Kid", SearchChildrenForName: true}), "child name")

	// Confined one level higher, the parent is visible and the issue's queries
	// find the group below it.
	wider := scopedTo(t, tc, f.parent)
	for _, lq := range parentLeakQueries(f) {
		assert.Equal(t, lq.visible, groupNames(t, wider, lq.query()), "scoped to parent/%s", lq.name)
	}
}

// The id-list filters name a group, note or resource by id. A hidden one must
// read as missing: its link to a visible group cannot be tested.
func TestGroupIDListFiltersTreatHiddenRowsAsMissing(t *testing.T) {
	tc := SetupTestEnv(t)
	f := buildParentFilterFixture(t, tc)
	scoped := scopedTo(t, tc, f.mine)

	cases := map[string]struct {
		query  func() *query_models.GroupQuery
		filter string
		admin  []string
	}{
		"related group": {func() *query_models.GroupQuery { return &query_models.GroupQuery{Groups: []uint{f.hidden}} },
			fmt.Sprintf("Groups=%d", f.hidden), []string{"Mine"}},
		"hidden parent as group": {func() *query_models.GroupQuery { return &query_models.GroupQuery{Groups: []uint{f.parent}} },
			fmt.Sprintf("Groups=%d", f.parent), []string{"Mine"}},
		"related note": {func() *query_models.GroupQuery { return &query_models.GroupQuery{Notes: []uint{f.hiddenNote}} },
			fmt.Sprintf("Notes=%d", f.hiddenNote), []string{"Hidden", "Mine"}},
		"related resource": {func() *query_models.GroupQuery { return &query_models.GroupQuery{Resources: []uint{f.hiddenRes}} },
			fmt.Sprintf("Resources=%d", f.hiddenRes), []string{"Hidden", "Mine"}},
	}
	for name, c := range cases {
		assertGroupFilterFindsNothing(t, scoped, name, c.query, c.filter, f.mineTag)
		assert.Equal(t, c.admin, groupNames(t, tc.AppCtx, c.query()), "admin/%s", name)
	}

	// A visible note still finds both its owner and the group it is related to.
	assert.Equal(t, []string{"Kid", "Mine"},
		groupNames(t, scoped, &query_models.GroupQuery{Notes: []uint{f.kidNote}}), "visible note")
}

// Over the wire: GET /v1/groups as a guest confined to Mine.
func TestParentFiltersOverHTTP(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildParentFilterFixture(t, tc)

	guest, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "parent-guest", Password: "password1", Role: models.RoleGuest, ScopeGroupId: &f.mine,
	})
	require.NoError(t, err)
	raw, _, err := tc.AppCtx.CreateApiToken(guest.ID, "t", nil)
	require.NoError(t, err)
	guestBearer := "Bearer " + raw
	adminBearer := unscopedAdminBearer(t, tc)

	list := func(bearer, query string) []string {
		rr := doReq(tc, http.MethodGet, "/v1/groups?"+query,
			map[string]string{"Accept": "application/json", "Authorization": bearer}, nil, nil)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		var groups []models.Group
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &groups), rr.Body.String())
		names := []string{}
		for _, g := range groups {
			names = append(names, g.Name)
		}
		sort.Strings(names)
		return names
	}

	for _, lq := range parentLeakQueries(f) {
		assert.Empty(t, list(guestBearer, lq.filter), "guest/%s", lq.name)
		assert.Equal(t, lq.visible, list(adminBearer, lq.filter), "admin/%s", lq.name)
	}
}
