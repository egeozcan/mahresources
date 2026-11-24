package application_context

import (
	"database/sql"
	"encoding/json"
	"errors"
	"mahresources/contracts"
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
//
// The selection lives in rows (design D5): explicit Resources and Groups in their
// own tables, the whole-library flag on the Reduction. One create or widen names at
// most reductionSelectionCap ids, and a Group's own Resources are inserted by one
// scoped INSERT ... SELECT per chunk, never held in Go. The whole request is one
// transaction, so a refused selection leaves nothing behind.
func (ctx *MahresourcesContext) CreateOrExtendResourceReduction(creator *query_models.ResourceReductionCreator, ownerUserID *uint, ownerRestricted bool) (*models.ResourceReduction, error) {
	if creator == nil {
		return nil, errors.New("no Resource Reduction given")
	}
	return retryReductionWrite(func() (*models.ResourceReduction, error) {
		return ctx.createOrExtendResourceReductionOnce(creator, ownerUserID, ownerRestricted)
	})
}

// createOrExtendResourceReductionOnce is one attempt of
// CreateOrExtendResourceReduction.
func (ctx *MahresourcesContext) createOrExtendResourceReductionOnce(creator *query_models.ResourceReductionCreator, ownerUserID *uint, ownerRestricted bool) (*models.ResourceReduction, error) {
	resourceIDs := dedupeUints(creator.ResourceIds)
	groupIDs := dedupeUints(creator.GroupIds)
	if len(resourceIDs) > reductionSelectionCap || len(groupIDs) > reductionSelectionCap {
		return nil, ErrReductionSelectionTooLarge
	}
	if creator.ID == 0 && len(resourceIDs) == 0 && len(groupIDs) == 0 && !creator.All && creator.OwnerId == 0 {
		return nil, errors.New("a Resource Reduction needs at least one Resource or Group")
	}

	var existing *models.ResourceReduction
	if creator.ID != 0 {
		var err error
		if existing, err = ctx.loadReductionForUpdate(creator.ID, ownerUserID, ownerRestricted); err != nil {
			return nil, err
		}
		if err := reductionUpgrading(existing); err != nil {
			return nil, err
		}
		if err := reductionEditable(existing); err != nil {
			return nil, err
		}
	}

	var out *models.ResourceReduction
	err := ctx.withSelectionTransaction(func(txCtx *MahresourcesContext) error {
		row := existing
		if row == nil {
			name := strings.TrimSpace(creator.Name)
			if name == "" {
				name = "Resource Reduction"
			}
			ruleJSON, err := encodeJSON(models.NormalizeWinnerRule(creator.WinnerRule))
			if err != nil {
				return err
			}
			row = &models.ResourceReduction{
				Name:                     truncateRunes(name, maxReductionName),
				Status:                   models.ReductionStatusDraft,
				MatchingMode:             normalizeMatchingMode(creator.MatchingMode),
				ExcludeExternalResources: creator.ExcludeExternalResources,
				KeepAsVersionIdentical:   boolOr(creator.KeepAsVersionIdentical, false),
				KeepAsVersionNear:        boolOr(creator.KeepAsVersionNear, true),
				WinnerRule:               ruleJSON,
				PlanSchema:               1,
				ExtentAll:                creator.All,
			}
			if err := txCtx.db.Create(row).Error; err != nil {
				return err
			}
		} else {
			// The version is bumped even though only the Extent moved: the page
			// shows "how much has entered the Extent since the last compute", and
			// a concurrent recompute reading a narrower Extent would report that
			// figure against a plan it no longer describes.
			ok, err := casReductionTx(txCtx.db, row)
			if err != nil {
				return err
			}
			if !ok {
				return ErrReductionConflict
			}
			if creator.All && !row.ExtentAll {
				if err := txCtx.db.Exec(`UPDATE resource_reductions SET extent_all = ? WHERE id = ?`, true, row.ID).Error; err != nil {
					return err
				}
			}
		}

		store := txCtx.reductionStore()
		if err := store.addExtentResources(row, resourceIDs); err != nil {
			return err
		}
		if err := store.addExtentGroups(row, groupIDs); err != nil {
			return err
		}
		if creator.OwnerId != 0 {
			added, err := txCtx.addOwnedResourcesToExtent(store, row, creator.OwnerId, creator.IncludeDescendants)
			if err != nil {
				return err
			}
			if added == 0 {
				return errors.New("no owned Resources found in the selected group scope")
			}
		}
		var err error
		out, err = reloadReduction(txCtx.db, row.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// withSelectionTransaction is WithTransaction with the isolation a selection needs:
// the Groups of a subtree are read one statement apiece, and under Postgres' default
// READ COMMITTED each would see whatever committed since the last, so a Resource moved
// from a later Group into an earlier one after the earlier was read is in the subtree
// throughout and in no statement's answer. REPEATABLE READ gives them one snapshot.
// SQLite's writer lock already does.
func (ctx *MahresourcesContext) withSelectionTransaction(fn func(txCtx *MahresourcesContext) error) error {
	var options []*sql.TxOptions
	if ctx.db.Dialector.Name() == "postgres" {
		options = append(options, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	}
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		txCtx := *ctx
		txCtx.db = tx
		return fn(&txCtx)
	}, options...)
}

// addOwnedResourcesToExtent selects the Resources a Group owns, or that it and
// every Group below it own, and reports how many were selected (new or already
// there). The root is checked before the subtree is walked: collecting the subtree
// is raw SQL, while the Resource insert applies the principal's scope itself. The
// request as a whole stays within reductionSelectionCap.
func (ctx *MahresourcesContext) addOwnedResourcesToExtent(store *reductionStore, row *models.ResourceReduction, ownerID uint, includeDescendants bool) (int, error) {
	if !ctx.GroupVisible(ownerID) {
		return 0, ErrReductionGroupNotVisible
	}
	owners := []uint{ownerID}
	if includeDescendants {
		var err error
		if owners, err = ctx.collectSubtreeGroupIDs(ownerID); err != nil {
			return 0, err
		}
	}
	total := 0
	for _, owner := range owners {
		n, err := store.addExtentFromGroupResources(row, owner)
		if err != nil {
			return 0, err
		}
		if total += n; total > reductionSelectionCap {
			return 0, ErrReductionSelectionTooLarge
		}
	}
	return total, nil
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
// the version so it cannot land on top of a recompute or an apply, and refused
// while a compute is in flight or the plan is still the legacy document: the
// settings are what the compute reads and what the review compares the plan to.
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
		if err := reductionUpgrading(existing); err != nil {
			return nil, err
		}
		if err := reductionEditable(existing); err != nil {
			return nil, err
		}
		if editor.Version != existing.Version {
			return nil, ErrReductionConflict
		}
		var out *models.ResourceReduction
		err = ctx.WithTransaction(func(txCtx *MahresourcesContext) error {
			ok, err := casReductionTx(txCtx.db, existing)
			if err != nil {
				return err
			}
			if !ok {
				return ErrReductionConflict
			}
			if err := txCtx.db.Model(&models.ResourceReduction{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
				return err
			}
			out, err = reloadReduction(txCtx.db, existing.ID)
			return err
		})
		return out, err
	})
}

// GetResourceReductions lists the Reductions the caller may see.
func (ctx *MahresourcesContext) GetResourceReductions(offset, limit int, query *query_models.ResourceReductionQuery) ([]models.ResourceReduction, error) {
	var rows []models.ResourceReduction
	return rows, ctx.db.Select(reductionColumns).
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

// ListResourceReductions is one list page, its count and each row's progress from
// one snapshot (design D8). A Reduction still holding the legacy plan comes back
// with Upgrading set and no counts.
func (ctx *MahresourcesContext) ListResourceReductions(offset, limit int, query *query_models.ResourceReductionQuery) ([]models.ResourceReduction, int64, map[uint]contracts.ReductionProgress, error) {
	rows, count, progress, err := ctx.storeListReductions(offset, limit, query)
	if err != nil {
		return nil, 0, nil, err
	}
	return rows, count, progressOf(progress), nil
}

// GetReductionProgress counts the Clusters of the given Reductions by state.
func (ctx *MahresourcesContext) GetReductionProgress(reductions []models.ResourceReduction) (map[uint]contracts.ReductionProgress, error) {
	progress, err := ctx.storeGetReductionProgress(reductions)
	if err != nil {
		return nil, err
	}
	return progressOf(progress), nil
}

// progressOf flattens the store's list progress into the contract's map, marking
// the Reductions the migration has not reached.
func progressOf(p *reductionListProgress) map[uint]contracts.ReductionProgress {
	out := make(map[uint]contracts.ReductionProgress, len(p.Progress)+len(p.Upgrading))
	for id, progress := range p.Progress {
		out[id] = progress
	}
	for id := range p.Upgrading {
		out[id] = contracts.ReductionProgress{Upgrading: true}
	}
	return out
}

// GetResourceReduction loads one, or reports it as absent when the caller may not
// see it.
func (ctx *MahresourcesContext) GetResourceReduction(id uint, ownerUserID *uint, ownerRestricted bool) (*models.ResourceReduction, error) {
	return ctx.loadReductionForUpdate(id, ownerUserID, ownerRestricted)
}

// DeleteResourceReduction removes one. A Reduction never expires, so this is the
// only way one goes. Its row goes in one small transaction and its Clusters,
// members and selections after it, in chunks.
func (ctx *MahresourcesContext) DeleteResourceReduction(id uint, ownerUserID *uint, ownerRestricted bool) error {
	reduction, err := ctx.loadReductionForUpdate(id, ownerUserID, ownerRestricted)
	if err != nil {
		return err
	}
	return ctx.storeDeleteReduction(reduction)
}

// loadReductionForUpdate reads a row under the visibility predicate with the
// ordinary projection: the legacy plan and extent documents never cross the wire.
func (ctx *MahresourcesContext) loadReductionForUpdate(id uint, ownerUserID *uint, ownerRestricted bool) (*models.ResourceReduction, error) {
	var row models.ResourceReduction
	err := ctx.db.Select(reductionColumns).
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
// A row still computing past its deadline reads as failed. Generic queue jobs
// take the shared five-second shutdown drain, but a process crash or a worker
// that outlives the bounded drain can still leave persisted state without a
// completed compute. On a table that never expires, the deadline makes that
// Reduction recomputable. Deriving the answer at read time rather than sweeping
// for it means a Reduction nobody opens costs nothing.
func EffectiveReductionStatus(r *models.ResourceReduction) string {
	if r == nil {
		return ""
	}
	if r.Status == models.ReductionStatusComputing && r.ComputeDeadline != nil && time.Now().After(*r.ComputeDeadline) {
		return models.ReductionStatusFailed
	}
	return r.Status
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
