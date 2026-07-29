# Audit P0-P2 Fixes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix 10 audit findings across concurrency, performance, memory, and FS consistency (P0 through P2).

**Architecture:** Targeted fixes to existing code — no new packages or major restructuring. Uses existing IDLock, GORM patterns, and FS abstractions. Each task is independently testable.

**Tech Stack:** Go, GORM, SQLite/PostgreSQL, Afero (filesystem abstraction), existing IDLock library.

---

### Task 1: Fix RunWithLockTimeout to hold lock until fn completes

**Finding:** 1a (P0) — lock releases on timeout while goroutine continues.

**Files:**
- Modify: `lib/id_lock.go:283-320`
- Modify: `lib/id_lock_test.go:221-232` (update RunTimeout test)

**Context:** Currently `RunWithLockTimeout` uses `select` to race between `ctx.Done()` (run timeout) and `errChan`. When the timeout wins, it returns immediately but `defer l.Release(id)` runs, freeing the lock while the goroutine running `fn()` continues. The fix: always wait for `fn()` to complete before returning, regardless of timeout.

**Step 1: Write a new test that verifies mutual exclusion is maintained even when fn exceeds runTimeout**

Add to `lib/id_lock_test.go`:

```go
// TestRunWithLockTimeout_HoldsLockUntilFnCompletes verifies that when fn exceeds
// runTimeout, the lock is NOT released until fn actually finishes.
func TestRunWithLockTimeout_HoldsLockUntilFnCompletes(t *testing.T) {
	lock := NewIDLock[string](0, nil)
	id := "testHoldsLock"

	fnStarted := make(chan struct{})
	fnDone := make(chan struct{})

	// Start RunWithLockTimeout with a short runTimeout but long fn
	go func() {
		lock.RunWithLockTimeout(id, 1*time.Second, 50*time.Millisecond, func() error {
			close(fnStarted)
			time.Sleep(300 * time.Millisecond) // Exceeds runTimeout
			close(fnDone)
			return nil
		})
	}()

	<-fnStarted
	// fn is running and runTimeout will expire. Try to acquire the same lock.
	// It should NOT succeed until fn finishes (fnDone closes).
	acquired := lock.AcquireWithTimeout(id, 100*time.Millisecond)
	if acquired {
		lock.Release(id)
		t.Fatal("Lock was acquired while fn was still running — mutual exclusion violated")
	}

	// Now wait for fn to complete, then the lock should be available
	<-fnDone
	time.Sleep(50 * time.Millisecond) // Give time for Release to execute

	acquired = lock.AcquireWithTimeout(id, 500*time.Millisecond)
	if !acquired {
		t.Fatal("Lock should be free after fn completes")
	}
	lock.Release(id)
}
```

**Step 2: Run the test to verify it fails**

Run: `go test ./lib/ -run TestRunWithLockTimeout_HoldsLockUntilFnCompletes -v -count=1`
Expected: FAIL — "Lock was acquired while fn was still running"

**Step 3: Modify RunWithLockTimeout to always wait for fn**

In `lib/id_lock.go`, replace the `select` block in `RunWithLockTimeout` (lines 311-319) with code that always waits for `errChan`:

```go
func (l *IDLock[T]) RunWithLockTimeout(id T, lockTimeout, runTimeout time.Duration, fn func() error) (lockAcquired bool, err error) {
	acquired := l.AcquireWithTimeout(id, lockTimeout)
	if !acquired {
		return false, nil
	}
	defer l.Release(id)

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	errChan := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				l.log.Printf("IDLock.RunWithLockTimeout: Recovered panic in function for ID '%v': %v\n", id, r)
				errChan <- fmt.Errorf("panic in locked function: %v", r)
			}
		}()
		errChan <- fn()
	}()

	// Check if the run timed out, but always wait for fn to complete.
	// This guarantees mutual exclusion: the lock is not released until fn exits.
	select {
	case <-ctx.Done():
		// Timeout exceeded. Wait for fn to finish before releasing the lock.
		<-errChan
		return true, context.DeadlineExceeded
	case runErr := <-errChan:
		return true, runErr
	}
}
```

The key change: `<-ctx.Done()` case now does `<-errChan` before returning, ensuring `fn()` has exited before `defer l.Release(id)` runs.

**Step 4: Update the existing RunTimeout test to expect the new behavior**

The test `TestRunWithLockTimeout_RunTimeout` (line 221) has `fn` sleeping 200ms with a 50ms timeout. Previously this returned immediately on timeout. Now it waits for fn to finish. The test still expects `(true, DeadlineExceeded)` — which is correct. However, the test will now take ~200ms instead of ~50ms. That's expected behavior. No change needed to the assertion, just understand it will be slower.

Also update `TestRunWithLockTimeout_TimeoutButAcquired` — same logic, same assertions, just takes longer.

**Step 5: Run all lock tests to verify everything passes**

Run: `go test ./lib/ -v -count=1`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add lib/id_lock.go lib/id_lock_test.go
git commit -m "fix: hold lock until fn completes in RunWithLockTimeout

Previously, RunWithLockTimeout released the lock when runTimeout expired
even though the goroutine running fn() was still executing. This violated
mutual exclusion. Now the timeout case waits for fn to finish before
releasing the lock."
```

---

### Task 2: Serialize UploadNewVersion with IDLock (version race + lazy migration)

**Finding:** 1c+1d (P1) — version number race condition and non-transactional lazy migration.

**Files:**
- Modify: `application_context/resource_version_context.go:100-201`
- Modify: `application_context/context.go:98-103` (add VersionUploadLock to MahresourcesLocks)

**Context:** `UploadNewVersion` has two race conditions: (1) `MAX(version_number)` runs on `ctx.db` not the transaction, so two concurrent uploads get the same version number; (2) the lazy v1 migration also runs outside any lock. Fix: wrap the entire operation in an IDLock keyed on resource ID. The existing `ResourceHashLock` is keyed on hash (string), but we need a lock keyed on resource ID (uint). Add a new `VersionUploadLock *lib.IDLock[uint]`.

**Step 1: Add VersionUploadLock to MahresourcesLocks**

In `application_context/context.go`, add to the `MahresourcesLocks` struct (after line 102):

```go
type MahresourcesLocks struct {
	ThumbnailGenerationLock      *lib.IDLock[uint]
	VideoThumbnailGenerationLock *lib.IDLock[uint]
	OfficeDocumentGenerationLock *lib.IDLock[uint]
	ResourceHashLock             *lib.IDLock[string]
	VersionUploadLock            *lib.IDLock[uint]
}
```

And initialize it in `NewMahresourcesContext` (around line 149):

```go
versionUploadLock := lib.NewIDLock[uint](uint(0), nil)
```

Add it to the locks struct initialization (around line 175):

```go
VersionUploadLock: versionUploadLock,
```

**Step 2: Wrap UploadNewVersion in the lock**

In `application_context/resource_version_context.go`, modify `UploadNewVersion` to acquire the lock at the start:

```go
func (ctx *MahresourcesContext) UploadNewVersion(resourceID uint, file multipart.File, header *multipart.FileHeader, comment string) (*models.ResourceVersion, error) {
	// Serialize per-resource to prevent version number races and
	// lazy migration conflicts under concurrent uploads
	ctx.locks.VersionUploadLock.Acquire(resourceID)
	defer ctx.locks.VersionUploadLock.Release(resourceID)

	// ... rest of existing function unchanged ...
```

**Step 3: Also protect RestoreVersion with the same lock**

`RestoreVersion` (line 283) has the same `MAX(version_number)` pattern. Add the same lock:

```go
func (ctx *MahresourcesContext) RestoreVersion(resourceID, versionID uint, comment string) (*models.ResourceVersion, error) {
	ctx.locks.VersionUploadLock.Acquire(resourceID)
	defer ctx.locks.VersionUploadLock.Release(resourceID)

	// ... rest unchanged ...
```

**Step 4: Run Go tests**

Run: `go test ./... --tags 'json1 fts5' -count=1`
Expected: ALL PASS (compilation check; the version functions are exercised by E2E tests)

**Step 5: Commit**

```bash
git add application_context/context.go application_context/resource_version_context.go
git commit -m "fix: serialize version uploads per-resource with IDLock

Adds VersionUploadLock to prevent concurrent UploadNewVersion/RestoreVersion
calls from creating duplicate version numbers. Also ensures the lazy v1
migration runs under the same lock, preventing duplicate v1 creation."
```

---

### Task 3: Add lightweight GetTagByID, GetResourceByID, GetGroupByID methods

**Finding:** 2a (P1) — clause.Associations over-preloads everywhere.

**Files:**
- Modify: `application_context/tags_context.go` (add GetTagByID)
- Modify: `application_context/resource_crud_context.go` (add GetResourceByID)
- Modify: `application_context/group_crud_context.go` (add GetGroupByID)

**Context:** `GetTag`, `GetResource`, and `GetGroup` use `Preload(clause.Associations)` which loads all relationships. Internal callers (bulk ops, merge) only need the entity itself. Add lightweight methods that skip preloads.

**Step 1: Add GetTagByID**

In `application_context/tags_context.go`, after `GetTag` (line 30):

```go
// GetTagByID returns a tag without preloading associations.
// Use this for internal operations that only need the tag entity itself.
func (ctx *MahresourcesContext) GetTagByID(id uint) (*models.Tag, error) {
	var tag models.Tag
	return &tag, ctx.db.First(&tag, id).Error
}
```

**Step 2: Add GetResourceByID**

In `application_context/resource_crud_context.go`, after `GetResource` (line 17):

```go
// GetResourceByID returns a resource without preloading associations.
// Use this for internal operations that only need the resource entity itself.
func (ctx *MahresourcesContext) GetResourceByID(id uint) (*models.Resource, error) {
	var resource models.Resource
	return &resource, ctx.db.First(&resource, id).Error
}
```

**Step 3: Add GetGroupByID**

In `application_context/group_crud_context.go`, after `GetGroup` (line 187):

```go
// GetGroupByID returns a group without preloading associations.
// Use this for internal operations that only need the group entity itself.
func (ctx *MahresourcesContext) GetGroupByID(id uint) (*models.Group, error) {
	var group models.Group
	return &group, ctx.db.First(&group, id).Error
}
```

**Step 4: Run Go tests**

Run: `go test ./... --tags 'json1 fts5' -count=1`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add application_context/tags_context.go application_context/resource_crud_context.go application_context/group_crud_context.go
git commit -m "feat: add lightweight ByID methods that skip preloads

GetTagByID, GetResourceByID, GetGroupByID return entities without
association preloading. For use in bulk operations and merge where
only the entity itself is needed."
```

---

### Task 4: Replace per-entity bulk tag operations with batch SQL

**Finding:** 2b (P2) — bulk tag operations do individual association updates.

**Files:**
- Modify: `application_context/resource_bulk_context.go:121-239` (BulkAddTagsToResources, BulkRemoveTagsFromResources, BulkReplaceTagsFromResources)
- Modify: `application_context/group_bulk_context.go:166-206` (BulkAddTagsToGroups, BulkRemoveTagsFromGroups)

**Context:** Currently, `BulkAddTagsToResources` calls `GetTag(editedId)` in a loop (loading all associations), then loops over resource IDs calling `Association("Tags").Append` one by one. Replace with: (1) validate tag IDs exist using lightweight query, (2) batch INSERT/DELETE in a single SQL statement.

**Step 1: Rewrite BulkAddTagsToResources**

In `application_context/resource_bulk_context.go`, replace `BulkAddTagsToResources` (lines 211-239):

```go
func (ctx *MahresourcesContext) BulkAddTagsToResources(query *query_models.BulkEditQuery) error {
	if len(query.ID) == 0 || len(query.EditedId) == 0 {
		return nil
	}

	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		// Validate all tags exist (single query, no preloads)
		var tagCount int64
		if err := tx.Model(&models.Tag{}).Where("id IN ?", query.EditedId).Count(&tagCount).Error; err != nil {
			return err
		}
		if int(tagCount) != len(query.EditedId) {
			return fmt.Errorf("one or more tags not found")
		}

		// Batch insert: one INSERT per (resource, tag) pair, skip conflicts
		for _, tagID := range query.EditedId {
			if err := tx.Exec(
				"INSERT INTO resource_tags (resource_id, tag_id) SELECT id, ? FROM resources WHERE id IN ? ON CONFLICT DO NOTHING",
				tagID, query.ID,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})

	if err == nil {
		ctx.Logger().Info(models.LogActionUpdate, "resource", nil, "", "Bulk added tags to resources", map[string]interface{}{
			"resourceIds": query.ID,
			"tagIds":      query.EditedId,
		})
	}

	return err
}
```

**Step 2: Rewrite BulkRemoveTagsFromResources**

Replace `BulkRemoveTagsFromResources` (lines 121-149):

```go
func (ctx *MahresourcesContext) BulkRemoveTagsFromResources(query *query_models.BulkEditQuery) error {
	if len(query.ID) == 0 || len(query.EditedId) == 0 {
		return nil
	}

	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(
			"DELETE FROM resource_tags WHERE resource_id IN ? AND tag_id IN ?",
			query.ID, query.EditedId,
		).Error
	})

	if err == nil {
		ctx.Logger().Info(models.LogActionUpdate, "resource", nil, "", "Bulk removed tags from resources", map[string]interface{}{
			"resourceIds": query.ID,
			"tagIds":      query.EditedId,
		})
	}

	return err
}
```

**Step 3: Rewrite BulkReplaceTagsFromResources**

Replace `BulkReplaceTagsFromResources` (lines 151-184):

```go
func (ctx *MahresourcesContext) BulkReplaceTagsFromResources(query *query_models.BulkEditQuery) error {
	if len(query.ID) == 0 {
		return nil
	}

	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		// Validate all tags exist
		if len(query.EditedId) > 0 {
			var tagCount int64
			if err := tx.Model(&models.Tag{}).Where("id IN ?", query.EditedId).Count(&tagCount).Error; err != nil {
				return err
			}
			if int(tagCount) != len(query.EditedId) {
				return fmt.Errorf("one or more tags not found")
			}
		}

		// Remove all existing tags for these resources
		if err := tx.Exec("DELETE FROM resource_tags WHERE resource_id IN ?", query.ID).Error; err != nil {
			return err
		}

		// Add the new tags
		for _, tagID := range query.EditedId {
			if err := tx.Exec(
				"INSERT INTO resource_tags (resource_id, tag_id) SELECT id, ? FROM resources WHERE id IN ? ON CONFLICT DO NOTHING",
				tagID, query.ID,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})

	if err == nil {
		ctx.Logger().Info(models.LogActionUpdate, "resource", nil, "", "Bulk replaced tags on resources", map[string]interface{}{
			"resourceIds": query.ID,
			"tagIds":      query.EditedId,
		})
	}

	return err
}
```

**Step 4: Rewrite BulkAddTagsToGroups and BulkRemoveTagsFromGroups**

In `application_context/group_bulk_context.go`, replace `BulkAddTagsToGroups` (lines 166-185):

```go
func (ctx *MahresourcesContext) BulkAddTagsToGroups(query *query_models.BulkEditQuery) error {
	if len(query.ID) == 0 || len(query.EditedId) == 0 {
		return nil
	}

	return ctx.db.Transaction(func(tx *gorm.DB) error {
		var tagCount int64
		if err := tx.Model(&models.Tag{}).Where("id IN ?", query.EditedId).Count(&tagCount).Error; err != nil {
			return err
		}
		if int(tagCount) != len(query.EditedId) {
			return fmt.Errorf("one or more tags not found")
		}

		for _, tagID := range query.EditedId {
			if err := tx.Exec(
				"INSERT INTO group_tags (group_id, tag_id) SELECT id, ? FROM groups WHERE id IN ? ON CONFLICT DO NOTHING",
				tagID, query.ID,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
```

Replace `BulkRemoveTagsFromGroups` (lines 187-206):

```go
func (ctx *MahresourcesContext) BulkRemoveTagsFromGroups(query *query_models.BulkEditQuery) error {
	if len(query.ID) == 0 || len(query.EditedId) == 0 {
		return nil
	}

	return ctx.db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(
			"DELETE FROM group_tags WHERE group_id IN ? AND tag_id IN ?",
			query.ID, query.EditedId,
		).Error
	})
}
```

**Step 5: Also update BulkAddGroupsToResources to use batch SQL**

Replace `BulkAddGroupsToResources` (lines 241-269) — same N+1 pattern with `GetGroup`:

```go
func (ctx *MahresourcesContext) BulkAddGroupsToResources(query *query_models.BulkEditQuery) error {
	if len(query.ID) == 0 || len(query.EditedId) == 0 {
		return nil
	}

	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		var groupCount int64
		if err := tx.Model(&models.Group{}).Where("id IN ?", query.EditedId).Count(&groupCount).Error; err != nil {
			return err
		}
		if int(groupCount) != len(query.EditedId) {
			return fmt.Errorf("one or more groups not found")
		}

		for _, groupID := range query.EditedId {
			if err := tx.Exec(
				"INSERT INTO groups_related_resources (resource_id, group_id) SELECT id, ? FROM resources WHERE id IN ? ON CONFLICT DO NOTHING",
				groupID, query.ID,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})

	if err == nil {
		ctx.Logger().Info(models.LogActionUpdate, "resource", nil, "", "Bulk added groups to resources", map[string]interface{}{
			"resourceIds": query.ID,
			"groupIds":    query.EditedId,
		})
	}

	return err
}
```

**Step 6: Remove unused imports**

After removing `GetTag`/`GetGroup` calls from bulk operations, remove the `clause` import from `resource_bulk_context.go` if no longer needed (check if `clause` is used elsewhere in the file — it's still used in `DeleteResource` via `clause.Associations` and in `BulkAddMetaToResources` via `clause.Expr`). Similarly for `group_bulk_context.go`.

**Step 7: Run Go tests then E2E tests**

Run: `go test ./... --tags 'json1 fts5' -count=1`
Then: `cd e2e && npm run test:with-server`
Expected: ALL PASS

**Step 8: Commit**

```bash
git add application_context/resource_bulk_context.go application_context/group_bulk_context.go
git commit -m "perf: replace per-entity bulk tag operations with batch SQL

Bulk add/remove/replace tags now use single SQL statements instead of
looping over each entity with GORM Association API. Reduces N roundtrips
to 1-2. Also removes unnecessary GetTag/GetGroup preloads in bulk paths."
```

---

### Task 5: Align note scope tag filtering to JOIN+GROUP BY pattern

**Finding:** 2c (P2) — correlated subqueries in note and group scopes.

**Files:**
- Modify: `models/database_scopes/note_scope.go:18-24`

**Context:** Note tag filtering uses a correlated subquery `(SELECT COUNT(*) FROM note_tags WHERE ... AND note_id = notes.id)` — one per row. The resource scope already uses the efficient `JOIN + GROUP BY + HAVING` pattern. Align notes to match.

**Step 1: Replace the correlated subquery in NoteQuery**

In `models/database_scopes/note_scope.go`, replace lines 18-24:

```go
		if len(query.Tags) > 0 {
			subQuery := originalDB.
				Table("note_tags nt").
				Where("nt.tag_id IN ?", query.Tags).
				Group("nt.note_id").
				Having("count(*) = ?", len(query.Tags)).
				Select("nt.note_id")

			dbQuery = dbQuery.Where("notes.id IN (?)", subQuery)
		}
```

Note: `NoteQuery` doesn't currently take an `originalDB` parameter like `ResourceQuery` and `GroupQuery` do. We need to add it.

**Step 2: Update NoteQuery signature to accept originalDB**

Change `NoteQuery` signature from:
```go
func NoteQuery(query *query_models.NoteQuery, ignoreSort bool) func(db *gorm.DB) *gorm.DB {
```
to:
```go
func NoteQuery(query *query_models.NoteQuery, ignoreSort bool, originalDB *gorm.DB) func(db *gorm.DB) *gorm.DB {
```

Use `originalDB` for the subquery (like ResourceQuery does). If `originalDB` is not available at certain call sites, use `db` as a fallback — but check the callers first.

**Step 3: Update all callers of NoteQuery**

Search for all callers:
```
grep -rn "NoteQuery(" --include="*.go"
```

Update each to pass the original DB. The callers should be in `application_context/` files. Typical pattern already used for `ResourceQuery` and `GroupQuery` — pass `ctx.db` as the third argument.

**Step 4: Run Go tests then E2E tests**

Run: `go test ./... --tags 'json1 fts5' -count=1`
Then: `cd e2e && npm run test:with-server`
Expected: ALL PASS — note filtering should produce identical results

**Step 5: Commit**

```bash
git add models/database_scopes/note_scope.go
git add -u  # catch caller changes
git commit -m "perf: use JOIN+GROUP BY for note tag filtering

Replaces the correlated subquery pattern in NoteQuery with the same
JOIN+GROUP BY+HAVING pattern used by ResourceQuery. Eliminates per-row
subquery execution for note tag filtering."
```

---

### Task 6: Replace merge association loading with direct SQL transfers

**Finding:** 2d (P2) — merge operations preload all associations just for IDs.

**Files:**
- Modify: `application_context/resource_bulk_context.go:297-417` (MergeResources)
- Modify: `application_context/group_bulk_context.go:19-160` (MergeGroups)

**Context:** `MergeResources` does `Preload(clause.Associations).Find(&losers)` to load every association of each loser resource into Go memory, then iterates to INSERT them into the winner's associations. But the INSERT statements already use raw SQL with IDs — they don't need the full Go objects. Same for `MergeGroups`. Replace the `Preload(clause.Associations)` with direct SQL that transfers associations without loading into memory.

**Step 1: Rewrite MergeResources to skip Preload on losers**

In `application_context/resource_bulk_context.go`, the key change: replace `tx.Preload(clause.Associations).Find(&losers)` (line 317) with a simple `tx.Find(&losers, &loserIds)`. Then replace the per-loser Go iteration with batch SQL:

```go
return ctx.WithTransaction(func(transactionCtx *MahresourcesContext) error {
	tx := transactionCtx.db

	// Load losers WITHOUT associations — we only need their basic fields for backup
	var losers []*models.Resource
	if loadResourcesErr := tx.Find(&losers, &loserIds).Error; loadResourcesErr != nil {
		return loadResourcesErr
	}

	// Load winner WITHOUT associations
	var winner models.Resource
	if err := tx.First(&winner, winnerId).Error; err != nil {
		return err
	}

	// Transfer associations via direct SQL (no Go-side loading needed)
	if err := tx.Exec("INSERT INTO resource_tags (resource_id, tag_id) SELECT ?, tag_id FROM resource_tags WHERE resource_id IN ? ON CONFLICT DO NOTHING", winnerId, loserIds).Error; err != nil {
		return err
	}
	if err := tx.Exec("INSERT INTO resource_notes (resource_id, note_id) SELECT ?, note_id FROM resource_notes WHERE resource_id IN ? ON CONFLICT DO NOTHING", winnerId, loserIds).Error; err != nil {
		return err
	}
	if err := tx.Exec("INSERT INTO groups_related_resources (resource_id, group_id) SELECT ?, group_id FROM groups_related_resources WHERE resource_id IN ? ON CONFLICT DO NOTHING", winnerId, loserIds).Error; err != nil {
		return err
	}
	// Also add losers' owners as related groups
	if err := tx.Exec("INSERT INTO groups_related_resources (resource_id, group_id) SELECT ?, owner_id FROM resources WHERE id IN ? AND owner_id IS NOT NULL ON CONFLICT DO NOTHING", winnerId, loserIds).Error; err != nil {
		return err
	}

	deletedResBackups := make(map[string]types.JSON)

	for _, loser := range losers {
		backupData, err := json.Marshal(loser)
		if err != nil {
			return err
		}
		deletedResBackups[fmt.Sprintf("resource_%v", loser.ID)] = backupData

		// Merge meta
		switch transactionCtx.Config.DbType {
		case constants.DbTypePosgres:
			err = tx.Exec(`UPDATE resources SET meta = coalesce((SELECT meta FROM resources WHERE id = ?), '{}'::jsonb) || meta WHERE id = ?`, loser.ID, winnerId).Error
		case constants.DbTypeSqlite:
			err = tx.Exec(`UPDATE resources SET meta = json_patch(meta, coalesce((SELECT meta FROM resources WHERE id = ?), '{}')) WHERE id = ?`, loser.ID, winnerId).Error
		default:
			err = errors.New("db doesn't support merging meta")
		}
		if err != nil {
			return err
		}

		if err := transactionCtx.DeleteResource(loser.ID); err != nil {
			return err
		}
	}

	// Save backups to winner's meta
	backupObj := make(map[string]any)
	backupObj["backups"] = deletedResBackups
	backups, err := json.Marshal(&backupObj)
	if err != nil {
		return err
	}

	if transactionCtx.Config.DbType == constants.DbTypePosgres {
		if err := tx.Exec("update resources set meta = meta || ? where id = ?", backups, winner.ID).Error; err != nil {
			return err
		}
	} else if transactionCtx.Config.DbType == constants.DbTypeSqlite {
		if err := tx.Exec("update resources set meta = json_patch(meta, ?) where id = ?", backups, winner.ID).Error; err != nil {
			return err
		}
	}

	transactionCtx.Logger().Info(models.LogActionUpdate, "resource", &winnerId, winner.Name, "Merged resources", map[string]interface{}{
		"winnerId": winnerId,
		"loserIds": loserIds,
	})

	return nil
})
```

Also remove the dead code check at lines 321-323 (`if winnerId == 0 || loserIds == nil...` — already validated at function entry).

**Step 2: Rewrite MergeGroups similarly**

In `application_context/group_bulk_context.go`, replace `Preload(clause.Associations).Find(&losers)` (line 33) and `Preload(clause.Associations).First(&winner)` (line 39) with simple loads. Replace per-loser Go iterations with batch SQL:

```go
// Load without preloads
var losers []*models.Group
if loadErr := altCtx.db.Find(&losers, &loserIds).Error; loadErr != nil {
	return loadErr
}
var winner models.Group
if err := altCtx.db.First(&winner, winnerId).Error; err != nil {
	return err
}

// Batch SQL transfers
if err := altCtx.db.Exec("INSERT INTO group_tags (group_id, tag_id) SELECT ?, tag_id FROM group_tags WHERE group_id IN ? ON CONFLICT DO NOTHING", winnerId, loserIds).Error; err != nil {
	return err
}
```

Similarly for `groups_related_notes`, `groups_related_resources`, `group_related_groups`, `group_relations`. The existing per-loser pattern for `UPDATE groups SET owner_id` and `UPDATE notes SET owner_id` and `UPDATE resources SET owner_id` can stay as batch SQL but operate on all losers at once:

```go
if err := altCtx.db.Exec("UPDATE groups SET owner_id = ? WHERE owner_id IN ?", winnerId, loserIds).Error; err != nil {
	return err
}
if err := altCtx.db.Exec("UPDATE notes SET owner_id = ? WHERE owner_id IN ?", winnerId, loserIds).Error; err != nil {
	return err
}
if err := altCtx.db.Exec("UPDATE resources SET owner_id = ? WHERE owner_id IN ?", winnerId, loserIds).Error; err != nil {
	return err
}
```

For related groups, exclude self-references:

```go
if err := altCtx.db.Exec("INSERT INTO group_related_groups (group_id, related_group_id) SELECT ?, related_group_id FROM group_related_groups WHERE group_id IN ? AND related_group_id != ? ON CONFLICT DO NOTHING", winnerId, loserIds, winnerId).Error; err != nil {
	return err
}
```

Keep the per-loser loop for: (1) the owner_id self-reference check (`loser.ID == *winner.OwnerId`), (2) JSON backup serialization, (3) meta merge, (4) `DeleteGroup` call — these need individual loser data.

**Step 3: Run Go tests then E2E tests**

Run: `go test ./... --tags 'json1 fts5' -count=1`
Then: `cd e2e && npm run test:with-server`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add application_context/resource_bulk_context.go application_context/group_bulk_context.go
git commit -m "perf: use direct SQL for merge association transfers

MergeResources and MergeGroups no longer Preload(clause.Associations)
on losers. Association transfers use batch SQL INSERT...SELECT instead
of loading into Go memory. Removes dead code validation checks."
```

---

### Task 7: Two-phase bulk delete (DB then FS)

**Finding:** 1b (P0) — FS operations not rolled back on DB transaction failure.

**Files:**
- Modify: `application_context/resource_bulk_context.go:21-115` (DeleteResource)
- Modify: `application_context/resource_bulk_context.go:271-280` (BulkDeleteResources)

**Context:** `DeleteResource` performs file backup (copy to /deleted/) and file removal inside the transaction. If a later operation fails and the tx rolls back, files are already gone. Fix: separate `DeleteResource` into DB-only and FS operations for the bulk path. The single-delete path can stay as-is (no transaction wrapping issue). For bulk, collect file operations and execute them after commit.

**Step 1: Create a DeleteResourceDBOnly method that returns file cleanup info**

Add a new type and method in `application_context/resource_bulk_context.go`:

```go
// FileCleanupAction describes a file operation to perform after a transaction commits.
type FileCleanupAction struct {
	// SourceFS is the filesystem containing the resource file
	SourceFS afero.Fs
	// SourcePath is the path to the original resource file
	SourcePath string
	// BackupPath is the path to write the backup copy (in /deleted/)
	BackupPath string
	// ShouldRemoveSource indicates if the source file should be deleted (no other references)
	ShouldRemoveSource bool
}

// deleteResourceDBOnly performs only the database operations of DeleteResource.
// Returns file cleanup actions to be performed after the transaction commits.
func (ctx *MahresourcesContext) deleteResourceDBOnly(resourceId uint) (*FileCleanupAction, error) {
	resource := models.Resource{ID: resourceId}
	if err := ctx.db.Model(&resource).First(&resource).Error; err != nil {
		return nil, err
	}

	fs, storageErr := ctx.GetFsForStorageLocation(resource.StorageLocation)
	if storageErr != nil {
		return nil, storageErr
	}

	subFolder := "deleted"
	if resource.StorageLocation != nil && *resource.StorageLocation != "" {
		subFolder = *resource.StorageLocation
	}
	folder := fmt.Sprintf("/deleted/%v/", subFolder)

	ownerIdStr := "nil"
	if resource.OwnerId != nil {
		ownerIdStr = fmt.Sprintf("%v", *resource.OwnerId)
	}
	backupPath := path.Join(folder, fmt.Sprintf("%v__%v__%v___%v", resource.Hash, resource.ID, ownerIdStr, strings.ReplaceAll(path.Clean(path.Base(resource.GetCleanLocation())), "\\", "_")))

	// Clear CurrentVersionID to break circular reference before deletion
	if resource.CurrentVersionID != nil {
		if err := ctx.db.Model(&resource).Update("current_version_id", nil).Error; err != nil {
			return nil, err
		}
	}

	if err := ctx.db.Select(clause.Associations).Delete(&resource).Error; err != nil {
		return nil, err
	}

	// Auto-delete empty series
	if resource.SeriesID != nil {
		seriesID := *resource.SeriesID
		result := ctx.db.Where("id = ? AND NOT EXISTS (SELECT 1 FROM resources WHERE series_id = ?)", seriesID, seriesID).Delete(&models.Series{})
		if result.Error != nil {
			ctx.Logger().Warning(models.LogActionDelete, "series", &seriesID, "Failed to auto-delete empty series", result.Error.Error(), nil)
		}
	}

	// Check hash references for file deletion decision
	refCount, countErr := ctx.CountHashReferences(resource.Hash)
	if countErr != nil {
		ctx.Logger().Warning(models.LogActionDelete, "resource", &resourceId, "Failed to count hash references", countErr.Error(), nil)
		refCount = 1
	}

	ctx.Logger().Info(models.LogActionDelete, "resource", &resourceId, resource.Name, "Deleted resource", nil)
	ctx.InvalidateSearchCacheByType(EntityTypeResource)

	return &FileCleanupAction{
		SourceFS:           fs,
		SourcePath:         resource.GetCleanLocation(),
		BackupPath:         backupPath,
		ShouldRemoveSource: refCount == 0,
	}, nil
}
```

**Step 2: Rewrite BulkDeleteResources to use two-phase approach**

```go
func (ctx *MahresourcesContext) BulkDeleteResources(query *query_models.BulkQuery) error {
	var cleanupActions []*FileCleanupAction

	err := ctx.WithTransaction(func(altCtx *MahresourcesContext) error {
		for _, id := range query.ID {
			action, err := altCtx.deleteResourceDBOnly(id)
			if err != nil {
				return err
			}
			if action != nil {
				cleanupActions = append(cleanupActions, action)
			}
		}
		return nil
	})

	if err != nil {
		return err // Transaction rolled back, no file operations performed
	}

	// Phase 2: File operations after successful commit
	for _, action := range cleanupActions {
		// Create backup
		if err := ctx.fs.MkdirAll(path.Dir(action.BackupPath), 0777); err != nil {
			ctx.Logger().Warning(models.LogActionDelete, "resource", nil, "Failed to create backup dir", err.Error(), nil)
			continue
		}

		file, openErr := action.SourceFS.Open(action.SourcePath)
		if openErr == nil {
			backup, createErr := ctx.fs.Create(action.BackupPath)
			if createErr == nil {
				io.Copy(backup, file)
				backup.Close()
			}
			file.Close()
		}

		// Remove source file if no other references
		if action.ShouldRemoveSource {
			_ = action.SourceFS.Remove(action.SourcePath)
		}
	}

	return nil
}
```

**Step 3: Run Go tests then E2E tests**

Run: `go test ./... --tags 'json1 fts5' -count=1`
Then: `cd e2e && npm run test:with-server`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add application_context/resource_bulk_context.go
git commit -m "fix: two-phase bulk delete — DB operations then file cleanup

BulkDeleteResources now performs all DB operations inside the transaction
and defers file operations (backup copy, file deletion) until after the
transaction commits. If the transaction rolls back, no files are touched.
Single-resource DeleteResource retains its existing behavior."
```

---

### Task 8: Replace unbounded hash cache with LRU

**Finding:** 3a (P1) — hash cache loads all records and grows unboundedly.

**Files:**
- Modify: `hash_worker/worker.go:32-49,295-341,343-403,405-438`
- Modify: `hash_worker/config.go` (add CacheSize field)
- Modify: `main.go` (add `-hash-cache-size` flag)
- Add: `go get github.com/hashicorp/golang-lru/v2`

**Context:** `ensureCacheLoaded` does `Find(&hashes)` with no LIMIT, loading every `ImageHash` row into memory. For millions of resources this is hundreds of MB. The cache never evicts entries. Also, if the DB query fails, `cacheLoaded` is never set to `true` (line 337 — wait, re-reading the code: actually line 337 sets `w.cacheLoaded = true` ONLY after successful loading. But the `return` on line 328 means on error it returns without setting it, so it will retry next cycle. That's actually correct — the audit report's claim about silent failure is wrong for this specific code. Let me re-check... yes, on error at line 328, it returns before reaching line 337. So the "cacheLoaded set on error" bug isn't present. But the unbounded loading IS the problem.)

Replace with a bounded LRU cache that's populated lazily.

**Step 1: Add golang-lru dependency**

Run: `go get github.com/hashicorp/golang-lru/v2`

**Step 2: Add CacheSize to Config**

In `hash_worker/config.go`:

```go
type Config struct {
	WorkerCount         int
	BatchSize           int
	PollInterval        time.Duration
	SimilarityThreshold int
	Disabled            bool
	// CacheSize is the maximum number of entries in the hash LRU cache.
	CacheSize int
}

func DefaultConfig() Config {
	return Config{
		WorkerCount:         4,
		BatchSize:           500,
		PollInterval:        time.Minute,
		SimilarityThreshold: 10,
		Disabled:            false,
		CacheSize:           100000,
	}
}
```

**Step 3: Replace hashCache map with LRU in HashWorker**

In `hash_worker/worker.go`, replace the cache fields:

```go
import (
	lru "github.com/hashicorp/golang-lru/v2"
	// ... existing imports
)

type HashWorker struct {
	db        *gorm.DB
	fs        afero.Fs
	altFS     map[string]afero.Fs
	config    Config
	appLogger AppLogger

	// hashCache is a bounded LRU cache mapping resource ID to DHash
	hashCache *lru.Cache[uint, uint64]

	hashQueue chan uint
	stopCh    chan struct{}
	wg        sync.WaitGroup
}
```

Update `New`:

```go
func New(db *gorm.DB, fs afero.Fs, altFS map[string]afero.Fs, config Config, appLogger AppLogger) *HashWorker {
	cacheSize := config.CacheSize
	if cacheSize <= 0 {
		cacheSize = 100000
	}
	cache, _ := lru.New[uint, uint64](cacheSize)

	return &HashWorker{
		db:        db,
		fs:        fs,
		altFS:     altFS,
		config:    config,
		appLogger: appLogger,
		hashCache: cache,
		hashQueue: make(chan uint, 1000),
		stopCh:    make(chan struct{}),
	}
}
```

**Step 4: Remove ensureCacheLoaded, populate cache lazily**

Delete the `ensureCacheLoaded` method entirely. Remove the `cacheMutex` and `cacheLoaded` fields.

In `hashNewResources` (line 274), remove the `w.ensureCacheLoaded()` call.
In `processResource` (line 313), remove the `w.ensureCacheLoaded()` call.

In `hashAndStoreSimilarities`, update cache access — the LRU cache is already thread-safe:

```go
func (w *HashWorker) hashAndStoreSimilarities(resource models.Resource) {
	// ... (file open, decode, hash calculation unchanged) ...

	// Update cache
	w.hashCache.Add(resource.ID, dHashInt)

	// Find and store similarities
	w.findAndStoreSimilarities(resource.ID, dHashInt)
}
```

**Step 5: Update findAndStoreSimilarities to use LRU**

Replace the mutex-guarded map iteration with LRU cache iteration:

```go
func (w *HashWorker) findAndStoreSimilarities(resourceID uint, dHash uint64) {
	var similarities []models.ResourceSimilarity

	// LRU cache Keys() is thread-safe
	for _, otherID := range w.hashCache.Keys() {
		if otherID == resourceID {
			continue
		}
		otherHash, ok := w.hashCache.Peek(otherID)
		if !ok {
			continue
		}

		distance := HammingDistance(dHash, otherHash)
		if distance <= w.config.SimilarityThreshold {
			id1, id2 := resourceID, otherID
			if id1 > id2 {
				id1, id2 = id2, id1
			}
			similarities = append(similarities, models.ResourceSimilarity{
				ResourceID1:     id1,
				ResourceID2:     id2,
				HammingDistance: uint8(distance),
			})
		}
	}

	if len(similarities) == 0 {
		return
	}

	if err := w.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&similarities).Error; err != nil {
		log.Printf("Hash worker: error saving similarities for resource %d: %v", resourceID, err)
	}
}
```

**Step 6: Seed cache during batch processing**

When `hashNewResources` runs, before processing the batch, load existing hashes for comparison. Add a cache-warming step that loads hashes in batches (not all at once):

```go
func (w *HashWorker) warmCache() {
	// Load hashes in pages to seed the LRU cache without loading everything at once
	batchSize := w.config.CacheSize
	if batchSize > 50000 {
		batchSize = 50000
	}
	offset := 0

	for {
		var hashes []models.ImageHash
		if err := w.db.Select("resource_id, d_hash, d_hash_int").
			Offset(offset).Limit(batchSize).
			Find(&hashes).Error; err != nil {
			log.Printf("Hash worker: error warming cache: %v", err)
			return
		}

		if len(hashes) == 0 {
			break
		}

		for _, h := range hashes {
			if h.ResourceId != nil {
				w.hashCache.Add(*h.ResourceId, h.GetDHash())
			}
		}

		if w.hashCache.Len() >= w.config.CacheSize {
			break // Cache is full
		}

		offset += batchSize
	}

	log.Printf("Hash worker: cache warmed with %d entries (max %d)", w.hashCache.Len(), w.config.CacheSize)
}
```

Call `w.warmCache()` in `hashNewResources` before processing, instead of `ensureCacheLoaded`:

```go
func (w *HashWorker) hashNewResources() {
	// ... (find resources unchanged) ...

	// Warm cache if needed
	if w.hashCache.Len() == 0 {
		w.warmCache()
	}

	// ... (process with concurrency limit unchanged) ...
}
```

**Step 7: Add `-hash-cache-size` flag to main.go**

In `main.go`, find where hash worker flags are defined and add:

```go
hashCacheSize := flag.Int("hash-cache-size", 100000, "Maximum entries in the hash similarity cache (0 = use default 100000)")
```

And pass it to config:

```go
hashWorkerConfig := hash_worker.Config{
	WorkerCount:         *hashWorkerCount,
	BatchSize:           *hashBatchSize,
	PollInterval:        *hashPollInterval,
	SimilarityThreshold: *hashSimilarityThreshold,
	Disabled:            *hashWorkerDisabled,
	CacheSize:           *hashCacheSize,
}
```

**Step 8: Remove unused imports**

Remove `sync` import from `hash_worker/worker.go` if `cacheMutex` and related sync primitives are gone. (Keep `sync.WaitGroup` — that's still used for `wg`.)

**Step 9: Run Go tests**

Run: `go test ./... --tags 'json1 fts5' -count=1`
Expected: ALL PASS

**Step 10: Commit**

```bash
git add hash_worker/worker.go hash_worker/config.go main.go go.mod go.sum
git commit -m "perf: replace unbounded hash cache with LRU

The hash worker's in-memory cache is now bounded by a configurable
-hash-cache-size flag (default 100k entries). Uses hashicorp/golang-lru
for thread-safe LRU eviction. Removes the bulk-load-all pattern that
could consume hundreds of MB on large deployments."
```

---

### Task 9: Fix RotateResource — correct FS + create new version

**Finding:** 4a+4b (P2) — rotation ignores alt FS and doesn't update hash.

**Files:**
- Modify: `application_context/resource_media_context.go:1154-1215`

**Context:** `RotateResource` always uses `ctx.fs` (line 1165) even for resources on alt filesystems. After rotation, it overwrites the file in place without updating the hash. Fix: (1) look up correct FS from `resource.StorageLocation`, (2) instead of overwriting in place, create a new version via the versioning system.

**Step 1: Rewrite RotateResource**

Replace lines 1154-1215 in `application_context/resource_media_context.go`:

```go
func (ctx *MahresourcesContext) RotateResource(resourceId uint, degrees int) error {
	var resource models.Resource
	if err := ctx.db.First(&resource, resourceId).Error; err != nil {
		return err
	}

	if !resource.IsImage() {
		return errors.New("not an image")
	}

	// Use correct filesystem for this resource's storage location
	fs, err := ctx.GetFsForStorageLocation(resource.StorageLocation)
	if err != nil {
		return err
	}

	f, err := fs.Open(resource.GetCleanLocation())
	if err != nil {
		return err
	}

	img, _, err := image.Decode(f)
	if err != nil {
		f.Close()
		return err
	}
	f.Close()

	rotatedImage := transform.Rotate(img, float64(degrees), &transform.RotationOptions{ResizeBounds: true})

	var buf bytes.Buffer
	if err := imgio.JPEGEncoder(100)(&buf, rotatedImage); err != nil {
		return err
	}

	// Create a new version with the rotated content instead of overwriting in place.
	// This preserves the original, updates the hash, and respects the versioning system.
	rotatedBytes := buf.Bytes()
	hash := computeSHA1(rotatedBytes)
	contentType := detectContentType(rotatedBytes)
	width, height := getDimensionsFromContent(rotatedBytes, contentType)
	ext := getExtensionFromFilename(resource.Name, contentType)
	if ext == "" {
		ext = ".jpg" // Rotation always re-encodes as JPEG
	}
	location := buildVersionResourcePath(hash, ext)

	// Store the rotated file (deduplication: skip if already exists)
	if exists, _ := afero.Exists(ctx.fs, location); !exists {
		if err := ctx.storeVersionFile(location, rotatedBytes); err != nil {
			return err
		}
	}

	// Ensure resource has versions (lazy migration)
	var versionCount int64
	ctx.db.Model(&models.ResourceVersion{}).Where("resource_id = ?", resourceId).Count(&versionCount)
	if versionCount == 0 {
		v1 := models.ResourceVersion{
			ResourceID:      resourceId,
			VersionNumber:   1,
			Hash:            resource.Hash,
			HashType:        resource.HashType,
			FileSize:        resource.FileSize,
			ContentType:     resource.ContentType,
			Width:           resource.Width,
			Height:          resource.Height,
			Location:        resource.Location,
			StorageLocation: resource.StorageLocation,
			Comment:         "Original (before rotation)",
		}
		if err := ctx.db.Create(&v1).Error; err != nil {
			return fmt.Errorf("failed to create initial version: %w", err)
		}
	}

	// Get next version number
	var maxVersion int
	ctx.db.Model(&models.ResourceVersion{}).Where("resource_id = ?", resourceId).Select("COALESCE(MAX(version_number), 0)").Scan(&maxVersion)

	version := models.ResourceVersion{
		ResourceID:    resourceId,
		VersionNumber: maxVersion + 1,
		Hash:          hash,
		HashType:      "SHA1",
		FileSize:      int64(len(rotatedBytes)),
		ContentType:   contentType,
		Width:         uint(width),
		Height:        uint(height),
		Location:      location,
		Comment:       fmt.Sprintf("Rotated %d degrees", degrees),
	}

	tx := ctx.db.Begin()
	if err := tx.Create(&version).Error; err != nil {
		tx.Rollback()
		return err
	}

	resourceUpdates := map[string]interface{}{
		"current_version_id": version.ID,
		"hash":               version.Hash,
		"location":           version.Location,
		"storage_location":   version.StorageLocation,
		"content_type":       version.ContentType,
		"width":              version.Width,
		"height":             version.Height,
		"file_size":          version.FileSize,
	}
	if err := tx.Model(&models.Resource{}).Where("id = ?", resourceId).Updates(resourceUpdates).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Delete cached thumbnails
	if err := tx.Where("resource_id = ?", resourceId).Delete(&models.Preview{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit().Error; err != nil {
		return err
	}

	ctx.OnResourceFileChanged(resourceId)

	return nil
}
```

Note: This uses the VersionUploadLock added in Task 2 implicitly — the lock is keyed on resource ID. Since RotateResource now does version operations, it should also acquire the lock. Add at the start of the function:

```go
ctx.locks.VersionUploadLock.Acquire(resourceId)
defer ctx.locks.VersionUploadLock.Release(resourceId)
```

**Step 2: Run Go tests then E2E tests**

Run: `go test ./... --tags 'json1 fts5' -count=1`
Then: `cd e2e && npm run test:with-server`
Expected: ALL PASS

**Step 3: Commit**

```bash
git add application_context/resource_media_context.go
git commit -m "fix: RotateResource uses correct FS and creates new version

RotateResource now looks up the correct filesystem from StorageLocation
instead of always using ctx.fs. After rotation, it creates a new version
with the rotated content instead of overwriting in place. This preserves
the original, updates the hash, and integrates with the versioning system."
```

---

## Summary

| Task | Finding | Type | Key Change |
|------|---------|------|------------|
| 1 | 1a | P0 fix | Hold lock until fn completes |
| 2 | 1c+1d | P1 fix | VersionUploadLock per resource |
| 3 | 2a | P1 perf | GetTagByID/GetResourceByID/GetGroupByID |
| 4 | 2b | P2 perf | Batch SQL for bulk tag operations |
| 5 | 2c | P2 perf | JOIN+GROUP BY for note scope |
| 6 | 2d | P2 perf | Direct SQL for merge transfers |
| 7 | 1b | P0 fix | Two-phase bulk delete (DB then FS) |
| 8 | 3a | P1 perf | LRU hash cache with configurable size |
| 9 | 4a+4b | P2 fix | Correct FS + version on rotation |

**Run full test suite after all tasks:**

```bash
go test ./... --tags 'json1 fts5' -count=1
cd e2e && npm run test:with-server
```
