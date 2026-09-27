package application_context

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"mahresources/models/types"
)

// ErrReductionConflict is what a compare-and-set on the Reduction's version
// integer returns when it loses.
//
// Recompute, each override and apply are three independent read-modify-write
// writers on one JSON document, so the last-writer-wins merge a plain UPDATE
// would perform is exactly wrong here: it silently discards a decision about
// which files to delete. A stale write is refused and the caller re-reads.
var ErrReductionConflict = errors.New("this Resource Reduction changed since you loaded it; reload and try again")

// ErrReductionNotFound is returned for an id the caller may not see as well as
// for one that does not exist. The two are deliberately indistinguishable: the
// row ids of other people's pending destructive decisions are not probeable.
var ErrReductionNotFound = errors.New("no such Resource Reduction")

// ErrReductionBusy refuses a write while a clustering job is in flight. The job
// is going to overwrite the plan when it lands, so an override taken now is a
// decision about a document that is about to stop existing.
var ErrReductionBusy = errors.New("this Resource Reduction is still computing")

// maxReductionName bounds the name column. Truncated rather than rejected, in
// line with the download history's treatment of an over-long URL.
const maxReductionName = 1000

// reductionQueryFor builds the visibility predicate. Every read and every
// mutation goes through it.
func reductionQueryFor(ownerUserID *uint, ownerRestricted bool) *query_models.ResourceReductionQuery {
	return &query_models.ResourceReductionQuery{OwnerUserID: ownerUserID, OwnerRestricted: ownerRestricted}
}

// CreateOrExtendResourceReduction creates a Resource Reduction from a selection,
// or widens the Extent of one that already exists.
//
// One entry point for both because the bulk bar offers them as one gesture with
// two arms, and because widening has to reuse the create path's validation of
// the selection. Widening leaves the plan alone: D23 makes re-scanning an
// explicit act, so the page reports the drift instead of acting on it.
func (ctx *MahresourcesContext) CreateOrExtendResourceReduction(creator *query_models.ResourceReductionCreator, ownerUserID *uint, ownerRestricted bool) (*models.ResourceReduction, error) {
	if creator == nil {
		return nil, errors.New("no Resource Reduction given")
	}
	return retryReductionWrite(func() (*models.ResourceReduction, error) {
		return ctx.createOrExtendResourceReductionOnce(creator, ownerUserID, ownerRestricted)
	})
}

// createOrExtendResourceReductionOnce is one attempt of
// CreateOrExtendResourceReduction: every read first, then the one write.
func (ctx *MahresourcesContext) createOrExtendResourceReductionOnce(creator *query_models.ResourceReductionCreator, ownerUserID *uint, ownerRestricted bool) (*models.ResourceReduction, error) {
	resourceIDs := dedupeUints(creator.ResourceIds)
	groupIDs := dedupeUints(creator.GroupIds)
	if creator.OwnerId != 0 {
		owned, err := ctx.resourcesOwnedWithin(creator.OwnerId, creator.IncludeDescendants)
		if err != nil {
			return nil, err
		}
		if len(owned) == 0 {
			return nil, errors.New("no owned Resources found in the selected group scope")
		}
		resourceIDs = dedupeUints(append(resourceIDs, owned...))
	}
	if creator.ID == 0 && len(resourceIDs) == 0 && len(groupIDs) == 0 {
		return nil, errors.New("a Resource Reduction needs at least one Resource or Group")
	}
	if creator.ID != 0 {
		existing, err := ctx.loadReductionForUpdate(creator.ID, ownerUserID, ownerRestricted)
		if err != nil {
			return nil, err
		}
		extent, err := DecodeReductionExtent(existing.Extent)
		if err != nil {
			return nil, err
		}
		extent.ResourceIDs = dedupeUints(append(extent.ResourceIDs, resourceIDs...))
		extent.GroupIDs = dedupeUints(append(extent.GroupIDs, groupIDs...))
		encoded, err := encodeJSON(extent)
		if err != nil {
			return nil, err
		}
		// The version is bumped even though only the Extent moved: the page
		// shows "how much has entered the Extent since the last compute", and
		// a concurrent recompute reading a narrower Extent would report that
		// figure against a plan it no longer describes.
		return ctx.casReductionAndReload(existing.ID, existing.Version, map[string]any{"extent": encoded}, ownerUserID, ownerRestricted)
	}

	name := strings.TrimSpace(creator.Name)
	if name == "" {
		name = "Resource Reduction"
	}
	extentJSON, err := encodeJSON(models.ResourceReductionExtent{ResourceIDs: resourceIDs, GroupIDs: groupIDs})
	if err != nil {
		return nil, err
	}
	ruleJSON, err := encodeJSON(models.NormalizeWinnerRule(creator.WinnerRule))
	if err != nil {
		return nil, err
	}
	row := &models.ResourceReduction{
		Name:                     truncateRunes(name, maxReductionName),
		Status:                   models.ReductionStatusDraft,
		MatchingMode:             normalizeMatchingMode(creator.MatchingMode),
		ExcludeExternalResources: creator.ExcludeExternalResources,
		KeepAsVersionIdentical:   boolOr(creator.KeepAsVersionIdentical, false),
		KeepAsVersionNear:        boolOr(creator.KeepAsVersionNear, true),
		WinnerRule:               ruleJSON,
		Extent:                   extentJSON,
	}
	if err := ctx.db.Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

// resourcesOwnedWithin lists the Resources owned by a group, or by it and every
// group below it.
//
// The owners are queried in chunks, so the reads share one snapshot: otherwise each
// chunk sees whatever committed since the last, and a Resource moved from a group
// in a later chunk into one in an earlier chunk, after that chunk was read, is in
// the subtree throughout and in neither answer. The snapshot's transaction only
// reads, so on SQLite it never has a write to promote.
func (ctx *MahresourcesContext) resourcesOwnedWithin(ownerID uint, includeDescendants bool) ([]uint, error) {
	var owned []uint
	err := ctx.withReadSnapshot(func(snapCtx *MahresourcesContext) error {
		// Check the root before traversing: subtree collection uses raw SQL,
		// while the Resource query below retains the caller's scope filter.
		if !snapCtx.GroupVisible(ownerID) {
			return errors.New("no such Group")
		}
		owners := []uint{ownerID}
		if includeDescendants {
			var err error
			owners, err = snapCtx.collectSubtreeGroupIDs(ownerID)
			if err != nil {
				return err
			}
		}
		for _, chunk := range chunkUints(owners, idChunk) {
			var ids []uint
			if err := snapCtx.db.Model(&models.Resource{}).
				Where("resources.owner_id IN ?", chunk).
				Pluck("resources.id", &ids).Error; err != nil {
				return fmt.Errorf("selecting owned Resources: %w", err)
			}
			owned = append(owned, ids...)
		}
		return nil
	})
	return owned, err
}

// withReadSnapshot runs reads that must agree with one another against one
// snapshot, in a transaction that never writes. On SQLite a read-only transaction
// gives that on its own (a WAL reader keeps its snapshot until it ends, and it takes
// no writer lock); Postgres needs REPEATABLE READ, because its default gives every
// statement a fresh snapshot.
func (ctx *MahresourcesContext) withReadSnapshot(read func(snapCtx *MahresourcesContext) error) error {
	options := models.ReadOnlyTxOptions(ctx.db)
	if ctx.db.Dialector.Name() == "postgres" {
		options = append(options, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	}
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		snapCtx := *ctx
		snapCtx.db = tx
		return read(&snapCtx)
	}, options...)
}

// UpdateResourceReductionSettings changes a Reduction's own settings. Guarded by
// the version so it cannot land on top of a recompute or an apply.
func (ctx *MahresourcesContext) UpdateResourceReductionSettings(editor *query_models.ResourceReductionEditor, ownerUserID *uint, ownerRestricted bool) (*models.ResourceReduction, error) {
	if editor == nil || editor.ID == 0 {
		return nil, errors.New("no Resource Reduction given")
	}

	updates := map[string]any{}
	if name := strings.TrimSpace(editor.Name); name != "" {
		updates["name"] = truncateRunes(name, maxReductionName)
	}
	if editor.MatchingMode != "" {
		updates["matching_mode"] = normalizeMatchingMode(editor.MatchingMode)
	}
	if len(editor.WinnerRule) > 0 {
		ruleJSON, err := encodeJSON(models.NormalizeWinnerRule(editor.WinnerRule))
		if err != nil {
			return nil, err
		}
		updates["winner_rule"] = ruleJSON
	}
	if editor.KeepAsVersionIdentical != nil {
		updates["keep_as_version_identical"] = *editor.KeepAsVersionIdentical
	}
	if editor.KeepAsVersionNear != nil {
		updates["keep_as_version_near"] = *editor.KeepAsVersionNear
	}

	return retryReductionWrite(func() (*models.ResourceReduction, error) {
		// Read first even with updates to make: it is the visibility check, and a
		// row the caller may not see has to answer as absent rather than as a
		// version conflict.
		existing, err := ctx.loadReductionForUpdate(editor.ID, ownerUserID, ownerRestricted)
		if err != nil || len(updates) == 0 {
			return existing, err
		}
		return ctx.casReductionAndReload(existing.ID, editor.Version, updates, ownerUserID, ownerRestricted)
	})
}

// GetResourceReductions lists the Reductions the caller may see.
func (ctx *MahresourcesContext) GetResourceReductions(offset, limit int, query *query_models.ResourceReductionQuery) ([]models.ResourceReduction, error) {
	var rows []models.ResourceReduction
	return rows, ctx.db.
		Scopes(database_scopes.ResourceReductionQuery(query, false)).
		Offset(offset).
		Limit(limit).
		Find(&rows).Error
}

// GetResourceReductionCount counts them under the same predicate.
func (ctx *MahresourcesContext) GetResourceReductionCount(query *query_models.ResourceReductionQuery) (int64, error) {
	var count int64
	return count, ctx.db.
		Model(&models.ResourceReduction{}).
		Scopes(database_scopes.ResourceReductionQuery(query, true)).
		Count(&count).Error
}

// GetResourceReduction loads one, or reports it as absent when the caller may not
// see it.
func (ctx *MahresourcesContext) GetResourceReduction(id uint, ownerUserID *uint, ownerRestricted bool) (*models.ResourceReduction, error) {
	return ctx.loadReductionForUpdate(id, ownerUserID, ownerRestricted)
}

// DeleteResourceReduction removes one. A Reduction never expires, so this is the
// only way one goes.
func (ctx *MahresourcesContext) DeleteResourceReduction(id uint, ownerUserID *uint, ownerRestricted bool) error {
	res := ctx.db.
		Scopes(database_scopes.ResourceReductionQuery(reductionQueryFor(ownerUserID, ownerRestricted), true)).
		Where("id = ?", id).
		Delete(&models.ResourceReduction{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrReductionNotFound
	}
	return nil
}

// loadReductionForUpdate reads a row under the visibility predicate.
func (ctx *MahresourcesContext) loadReductionForUpdate(id uint, ownerUserID *uint, ownerRestricted bool) (*models.ResourceReduction, error) {
	var row models.ResourceReduction
	err := ctx.db.
		Scopes(database_scopes.ResourceReductionQuery(reductionQueryFor(ownerUserID, ownerRestricted), true)).
		Where("id = ?", id).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrReductionNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// casReduction is the compare-and-set every writer goes through.
//
// One conditional UPDATE whose RowsAffected is the answer, the shape
// ClaimDownloadHistoryRetry already uses. The version is incremented in the same
// statement, so two writers holding the same version cannot both win.
func (ctx *MahresourcesContext) casReduction(id, expectedVersion uint, updates map[string]any) (bool, error) {
	assignments := make(map[string]any, len(updates)+2)
	for k, v := range updates {
		assignments[k] = v
	}
	assignments["version"] = gorm.Expr("version + 1")
	assignments["updated_at"] = time.Now()

	res := ctx.db.Model(&models.ResourceReduction{}).
		Where("id = ? AND version = ?", id, expectedVersion).
		Updates(assignments)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// casReductionAndReload lands one compare-and-set and reads the row back in the
// same transaction, so the caller is answered with the state its own write made.
//
// The compare-and-set is the transaction's first statement, and that is the point
// of this helper; see retryReductionWrite for why. Reads after it are safe: the
// transaction already holds the writer lock.
func (ctx *MahresourcesContext) casReductionAndReload(id, expectedVersion uint, updates map[string]any, ownerUserID *uint, ownerRestricted bool) (*models.ResourceReduction, error) {
	var reduction *models.ResourceReduction
	err := ctx.WithTransaction(func(txCtx *MahresourcesContext) error {
		ok, err := txCtx.casReduction(id, expectedVersion, updates)
		if err != nil {
			return err
		}
		if !ok {
			return ErrReductionConflict
		}
		reduction, err = txCtx.loadReductionForUpdate(id, ownerUserID, ownerRestricted)
		return err
	})
	if err != nil {
		return nil, err
	}
	return reduction, nil
}

// retryReductionWrite runs one request-side Reduction writer, from its first read,
// again while it loses to lock contention.
//
// Each writer reads everything it needs outside any transaction and then writes
// through casReductionAndReload or a single INSERT, so the write is the first
// statement its transaction sends. On SQLite in WAL mode that order is what keeps
// busy_timeout in play: a deferred transaction that reads first holds a snapshot,
// and a commit landing before its write (the compute Job's bookkeeping, a hash or
// thumbnail worker, a job migration pass) makes promoting that snapshot fail at
// once with SQLITE_BUSY_SNAPSHOT, which SQLite never hands to the busy handler.
// Re-reading and trying again does not escape that under sustained writes, because
// every attempt reads first. The reads lose nothing by leaving the transaction: the
// version compare-and-set refuses a write whose reads went stale, and on Postgres
// READ COMMITTED gave them no shared snapshot to begin with.
//
// The retry covers what the order does not remove: the small residue AddResource's
// write-first transaction still sees, and a shared-cache table lock on one of the
// reads. A rolled-back attempt left nothing behind, so starting over is safe.
func retryReductionWrite(attempt func() (*models.ResourceReduction, error)) (*models.ResourceReduction, error) {
	var reduction *models.ResourceReduction
	err := retryOnLockContention(reductionCASRetries, func() (err error) {
		reduction, err = attempt()
		return err
	})
	if err != nil {
		return nil, err
	}
	return reduction, nil
}

// EffectiveReductionStatus reports what a Reduction's status actually reads as.
//
// A row still `computing` past its deadline is a failed one. Generic queue jobs
// are not drained at shutdown — workers.Add exists only on the download path — so
// a restart mid-clustering leaves the row saying `computing` with nothing left
// alive to move it off. On a table that never expires that is a Reduction
// stranded forever, and the deadline rather than the queue is what prevents it.
// Deriving the answer at read time rather than sweeping for it means a Reduction
// nobody opens costs nothing.
func EffectiveReductionStatus(r *models.ResourceReduction) string {
	if r == nil {
		return ""
	}
	if r.Status == models.ReductionStatusComputing && r.ComputeDeadline != nil && time.Now().After(*r.ComputeDeadline) {
		return models.ReductionStatusFailed
	}
	return r.Status
}

// DecodeReductionExtent reads the stored Extent, treating an absent or null
// document as an empty one.
func DecodeReductionExtent(raw types.JSON) (models.ResourceReductionExtent, error) {
	var extent models.ResourceReductionExtent
	if len(raw) == 0 || string(raw) == "null" {
		return extent, nil
	}
	if err := json.Unmarshal(raw, &extent); err != nil {
		return extent, fmt.Errorf("could not read the Extent: %w", err)
	}
	return extent, nil
}

// DecodeReductionPlan reads the stored plan, treating an absent or null document
// as an empty one — which is what a Reduction that has never been computed holds.
func DecodeReductionPlan(raw types.JSON) (models.ResourceReductionPlan, error) {
	var plan models.ResourceReductionPlan
	if len(raw) == 0 || string(raw) == "null" {
		return plan, nil
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		return plan, fmt.Errorf("could not read the plan: %w", err)
	}
	return plan, nil
}

// DecodeWinnerRule reads the stored rule, falling back to the default.
func DecodeWinnerRule(raw types.JSON) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return models.DefaultWinnerRule()
	}
	var rule []string
	if err := json.Unmarshal(raw, &rule); err != nil {
		return models.DefaultWinnerRule()
	}
	return models.NormalizeWinnerRule(rule)
}

func encodeJSON(v any) (types.JSON, error) {
	encoded, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return types.JSON(encoded), nil
}

func normalizeMatchingMode(mode string) string {
	if mode == models.MatchingModeIdenticalOnly {
		return models.MatchingModeIdenticalOnly
	}
	return models.MatchingModeBothTiers
}

func boolOr(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

func dedupeUints(in []uint) []uint {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[uint]bool, len(in))
	out := make([]uint, 0, len(in))
	for _, v := range in {
		if v == 0 || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
