package application_context

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"mahresources/download_queue"
	"mahresources/models"
)

// The streaming compute (design D5): the Extent is staged into reduction_extent in
// keyset chunks, the Identical and Near tiers read it back in chunks, and every
// Cluster is inserted the moment it is built, so what is held in Go at any time is
// bounded by the chunk size and the member cap, never by the library. Availability
// (a Resource a Cluster of this generation already claims) is an anti-join in the
// statements that read holders and neighbours, so there is no set of claimed ids.

// computeChunk is how many rows one streaming step reads or writes (design rule 1).
// A variable only so a test can run the whole compute at chunk size 1.
var computeChunk = reductionRowChunk

// computeNameBytes is how much of a name or content type the Winner Rule and its
// byte key compare (design D5): the first 4,096 bytes.
const computeNameBytes = 4096

// computeInsertAttempts bounds the discard-and-rebuild of a Cluster whose
// classification changed between its build and its insertion.
const computeInsertAttempts = 3

// Test seams: nil outside tests.
var (
	// computeAfterNominateHook runs between the Identical tier's grouping (the hashes
	// it nominated) and the load of their holders.
	computeAfterNominateHook func(hashes []string)
	// computeBeforeInsertHook runs before a built Cluster's insertion transaction.
	computeBeforeInsertHook func(tier string, attempt int)
)

// errClusterReclassified says a built Cluster no longer matches the rows it was
// built from: a member's hash, Extent membership or existence changed.
var errClusterReclassified = errors.New("a Cluster's members changed while it was built")

// reductionStream is one compute attempt's state. Nothing in it grows with the
// library: scalars, the rule, and one cursor.
type reductionStream struct {
	ctx        *MahresourcesContext
	s          *reductionStore
	r          *models.ResourceReduction
	generation string
	rule       []string
	threshold  int
	aThreshold uint64
	assoc      bool
	jobCtx     context.Context
	progress   download_queue.ProgressSink
	position   int64
	extentSize int
}

// streamReductionPlan is the whole compute: stage, measure, carry, cluster, land.
func (ctx *MahresourcesContext) streamReductionPlan(jobCtx context.Context, reductionID uint, jobID, generation string, progress download_queue.ProgressSink) error {
	return ctx.streamReductionPlanOwned(jobCtx, reductionID, jobID, generation, progress, nil, false)
}

// streamReductionPlanOwned is streamReductionPlan landing against the owner captured
// at claim (claimOwner) when checkOwner is set.
func (ctx *MahresourcesContext) streamReductionPlanOwned(jobCtx context.Context, reductionID uint, jobID, generation string, progress download_queue.ProgressSink, claimOwner *uint, checkOwner bool) error {
	// Read unscoped by owner: this runs for the Reduction the request already
	// authorized, and the principal's *data* scope applies through the store.
	reduction, err := ctx.loadReductionForUpdate(reductionID, nil, false)
	if err != nil {
		return err
	}
	rule := DecodeWinnerRule(reduction.WinnerRule)
	run := &reductionStream{
		ctx: ctx, s: ctx.reductionStore().boundTo(jobCtx), r: reduction, generation: generation, rule: rule,
		assoc:  ruleUsesAssociations(rule),
		jobCtx: jobCtx, progress: progress,
	}
	phase := func(name string) {
		if progress != nil {
			progress.SetPhase(name)
		}
	}

	// Generations nobody owns any more (a failed or crashed compute's) go first, at
	// compute start as design D4 says; a failure here is logged, not fatal.
	if err := run.s.sweepGenerations(); err != nil {
		ctx.logReductionWriteFailure(reductionID, "sweep at compute start", err)
	}
	// An earlier attempt's rows of this generation go next.
	if err := run.s.clearGeneration(reduction.ID, generation); err != nil {
		return err
	}
	if err := run.s.clearStaging(reduction.ID, generation); err != nil {
		return err
	}

	phase("resolving the Extent")
	if err := run.stageExtent(); err != nil {
		return err
	}
	phase("measuring coverage")
	coverage, err := run.coverage()
	if err != nil {
		return err
	}
	run.extentSize = coverage.ExtentSize

	// A judgement already made is never rearranged by a later compute: the carried
	// Clusters are in the new generation before any fresh one is built, and what they
	// claim is unavailable to the fresh ones.
	if err := run.s.carryOver(reduction.ID, generation); err != nil {
		return err
	}
	if reductionLandHook != nil {
		reductionLandHook()
	}
	if err := run.s.db.Raw(`SELECT COALESCE(MAX(position), 0) FROM reduction_clusters WHERE reduction_id = ? AND generation = ?`, reduction.ID, generation).
		Scan(&run.position).Error; err != nil {
		return err
	}

	phase("Identical Resources")
	if err := run.identicalTier(); err != nil {
		return err
	}
	if reduction.MatchingMode != models.MatchingModeIdenticalOnly {
		phase("Near-Identical Resources")
		if err := run.nearTier(); err != nil {
			return err
		}
	}
	if err := jobCtx.Err(); err != nil {
		return err
	}
	settings := &models.ReductionSettings{
		MatchingMode:    normalizeMatchingMode(reduction.MatchingMode),
		WinnerRule:      rule,
		ExcludeExternal: reduction.ExcludeExternalResources,
	}
	// The landing compares the owner captured at claim, not the one this run re-read
	// (a caller with no claim, the test seams, falls back to the re-read).
	landing := *reduction
	if checkOwner {
		landing.CreatedByUserId = claimOwner
	}
	return ctx.finishLandingOwned(&landing, jobID, generation, reduction.Version, coverage, settings, true)
}

func (run *reductionStream) report(done, total int64) {
	if run.progress != nil {
		run.progress.SetPhaseProgress(done, total)
	}
}

// visibleOwner is the owner predicate a confined principal's Resources must meet,
// over the Resource alias; empty for everyone else. A deny scope never reaches a
// statement (every caller returns early), so it is left to fail closed anyway.
func (run *reductionStream) visibleOwner(alias string) string {
	switch run.s.scope.mode {
	case scopeSubtree:
		return " AND " + alias + ".owner_id IN " + subtreeIDs
	case scopeDeny:
		return " AND 1 = 0"
	}
	return ""
}

// ---- staging the Extent ---------------------------------------------------------------

// extentTree is the WITH clause naming the selected Groups' subtrees: ext_tree is
// every Group under a selected root the principal can see now. A confined
// principal's own subtree (scope_tree) is bound once beside it.
func (run *reductionStream) extentTree() (string, []any) {
	prefix, args := run.s.with()
	tree := `ext_tree(id) AS (
	SELECT g.id FROM groups g WHERE g.id IN (SELECT eg.group_id FROM reduction_extent_groups eg WHERE eg.reduction_id = ?)`
	if run.s.scope.mode == scopeSubtree {
		tree += ` AND g.id IN ` + subtreeIDs
		prefix += ", "
	} else {
		prefix = "WITH RECURSIVE "
	}
	tree += `
	UNION
	SELECT g.id FROM groups g INNER JOIN ext_tree et ON g.owner_id = et.id
)`
	return prefix + tree + " ", append(args, run.r.ID)
}

// stageExtent copies the Extent into reduction_extent(G) in keyset chunks, each
// fenced. Staged membership is never authorization: every statement that reads
// Resources joins the principal's own subtree again.
func (run *reductionStream) stageExtent() error {
	if run.s.scope.mode == scopeDeny {
		return nil
	}
	r := run.r
	visible := run.visibleOwner("r")
	stage := func(ids []uint) error {
		prefix, args := run.s.with()
		return run.s.fenced(r.ID, run.generation, func(tx *gorm.DB) error {
			return tx.Exec(prefix+`INSERT INTO reduction_extent (reduction_id, generation, resource_id, hash)
				SELECT ?, ?, r.id, COALESCE(r.hash, '') FROM resources r WHERE r.id IN ?`+visible+` ON CONFLICT DO NOTHING`,
				append(args, r.ID, run.generation, ids)...).Error
		})
	}
	// walk pages ids by keyset until the query runs dry.
	walk := func(sql string, args ...any) error {
		after := uint(0)
		for {
			if err := run.jobCtx.Err(); err != nil {
				return err
			}
			var ids []uint
			if err := run.s.db.Raw(sql, append(append([]any{}, args...), after, computeChunk)...).Scan(&ids).Error; err != nil {
				return err
			}
			if len(ids) == 0 {
				return nil
			}
			if err := stage(ids); err != nil {
				return err
			}
			after = ids[len(ids)-1]
		}
	}

	if r.ExtentAll {
		prefix, args := run.s.with()
		if err := walk(prefix+`SELECT r.id FROM resources r WHERE r.id > ?`+visible+` ORDER BY r.id LIMIT ?`, args...); err != nil {
			return err
		}
		// The whole library as the principal sees it: nothing else can add to it.
		return nil
	}

	// Explicit Resources: the selection rows in id order, staged through the join.
	if err := walk(`SELECT resource_id FROM reduction_extent_resources WHERE reduction_id = ? AND resource_id > ? ORDER BY resource_id LIMIT ?`, r.ID); err != nil {
		return err
	}
	// Groups: the Resources owned by, then related to, a Group of the selected
	// subtrees, by keyset over the matching Resources.
	var hasGroups int64
	if err := run.s.db.Raw(`SELECT COUNT(*) FROM (SELECT 1 FROM reduction_extent_groups WHERE reduction_id = ? LIMIT 1) g`, r.ID).Scan(&hasGroups).Error; err != nil {
		return err
	}
	if hasGroups == 0 {
		return nil
	}
	tree, targs := run.extentTree()
	if err := walk(tree+`SELECT r.id FROM resources r WHERE r.id > ? AND r.owner_id IN (SELECT id FROM ext_tree)`+visible+` ORDER BY r.id LIMIT ?`, targs...); err != nil {
		return err
	}
	return walk(tree+`SELECT r.id FROM resources r WHERE r.id > ? AND r.id IN
		(SELECT grr.resource_id FROM groups_related_resources grr WHERE grr.group_id IN (SELECT id FROM ext_tree))`+visible+` ORDER BY r.id LIMIT ?`, targs...)
}

// liveInExtent answers, for at most a Cluster's worth of ids, which of them are in
// the Extent now: the Extent definition itself (the whole library, the explicit
// ids, or owned or related Resources of the visible selected Group subtrees), not
// the staged copy. Read inside the insertion transaction.
func (s *reductionStore) liveInExtent(run *reductionStream, ids []uint) (map[uint]bool, error) {
	in := map[uint]bool{}
	if len(ids) == 0 || s.scope.mode == scopeDeny {
		return in, nil
	}
	visible := run.visibleOwner("r")
	var sql string
	var args []any
	if run.r.ExtentAll {
		prefix, pargs := s.with()
		sql = prefix + `SELECT r.id FROM resources r WHERE r.id IN ?` + visible
		args = append(pargs, ids)
	} else {
		tree, targs := run.extentTree()
		sql = tree + `SELECT r.id FROM resources r WHERE r.id IN ?` + visible + ` AND (
			r.id IN (SELECT e.resource_id FROM reduction_extent_resources e WHERE e.reduction_id = ?)
			OR r.owner_id IN (SELECT id FROM ext_tree)
			OR r.id IN (SELECT grr.resource_id FROM groups_related_resources grr WHERE grr.group_id IN (SELECT id FROM ext_tree)))`
		args = append(targs, ids, run.r.ID)
	}
	var found []uint
	if err := s.db.Raw(sql, args...).Scan(&found).Error; err != nil {
		return nil, err
	}
	for _, id := range found {
		in[id] = true
	}
	return in, nil
}

// coverage measures the staged Extent in id-range chunks.
func (run *reductionStream) coverage() (models.ReductionCoverage, error) {
	var coverage models.ReductionCoverage
	eligible := hashableContentTypes()
	after := uint(0)
	for {
		if err := run.jobCtx.Err(); err != nil {
			return coverage, err
		}
		var hi uint
		if err := run.s.db.Raw(`SELECT COALESCE(MAX(resource_id), 0) FROM
			(SELECT resource_id FROM reduction_extent WHERE reduction_id = ? AND generation = ? AND resource_id > ? ORDER BY resource_id LIMIT ?) c`,
			run.r.ID, run.generation, after, computeChunk).Scan(&hi).Error; err != nil {
			return coverage, err
		}
		if hi == 0 {
			return coverage, nil
		}
		var row struct{ N, Hashed, Eligible, Perceptual int64 }
		if err := run.s.db.Raw(`SELECT COUNT(*) AS n,
				CAST(COALESCE(SUM(CASE WHEN e.hash <> '' THEN 1 ELSE 0 END), 0) AS BIGINT) AS hashed,
				CAST(COALESCE(SUM(CASE WHEN r.content_type IN ? THEN 1 ELSE 0 END), 0) AS BIGINT) AS eligible,
				CAST(COALESCE(SUM((SELECT COUNT(*) FROM image_hashes ih WHERE ih.resource_id = e.resource_id)), 0) AS BIGINT) AS perceptual
			FROM reduction_extent e LEFT JOIN resources r ON r.id = e.resource_id
			WHERE e.reduction_id = ? AND e.generation = ? AND e.resource_id > ? AND e.resource_id <= ?`,
			eligible, run.r.ID, run.generation, after, hi).Scan(&row).Error; err != nil {
			return coverage, err
		}
		coverage.ExtentSize += int(row.N)
		coverage.ContentHashed += int(row.Hashed)
		coverage.PerceptualEligible += int(row.Eligible)
		coverage.PerceptualHashed += int(row.Perceptual)
		after = hi
	}
}

// ---- availability ------------------------------------------------------------------------

// available is the anti-join every holder and neighbour read applies: a Resource
// is unavailable when it is a member of a Cluster of this generation that is not
// an applied-and-merged one, except that read-only history reserves only the
// members a reviewer ejected (an explicit "don't remove" must not return in an
// auto-checked Cluster). col is the Resource id expression.
//
// LIMIT 1 OFFSET 0 is a fence, not a bound: it stops Postgres from flattening the
// NOT EXISTS into a join, which it plans as a merge or hash anti join that reads the
// generation's members and Clusters whole, once per statement, however few rows
// the statement returns. Fenced, each candidate row costs one index probe.
func (run *reductionStream) available(col string) (string, []any) {
	return `NOT EXISTS (SELECT 1 FROM reduction_members am JOIN reduction_clusters ac ON ac.id = am.cluster_id
		WHERE am.reduction_id = ? AND am.generation = ? AND am.resource_id = ` + col + `
		AND ((ac.state = 'applied' AND NOT ac.merged) OR (ac.state <> 'applied' AND (NOT ac.read_only OR am.ejected)))
		LIMIT 1 OFFSET 0)`,
		[]any{run.r.ID, run.generation}
}

// isAvailable asks it of one Resource that must also still be visible.
func (run *reductionStream) isAvailable(id uint) (bool, error) {
	prefix, pargs := run.s.with()
	avail, aargs := run.available("r.id")
	var n int64
	err := run.s.db.Raw(prefix+`SELECT COUNT(*) FROM resources r WHERE r.id = ?`+run.visibleOwner("r")+` AND `+avail,
		append(append(pargs, id), aargs...)...).Scan(&n).Error
	return n > 0, err
}

// ---- narrow candidate loads ----------------------------------------------------------------

// inStagedExtent is the test that the Resource id expression is in this
// generation's staged Extent, binding the Reduction id and the generation. It is a
// scalar subquery with LIMIT 1, a key probe per row the statement returns, and
// deliberately not EXISTS: Postgres turns a correlated EXISTS into a hashed subplan
// that reads the whole staged Extent once per statement whenever it expects many
// rows (a hub with thousands of pairs, a hash with thousands of holders), which on
// a two million row Extent is a scan for every 400-row page.
func inStagedExtent(id string) string {
	return `((SELECT 1 FROM reduction_extent xe WHERE xe.reduction_id = ? AND xe.generation = ? AND xe.resource_id = ` + id + ` LIMIT 1) IS NOT NULL)`
}

// candidateRow is the narrow projection of a Resource the Winner Rule and the
// Cluster build read: no description, meta or own meta ever enters Go.
type candidateRow struct {
	ID                 uint
	Hash               string
	Width, Height      uint
	FileSize           int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Name               string
	ContentType        string
	SeriesID           *uint
	ResourceCategoryID uint `gorm:"column:resource_category_id"`
	HasDescription     bool
	InExtent           bool
}

// candidateSelect is the select list of candidateRow over a Resource aliased r,
// with its arguments (in text order). has_description is computed in SQL with the
// full unicode.IsSpace trim set; name and content type are cut to their first 4,096
// bytes.
func (run *reductionStream) candidateSelect() (string, []any) {
	name, contentType := `substr(CAST(COALESCE(r.name, '') AS BLOB), 1, `, `substr(CAST(COALESCE(r.content_type, '') AS BLOB), 1, `
	trim := `TRIM(COALESCE(r.description, ''), ?)`
	cut := fmt.Sprintf("%d)", computeNameBytes)
	if run.s.postgres() {
		name, contentType = `substring(convert_to(COALESCE(r.name, ''), 'UTF8') from 1 for `, `substring(convert_to(COALESCE(r.content_type, ''), 'UTF8') from 1 for `
		trim = `BTRIM(COALESCE(r.description, ''), ?)`
	}
	sql := `r.id, COALESCE(r.hash, '') AS hash, r.width, r.height, r.file_size, r.created_at, r.updated_at, ` +
		name + cut + ` AS name, ` + contentType + cut + ` AS content_type, r.series_id, r.resource_category_id, ` +
		`CASE WHEN ` + trim + ` <> '' THEN TRUE ELSE FALSE END AS has_description, ` +
		inStagedExtent("r.id") + ` AS in_extent`
	return sql, []any{spaceChars, run.r.ID, run.generation}
}

func (row candidateRow) candidate() clusterCandidate {
	res := &models.Resource{
		ID: row.ID, Hash: row.Hash, Width: row.Width, Height: row.Height, FileSize: row.FileSize,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Name: row.Name, ContentType: row.ContentType,
		SeriesID: row.SeriesID, ResourceCategoryId: row.ResourceCategoryID,
	}
	if row.HasDescription {
		// Only whether it is blank is ever compared.
		res.Description = "x"
	}
	return clusterCandidate{WinnerCandidate: models.WinnerCandidate{Resource: res}, InExtent: row.InExtent}
}

// withAssociations fills the association counts of a chunk when the rule reads them.
func (run *reductionStream) withAssociations(cands []clusterCandidate) error {
	if !run.assoc || len(cands) == 0 {
		return nil
	}
	ids := make([]uint, len(cands))
	for i, c := range cands {
		ids[i] = c.Resource.ID
	}
	counts, err := run.ctx.countResourceAssociations(ids)
	if err != nil {
		return err
	}
	for i := range cands {
		cands[i].Associations = counts[cands[i].Resource.ID]
	}
	return nil
}

// loadCandidates reads the named Resources (the ones the principal can see) narrowly,
// dropping an outsider when the Reduction confines clustering to its Extent.
func (run *reductionStream) loadCandidates(ids []uint) ([]clusterCandidate, error) {
	var out []clusterCandidate
	for _, chunk := range chunkUints(dedupeUints(ids), reductionRowChunk) {
		prefix, pargs := run.s.with()
		sel, sargs := run.candidateSelect()
		var rows []candidateRow
		if err := run.s.db.Raw(prefix+`SELECT `+sel+` FROM resources r WHERE r.id IN ?`+run.visibleOwner("r"),
			append(append(pargs, sargs...), chunk)...).Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("loading Cluster candidates: %w", err)
		}
		cands := make([]clusterCandidate, 0, len(rows))
		for _, row := range rows {
			if run.r.ExcludeExternalResources && !row.InExtent {
				continue
			}
			cands = append(cands, row.candidate())
		}
		if err := run.withAssociations(cands); err != nil {
			return nil, err
		}
		out = append(out, cands...)
	}
	return out, nil
}

// ---- insertion ---------------------------------------------------------------------------------

// pendingCluster is a built Cluster waiting for its insertion transaction.
type pendingCluster struct {
	stored *storedCluster
	// hash is the content hash every member of an Identical Cluster must still hold
	// when its Resources are locked.
	hash string
}

// insertBuilt inserts what build produces, one Cluster per transaction. Under the
// members' share locks the insertion re-reads each member's hash and live Extent
// membership; a Cluster whose rows no longer match what it was built from is
// discarded and rebuilt, at most computeInsertAttempts times.
func (run *reductionStream) insertBuilt(build func() (*pendingCluster, error)) error {
	for attempt := 0; attempt < computeInsertAttempts; attempt++ {
		if err := run.jobCtx.Err(); err != nil {
			return err
		}
		pc, err := build()
		if err != nil || pc == nil {
			return err
		}
		if computeBeforeInsertHook != nil {
			computeBeforeInsertHook(pc.stored.Row.Tier, attempt)
		}
		err = run.s.fenced(run.r.ID, run.generation, func(tx *gorm.DB) error {
			st := run.s.in(tx)
			return st.insertStoredFresh(run.r, run.generation, pc.stored, run.position+1, func(c *storedCluster, locked map[uint]lockedResource) error {
				return run.recheck(st, pc, c, locked)
			})
		})
		if errors.Is(err, errClusterReclassified) {
			continue
		}
		if err != nil {
			return err
		}
		run.position++
		return nil
	}
	id := run.r.ID
	run.ctx.Logger().Warning(models.LogActionUpdate, "resource_reduction", &id, "",
		fmt.Sprintf("Skipped a Cluster whose members kept changing while it was built (%d attempts)", computeInsertAttempts), nil)
	return nil
}

// recheck compares a built Cluster against the locked rows: every member exists,
// an Identical Cluster's members all hold its hash, and each member's Extent
// membership is the one it was built with.
func (run *reductionStream) recheck(st *reductionStore, pc *pendingCluster, c *storedCluster, locked map[uint]lockedResource) error {
	ids := make([]uint, 0, len(c.Members))
	for i := range c.Members {
		l, found := locked[c.Members[i].ResourceID]
		if !found || (pc.hash != "" && l.Hash != pc.hash) {
			return errClusterReclassified
		}
		c.Members[i].Hash = l.Hash
		ids = append(ids, c.Members[i].ResourceID)
	}
	live, err := st.liveInExtent(run, ids)
	if err != nil {
		return err
	}
	for _, m := range c.Members {
		if live[m.ResourceID] != m.InExtent {
			return errClusterReclassified
		}
	}
	return nil
}

// storedStreamCluster is a built Cluster as rows, with the columns only the
// streaming compute sets.
func storedStreamCluster(cluster *models.ReductionCluster, truncated int, seed uint, fixedSeed bool) *storedCluster {
	c := storedFromPlanCluster(cluster)
	c.Row.Truncated = truncated
	c.Row.SeedResourceID = seed
	c.Row.FixedSeed = fixedSeed
	return c
}
