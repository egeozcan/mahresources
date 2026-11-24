package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/types"
)

// ---- fixtures -----------------------------------------------------------------------------

type streamRes struct {
	Hash  string
	W, H  uint
	Size  int64
	Owner *uint
	Desc  string
	Name  string
	Image bool
}

func mkStreamRes(t *testing.T, db *gorm.DB, o streamRes) uint {
	t.Helper()
	if o.Name == "" {
		o.Name = fmt.Sprintf("res-%d", rand.Int63())
	}
	res := &models.Resource{Name: o.Name, Hash: o.Hash, Width: o.W, Height: o.H, FileSize: o.Size, OwnerId: o.Owner, Description: o.Desc, ContentType: "image/png", ResourceCategoryId: 1}
	require.NoError(t, db.Create(res).Error)
	if o.Image {
		p := int64(res.ID)
		require.NoError(t, db.Create(&models.ImageHash{ResourceId: &res.ID, PHashInt: &p}).Error)
	}
	return res.ID
}

func mkPair(t *testing.T, db *gorm.DB, a, b uint, d uint8) {
	t.Helper()
	if a > b {
		a, b = b, a
	}
	require.NoError(t, db.Create(&models.ResourceSimilarity{ResourceID1: a, ResourceID2: b, HammingDistance: d, PDistance: &d, ADistance: &d}).Error)
}

type streamSel struct {
	All        bool
	Resources  []uint
	Groups     []uint
	Rule       []string
	ExcludeExt bool
	Identical  bool
}

func mkStreamReduction(t *testing.T, ctx *MahresourcesContext, sel streamSel) *models.ResourceReduction {
	t.Helper()
	rule := sel.Rule
	if rule == nil {
		rule = models.DefaultWinnerRule()
	}
	raw, err := json.Marshal(rule)
	require.NoError(t, err)
	mode := models.MatchingModeBothTiers
	if sel.Identical {
		mode = models.MatchingModeIdenticalOnly
	}
	r := &models.ResourceReduction{Name: "stream", Status: models.ReductionStatusReady, MatchingMode: mode, WinnerRule: types.JSON(raw), ExcludeExternalResources: sel.ExcludeExt}
	require.NoError(t, ctx.db.Create(r).Error)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET plan_schema = 1, extent_all = ? WHERE id = ?", sel.All, r.ID).Error)
	for _, id := range sel.Resources {
		require.NoError(t, ctx.db.Create(&models.ReductionExtentResource{ReductionID: r.ID, ResourceID: id}).Error)
	}
	for _, id := range sel.Groups {
		require.NoError(t, ctx.db.Create(&models.ReductionExtentGroup{ReductionID: r.ID, GroupID: id}).Error)
	}
	return loadReductionRow(t, ctx.db, r.ID)
}

// streamCompute runs the streaming compute inline, as the queue job would.
func streamCompute(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction) {
	t.Helper()
	cur := loadReductionRow(t, ctx.db, r.ID)
	_, gen, err := ctx.beginReductionComputeStore(cur.ID, cur.Version)
	require.NoError(t, err)
	require.NoError(t, ctx.runReductionCompute(context.Background(), cur.ID, "", gen, nil))
}

// oracleCompute is the in-memory compute landed through the same generation machinery.
func oracleCompute(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction) {
	t.Helper()
	cur := loadReductionRow(t, ctx.db, r.ID)
	_, gen, err := ctx.beginReductionComputeStore(cur.ID, cur.Version)
	require.NoError(t, err)
	require.NoError(t, ctx.storeAdmitComputeLease(cur.ID, gen))
	built, err := ctx.computeReductionPlan(context.Background(), cur.ID, nil)
	require.NoError(t, err)
	require.NoError(t, ctx.storeLandComputedPlan(built.reduction, "", gen, built.version, built.fresh, built.coverage, built.settings))
}

type planRow struct {
	Row     models.ReductionClusterRow
	Members []models.ReductionMemberRow
}

func planOf(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction) map[string]planRow {
	t.Helper()
	cur := loadReductionRow(t, ctx.db, r.ID)
	require.NotNil(t, cur.PlanGeneration, "the compute landed")
	var rows []models.ReductionClusterRow
	require.NoError(t, ctx.db.Where("reduction_id = ? AND generation = ?", r.ID, *cur.PlanGeneration).Find(&rows).Error)
	out := map[string]planRow{}
	for _, row := range rows {
		var members []models.ReductionMemberRow
		require.NoError(t, ctx.db.Where("cluster_id = ?", row.ID).Order("position, id").Find(&members).Error)
		out[row.Key] = planRow{Row: row, Members: members}
	}
	return out
}

func (p planRow) ids() []uint {
	var out []uint
	for _, m := range p.Members {
		out = append(out, m.ResourceID)
	}
	return out
}

// shape is what the two computes must agree on.
func shape(p planRow) string {
	var b strings.Builder
	r := p.Row
	fmt.Fprintf(&b, "%s w=%d by=%s m=%s u=%v chk=%v os=%v ks=%v lossy=%s |", r.Tier, r.WinnerID, r.DecidedBy, r.Margin, r.Undecided, r.Checked, r.Oversized, r.KeepsSmaller, string(r.Lossy))
	for _, m := range p.Members {
		d := -1
		if m.Distance != nil {
			d = *m.Distance
		}
		fmt.Fprintf(&b, " %d:%v/%v/%s/%d", m.ResourceID, m.InExtent, m.Ejected, m.EjectedReason, d)
	}
	return b.String()
}

func requireSamePlans(t *testing.T, want, got map[string]planRow) {
	t.Helper()
	keys := func(m map[string]planRow) []string {
		var k []string
		for key := range m {
			k = append(k, key)
		}
		sort.Strings(k)
		return k
	}
	require.Equal(t, keys(want), keys(got), "the same Clusters")
	for key, w := range want {
		require.Equal(t, shape(w), shape(got[key]), "Cluster %s", key)
	}
}

// ---- the differential test ------------------------------------------------------------------

func randomStreamLibrary(t *testing.T, ctx *MahresourcesContext, rng *rand.Rand) streamSel {
	t.Helper()
	var groups []uint
	for i := 0; i < 4; i++ {
		g := &models.Group{Name: fmt.Sprintf("g%d", i)}
		if i > 0 && rng.Intn(2) == 0 {
			parent := groups[rng.Intn(len(groups))]
			g.OwnerId = &parent
		}
		require.NoError(t, ctx.db.Create(g).Error)
		groups = append(groups, g.ID)
	}
	var ids, images []uint
	n := 24 + rng.Intn(14)
	for i := 0; i < n; i++ {
		o := streamRes{
			W: uint(rng.Intn(4)) * 50, H: uint(rng.Intn(3)) * 50, Size: int64(rng.Intn(5)) * 100,
			Image: rng.Intn(3) > 0, Name: fmt.Sprintf("n%d", rng.Intn(6)),
		}
		if rng.Intn(5) > 0 {
			o.Hash = fmt.Sprintf("h%d", rng.Intn(8))
		}
		switch rng.Intn(4) {
		case 0:
			o.Desc = "  "
		case 1:
			o.Desc = "d"
		}
		if rng.Intn(6) > 0 {
			g := groups[rng.Intn(len(groups))]
			o.Owner = &g
		}
		id := mkStreamRes(t, ctx.db, o)
		ids = append(ids, id)
		if o.Image {
			images = append(images, id)
		}
	}
	for i := 0; i < len(images)*2; i++ {
		a, b := images[rng.Intn(len(images))], images[rng.Intn(len(images))]
		if a == b {
			continue
		}
		lo, hi := a, b
		if lo > hi {
			lo, hi = hi, lo
		}
		var have int64
		require.NoError(t, ctx.db.Raw("SELECT COUNT(*) FROM resource_similarities WHERE resource_id1 = ? AND resource_id2 = ?", lo, hi).Scan(&have).Error)
		if have == 0 {
			mkPair(t, ctx.db, lo, hi, uint8(rng.Intn(6)))
		}
	}
	rules := [][]string{nil, {models.WinnerCriterionNameAsc, models.WinnerCriterionPixelsDesc}, {models.WinnerCriterionHasDescription, models.WinnerCriterionSizeAsc}}
	sel := streamSel{Rule: rules[rng.Intn(len(rules))], ExcludeExt: rng.Intn(3) == 0}
	switch rng.Intn(3) {
	case 0:
		sel.All = true
	case 1:
		for _, id := range ids {
			if rng.Intn(2) == 0 {
				sel.Resources = append(sel.Resources, id)
			}
		}
	default:
		sel.Groups = dedupeUints([]uint{groups[rng.Intn(len(groups))], groups[rng.Intn(len(groups))]})
		sel.Resources = ids[:3]
	}
	return sel
}

// The streaming compute proposes exactly the Clusters the in-memory one did, on a
// library below every cap, and the answer does not depend on the chunk size.
func testStreamMatchesInMemoryCompute(t *testing.T, newCtx func() *MahresourcesContext, seed int64, chunk int) {
	old := computeChunk
	computeChunk = chunk
	defer func() { computeChunk = old }()
	rng := rand.New(rand.NewSource(seed))
	ctx := newCtx()
	sel := randomStreamLibrary(t, ctx, rng)
	a, b := mkStreamReduction(t, ctx, sel), mkStreamReduction(t, ctx, sel)
	oracleCompute(t, ctx, a)
	streamCompute(t, ctx, b)
	want, got := planOf(t, ctx, a), planOf(t, ctx, b)
	requireSamePlans(t, want, got)
	var covA, covB models.ReductionCoverage
	ra, rb := loadReductionRow(t, ctx.db, a.ID), loadReductionRow(t, ctx.db, b.ID)
	require.NoError(t, json.Unmarshal(ra.Coverage, &covA))
	require.NoError(t, json.Unmarshal(rb.Coverage, &covB))
	require.Equal(t, covA, covB)
	var staged int64
	require.NoError(t, ctx.db.Raw("SELECT COUNT(*) FROM reduction_extent WHERE reduction_id = ?", b.ID).Scan(&staged).Error)
	require.Zero(t, staged, "staging rows are deleted after landing")
	require.NoError(t, ctx.db.Raw("SELECT COUNT(*) FROM reduction_seeds WHERE reduction_id = ?", b.ID).Scan(&staged).Error)
	require.Zero(t, staged)
	t.Logf("seed %d chunk %d: %d Clusters", seed, chunk, len(got))
}

func TestStreamMatchesInMemoryCompute(t *testing.T) {
	for seed := int64(1); seed <= 12; seed++ {
		for _, chunk := range []int{500, 1, 3} {
			testStreamMatchesInMemoryCompute(t, func() *MahresourcesContext { return withSettings(newReductionSQLiteContext(t)) }, seed, chunk)
		}
	}
}

// ---- caps ------------------------------------------------------------------------------------

func streamRows(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction, tier string) []planRow {
	t.Helper()
	var out []planRow
	for _, p := range planOf(t, ctx, r) {
		if p.Row.Tier == tier {
			out = append(out, p)
		}
	}
	return out
}

func testStreamIdenticalCap(t *testing.T, ctx *MahresourcesContext) {
	var best uint
	for i := 0; i < 250; i++ {
		id := mkStreamRes(t, ctx.db, streamRes{Hash: "same", W: 10, H: 10, Size: int64(1000 + i)})
		best = id
	}
	r := mkStreamReduction(t, ctx, streamSel{All: true, Identical: true})
	streamCompute(t, ctx, r)
	rows := streamRows(t, ctx, r, models.ReductionTierIdentical)
	require.Len(t, rows, 1)
	require.Equal(t, 200, rows[0].Row.MemberCount)
	require.Equal(t, 50, rows[0].Row.Truncated)
	require.Equal(t, best, rows[0].Row.WinnerID, "the largest file wins, and the 200 best holders are the ones kept")
	require.False(t, rows[0].Row.FixedSeed)
}

// A star of 250 neighbours is cut at the cap and is full: its seed is its Winner
// for good, whatever the Winner Rule says of an outsider that would beat it.
func testStreamFullStar(t *testing.T, ctx *MahresourcesContext, neighbours int, wantFull bool) {
	seed := mkStreamRes(t, ctx.db, streamRes{Hash: "s", W: 100, H: 100, Size: 500, Image: true})
	members := []uint{seed}
	for i := 0; i < neighbours; i++ {
		id := mkStreamRes(t, ctx.db, streamRes{Hash: fmt.Sprintf("n%d", i), W: 50, H: 50, Size: 100, Image: true})
		mkPair(t, ctx.db, seed, id, uint8(1+i%3))
		members = append(members, id)
	}
	outsider := mkStreamRes(t, ctx.db, streamRes{Hash: "o", W: 900, H: 900, Size: 9000, Image: true})
	mkPair(t, ctx.db, seed, outsider, 1)
	r := mkStreamReduction(t, ctx, streamSel{Resources: members})
	streamCompute(t, ctx, r)
	rows := streamRows(t, ctx, r, models.ReductionTierNear)
	require.Len(t, rows, 1)
	row := rows[0]
	require.Equal(t, seed, row.Row.SeedResourceID)
	require.Equal(t, wantFull, row.Row.FixedSeed)
	if wantFull {
		require.Equal(t, 200, row.Row.MemberCount)
		require.Equal(t, seed, row.Row.WinnerID)
		require.Equal(t, models.SeedCriterion, row.Row.DecidedBy)
		require.Empty(t, row.Row.Margin)
		require.False(t, row.Row.Undecided)
		require.NotContains(t, row.ids(), outsider, "a full star admits no outsider")
		if neighbours > 199 {
			require.Equal(t, 1, row.Row.Truncated)
		}
	} else {
		require.Equal(t, outsider, row.Row.WinnerID, "below the cap the better outsider wins")
		require.Contains(t, row.ids(), outsider)
		require.Equal(t, neighbours+2, row.Row.MemberCount)
	}
}

func TestStreamFullStar250(t *testing.T) {
	testStreamFullStar(t, withSettings(newReductionSQLiteContext(t)), 250, true)
}
func TestStreamFullStarExactly199(t *testing.T) {
	testStreamFullStar(t, withSettings(newReductionSQLiteContext(t)), 199, true)
}
func TestStreamStarWithOutsider(t *testing.T) {
	testStreamFullStar(t, withSettings(newReductionSQLiteContext(t)), 30, false)
}

// 198 insiders, the seed and a winning outsider also make 200 rows and are not full.
func TestStreamStarOfTwoHundredRowsIsNotFull(t *testing.T) {
	testStreamFullStar(t, withSettings(newReductionSQLiteContext(t)), 198, false)
}

func TestStreamIdenticalCap(t *testing.T) {
	testStreamIdenticalCap(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- a version upload between the grouping and the load -----------------------------------------

func testStreamVersionUploadBetweenGroupingAndLoad(t *testing.T, ctx *MahresourcesContext, atInsert bool) {
	stays := mkStreamRes(t, ctx.db, streamRes{Hash: "shared", W: 40, H: 40})
	moves := mkStreamRes(t, ctx.db, streamRes{Hash: "shared", W: 10, H: 10})
	third := mkStreamRes(t, ctx.db, streamRes{Hash: "shared", W: 20, H: 20})
	fired := 0
	rehash := func() {
		fired++
		require.NoError(t, ctx.db.Exec("UPDATE resources SET hash = 'different-now' WHERE id = ?", moves).Error)
	}
	if atInsert {
		computeBeforeInsertHook = func(string, int) {
			if fired == 0 {
				rehash()
			}
		}
		defer func() { computeBeforeInsertHook = nil }()
	} else {
		computeAfterNominateHook = func([]string) {
			if fired == 0 {
				rehash()
			}
		}
		defer func() { computeAfterNominateHook = nil }()
	}
	r := mkStreamReduction(t, ctx, streamSel{All: true, Identical: true})
	streamCompute(t, ctx, r)
	require.Equal(t, 1, fired)
	rows := streamRows(t, ctx, r, models.ReductionTierIdentical)
	require.Len(t, rows, 1)
	require.ElementsMatch(t, []uint{stays, third}, rows[0].ids(), "no Cluster holds a Resource that stopped sharing the hash")
	for _, m := range rows[0].Members {
		require.Equal(t, "shared", m.Hash)
	}
}

func TestStreamVersionUploadBetweenGroupingAndLoad(t *testing.T) {
	testStreamVersionUploadBetweenGroupingAndLoad(t, withSettings(newReductionSQLiteContext(t)), false)
}
func TestStreamVersionUploadBetweenLoadAndInsert(t *testing.T) {
	testStreamVersionUploadBetweenGroupingAndLoad(t, withSettings(newReductionSQLiteContext(t)), true)
}

// ---- a scoped principal ---------------------------------------------------------------------------

func testStreamScopedPrincipal(t *testing.T, ctx *MahresourcesContext) {
	a, b := &models.Group{Name: "A"}, &models.Group{Name: "B"}
	require.NoError(t, ctx.db.Create(a).Error)
	require.NoError(t, ctx.db.Create(b).Error)
	inA := mkStreamRes(t, ctx.db, streamRes{Hash: "dup", W: 10, H: 10, Owner: &a.ID, Image: true})
	inA2 := mkStreamRes(t, ctx.db, streamRes{Hash: "dup", W: 20, H: 20, Owner: &a.ID, Image: true})
	inB := mkStreamRes(t, ctx.db, streamRes{Hash: "dup", W: 90, H: 90, Owner: &b.ID, Image: true})
	nearA := mkStreamRes(t, ctx.db, streamRes{Hash: "x1", W: 30, H: 30, Owner: &a.ID, Image: true})
	nearB := mkStreamRes(t, ctx.db, streamRes{Hash: "x2", W: 99, H: 99, Owner: &b.ID, Image: true})
	mkPair(t, ctx.db, inA, nearA, 1)
	mkPair(t, ctx.db, nearA, nearB, 1)
	mkPair(t, ctx.db, inA2, inB, 1)
	outsideA := uint(0)
	_ = outsideA

	scoped := ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, UserID: 7, ScopeGroupID: &a.ID})
	r := mkStreamReduction(t, ctx, streamSel{All: true})
	var leaked int64
	computeAfterNominateHook = func([]string) {
		var n int64
		require.NoError(t, ctx.db.Raw(`SELECT COUNT(*) FROM reduction_extent e JOIN resources r ON r.id = e.resource_id WHERE e.reduction_id = ? AND (r.owner_id IS NULL OR r.owner_id <> ?)`, r.ID, a.ID).Scan(&n).Error)
		leaked += n
	}
	defer func() { computeAfterNominateHook = nil }()
	cur := loadReductionRow(t, ctx.db, r.ID)
	_, gen, err := scoped.beginReductionComputeStore(cur.ID, cur.Version)
	require.NoError(t, err)
	require.NoError(t, scoped.runReductionCompute(context.Background(), cur.ID, "", gen, nil))
	require.Zero(t, leaked, "an out-of-subtree Resource is never staged")
	for _, p := range planOf(t, ctx, r) {
		for _, id := range p.ids() {
			require.NotContains(t, []uint{inB, nearB}, id, "a scoped principal is never proposed a Resource outside its subtree")
		}
	}
}

func TestStreamScopedPrincipal(t *testing.T) {
	testStreamScopedPrincipal(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- sort-key seed order ------------------------------------------------------------------------

func testStreamSeedOrderIsTheWinnerRuleOrder(t *testing.T, ctx *MahresourcesContext) {
	rng := rand.New(rand.NewSource(3))
	var ids []uint
	for i := 0; i < 40; i++ {
		id := mkStreamRes(t, ctx.db, streamRes{W: uint(rng.Intn(4)) * 10, H: uint(rng.Intn(3)) * 10, Size: int64(rng.Intn(4)), Name: fmt.Sprintf("n%d", rng.Intn(5)), Image: true,
			Desc: []string{"", " ", "d"}[rng.Intn(3)]})
		ids = append(ids, id)
	}
	for i := 1; i < len(ids); i++ {
		mkPair(t, ctx.db, ids[0], ids[i], 1)
	}
	rule := []string{models.WinnerCriterionHasDescription, models.WinnerCriterionNameDesc, models.WinnerCriterionSizeAsc, models.WinnerCriterionPixelsDesc}
	r := mkStreamReduction(t, ctx, streamSel{All: true, Rule: rule})
	cur := loadReductionRow(t, ctx.db, r.ID)
	_, gen, err := ctx.beginReductionComputeStore(cur.ID, cur.Version)
	require.NoError(t, err)
	require.NoError(t, ctx.storeAdmitComputeLease(cur.ID, gen))
	run := &reductionStream{ctx: ctx, s: ctx.reductionStore(), r: cur, generation: gen, rule: rule, jobCtx: context.Background()}
	require.NoError(t, run.stageExtent())
	n, err := run.stageSeeds()
	require.NoError(t, err)
	require.EqualValues(t, len(ids), n)
	var got []uint
	require.NoError(t, ctx.db.Raw(`SELECT resource_id FROM reduction_seeds WHERE reduction_id = ? AND generation = ? ORDER BY key_prefix, sort_key`, cur.ID, gen).Scan(&got).Error)
	cands, err := run.loadCandidates(ids)
	require.NoError(t, err)
	sort.SliceStable(cands, func(i, j int) bool {
		return models.CompareByWinnerRule(rule, cands[i].WinnerCandidate, cands[j].WinnerCandidate) < 0
	})
	var want []uint
	for _, c := range cands {
		want = append(want, c.Resource.ID)
	}
	require.Equal(t, want, got)
}

func TestStreamSeedOrderIsTheWinnerRuleOrder(t *testing.T) {
	testStreamSeedOrderIsTheWinnerRuleOrder(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- no statement writes or returns more than a chunk -------------------------------------------

var limitRe = regexp.MustCompile(`(?i)LIMIT\s+(\?|\$(\d+)|\d+)`)

func testStreamStatementsAreBounded(t *testing.T, ctx *MahresourcesContext) {
	for i := 0; i < 700; i++ {
		mkStreamRes(t, ctx.db, streamRes{Hash: fmt.Sprintf("h%d", i/3), W: uint(10 + i%7), H: 10, Size: int64(i)})
	}
	var worst int64
	var offenders []string
	check := func(db *gorm.DB) {
		if db.Error != nil {
			return
		}
		if db.Statement.RowsAffected > worst {
			worst = db.Statement.RowsAffected
		}
		if db.Statement.RowsAffected > int64(reductionRowChunk) {
			offenders = append(offenders, fmt.Sprintf("%d rows: %s", db.Statement.RowsAffected, db.Statement.SQL.String()))
		}
	}
	checkRow := func(db *gorm.DB) {
		sql := db.Statement.SQL.String()
		m := limitRe.FindStringSubmatchIndex(sql)
		if m == nil {
			return
		}
		lit := sql[m[2]:m[3]]
		var limit int
		switch {
		case lit == "?":
			idx := strings.Count(sql[:m[0]], "?")
			limit, _ = strconv.Atoi(fmt.Sprint(db.Statement.Vars[idx]))
		case strings.HasPrefix(lit, "$"):
			idx, _ := strconv.Atoi(lit[1:])
			limit, _ = strconv.Atoi(fmt.Sprint(db.Statement.Vars[idx-1]))
		default:
			limit, _ = strconv.Atoi(lit)
		}
		if limit > reductionRowChunk {
			offenders = append(offenders, fmt.Sprintf("LIMIT %d: %s", limit, sql))
		}
	}
	cb := ctx.db.Callback()
	require.NoError(t, cb.Create().After("gorm:create").Register("test:bounded_create", check))
	require.NoError(t, cb.Update().After("gorm:update").Register("test:bounded_update", check))
	require.NoError(t, cb.Delete().After("gorm:delete").Register("test:bounded_delete", check))
	require.NoError(t, cb.Query().After("gorm:query").Register("test:bounded_query", check))
	require.NoError(t, cb.Raw().After("gorm:raw").Register("test:bounded_raw", check))
	require.NoError(t, cb.Row().After("gorm:row").Register("test:bounded_row", checkRow))
	t.Cleanup(func() {
		_ = cb.Create().Remove("test:bounded_create")
		_ = cb.Update().Remove("test:bounded_update")
		_ = cb.Delete().Remove("test:bounded_delete")
		_ = cb.Query().Remove("test:bounded_query")
		_ = cb.Raw().Remove("test:bounded_raw")
		_ = cb.Row().Remove("test:bounded_row")
	})
	r := mkStreamReduction(t, ctx, streamSel{All: true})
	streamCompute(t, ctx, r)
	require.Empty(t, offenders)
	require.Len(t, planOf(t, ctx, r), 233, "700 Resources in threes, the last hash holding one")
	t.Logf("largest statement: %d rows", worst)
}

func TestStreamStatementsAreBounded(t *testing.T) {
	testStreamStatementsAreBounded(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- availability against carried Clusters ----------------------------------------------------

// Carried Clusters are in the generation before any fresh one is built, and what
// they claim is unavailable to it: a reviewed or skipped Cluster claims all its
// members, read-only history only its ejected ones, an applied-and-merged Cluster
// none, and an open one is simply rebuilt.
func testStreamAvailabilityAgainstCarriedClusters(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	res := func(hash string) uint { return makeResource(t, ctx.db, "r-"+hash, opsRes{Hash: hash, W: 10, H: 10}) }
	a1, a2 := res("ha"), res("ha")
	b1, b2, b3 := res("hb"), res("hb"), res("hb")
	c1, c3 := res("hc"), res("hc")
	d1, d2 := res("hd"), res("hd")
	mk := func(key, state string, pos int64, ro, merged bool, winner uint, members ...models.ReductionMemberRow) {
		c := &storedCluster{Row: models.ReductionClusterRow{Key: key, Tier: models.ReductionTierIdentical, State: state, Position: pos, ReadOnly: ro, Merged: merged, WinnerID: winner}, Members: members}
		require.NoError(t, ctx.reductionStore().insertCluster(r, opsGen, c))
	}
	mk("skipped", models.ReductionClusterSkipped, 1, false, false, a1, opsMember(a1, "ha"), opsMember(a2, "ha"))
	mk("history", models.ReductionClusterOpen, 2, true, false, b1, opsMember(b1, "hb"), ejectedMember(b2, "hb"))
	mk("merged", models.ReductionClusterApplied, 3, false, true, c1, opsMember(c1, "hc"), opsMember(c3+1000, "hc"))
	mk("open", models.ReductionClusterOpen, 4, false, false, d1, opsMember(d1, "hd"), opsMember(d2, "hd"))
	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET reviewed = (key = 'skipped')`).Error)

	streamCompute(t, ctx, r)
	plan := planOf(t, ctx, r)
	owner := map[uint]string{}
	for key, p := range plan {
		for _, id := range p.ids() {
			if p.Row.Tier == models.ReductionTierIdentical && key != "skipped" && key != "history" && key != "merged" {
				owner[id] = key
			}
		}
	}
	require.NotContains(t, owner, a1, "a skipped Cluster's members are claimed")
	require.NotContains(t, owner, a2)
	require.NotContains(t, owner, b2, "read-only history keeps its ejected member out")
	require.Contains(t, owner, b1, "and releases its other members")
	require.Contains(t, owner, b3)
	require.Equal(t, owner[b1], owner[b3])
	require.Equal(t, owner[c1], owner[c3], "an applied-and-merged Cluster claims nothing")
	require.Contains(t, owner, c1)
	require.Equal(t, owner[d1], owner[d2], "an open Cluster is rebuilt")
	for _, key := range []string{"skipped", "history", "merged"} {
		require.Contains(t, plan, key, "carried")
	}
	require.False(t, plan[owner[b1]].Row.Checked, "a proposal over read-only history needs fresh approval")
}

func TestStreamAvailabilityAgainstCarriedClusters(t *testing.T) {
	testStreamAvailabilityAgainstCarriedClusters(t, withSettings(newReductionSQLiteContext(t)))
}

// The streaming compute never hydrates a full Resource row.
func TestStreamLoadsNoFullResourceRows(t *testing.T) {
	ctx := withSettings(newReductionSQLiteContext(t))
	ids := seedHashedImages(t, ctx, 40)
	mkPair(t, ctx.db, ids[0], ids[1], 2)
	full := countFullResourceRows(t, ctx)
	r := mkStreamReduction(t, ctx, streamSel{All: true})
	streamCompute(t, ctx, r)
	require.Len(t, planOf(t, ctx, r), 1)
	require.Zero(t, full.Load())
}
