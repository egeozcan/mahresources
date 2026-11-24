package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"mahresources/mrql"
)

// Table-backed writers for a Reduction's review decisions (design D3). They sit
// beside today's JSON-plan writers (resource_reduction_override.go and its
// neighbours), keep their request and response semantics and reuse their error
// values, and are not wired to a handler yet.
//
// Every writer here follows the one order the design fixes: the Reduction row
// (the owner- and version-qualified compare-and-set is the transaction's first
// statement), then Resources in ascending id (share locks), then Cluster rows in
// ascending id (FOR UPDATE on Postgres), then member rows. After the Cluster lock
// it re-reads the Cluster and its members, so a deletion hook's `gone` update
// that committed meanwhile is never overwritten by a copy read before.

var (
	// ErrReductionClusterReadOnly refuses acting on history that cannot be acted
	// on: a legacy Cluster over the member cap (D12).
	ErrReductionClusterReadOnly = errors.New("this Cluster is too large to act on; compute again")
	// ErrReductionClusterFixedSeed refuses promoting a member of a full Near star,
	// whose kept copy is its seed for good (D5).
	ErrReductionClusterFixedSeed = errors.New("this Cluster is at the size limit; its kept copy is fixed")
	// ErrReductionBulkTooMany refuses a bulk request naming more Clusters than the cap.
	ErrReductionBulkTooMany = fmt.Errorf("a bulk request may name at most %d Clusters", reductionBulkIDCap)
)

// reductionBulkIDCap is the most Cluster keys one bulk-by-ids request may name (D15).
const reductionBulkIDCap = 500

// in is the store bound to a transaction's handle. The scope stays the one the
// store was built with.
func (s *reductionStore) in(tx *gorm.DB) *reductionStore {
	bound := *s
	bound.db = tx
	return &bound
}

func (s *reductionStore) postgres() bool { return s.db.Dialector.Name() == "postgres" }

// reductionEditable refuses a review decision while the plan is not a settled
// table plan: a compute is in flight (it will replace the plan), or the row still
// holds a legacy JSON plan that the migration has not moved (D3, D12).
func reductionEditable(r *models.ResourceReduction) error {
	if EffectiveReductionStatus(r) == models.ReductionStatusComputing || r.PlanSchema == 0 {
		return ErrReductionBusy
	}
	return nil
}

// casReductionTx is the compare-and-set every table writer begins with: one
// conditional UPDATE qualified by the owner the caller loaded as well as the
// version, so a Reduction whose owner was nulled since cannot be written through
// a stale pointer. It takes the Reduction row lock on Postgres and the writer
// lock is already held on SQLite, and it reports whether the version still matched.
func casReductionTx(tx *gorm.DB, r *models.ResourceReduction) (bool, error) {
	q := tx.Model(&models.ResourceReduction{}).Where("id = ? AND version = ?", r.ID, r.Version)
	if r.CreatedByUserId == nil {
		q = q.Where("created_by_user_id IS NULL")
	} else {
		q = q.Where("created_by_user_id = ?", *r.CreatedByUserId)
	}
	res := q.Updates(map[string]any{"version": gorm.Expr("version + 1"), "updated_at": time.Now()})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// reloadReduction reads the Reduction row back with the ordinary projection.
func reloadReduction(tx *gorm.DB, id uint) (*models.ResourceReduction, error) {
	var out models.ResourceReduction
	if err := tx.Select(reductionColumns).First(&out, id).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

// lockReductionRowTx takes the Reduction row's lock without changing it. A write
// on SQLite (the writer lock is already held by BEGIN IMMEDIATE) and a row lock on
// Postgres, so it is also the first statement a transaction needs.
func lockReductionRowTx(tx *gorm.DB, id uint) error {
	res := tx.Exec(`UPDATE resource_reductions SET id = id WHERE id = ?`, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrReductionNotFound
	}
	return nil
}

// shareLockResources share-locks the Resources that exist among ids, ascending and
// in chunks, so no statement locks in scan order and two writers cannot deadlock.
// A no-op on SQLite, whose writer lock already excludes every other writer.
func (s *reductionStore) shareLockResources(ids []uint) error {
	if !s.postgres() || len(ids) == 0 {
		return nil
	}
	ids = append([]uint(nil), dedupeUints(ids)...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, chunk := range chunkUints(ids, reductionRowChunk) {
		var locked []uint
		if err := s.db.Model(&models.Resource{}).Clauses(clause.Locking{Strength: "SHARE"}).
			Where("resources.id IN ?", chunk).Order("resources.id").Pluck("resources.id", &locked).Error; err != nil {
			return err
		}
	}
	return nil
}

// lockClusterRows locks Cluster rows FOR UPDATE in ascending id, in chunks. A
// no-op on SQLite.
func (s *reductionStore) lockClusterRows(ids []uint) error {
	if !s.postgres() || len(ids) == 0 {
		return nil
	}
	ids = append([]uint(nil), ids...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, chunk := range chunkUints(ids, reductionRowChunk) {
		var locked []uint
		if err := s.db.Model(&models.ReductionClusterRow{}).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id IN ?", chunk).Order("id").Pluck("id", &locked).Error; err != nil {
			return err
		}
	}
	return nil
}

// memberResourceIDs are the Resource ids of a Cluster's first members, the ones a
// load reads.
func (s *reductionStore) memberResourceIDs(clusterID uint) ([]uint, error) {
	var ids []uint
	err := s.db.Raw(`SELECT resource_id FROM (SELECT resource_id FROM reduction_members WHERE cluster_id = ? ORDER BY position, id LIMIT ?) m`,
		clusterID, reductionMemberCap).Scan(&ids).Error
	return ids, err
}

// reductionOpsAfterLocksHook runs inside a writer's transaction right after it has
// taken its Resource and Cluster locks and before it decides anything from what
// they protect. Only a test sets it, to park the transaction there while another
// one tries to move what is locked.
var reductionOpsAfterLocksHook atomic.Pointer[func()]

func afterLocks() {
	if hook := reductionOpsAfterLocksHook.Load(); hook != nil {
		(*hook)()
	}
}

// errClusterWithheld says the Cluster exists but the principal may not fully see
// it. Callers that must not tell it from an unknown key (every request-side
// writer) treat it as gorm.ErrRecordNotFound.
var errClusterWithheld = errors.New("the Cluster is withheld from this principal")

// lockHead finds a Cluster by key (not yet judged visible) and takes the locks a
// writer of it needs, in the one order: its member Resources share-locked
// ascending (when resources is set), then the Cluster row. It returns the row id,
// or gorm.ErrRecordNotFound for an unknown key.
func (s *reductionStore) lockHead(reductionID uint, generation, key string, resources bool) (uint, error) {
	var head []uint
	if err := s.db.Raw(`SELECT id FROM reduction_clusters WHERE reduction_id = ? AND generation = ? AND "key" = ?`, reductionID, generation, key).Scan(&head).Error; err != nil {
		return 0, err
	}
	if len(head) == 0 {
		return 0, gorm.ErrRecordNotFound
	}
	if resources {
		ids, err := s.memberResourceIDs(head[0])
		if err != nil {
			return 0, err
		}
		if err := s.shareLockResources(ids); err != nil {
			return 0, err
		}
	}
	return head[0], s.lockClusterRows(head)
}

// lockedCluster locks a Cluster and re-reads it the way a reader would: a Cluster
// the principal may not fully see is errClusterWithheld, decided with the members'
// Resources locked, so an owner move cannot slip between the check and the write.
// An unknown key is gorm.ErrRecordNotFound.
func (s *reductionStore) lockedCluster(r *models.ResourceReduction, generation, key string) (*storedCluster, error) {
	if _, err := s.lockHead(r.ID, generation, key, true); err != nil {
		return nil, err
	}
	afterLocks()
	c, err := s.loadCluster(r, generation, key)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errClusterWithheld
	}
	return c, err
}

// lockedClusterUnfiltered locks a Cluster and re-reads it with no visibility test:
// for the bookkeeping that settles a claim this same caller took, which judged
// visibility when it took it.
func (s *reductionStore) lockedClusterUnfiltered(reductionID uint, generation, key string, resources bool) (*storedCluster, error) {
	id, err := s.lockHead(reductionID, generation, key, resources)
	if err != nil {
		return nil, err
	}
	return s.loadClusterByID(id)
}

// loadClusterByID reads one Cluster row and its first members, with no visibility
// test.
func (s *reductionStore) loadClusterByID(id uint) (*storedCluster, error) {
	var row models.ReductionClusterRow
	if err := s.db.First(&row, id).Error; err != nil {
		return nil, err
	}
	members, err := s.loadMembers(id, reductionMemberCap)
	if err != nil {
		return nil, err
	}
	return &storedCluster{Row: row, Members: members}, nil
}

// clusterColumns are the Cluster columns a decision or an apply write may change,
// plus the derived ones. Written as one map so a column cannot be forgotten.
func clusterColumns(row *models.ReductionClusterRow) map[string]any {
	var lossy any
	if len(row.Lossy) > 0 && string(row.Lossy) != "null" {
		lossy = types.JSON(row.Lossy)
	}
	return map[string]any{
		"state": row.State, "winner_id": row.WinnerID, "checked": row.Checked, "reviewed": row.Reviewed,
		"decided_by": row.DecidedBy, "margin": row.Margin, "undecided": row.Undecided,
		"keeps_smaller": row.KeepsSmaller, "lossy": lossy, "stale_reason": row.StaleReason,
		"merged": row.Merged, "claim_id": row.ClaimID, "applied_at": row.AppliedAt, "restore_seq": row.RestoreSeq,
		"member_count": row.MemberCount, "loser_count": row.LoserCount, "needs_attention": row.NeedsAttention,
		"fingerprint": row.Fingerprint, "total_size": row.TotalSize, "gone_members": row.GoneMembers,
		"outcome_batch": row.OutcomeBatch,
	}
}

// writeCluster writes a Cluster the caller changed in memory: it derives the
// derived columns from the members, writes the Cluster row, and writes only the
// member rows that differ from `before`. The caller holds the Cluster row lock and
// re-read the Cluster after taking it.
func (s *reductionStore) writeCluster(c *storedCluster, before []models.ReductionMemberRow) error {
	deriveClusterColumns(c)
	if err := s.db.Model(&models.ReductionClusterRow{}).Where("id = ?", c.Row.ID).Updates(clusterColumns(&c.Row)).Error; err != nil {
		return err
	}
	was := make(map[uint]models.ReductionMemberRow, len(before))
	for _, m := range before {
		was[m.ID] = m
	}
	for _, m := range c.Members {
		old := was[m.ID]
		if sameMember(old, m) {
			continue
		}
		var distance any
		if m.Distance != nil {
			distance = *m.Distance
		}
		if err := s.db.Model(&models.ReductionMemberRow{}).Where("id = ?", m.ID).Updates(map[string]any{
			"ejected": m.Ejected, "ejected_reason": m.EjectedReason, "distance": distance,
			"gone": m.Gone, "changed": m.Changed, "owner_at_merge": m.OwnerAtMerge,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func sameMember(a, b models.ReductionMemberRow) bool {
	if a.Ejected != b.Ejected || a.EjectedReason != b.EjectedReason || a.Gone != b.Gone || a.Changed != b.Changed || a.OwnerAtMerge != b.OwnerAtMerge {
		return false
	}
	if (a.Distance == nil) != (b.Distance == nil) {
		return false
	}
	return a.Distance == nil || *a.Distance == *b.Distance
}

func copyMembers(in []models.ReductionMemberRow) []models.ReductionMemberRow {
	out := make([]models.ReductionMemberRow, len(in))
	for i, m := range in {
		out[i] = m
		if m.Distance != nil {
			d := *m.Distance
			out[i].Distance = &d
		}
	}
	return out
}

// ---- the Cluster in memory ----------------------------------------------------

func (c *storedCluster) find(resourceID uint) *models.ReductionMemberRow {
	for i := range c.Members {
		if c.Members[i].ResourceID == resourceID {
			return &c.Members[i]
		}
	}
	return nil
}

// loserIDs are the Resources a merge of this Cluster would destroy, in member order.
func (c *storedCluster) loserIDs() []uint {
	var ids []uint
	for _, m := range c.Members {
		if !m.Ejected && m.ResourceID != c.Row.WinnerID {
			ids = append(ids, m.ResourceID)
		}
	}
	return ids
}

// reopen takes the Cluster back to open, forgetting why it was stale or claimed.
func (c *storedCluster) reopen() {
	c.Row.State = models.ReductionClusterOpen
	c.Row.StaleReason = ""
	c.Row.ClaimID = ""
	c.Row.AppliedAt = nil
	for i := range c.Members {
		c.Members[i].Changed = false
	}
}

// readDeletions is the review's reading of a pending Cluster with a deleted member
// (reviewReach, readDeletions): stale, with the reason, and unchecked. The stored
// state stays what a writer last wrote; this is how it is read.
func (c *storedCluster) readDeletions() {
	if (c.Row.State == models.ReductionClusterOpen || c.Row.State == models.ReductionClusterSkipped) && c.Row.GoneMembers > 0 {
		c.Row.State = models.ReductionClusterStale
		c.Row.StaleReason = staleReasonDeleted
		c.Row.Checked = false
	}
}

// ---- the winner rule and the pair probes ---------------------------------------

// storedWinnerRule is the rule the plan was computed with: the recorded settings
// when there are any, else the Reduction's own (planWinnerRule's rule).
func storedWinnerRule(r *models.ResourceReduction) []string {
	if len(r.PlanSettings) > 0 && string(r.PlanSettings) != "null" {
		var settings models.ReductionSettings
		if err := json.Unmarshal(r.PlanSettings, &settings); err == nil && len(settings.WinnerRule) > 0 {
			return settings.WinnerRule
		}
	}
	return DecodeWinnerRule(r.WinnerRule)
}

// pairProbe reads the stored perceptual pair between one Resource and each of a
// few named others, within the live threshold and the aHash guard: the
// named-pair probe the design requires in place of reading every pair of a Winner
// (a dense Winner has millions). It returns the closest distance per named id; an
// id with no qualifying stored pair is absent. The partner must still exist, as
// every read of the pair table requires.
func (s *reductionStore) pairProbe(winnerID uint, others []uint, threshold int, aThreshold uint64, lock bool) (map[uint]uint8, error) {
	out := map[uint]uint8{}
	if len(others) == 0 {
		return out, nil
	}
	distance := mrql.SimilarPairDistance("resource_similarities")
	within := mrql.SimilarPairPredicate("resource_similarities", threshold, aThreshold)
	for _, direction := range []struct{ self, other string }{{"resource_id1", "resource_id2"}, {"resource_id2", "resource_id1"}} {
		var rows []similarPair
		q := s.db.Model(&models.ResourceSimilarity{})
		if lock && s.postgres() {
			q = q.Clauses(clause.Locking{Strength: "SHARE", Table: clause.Table{Name: "resource_similarities"}})
		}
		if err := q.
			Joins("INNER JOIN resources ON resources.id = resource_similarities."+direction.other).
			Select("resource_similarities."+direction.other+" AS resource_id, "+distance+" AS distance").
			Where("resource_similarities."+direction.self+" = ?", winnerID).
			Where("resource_similarities."+direction.other+" IN ?", others).
			Where(within).
			Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("reading the similarity pairs of resource %d: %w", winnerID, err)
		}
		for _, row := range rows {
			if d, seen := out[row.ResourceID]; !seen || row.Distance < d {
				out[row.ResourceID] = row.Distance
			}
		}
	}
	return out, nil
}

// ---- narrow Resource reads for the curation warnings ----------------------------

// spaceChars is every rune unicode.IsSpace accepts, so SQL's TRIM with this
// character set trims exactly what strings.TrimSpace does.
var spaceChars = func() string {
	var b strings.Builder
	for r := rune(0); r <= 0x3000; r++ {
		if unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}()

// lossyInputs reads only what lossyFields and keepsSmaller compare, for the
// Resources that exist: series, category, pixel dimensions, and whether the
// description is non-blank (computed in SQL; no description enters Go).
func (s *reductionStore) lossyInputs(ids []uint) (map[uint]*models.Resource, error) {
	out := map[uint]*models.Resource{}
	trim := "TRIM(COALESCE(description, ''), ?)"
	if s.postgres() {
		trim = "BTRIM(COALESCE(description, ''), ?)"
	}
	for _, chunk := range chunkUints(dedupeUints(ids), reductionRowChunk) {
		var rows []struct {
			ID                 uint
			SeriesID           *uint
			ResourceCategoryID uint `gorm:"column:resource_category_id"`
			Width, Height      uint
			HasDescription     bool
		}
		if err := s.db.Raw(`SELECT id, series_id, resource_category_id, width, height, CASE WHEN `+trim+` <> '' THEN TRUE ELSE FALSE END AS has_description
			FROM resources WHERE id IN ?`, spaceChars, chunk).Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			res := &models.Resource{ID: row.ID, SeriesID: row.SeriesID, ResourceCategoryId: row.ResourceCategoryID, Width: row.Width, Height: row.Height}
			if row.HasDescription {
				res.Description = "x"
			}
			out[row.ID] = res
		}
	}
	return out, nil
}

// refreshLossy recomputes the curation warnings after a change of Winner or of
// membership (resource_reduction_override.go's refreshLossy, over narrow reads).
func (s *reductionStore) refreshLossy(c *storedCluster, rule []string) error {
	ids := make([]uint, 0, len(c.Members))
	for _, m := range c.Members {
		if !m.Ejected {
			ids = append(ids, m.ResourceID)
		}
	}
	resources, err := s.lossyInputs(ids)
	if err != nil {
		return err
	}
	candidates := make([]clusterCandidate, 0, len(ids))
	for _, id := range ids {
		if res := resources[id]; res != nil {
			candidates = append(candidates, clusterCandidate{WinnerCandidate: models.WinnerCandidate{Resource: res}})
		}
	}
	lossy := lossyFields(candidates, c.Row.WinnerID)
	c.Row.Lossy = nil
	if len(lossy) > 0 {
		encoded, err := json.Marshal(lossy)
		if err != nil {
			return err
		}
		c.Row.Lossy = types.JSON(encoded)
	}
	c.Row.KeepsSmaller = keepsSmaller(candidates, c.Row.WinnerID, rule, c.Row.DecidedBy)
	return nil
}

// ---- override ------------------------------------------------------------------

// storeOverrideCluster is OverrideReductionCluster on the tables: one review
// decision on one Cluster. r is the already-authorized Reduction row as the caller
// loaded it (reductionColumns); the request's version must be the loaded one.
//
// Refused while a compute is in flight, while the row is still a legacy JSON plan,
// and on read-only history. The transaction begins with the compare-and-set, takes
// the Cluster's member Resources share-locked ascending, then the Cluster row,
// re-reads both, and only then runs the decision.
func (ctx *MahresourcesContext) storeOverrideCluster(r *models.ResourceReduction, override *query_models.ReductionOverride) (*models.ResourceReduction, error) {
	if r == nil || override == nil || r.ID == 0 {
		return nil, errors.New("no Resource Reduction given")
	}
	if err := reductionEditable(r); err != nil {
		return nil, err
	}
	if override.Version != r.Version {
		return nil, ErrReductionConflict
	}
	if r.PlanGeneration == nil {
		return nil, ErrReductionClusterNotFound
	}
	generation := *r.PlanGeneration
	rule := storedWinnerRule(r)
	threshold := ctx.similarityThreshold()
	_, aThreshold := ctx.similarityThresholds()
	store := ctx.reductionStore()

	var updated *models.ResourceReduction
	err := retryOnLockContention(reductionCASRetries, func() error {
		return store.db.Transaction(func(tx *gorm.DB) error {
			ok, err := casReductionTx(tx, r)
			if err != nil {
				return err
			}
			if !ok {
				return ErrReductionConflict
			}
			st := store.in(tx)
			c, err := st.lockedCluster(r, generation, override.ClusterID)
			if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, errClusterWithheld) {
				// An unknown key and a Cluster reaching a Resource this caller may
				// not see are one answer, so the refusal is no oracle for hidden ids.
				return ErrReductionClusterNotFound
			}
			if err != nil {
				return err
			}
			if c.Row.State == models.ReductionClusterApplied {
				return ErrReductionClusterSettled
			}
			if c.Row.ReadOnly {
				return ErrReductionClusterReadOnly
			}
			before := copyMembers(c.Members)
			// Read the way the page does, so a Cluster the page shows as stale for a
			// deleted copy is refused as stale, not as unknown.
			c.readDeletions()
			env := overrideEnv{s: st, rule: rule, threshold: threshold, aThreshold: aThreshold}
			if err := env.apply(c, override); err != nil {
				return err
			}
			if err := st.writeCluster(c, before); err != nil {
				return err
			}
			updated, err = reloadReduction(tx, r.ID)
			return err
		})
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// overrideEnv is what a decision reads besides the Cluster: the store bound to its
// transaction, the Winner Rule, and the live similarity thresholds.
type overrideEnv struct {
	s          *reductionStore
	rule       []string
	threshold  int
	aThreshold uint64
}

// apply is applyOverride over the rows. Each branch is today's, with the pair
// reads narrowed to named pairs and the table-only rules added: a full Near star
// refuses a promotion and never has its seed made a Loser, promote and restore
// bump restore_seq (the only ways a Loser is added), and a restore re-reads the
// member's existence.
func (e *overrideEnv) apply(c *storedCluster, o *query_models.ReductionOverride) error {
	switch o.Action {
	case ReductionActionPromote:
		return e.promote(c, o.ResourceID)

	case ReductionActionEject:
		member := c.find(o.ResourceID)
		if member == nil {
			return ErrReductionMemberNotFound
		}
		if member.ResourceID == c.Row.WinnerID {
			return ErrReductionEjectWinner
		}
		member.Ejected = true
		member.EjectedReason = models.EjectReasonManual
		c.Row.Reviewed = true
		if c.Row.State == models.ReductionClusterStale {
			c.reopen()
		}
		if len(c.loserIDs()) == 0 {
			c.Row.Checked = false
		}
		return e.s.refreshLossy(c, e.rule)

	case ReductionActionRestore:
		member := c.find(o.ResourceID)
		if member == nil {
			return ErrReductionMemberNotFound
		}
		if !member.InExtent {
			return ErrReductionRestoreOutsider
		}
		if c.Row.FixedSeed && member.ResourceID == c.Row.SeedResourceID && member.ResourceID != c.Row.WinnerID {
			// The seed of a full star is what it keeps; putting it back as a Loser
			// would delete the one match every left-out neighbour is known to have.
			return ErrReductionClusterFixedSeed
		}
		member.Ejected = false
		member.EjectedReason = ""
		c.Row.Reviewed = true
		c.Row.RestoreSeq++
		// The member may have been deleted while it was ejected: say so, so the
		// Cluster reads stale instead of proposing a merge of nothing.
		var present int64
		if err := e.s.db.Model(&models.Resource{}).Where("id = ?", member.ResourceID).Count(&present).Error; err != nil {
			return err
		}
		member.Gone = present == 0
		if c.Row.Tier == models.ReductionTierNear {
			// Only this member is checked and only this member changes: a
			// whole-Cluster re-justification would re-arm others whose pair has
			// since reappeared. One named-pair probe.
			pairs, err := e.s.pairProbe(c.Row.WinnerID, []uint{member.ResourceID}, e.threshold, e.aThreshold, false)
			if err != nil {
				return err
			}
			distance, paired := pairs[member.ResourceID]
			if !paired {
				return ErrReductionRestoreUnpaired
			}
			d := int(distance)
			member.Distance = &d
		}
		return e.s.refreshLossy(c, e.rule)

	case ReductionActionSkip:
		c.Row.State = models.ReductionClusterSkipped
		c.Row.Checked = false
		c.Row.Reviewed = true
		return nil

	case ReductionActionReopen:
		c.reopen()
		c.Row.Reviewed = true
		return nil

	case ReductionActionCheck:
		if c.Row.Oversized && !o.AcknowledgeOversized {
			return ErrReductionOversizedUnexpanded
		}
		if c.Row.State == models.ReductionClusterStale && c.Row.StaleReason == staleReasonDeleted {
			return ErrReductionClusterStale
		}
		if len(c.loserIDs()) == 0 {
			return ErrReductionNothingToRemove
		}
		c.Row.Checked = true
		c.Row.Reviewed = true
		if c.Row.State == models.ReductionClusterSkipped || c.Row.State == models.ReductionClusterStale {
			c.reopen()
		}
		return nil

	case ReductionActionUncheck:
		c.Row.Checked = false
		c.Row.Reviewed = true
		return nil
	}
	return fmt.Errorf("%q is not a Cluster action", o.Action)
}

// promote is promoteMember: a different member becomes the Winner, and ADR 0002's
// pair-justification is re-established by ejecting every Loser with no stored pair
// to the new Winner, probing only the pairs between the new Winner and the members
// named in this Cluster.
func (e *overrideEnv) promote(c *storedCluster, resourceID uint) error {
	member := c.find(resourceID)
	if member == nil {
		return ErrReductionMemberNotFound
	}
	if member.ResourceID == c.Row.WinnerID {
		return nil
	}
	if c.Row.FixedSeed {
		return ErrReductionClusterFixedSeed
	}
	// A member outside the Extent may win and may never lose: promoting past one
	// ejects it rather than leaving it a Loser.
	for i := range c.Members {
		existing := &c.Members[i]
		if existing.InExtent || existing.Ejected {
			continue
		}
		existing.Ejected = true
		existing.EjectedReason = models.EjectReasonOutsiderDemoted
	}
	c.Row.WinnerID = member.ResourceID
	// An approval belongs to the proposal it was given to; a different kept copy is
	// a different proposal.
	c.Row.Checked = false
	member.Ejected = false
	member.EjectedReason = ""
	member.Distance = nil
	c.Row.Reviewed = true
	c.Row.DecidedBy = models.ManualCriterion
	c.Row.Margin = ""
	c.Row.Undecided = false
	c.Row.RestoreSeq++
	if c.Row.State == models.ReductionClusterStale {
		c.reopen()
	}
	if c.Row.Tier == models.ReductionTierNear {
		if err := e.rejustify(c); err != nil {
			return err
		}
	}
	return e.s.refreshLossy(c, e.rule)
}

// rejustify is rejustifyAgainstWinner with one probe of the named pairs instead of
// every pair of the new Winner.
func (e *overrideEnv) rejustify(c *storedCluster) error {
	var named []uint
	for _, m := range c.Members {
		if m.ResourceID != c.Row.WinnerID && m.InExtent {
			named = append(named, m.ResourceID)
		}
	}
	closest, err := e.s.pairProbe(c.Row.WinnerID, named, e.threshold, e.aThreshold, false)
	if err != nil {
		return err
	}
	for i := range c.Members {
		m := &c.Members[i]
		if m.ResourceID == c.Row.WinnerID {
			m.Distance = nil
			continue
		}
		// An outsider may never lose, whatever its pair state to the new Winner.
		if !m.InExtent {
			continue
		}
		distance, paired := closest[m.ResourceID]
		if !paired {
			m.Ejected = true
			m.EjectedReason = models.EjectReasonNoPairToWinner
			m.Distance = nil
			continue
		}
		if m.Ejected && m.EjectedReason == models.EjectReasonNoPairToWinner {
			m.Ejected = false
			m.EjectedReason = ""
		}
		d := int(distance)
		m.Distance = &d
	}
	return nil
}
