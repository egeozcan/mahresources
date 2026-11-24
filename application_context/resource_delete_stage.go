package application_context

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"mahresources/auth"
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
	// The account that deletes, for the replay to run the hooks as (0: no actor).
	actor := ctx.hookInvocation().ActorUserID
	entries := make([]models.ResourceCleanupManifest, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		entries = append(entries, models.ResourceCleanupManifest{
			Op: op, ResourceID: r.ID, Hash: r.Hash, Location: r.GetCleanLocation(), StorageLocation: r.StorageLocation,
			Name: r.Name, OwnerID: r.OwnerId, ResourceCategoryID: r.ResourceCategoryId, ActorUserID: actor,
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

// manifestAfterHookGap is a test seam called after a row's after-delete hook ran and
// before its manifest row is dropped. Production leaves it nil.
var manifestAfterHookGap func(resourceID uint)

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
// from its manifest: every file of the operation first (backed up, and removed only
// when no surviving row still holds the hash), then every after-delete hook, 500 rows
// at a time in each pass, dropping a row once its hook has run. The search cache is
// invalidated once. It runs detached from the request, as the request's principal.
func (ctx *MahresourcesContext) drainResourceDeleteManifest(op string) {
	d := ctx.detachedForDrain()
	d.drainManifestOp(op, false)
	d.InvalidateSearchCacheByType(EntityTypeResource)
}

// ReplayResourceCleanupManifest drains every manifest row a committed delete left
// behind, for a process that died or failed between that commit and its drain. It
// runs at startup, before the server takes requests, but a second instance may replay
// the same manifest, so every row is claimed before anything is done to it. A hook
// runs as the account that made the delete; when that account cannot be bound the
// hook is skipped (and the reason logged), never run unscoped.
func (ctx *MahresourcesContext) ReplayResourceCleanupManifest() {
	d := ctx.detachedForDrain()
	var last string
	replayed := false
	for {
		now := time.Now().Unix()
		var ops []string
		db, cancel := d.drainDB()
		err := db.Model(&models.ResourceCleanupManifest{}).
			Where("op > ? AND (claimed_by = '' OR claimed_until < ?) AND next_attempt_at <= ?", last, now, now).
			Group("op").Order("op ASC").Limit(100).Pluck("op", &ops).Error
		cancel()
		if err != nil {
			d.Logger().Warning(models.LogActionDelete, "resource", nil, "Failed to list undrained delete manifests", err.Error(), nil)
			break
		}
		if len(ops) == 0 {
			break
		}
		for _, op := range ops {
			d.drainManifestOp(op, true)
			replayed = true
		}
		last = ops[len(ops)-1]
	}
	if replayed {
		d.InvalidateSearchCacheByType(EntityTypeResource)
	}
}

// drainDB is the unscoped handle with a deadline, for one manifest read or write.
func (ctx *MahresourcesContext) drainDB() (*gorm.DB, context.CancelFunc) {
	db := ctx.unscopedDB()
	c, cancel := context.WithTimeout(db.Statement.Context, manifestChunkTimeout)
	return db.WithContext(c), cancel
}

// manifestClaimTTL is how long a claim holds. It is renewed before every chunk and just
// before each row's own work (the file step, then the hook), so it
// only has to outlast one row; a process that dies holding claims blocks its rows for
// at most this long.
var manifestClaimTTL = 5 * time.Minute

// manifestMaxFileAttempts is how many failed file steps a row survives. At the last one
// the file is left where it is and the row goes on to its hook, so a file that can never
// be removed (storage gone, a permanent permission error) does not hold the hook back
// for ever.
const manifestMaxFileAttempts = 5

// manifestBackoff is the wait before a failed file step is tried again.
func manifestBackoff(attempts int) time.Duration {
	if attempts > 7 {
		attempts = 7
	}
	d := 30 * time.Second << attempts
	if d > time.Hour {
		d = time.Hour
	}
	return d
}

// claimManifestChunk claims up to one chunk of op's rows past last that are unclaimed
// (or whose claim expired) and due, in ascending id order, and reads back the rows this
// drain now holds in that range. Postgres skips rows another transaction is claiming.
func (ctx *MahresourcesContext) claimManifestChunk(op, me string, last uint) ([]models.ResourceCleanupManifest, error) {
	now := time.Now()
	lock := ""
	if ctx.unscopedDB().Dialector.Name() == "postgres" {
		lock = " FOR UPDATE SKIP LOCKED"
	}
	db, cancel := ctx.drainDB()
	defer cancel()
	res := db.Exec(
		"UPDATE resource_cleanup_manifest SET claimed_by = ?, claimed_until = ? WHERE id IN ("+
			"SELECT id FROM resource_cleanup_manifest WHERE op = ? AND id > ? AND (claimed_by = '' OR claimed_until < ?) AND next_attempt_at <= ? ORDER BY id LIMIT ?"+lock+")",
		me, now.Add(manifestClaimTTL).Unix(), op, last, now.Unix(), now.Unix(), deleteStageChunk)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}
	var entries []models.ResourceCleanupManifest
	err := db.Where("op = ? AND claimed_by = ? AND id > ?", op, me, last).Order("id ASC").Limit(deleteStageChunk).Find(&entries).Error
	return entries, err
}

// drainManifestOp does op's after-commit work in two passes over its manifest: all
// files, then all hooks. A row is claimed before anything is done to it and dropped
// right after its own hook; a file step that fails releases the row with a backoff and
// an attempt count (kept for a later replay, its hook not yet run), and nothing already
// done is done again.
//
// Every write a drain makes is conditional on the claim being this drain's and still
// live, read from a fresh clock (claimed_by = me AND claimed_until > now). A drain that
// stalled past the TTL while another instance took its rows therefore cannot clear the
// successor's claim, mark its file step done, or delete its row; the row it lost is
// the successor's, and at worst its hook runs a second time (at-least-once).
func (ctx *MahresourcesContext) drainManifestOp(op string, replay bool) {
	me := newDeleteOp()
	warn := func(msg string, err error) {
		ctx.Logger().Warning(models.LogActionDelete, "resource", nil, msg, err.Error(), nil)
	}
	const owned = "claimed_by = ? AND claimed_until > ?"
	renew := func() {
		db, cancel := ctx.drainDB()
		defer cancel()
		// A chunk of rows per statement, by id keyset: a drain may hold every row of
		// a 10,000 id delete, and one statement over all of them would be the longest
		// write of the operation. Only rows still held live are renewed.
		now := time.Now()
		until := now.Add(manifestClaimTTL).Unix()
		var after uint
		for {
			var held []uint
			if rerr := db.Model(&models.ResourceCleanupManifest{}).Where("op = ? AND "+owned+" AND id > ?", op, me, now.Unix(), after).
				Order("id ASC").Limit(deleteStageChunk).Pluck("id", &held).Error; rerr != nil {
				warn("Failed to renew the delete manifest claim", rerr)
				return
			}
			if len(held) == 0 {
				return
			}
			if rerr := db.Exec("UPDATE resource_cleanup_manifest SET claimed_until = ? WHERE id IN ? AND "+owned, until, held, me, now.Unix()).Error; rerr != nil {
				warn("Failed to renew the delete manifest claim", rerr)
				return
			}
			after = held[len(held)-1]
		}
	}
	// hold extends this drain's claim on one row just before it is worked on, and says
	// whether the row is still ours and live. A row whose claim lapsed is no longer
	// ours, whether or not another instance has taken it yet: it is neither worked on
	// nor dropped, and the next replay picks it up.
	hold := func(id uint) bool {
		db, cancel := ctx.drainDB()
		defer cancel()
		now := time.Now()
		res := db.Exec("UPDATE resource_cleanup_manifest SET claimed_until = ? WHERE id = ? AND "+owned,
			now.Add(manifestClaimTTL).Unix(), id, me, now.Unix())
		if res.Error != nil {
			warn("Failed to renew a delete manifest row claim", res.Error)
			return false
		}
		return res.RowsAffected == 1
	}
	// Whatever this drain still holds when it stops is let go at once, so a failed read
	// or a failed drop does not park rows behind a claim nobody is working under.
	defer func() {
		db, cancel := ctx.drainDB()
		defer cancel()
		if rerr := db.Exec("UPDATE resource_cleanup_manifest SET claimed_by = '', claimed_until = 0 WHERE op = ? AND "+owned, op, me, time.Now().Unix()).Error; rerr != nil {
			warn("Failed to release the delete manifest claim", rerr)
		}
	}()

	var last uint
	for {
		renew()
		entries, err := ctx.claimManifestChunk(op, me, last)
		if err != nil {
			warn("Failed to claim the delete cleanup manifest", err)
			return
		}
		if len(entries) == 0 {
			break
		}
		last = entries[len(entries)-1].ID
		for i := range entries {
			e := &entries[i]
			if e.FilesDone {
				continue
			}
			if !hold(e.ID) {
				continue
			}
			if ferr := ctx.cleanupManifestFile(e); ferr != nil {
				if e.Attempts+1 >= manifestMaxFileAttempts {
					// This file step has failed often enough that it is not going to
					// succeed: leave the file, and let the hook (which must not be held
					// back for ever by a file nobody can remove) run.
					ctx.Logger().Warning(models.LogActionDelete, "resource", &e.ResourceID, "File cleanup failed too many times; leaving the file and running the delete hook", ferr.Error(), nil)
				} else {
					ctx.Logger().Warning(models.LogActionDelete, "resource", &e.ResourceID, "File cleanup failed; keeping the manifest row to retry", ferr.Error(), nil)
					db, cancel := ctx.drainDB()
					now := time.Now()
					uerr := db.Exec("UPDATE resource_cleanup_manifest SET claimed_by = '', claimed_until = 0, attempts = attempts + 1, next_attempt_at = ? WHERE id = ? AND "+owned,
						now.Add(manifestBackoff(e.Attempts)).Unix(), e.ID, me, now.Unix()).Error
					cancel()
					if uerr != nil {
						warn("Failed to record a failed file cleanup", uerr)
					}
					continue
				}
			}
			db, cancel := ctx.drainDB()
			uerr := db.Exec("UPDATE resource_cleanup_manifest SET files_done = ? WHERE id = ? AND "+owned, true, e.ID, me, time.Now().Unix()).Error
			cancel()
			if uerr != nil {
				warn("Failed to record finished file cleanup", uerr)
			}
		}
	}

	hookCtxs := map[uint]*MahresourcesContext{}
	last = 0
	for {
		renew()
		readDB, cancelRead := ctx.drainDB()
		var entries []models.ResourceCleanupManifest
		err := readDB.Where("op = ? AND claimed_by = ? AND files_done = ? AND id > ?", op, me, true, last).Order("id ASC").Limit(deleteStageChunk).Find(&entries).Error
		cancelRead()
		if err != nil {
			warn("Failed to read the delete cleanup manifest", err)
			return
		}
		if len(entries) == 0 {
			return
		}
		for i := range entries {
			e := &entries[i]
			if !hold(e.ID) {
				continue // lost to another drain, or lapsed: its hook and its row are not ours
			}
			if hc := ctx.manifestHookContext(e, replay, hookCtxs); hc != nil {
				hc.RunAfterPluginHooks("after_resource_delete", map[string]any{"id": float64(e.ResourceID), "name": e.Name, "resource_category_id": float64(e.ResourceCategoryID), "owner_id": hookID(e.OwnerID)})
			}
			if manifestAfterHookGap != nil {
				manifestAfterHookGap(e.ResourceID)
			}
			// Dropped right after its own hook, so a later row's slow hook cannot leave
			// this one behind for a successor to run again.
			dropDB, cancelDrop := ctx.drainDB()
			if err := dropDB.Where("id = ? AND "+owned, e.ID, me, time.Now().Unix()).Delete(&models.ResourceCleanupManifest{}).Error; err != nil {
				warn("Failed to drop a drained cleanup manifest row", err)
			}
			cancelDrop()
		}
		last = entries[len(entries)-1].ID
	}
}

// manifestHookContext is the context a row's after-delete hook runs on, or nil when it
// must not run. A live drain runs on the deleter's own context. A replay has none, so
// it binds the account the row names; an account that is gone, disabled or unreadable
// skips the hook, because the alternative is running a plugin's mah.db unscoped.
func (ctx *MahresourcesContext) manifestHookContext(e *models.ResourceCleanupManifest, replay bool, cache map[uint]*MahresourcesContext) *MahresourcesContext {
	if !replay || e.ActorUserID == 0 {
		return ctx
	}
	if hc, ok := cache[e.ActorUserID]; ok {
		return hc
	}
	var hc *MahresourcesContext
	user, err := ctx.GetUser(e.ActorUserID)
	switch {
	case err != nil:
		ctx.Logger().Warning(models.LogActionDelete, "resource", &e.ResourceID, "Skipping the after-delete hook of a replayed delete",
			fmt.Sprintf("the deleting user %d could not be loaded: %v", e.ActorUserID, err), nil)
	case user == nil || user.Disabled:
		ctx.Logger().Warning(models.LogActionDelete, "resource", &e.ResourceID, "Skipping the after-delete hook of a replayed delete",
			fmt.Sprintf("the deleting user %d is gone or disabled", e.ActorUserID), nil)
	default:
		hc = ctx.WithPrincipal(auth.FromUser(user))
	}
	cache[e.ActorUserID] = hc
	return hc
}

// cleanupManifestFile is the file step of one manifest row: the Resource's file is
// backed up to the deleted area and removed, unless a surviving row still holds its
// hash or it lives in private storage. It reports an error only when a retry could
// change the outcome (storage unavailable, a count that failed, a backup that did not
// complete); a file that is already gone is done.
func (ctx *MahresourcesContext) cleanupManifestFile(e *models.ResourceCleanupManifest) error {
	fs, err := ctx.GetFsForStorageLocation(e.StorageLocation)
	if err != nil {
		return err
	}
	if ctx.StoredLocationIsPrivate(e.StorageLocation, e.Location) {
		ctx.Logger().Warning(models.LogActionDelete, "resource", &e.ResourceID, "Refusing to remove a file in private storage", e.Location, nil)
		return nil
	}
	if resourceFileRemovalGap != nil {
		resourceFileRemovalGap()
	}
	ctx.locks.ResourceHashLock.Acquire(e.Hash)
	defer ctx.locks.ResourceHashLock.Release(e.Hash)
	db := ctx.unscopedDB()
	countCtx, cancel := context.WithTimeout(context.WithoutCancel(db.Statement.Context), fileRemovalCountTimeout)
	defer cancel()
	refs, err := countHashReferences(db.WithContext(countCtx), e.Hash, e.StorageLocation)
	if err != nil {
		return fmt.Errorf("count hash references: %w", err)
	}
	if refs != 0 {
		return nil
	}

	subFolder := "deleted"
	if e.StorageLocation != nil && *e.StorageLocation != "" {
		subFolder = *e.StorageLocation
	}
	ownerIdStr := "nil"
	if e.OwnerID != nil {
		ownerIdStr = fmt.Sprintf("%v", *e.OwnerID)
	}
	backupPath := path.Join(fmt.Sprintf("/deleted/%v/", subFolder), fmt.Sprintf("%v__%v__%v___%v", e.Hash, e.ResourceID, ownerIdStr, strings.ReplaceAll(path.Clean(path.Base(e.Location)), "\\", "_")))

	src, openErr := fs.Open(e.Location)
	if openErr != nil {
		if errors.Is(openErr, os.ErrNotExist) {
			return nil // already gone: nothing to back up or remove
		}
		return fmt.Errorf("open %s: %w", e.Location, openErr)
	}
	defer src.Close()
	if err := ctx.fs.MkdirAll(path.Dir(backupPath), 0777); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	backup, err := ctx.fs.Create(backupPath)
	if err != nil {
		return fmt.Errorf("create backup: %w", err)
	}
	_, copyErr := io.Copy(backup, src)
	closeErr := backup.Close()
	if copyErr != nil || closeErr != nil {
		return fmt.Errorf("back up %s: %w", e.Location, errors.Join(copyErr, closeErr))
	}
	if err := fs.Remove(e.Location); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", e.Location, err)
	}
	return nil
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

// ResourceManifestReplayInterval is how often the running server looks for manifest
// rows that became due or whose claim lapsed since the last look: a failed file step's
// backoff elapsing, or a drain that died holding rows. Without it only a restart
// would retry them.
const ResourceManifestReplayInterval = time.Minute

// StartResourceManifestReplay replays the delete manifest every interval until stop is
// cancelled, with the same claim logic as the startup replay (so it never works a row
// another drain holds, and a delete that has only just committed is at worst drained
// by whichever claims its rows first). The returned channel closes when the loop has
// stopped.
func (ctx *MahresourcesContext) StartResourceManifestReplay(stop context.Context, interval time.Duration) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop.Done():
				return
			case <-ticker.C:
				ctx.ReplayResourceCleanupManifest()
			}
		}
	}()
	return done
}
