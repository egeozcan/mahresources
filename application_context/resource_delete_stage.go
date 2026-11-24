package application_context

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"mahresources/models"
)

// The bounded Resource delete path (design D9, D13), shared by DeleteResource,
// BulkDeleteResources and the merge.
//
// Inside the deleting transaction, per ascending chunk of up to 500 Resources:
// stage their ids, clear current_version_id, delete every cascading child in
// statements of at most 500 rows, delete the parent rows, write the audit rows and
// the after-commit cleanup manifest. After the last chunk, finishResourceDeletion
// walks the affected Reduction Clusters once, scrubs the note blocks once and
// deletes the staged ids. Nothing here loads a column the path does not use, and
// no request-sized slice is built from rows beyond the ids the caller holds.

// MaxBulkDeleteResources is the most Resources one bulk delete request names.
const MaxBulkDeleteResources = 10000

// ErrTooManyResources refuses a request naming more Resources than the endpoint takes.
var ErrTooManyResources = errors.New("too many resources in one request")

// resourceDeleteColumns are the only Resource columns a delete path reads: what
// the backup path, the hooks, the audit row and the cascade need. Never meta,
// own_meta or description.
var resourceDeleteColumns = []string{
	"id", "name", "hash", "location", "storage_location", "owner_id", "series_id",
	"current_version_id", "resource_category_id",
}

// resourceCascadeChildren are the tables whose rows reference a Resource and
// cascade with it: previews, versions, the image hash, the three join tables,
// Job receipts and both sides of a similarity pair. A test derives the list from
// the database's own ON DELETE CASCADE constraints, so a new child fails until it
// is added here.
var resourceCascadeChildren = []struct{ Table, Column string }{
	{"resource_tags", "resource_id"},
	{"resource_notes", "resource_id"},
	{"groups_related_resources", "resource_id"},
	{"previews", "resource_id"},
	{"resource_versions", "resource_id"},
	{"image_hashes", "resource_id"},
	{"job_resource_receipts", "resource_id"},
	{"resource_similarities", "resource_id1"},
	{"resource_similarities", "resource_id2"},
}

// deleteChunkRows is the most rows one child-delete statement removes.
var deleteChunkRows = 500

// deleteChildChunks deletes every row of table whose column names one of ids, in
// statements of at most deleteChunkRows rows, until none is left.
func deleteChildChunks(db *gorm.DB, table, column string, ids []uint) error {
	rowRef := "rowid"
	if db.Dialector.Name() == "postgres" {
		rowRef = "ctid"
	}
	stmt := fmt.Sprintf("DELETE FROM %s WHERE %s IN (SELECT %s FROM %s WHERE %s IN ? LIMIT %d)", table, rowRef, rowRef, table, column, deleteChunkRows)
	for {
		res := db.Exec(stmt, ids)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected < int64(deleteChunkRows) {
			return nil
		}
	}
}

// deleteResourcesDBOnly deletes the given Resources, which the caller has already
// locked, validated and read with resourceDeleteColumns, inside the caller's
// transaction (the receiver is the transaction's context). Ids are staged under
// op; the caller finishes with finishResourceDeletion. With manifest set, each
// Resource's after-commit inputs are written to resource_cleanup_manifest under
// op for drainResourceDeleteManifest. logSeries says whether an auto-deleted empty
// Series is logged.
func (ctx *MahresourcesContext) deleteResourcesDBOnly(rows []models.Resource, op string, manifest, logSeries bool) error {
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	for start := 0; start < len(rows); start += deleteStageChunk {
		end := start + deleteStageChunk
		if end > len(rows) {
			end = len(rows)
		}
		if err := ctx.deleteResourceChunk(rows[start:end], op, manifest, logSeries); err != nil {
			return err
		}
	}
	return nil
}

func (ctx *MahresourcesContext) deleteResourceChunk(rows []models.Resource, op string, manifest, logSeries bool) error {
	db := ctx.db
	ids := make([]uint, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	if manifest {
		// A storage location that cannot be resolved aborts before anything is deleted,
		// as the per-Resource path always has.
		seen := map[string]bool{}
		for i := range rows {
			key := "<nil>"
			if rows[i].StorageLocation != nil {
				key = *rows[i].StorageLocation
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			if _, err := ctx.GetFsForStorageLocation(rows[i].StorageLocation); err != nil {
				return err
			}
		}
	}
	if err := stageDeletedIDs(db, op, ids); err != nil {
		return err
	}
	// Break the Resource -> version reference before the versions go.
	if err := db.Exec("UPDATE resources SET current_version_id = NULL WHERE id IN ? AND current_version_id IS NOT NULL", ids).Error; err != nil {
		return err
	}
	for _, child := range resourceCascadeChildren {
		if err := deleteChildChunks(db, child.Table, child.Column, ids); err != nil {
			return err
		}
	}
	if err := db.Where("id IN ?", ids).Delete(&models.Resource{}).Error; err != nil {
		return err
	}

	seriesSeen := map[uint]bool{}
	for i := range rows {
		r := &rows[i]
		if r.SeriesID != nil && !seriesSeen[*r.SeriesID] {
			seriesSeen[*r.SeriesID] = true
			seriesID := *r.SeriesID
			result := db.Where("id = ? AND NOT EXISTS (SELECT 1 FROM resources WHERE series_id = ?)", seriesID, seriesID).Delete(&models.Series{})
			if result.Error != nil {
				ctx.Logger().Warning(models.LogActionDelete, "series", &seriesID, "Failed to auto-delete empty series", result.Error.Error(), nil)
			} else if logSeries && result.RowsAffected > 0 {
				ctx.Logger().Info(models.LogActionDelete, "series", &seriesID, "", "Auto-deleted empty series", nil)
			}
		}
		id := r.ID
		ctx.Logger().Info(models.LogActionDelete, "resource", &id, r.Name, "Deleted resource", nil)
	}

	if !manifest {
		return nil
	}
	entries := make([]models.ResourceCleanupManifest, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		entries = append(entries, models.ResourceCleanupManifest{
			Op: op, ResourceID: r.ID, Hash: r.Hash, Location: r.GetCleanLocation(), StorageLocation: r.StorageLocation,
			Name: r.Name, OwnerID: r.OwnerId, ResourceCategoryID: r.ResourceCategoryId,
		})
	}
	return db.Create(&entries).Error
}

// finishResourceDeletion is the tail of every deleting transaction, in the order
// the design fixes: walk the affected Reduction Clusters, scrub the note blocks
// (both read the staged ids), then delete the staged ids.
func finishResourceDeletion(db *gorm.DB, op string) error {
	if err := reductionDeletionHook(db, op); err != nil {
		return err
	}
	if err := scrubDeletedResourcesFromBlocks(db, op); err != nil {
		return err
	}
	return purgeDeletedIDs(db, op)
}

// manifestChunkTimeout bounds each manifest read and each manifest delete of a drain. The
// drain is detached from the request that caused it, so something has to end it.
const manifestChunkTimeout = 2 * time.Minute

// detachedForDrain is the context a drain runs on: the request's values with none
// of its cancellation. The delete has committed, so a client that disconnects now
// must not strand the files, the manifest rows, the after-delete hooks or the cache
// invalidation, and still be told it succeeded.
func (ctx *MahresourcesContext) detachedForDrain() *MahresourcesContext {
	parent := context.Background()
	if ctx.db != nil && ctx.db.Statement != nil && ctx.db.Statement.Context != nil {
		parent = ctx.db.Statement.Context
	}
	cp := *ctx
	cp.db = ctx.db.WithContext(context.WithoutCancel(parent))
	return &cp
}

// drainResourceDeleteManifest runs the after-commit work of a committed delete
// from its manifest, 500 rows at a time: the file removal for every row of the
// chunk (backed up first, and only when no surviving row still holds the hash),
// then the after-delete hook for each, then the chunk's manifest rows are dropped.
// The search cache is invalidated once. It runs detached from the request.
func (ctx *MahresourcesContext) drainResourceDeleteManifest(op string) {
	d := ctx.detachedForDrain()
	d.drainManifestOp(op)
	d.InvalidateSearchCacheByType(EntityTypeResource)
}

// ReplayResourceCleanupManifest drains every manifest row a committed delete left
// behind, for a process that died or failed between that commit and its drain. It
// runs at startup, before the server takes requests, so no live drain races it.
func (ctx *MahresourcesContext) ReplayResourceCleanupManifest() {
	d := ctx.detachedForDrain()
	var last string
	replayed := false
	for {
		var ops []string
		if err := d.unscopedDB().Model(&models.ResourceCleanupManifest{}).Where("op > ?", last).Group("op").Order("op ASC").Limit(100).Pluck("op", &ops).Error; err != nil {
			d.Logger().Warning(models.LogActionDelete, "resource", nil, "Failed to list undrained delete manifests", err.Error(), nil)
			break
		}
		if len(ops) == 0 {
			break
		}
		for _, op := range ops {
			d.drainManifestOp(op)
			replayed = true
		}
		last = ops[len(ops)-1]
	}
	if replayed {
		d.InvalidateSearchCacheByType(EntityTypeResource)
	}
}

// drainDB is the unscoped handle with a deadline, for one manifest read or delete.
func (ctx *MahresourcesContext) drainDB() (*gorm.DB, context.CancelFunc) {
	db := ctx.unscopedDB()
	c, cancel := context.WithTimeout(db.Statement.Context, manifestChunkTimeout)
	return db.WithContext(c), cancel
}

func (ctx *MahresourcesContext) drainManifestOp(op string) {
	var last uint
	for {
		readDB, cancelRead := ctx.drainDB()
		var entries []models.ResourceCleanupManifest
		err := readDB.Where("op = ? AND id > ?", op, last).Order("id ASC").Limit(deleteStageChunk).Find(&entries).Error
		cancelRead()
		if err != nil {
			ctx.Logger().Warning(models.LogActionDelete, "resource", nil, "Failed to read the delete cleanup manifest", err.Error(), nil)
			return
		}
		if len(entries) == 0 {
			return
		}
		for i := range entries {
			e := &entries[i]
			fs, err := ctx.GetFsForStorageLocation(e.StorageLocation)
			if err != nil {
				ctx.Logger().Warning(models.LogActionDelete, "resource", &e.ResourceID, "Failed to resolve storage for cleanup", err.Error(), nil)
				continue
			}
			subFolder := "deleted"
			if e.StorageLocation != nil && *e.StorageLocation != "" {
				subFolder = *e.StorageLocation
			}
			ownerIdStr := "nil"
			if e.OwnerID != nil {
				ownerIdStr = fmt.Sprintf("%v", *e.OwnerID)
			}
			ctx.runFileCleanupActions([]*FileCleanupAction{{
				SourceFS:   fs,
				SourcePath: e.Location,
				BackupPath: path.Join(fmt.Sprintf("/deleted/%v/", subFolder), fmt.Sprintf("%v__%v__%v___%v", e.Hash, e.ResourceID, ownerIdStr, strings.ReplaceAll(path.Clean(path.Base(e.Location)), "\\", "_"))),
				Hash:       e.Hash, StorageLocation: e.StorageLocation,
			}})
		}
		// Every file of the chunk first, then the hooks: a hook observes a delete
		// that is finished, files included.
		for i := range entries {
			e := &entries[i]
			ctx.RunAfterPluginHooks("after_resource_delete", map[string]any{"id": float64(e.ResourceID), "name": e.Name, "resource_category_id": float64(e.ResourceCategoryID), "owner_id": hookID(e.OwnerID)})
		}
		last = entries[len(entries)-1].ID
		dropDB, cancelDrop := ctx.drainDB()
		if err := dropDB.Where("op = ? AND id <= ?", op, last).Delete(&models.ResourceCleanupManifest{}).Error; err != nil {
			ctx.Logger().Warning(models.LogActionDelete, "resource", nil, "Failed to drop drained cleanup manifest rows", err.Error(), nil)
		}
		cancelDrop()
	}
}

// lockResourceColumns is what the merge and bulk-delete participant locks read: the
// delete columns plus what a merge needs of a Loser's file for a version.
var lockResourceColumns = []string{
	"id", "name", "hash", "hash_type", "location", "storage_location", "file_size",
	"content_type", "width", "height", "owner_id", "series_id", "current_version_id",
	"resource_category_id",
}

// resourceRowsFromLocked is the locked rows as a slice.
func resourceRowsFromLocked(locked map[uint]*models.Resource, ids []uint) []models.Resource {
	rows := make([]models.Resource, 0, len(ids))
	seen := map[uint]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if r := locked[id]; r != nil {
			rows = append(rows, *r)
		}
	}
	return rows
}
