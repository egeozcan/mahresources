package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
)

// Review fixes for the table-backed readers: each test is one finding.

func scopedTo(ctx *MahresourcesContext, g *models.Group) *MahresourcesContext {
	return ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, UserID: 5, ScopeGroupID: &g.ID})
}

// ---- 1: association counts read the live subtree, bound once ----------------------

func testStoreAssociationCountsUseTheLiveSubtree(t *testing.T, ctx *MahresourcesContext) {
	home := &models.Group{Name: "home"}
	elsewhere := &models.Group{Name: "elsewhere"}
	require.NoError(t, ctx.db.Create(home).Error)
	require.NoError(t, ctx.db.Create(elsewhere).Error)
	var ids []uint
	for i := 0; i < 2; i++ {
		res := &models.Resource{Name: fmt.Sprintf("m%d", i), Hash: "h", OwnerId: &home.ID}
		require.NoError(t, ctx.db.Create(res).Error)
		ids = append(ids, res.ID)
	}
	tag := &models.Tag{Name: "t"}
	require.NoError(t, ctx.db.Create(tag).Error)
	require.NoError(t, ctx.db.Exec(`INSERT INTO resource_tags (resource_id, tag_id) VALUES (?, ?)`, ids[0], tag.ID).Error)
	for _, owner := range []*models.Group{home, elsewhere} {
		note := &models.Note{Name: "n-" + owner.Name, OwnerId: &owner.ID}
		require.NoError(t, ctx.db.Create(note).Error)
		require.NoError(t, ctx.db.Exec(`INSERT INTO resource_notes (resource_id, note_id) VALUES (?, ?)`, ids[0], note.ID).Error)
		require.NoError(t, ctx.db.Exec(`INSERT INTO groups_related_resources (resource_id, group_id) VALUES (?, ?)`, ids[0], owner.ID).Error)
	}
	r := tableReduction(t, ctx)
	require.NoError(t, ctx.reductionStore().insertCluster(r, "gen", &storedCluster{
		Row: models.ReductionClusterRow{Key: "c1", Tier: "identical", State: "open", WinnerID: ids[0]}, Members: memberRows(ids...)}))

	scoped := scopedTo(ctx, home) // the allow-list is flattened here
	counts := func() (tags, notes, groups int) {
		review, _, err := scoped.storeGetReductionReview(r.ID, nil, false, 1, nil)
		require.NoError(t, err)
		require.Len(t, review.Clusters, 1)
		m := review.Clusters[0].Members[0]
		return m.TagCount, m.NoteCount, m.GroupCount
	}
	tags, notes, groups := counts()
	assert.Equal(t, []int{1, 1, 1}, []int{tags, notes, groups}, "only the Note and the Group inside the subtree are counted")

	// The Group moves under home after the principal was bound: the live tree
	// decides, not the flattened list.
	require.NoError(t, ctx.db.Exec(`UPDATE groups SET owner_id = ? WHERE id = ?`, home.ID, elsewhere.ID).Error)
	tags, notes, groups = counts()
	assert.Equal(t, []int{1, 2, 2}, []int{tags, notes, groups}, "a Group moved into the subtree counts at once")

	// And the admin sees every one.
	review, _, err := ctx.storeGetReductionReview(r.ID, nil, false, 1, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, review.Clusters[0].Members[0].NoteCount)
}

func TestStoreAssociationCountsUseTheLiveSubtreeSQLite(t *testing.T) {
	testStoreAssociationCountsUseTheLiveSubtree(t, newReductionSQLiteContext(t))
}

func testStoreAssociationCountsOnAHugeSubtree(t *testing.T, ctx *MahresourcesContext) {
	home := &models.Group{Name: "home"}
	require.NoError(t, ctx.db.Create(home).Error)
	kids := make([]models.Group, 33000) // past SQLite's 32,766 bind limit
	for i := range kids {
		kids[i] = models.Group{Name: fmt.Sprintf("k%d", i), OwnerId: &home.ID}
	}
	require.NoError(t, ctx.db.CreateInBatches(kids, 500).Error)
	var ids []uint
	for i := 0; i < 2; i++ {
		res := &models.Resource{Name: fmt.Sprintf("m%d", i), Hash: "h", OwnerId: &home.ID}
		require.NoError(t, ctx.db.Create(res).Error)
		ids = append(ids, res.ID)
	}
	note := &models.Note{Name: "n", OwnerId: &kids[32999].ID}
	require.NoError(t, ctx.db.Create(note).Error)
	require.NoError(t, ctx.db.Exec(`INSERT INTO resource_notes (resource_id, note_id) VALUES (?, ?)`, ids[0], note.ID).Error)
	r := tableReduction(t, ctx)
	require.NoError(t, ctx.reductionStore().insertCluster(r, "gen", &storedCluster{
		Row: models.ReductionClusterRow{Key: "c1", Tier: "identical", State: "open", WinnerID: ids[0]}, Members: memberRows(ids...)}))

	review, _, err := scopedTo(ctx, home).storeGetReductionReview(r.ID, nil, false, 1, nil)
	require.NoError(t, err)
	require.Len(t, review.Clusters, 1)
	assert.Equal(t, 1, review.Clusters[0].Members[0].NoteCount)
}

func TestStoreAssociationCountsOnAHugeSubtreeSQLite(t *testing.T) {
	testStoreAssociationCountsOnAHugeSubtree(t, newReductionSQLiteContext(t))
}

// ---- 2: running and interrupted come from the leases -------------------------------

func testStoreLastApplyRunningFollowsTheLease(t *testing.T, ctx *MahresourcesContext) {
	r := tableReduction(t, ctx)
	rec, err := json.Marshal(map[string]any{"startedAt": time.Now().Add(-time.Minute), "requestId": "q"})
	require.NoError(t, err)
	set := func(updates map[string]any) {
		updates["last_apply"] = string(rec)
		require.NoError(t, ctx.db.Model(&models.ResourceReduction{}).Where("id = ?", r.ID).Updates(updates).Error)
	}
	view := func() (running, interrupted bool) {
		review, _, err := ctx.storeGetReductionReview(r.ID, nil, false, 1, nil)
		require.NoError(t, err)
		require.NotNil(t, review.LastApply)
		return review.LastApply.Running, review.LastApply.Interrupted
	}

	set(map[string]any{"apply_lease_until": time.Now().Add(-time.Minute).UnixNano()})
	running, interrupted := view()
	assert.False(t, running, "an expired lease is not a running apply, however recent its start")
	assert.True(t, interrupted, "so the recovery control shows")

	set(map[string]any{"apply_lease_until": time.Now().Add(time.Minute).UnixNano()})
	running, interrupted = view()
	assert.True(t, running)
	assert.False(t, interrupted)

	op := "settle-op"
	set(map[string]any{"apply_lease_until": 0, "settle_op": op, "settle_lease_until": time.Now().Add(time.Minute).UnixNano()})
	running, interrupted = view()
	assert.True(t, running, "a live settle lease also owns the Reduction")
	assert.False(t, interrupted)

	set(map[string]any{"apply_lease_until": 0, "settle_op": op, "settle_lease_until": time.Now().Add(-time.Minute).UnixNano()})
	running, interrupted = view()
	assert.False(t, running)
	assert.True(t, interrupted)
}

func TestStoreLastApplyRunningFollowsTheLeaseSQLite(t *testing.T) {
	testStoreLastApplyRunningFollowsTheLease(t, newReductionSQLiteContext(t))
}

// ---- 3: the last apply's counts need evidence, for every viewer --------------------

func testStoreLastApplyCountsNeedEvidence(t *testing.T, ctx *MahresourcesContext) {
	home := &models.Group{Name: "home"}
	require.NoError(t, ctx.db.Create(home).Error)
	var ids []uint
	for i := 0; i < 4; i++ {
		res := &models.Resource{Name: fmt.Sprintf("m%d", i), Hash: "h", OwnerId: &home.ID}
		require.NoError(t, ctx.db.Create(res).Error)
		ids = append(ids, res.ID)
	}
	r := tableReduction(t, ctx)
	for i := 0; i < 2; i++ {
		require.NoError(t, ctx.reductionStore().insertCluster(r, "gen", &storedCluster{
			Row: models.ReductionClusterRow{Key: fmt.Sprintf("c%d", i), Tier: "identical", State: "applied", Merged: true, WinnerID: ids[2*i]}, Members: memberRows(ids[2*i], ids[2*i+1])}))
	}
	stamp := func(keys ...string) {
		require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET outcome_batch = '' WHERE reduction_id = ?`, r.ID).Error)
		for _, k := range keys {
			require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET outcome_batch = 'b1' WHERE reduction_id = ? AND "key" = ?`, r.ID, k).Error)
		}
	}
	record := func(applied, stale int, batch string) {
		rec := map[string]any{"startedAt": time.Now().Add(-time.Hour), "finishedAt": time.Now(), "applied": applied, "removed": applied, "stale": stale}
		if batch != "" {
			rec["batchId"] = batch
		}
		raw, err := json.Marshal(rec)
		require.NoError(t, err)
		require.NoError(t, ctx.db.Model(&models.ResourceReduction{}).Where("id = ?", r.ID).Updates(map[string]any{"last_apply": string(raw), "apply_lease_until": 0}).Error)
	}
	viewers := map[string]*MahresourcesContext{"admin": ctx, "scoped": scopedTo(ctx, home)}
	check := func(label string, wantHidden bool, wantApplied int) {
		for who, v := range viewers {
			review, _, err := v.storeGetReductionReview(r.ID, nil, false, 1, nil)
			require.NoError(t, err, label+" "+who)
			require.NotNil(t, review.LastApply, label+" "+who)
			assert.Equal(t, wantHidden, review.LastApply.CountsHidden, label+" "+who+" hidden")
			assert.Equal(t, wantApplied, review.LastApply.Applied, label+" "+who+" applied")
		}
	}

	stamp("c0", "c1")
	record(2, 0, "b1")
	check("consistent", false, 2)

	record(2, 0, "")
	check("counts but no batch id", true, 0)

	stamp("c0")
	record(2, 0, "b1")
	check("one of two outcomes carries the batch", true, 0)

	stamp("c0", "c1")
	record(1, 0, "b1")
	check("two Clusters carry the batch, one was reported", true, 0)

	stamp("c0")
	record(0, 0, "b1")
	check("zero reported outcomes, one stamped Cluster", true, 0)

	stamp()
	record(0, 0, "b1")
	check("zero reported, none stamped", false, 0)

	record(0, 0, "")
	check("an apply that did nothing and names no batch", false, 0)
}

func TestStoreLastApplyCountsNeedEvidenceSQLite(t *testing.T) {
	testStoreLastApplyCountsNeedEvidence(t, newReductionSQLiteContext(t))
}

// ---- 4: widening to All keeps the explicit selection's count -----------------------

func testStoreWidenedExtentCountsTheSelection(t *testing.T, ctx *MahresourcesContext) {
	var ids []uint
	for i := 0; i < 3; i++ {
		res := &models.Resource{Name: fmt.Sprintf("m%d", i), Hash: "h"}
		require.NoError(t, ctx.db.Create(res).Error)
		ids = append(ids, res.ID)
	}
	r := tableReduction(t, ctx)
	require.NoError(t, ctx.reductionStore().addExtentResources(r, ids[:2]))
	coverage, _ := json.Marshal(models.ReductionCoverage{ExtentSize: 9, ContentHashed: 4})
	require.NoError(t, ctx.db.Model(&models.ResourceReduction{}).Where("id = ?", r.ID).Updates(map[string]any{"extent_all": true, "coverage": string(coverage)}).Error)

	review, _, err := ctx.storeGetReductionReview(r.ID, nil, false, 1, nil)
	require.NoError(t, err)
	assert.True(t, review.ExtentAll)
	assert.Equal(t, 2, review.SelectedResources, "the explicit selection survives widening, as the JSON reader counts it")
	assert.Empty(t, review.SelectionResources, "a whole-library Reduction names nothing")
	assert.Zero(t, review.MoreResources)
	assert.True(t, review.CoverageTrusted)
	assert.Equal(t, 9, review.ExtentSize)

	require.NoError(t, ctx.db.Exec(`DELETE FROM resources WHERE id = ?`, ids[1]).Error)
	review, _, err = ctx.storeGetReductionReview(r.ID, nil, false, 1, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, review.SelectedResources)
	assert.False(t, review.CoverageTrusted, "a selected Resource that is gone means the coverage describes more than is there")
	assert.Zero(t, review.ExtentSize)
}

func TestStoreWidenedExtentCountsTheSelectionSQLite(t *testing.T) {
	testStoreWidenedExtentCountsTheSelection(t, newReductionSQLiteContext(t))
}

// ---- 5: the member cursor is 64-bit throughout -------------------------------------

func TestMemberCursorRoundTripsAWideID(t *testing.T) {
	in := reductionMemberCursor{Position: 199, ID: 1 << 32}
	assert.Equal(t, "199.4294967296", in.String())
	out, err := parseReductionMemberCursor(in.String())
	require.NoError(t, err)
	assert.Equal(t, in, out)
	_, err = parseReductionMemberCursor("199.18446744073709551616")
	assert.Error(t, err, "past 64 bits is still refused")
	_, err = parseReductionMemberCursor("-1.5")
	assert.Error(t, err)
}

// ---- 6: ejections of a partly loaded Cluster come from stored counts ---------------

type sqlCapture struct {
	mu   sync.Mutex
	sqls []string
}

func (c *sqlCapture) LogMode(gormlogger.LogLevel) gormlogger.Interface { return c }
func (c *sqlCapture) Info(context.Context, string, ...interface{})     {}
func (c *sqlCapture) Warn(context.Context, string, ...interface{})     {}
func (c *sqlCapture) Error(context.Context, string, ...interface{})    {}
func (c *sqlCapture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	c.mu.Lock()
	c.sqls = append(c.sqls, sql)
	c.mu.Unlock()
}

func testStoreLeftOutCountsDoNotScanMembers(t *testing.T, ctx *MahresourcesContext) {
	r := tableReduction(t, ctx)
	store := ctx.reductionStore()
	build := func(key string, first uint, winnerIdx int, ejected map[int]bool) (int, uint) {
		ids := sequentialIDs(first, 250)
		members := memberRows(ids...)
		for i := range members {
			members[i].Position = i
			members[i].Ejected = ejected[i]
		}
		require.NoError(t, store.insertCluster(r, "gen", &storedCluster{
			Row: models.ReductionClusterRow{Key: key, Tier: "identical", State: "open", WinnerID: ids[winnerIdx], ReadOnly: true}, Members: members}))
		return len(ejected), ids[winnerIdx]
	}
	// Ejections past the first 200 members, a promoted Winner far down the list.
	wantA, _ := build("a", 1, 240, map[int]bool{205: true, 210: true, 3: true})
	// A Winner that is itself marked ejected still counts as one.
	wantB, _ := build("b", 1000, 5, map[int]bool{5: true, 230: true})

	capture := &sqlCapture{}
	spy := *ctx
	spy.db = ctx.db.Session(&gorm.Session{Logger: capture})
	review, _, err := spy.storeGetReductionReview(r.ID, nil, false, 1, &query_models.ResourceReductionReviewQuery{Sort: "computed"})
	require.NoError(t, err)
	got := map[string]int{}
	for _, v := range review.Clusters {
		got[v.ID] = v.LeftOutCount
	}
	assert.Equal(t, wantA, got["a"])
	assert.Equal(t, wantB, got["b"])

	capture.mu.Lock()
	defer capture.mu.Unlock()
	require.NotEmpty(t, capture.sqls, "the capture saw the statements")
	for _, sql := range capture.sqls {
		assert.NotContains(t, strings.ToLower(sql), "sum(case when ejected", "no statement aggregates over a Cluster's members")
	}
}

func TestStoreLeftOutCountsDoNotScanMembersSQLite(t *testing.T) {
	testStoreLeftOutCountsDoNotScanMembers(t, newReductionSQLiteContext(t))
}

// ---- 7: the Reduction name is bounded ----------------------------------------------

func testStoreReductionNameIsBounded(t *testing.T, ctx *MahresourcesContext) {
	name := strings.Repeat("€", 367) // 1,101 bytes
	r := tableReduction(t, ctx)
	require.NoError(t, ctx.db.Model(&models.ResourceReduction{}).Where("id = ?", r.ID).Update("name", name).Error)
	want := strings.Repeat("€", 341)

	review, _, err := ctx.storeGetReductionReview(r.ID, nil, false, 1, nil)
	require.NoError(t, err)
	assert.Equal(t, want, review.Reduction.Name)

	rows, count, _, err := ctx.storeListReductions(0, 10, &query_models.ResourceReductionQuery{})
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	assert.Equal(t, want, rows[0].Name)

	require.NoError(t, ctx.db.Model(&models.ResourceReduction{}).Where("id = ?", r.ID).Update("name", "short").Error)
	rows, _, _, err = ctx.storeListReductions(0, 10, &query_models.ResourceReductionQuery{})
	require.NoError(t, err)
	assert.Equal(t, "short", rows[0].Name)
}

func TestStoreReductionNameIsBoundedSQLite(t *testing.T) {
	testStoreReductionNameIsBounded(t, newReductionSQLiteContext(t))
}

// ---- 8: one clock for a list page and its count ------------------------------------

func testStoreListSamplesTheClockOnce(t *testing.T, ctx *MahresourcesContext) {
	r := tableReduction(t, ctx)
	deadline := time.Now().Add(time.Hour)
	require.NoError(t, ctx.db.Model(&models.ResourceReduction{}).Where("id = ?", r.ID).
		Updates(map[string]any{"status": models.ReductionStatusComputing, "compute_deadline": deadline}).Error)
	q := &query_models.ResourceReductionQuery{Status: []string{models.ReductionStatusComputing}}

	rows, count, _, err := ctx.storeListReductionsAt(deadline.Add(-time.Second), 0, 10, q)
	require.NoError(t, err)
	assert.Len(t, rows, 1)
	assert.EqualValues(t, 1, count)

	rows, count, _, err = ctx.storeListReductionsAt(deadline.Add(time.Second), 0, 10, q)
	require.NoError(t, err)
	assert.Len(t, rows, 0, "past its deadline it reads as failed")
	assert.EqualValues(t, 0, count, "and the count says the same as the rows")

	failed := &query_models.ResourceReductionQuery{Status: []string{models.ReductionStatusFailed}}
	rows, count, _, err = ctx.storeListReductionsAt(deadline.Add(time.Second), 0, 10, failed)
	require.NoError(t, err)
	assert.Len(t, rows, 1)
	assert.EqualValues(t, 1, count)
}

func TestStoreListSamplesTheClockOnceSQLite(t *testing.T) {
	testStoreListSamplesTheClockOnce(t, newReductionSQLiteContext(t))
}

// ---- 9: duplicate tier values do not exhaust the binds -----------------------------

func testStoreTierFilterIsDeduplicated(t *testing.T, ctx *MahresourcesContext) {
	r := tableReduction(t, ctx)
	store := ctx.reductionStore()
	for i, tier := range []string{"identical", "identical", "near"} {
		ids := sequentialIDs(uint(1+10*i), 2)
		require.NoError(t, store.insertCluster(r, "gen", &storedCluster{
			Row: models.ReductionClusterRow{Key: fmt.Sprintf("c%d", i), Tier: tier, State: "open", WinnerID: ids[0]}, Members: memberRows(ids...)}))
	}
	many := make([]string, 70000) // past both SQLite's and Postgres's bind limits
	for i := range many {
		many[i] = "identical"
	}
	review, _, err := ctx.storeGetReductionReview(r.ID, nil, false, 1, &query_models.ResourceReductionReviewQuery{Tier: many})
	require.NoError(t, err)
	assert.Equal(t, 2, review.ClusterCount)

	// Values no Cluster can hold match nothing, however many are sent.
	junk := make([]string, 70000)
	for i := range junk {
		junk[i] = fmt.Sprintf("junk-%d", i)
	}
	review, _, err = ctx.storeGetReductionReview(r.ID, nil, false, 1, &query_models.ResourceReductionReviewQuery{Tier: junk})
	require.NoError(t, err)
	assert.Zero(t, review.ClusterCount)
	review, _, err = ctx.storeGetReductionReview(r.ID, nil, false, 1, &query_models.ResourceReductionReviewQuery{Tier: append([]string{"near"}, junk...)})
	require.NoError(t, err)
	assert.Equal(t, 1, review.ClusterCount)
}

func TestStoreTierFilterIsDeduplicatedSQLite(t *testing.T) {
	testStoreTierFilterIsDeduplicated(t, newReductionSQLiteContext(t))
}

// ---- 10: the apply token follows the eligible set ----------------------------------

func testStoreApplyTokenFollowsTheEligibleSet(t *testing.T, ctx *MahresourcesContext) {
	r := tableReduction(t, ctx)
	store := ctx.reductionStore()
	for i := 0; i < 3; i++ {
		ids := sequentialIDs(uint(1+10*i), 3)
		require.NoError(t, store.insertCluster(r, "gen", &storedCluster{
			Row: models.ReductionClusterRow{Key: fmt.Sprintf("c%d", i), Tier: "identical", State: "open", WinnerID: ids[0], Checked: true}, Members: memberRows(ids...)}))
	}
	tokenOf := func() string {
		review, _, err := ctx.storeGetReductionReview(r.ID, nil, false, 1, nil)
		require.NoError(t, err)
		return review.ApplyToken
	}
	fingerprints := func(keys ...string) []string {
		var out []string
		for _, k := range keys {
			var fp string
			require.NoError(t, ctx.db.Raw(`SELECT fingerprint FROM reduction_clusters WHERE reduction_id = ? AND "key" = ?`, r.ID, k).Scan(&fp).Error)
			require.NotEmpty(t, fp)
			out = append(out, fp)
		}
		return out
	}
	assert.Equal(t, applyTokenOf(fingerprints("c0", "c1", "c2")), tokenOf())

	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET checked = ? WHERE reduction_id = ? AND "key" = ?`, false, r.ID, "c1").Error)
	assert.Equal(t, applyTokenOf(fingerprints("c0", "c2")), tokenOf(), "an unchecked Cluster leaves the token's set")

	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET checked = ? WHERE reduction_id = ?`, false, r.ID).Error)
	assert.Equal(t, applyTokenOf(nil), tokenOf(), "an empty set digests to the empty token")
	assert.NotEqual(t, applyTokenOf(fingerprints("c0")), applyTokenOf(nil))
}

func TestStoreApplyTokenFollowsTheEligibleSetSQLite(t *testing.T) {
	testStoreApplyTokenFollowsTheEligibleSet(t, newReductionSQLiteContext(t))
}
