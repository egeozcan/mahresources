# Audit P0-P2 Fixes: Design Document

> Addresses 10 of 16 findings from the codebase audit (P0 through P2).
> Reference: `docs/plans/audit-next-steps.md`

---

## Scope

| Priority | Findings | Count |
|----------|----------|-------|
| P0 | 1a (lock timeout), 1b (bulk delete FS) | 2 |
| P1 | 1c+1d (version races), 2a (N+1 preloads), 3a (hash cache OOM) | 3 |
| P2 | 2b+2c+2d (bulk query patterns), 4a+4b (rotation FS/hash) | 5 |

Excluded (P3): 1e (shutdown wait), 3b (file buffering), 3c (migration batching).

---

## Section 1: Concurrency & Data Integrity

### 1a. Lock timeout — hold lock until fn completes

**Decision:** Keep the lock held for the full duration of `fn()` execution.

**Changes to `lib/id_lock.go`:**
- `RunWithLockTimeout`: remove the timeout-based lock release path.
- Context cancellation still signals `fn` to stop, but the lock is not released until the goroutine actually exits.
- Callers may wait longer than `runTimeout`, but mutual exclusion is guaranteed.
- The `lockTimeout` for acquiring the lock remains unchanged.

### 1b. Bulk delete — two-phase mark-then-sweep

**Decision:** DB transaction only performs DB operations. File deletions happen after commit.

**Changes to `application_context/resource_bulk_context.go`:**
- Phase 1 (inside transaction): `DeleteResource` performs only DB operations. Collect file paths scheduled for deletion into a slice.
- Phase 2 (after `tx.Commit()` succeeds): iterate the collected paths and delete files from disk.
- If the transaction rolls back, no files are touched.
- Requires refactoring `DeleteResource` to separate DB-only logic from FS operations, or having the bulk path collect paths and defer cleanup.

### 1c+1d. Version races — serialize with ResourceHashLock

**Decision:** Use the existing `ResourceHashLock` (IDLock) to serialize per-resource version operations.

**Changes to `application_context/resource_version_context.go`:**
- Wrap the entire `UploadNewVersion` flow inside `RunWithLock` keyed on the resource ID.
- The locked section includes: check for existing versions, create v1 lazy migration if needed, compute next version number, insert new version, update `current_version_id`.
- Both the lazy migration (1d) and the version number query (1c) now run under the same lock.
- No DB-specific locking (FOR UPDATE) needed. Works on both SQLite and Postgres.

---

## Section 2: N+1 Query Patterns & Performance

### 2a. Lightweight ByID methods

**Decision:** Add no-preload methods for internal use.

**New methods:**
- `GetTagByID(id uint) (*models.Tag, error)` — `db.First(&tag, id)`, no preloads.
- `GetResourceByID(id uint) (*models.Resource, error)` — same pattern.
- `GetGroupByID(id uint) (*models.Group, error)` — same pattern.

**Callers to update:**
- `BulkAddTagsToResources` / `BulkRemoveTagsFromResources` (resource_bulk_context.go)
- `BulkAddTagsToGroups` / `BulkRemoveTagsFromGroups` (group_bulk_context.go)
- `MergeResources` / `MergeGroups` (where only IDs are needed)
- Any internal code that calls `GetTag`/`GetResource`/`GetGroup` without needing associations.

Existing full-preload methods (`GetTag`, `GetResource`, `GetGroup`) remain unchanged for template/API use.

### 2b. Batch SQL for bulk tag operations

**Decision:** Replace per-entity association loops with batch SQL.

**Changes:**
- `BulkAddTagsToResources`: replace loop with batch INSERT:
  ```sql
  INSERT INTO resource_tags (resource_id, tag_id)
  SELECT r.id, t.id FROM (VALUES ...) AS r(id), (VALUES ...) AS t(id)
  WHERE NOT EXISTS (SELECT 1 FROM resource_tags WHERE resource_id = r.id AND tag_id = t.id)
  ```
- `BulkRemoveTagsFromResources`: replace loop with batch DELETE:
  ```sql
  DELETE FROM resource_tags WHERE resource_id IN (?) AND tag_id IN (?)
  ```
- Same pattern for `group_tags` (group_bulk_context.go) and `note_tags`.
- Bypasses GORM association API. Reduces N roundtrips to 1-2.

### 2c. Align note/group scopes to JOIN+GROUP BY

**Decision:** Replace correlated subqueries with the efficient pattern already used in resource scopes.

**Changes to `models/database_scopes/note_scope.go`:**
- Replace `(SELECT COUNT(*) FROM note_tags WHERE ... AND note_id = notes.id) = ?` with:
  ```sql
  JOIN note_tags ON note_tags.note_id = notes.id AND note_tags.tag_id IN (?)
  GROUP BY notes.id
  HAVING COUNT(DISTINCT note_tags.tag_id) = ?
  ```

**Changes to `models/database_scopes/group_scope.go`:**
- Apply same pattern where applicable. The parent/child tag search logic adds complexity — keep that logic but restructure the base tag matching to use JOIN+GROUP BY.
- Test against both SQLite and PostgreSQL to ensure query planner handles it correctly.

### 2d. Direct SQL for merge association transfers

**Decision:** Replace Go-side association loading with direct SQL transfers.

**Changes to `resource_bulk_context.go` (MergeResources):**
- Remove `Preload(clause.Associations).Find(&losers)`.
- Replace with direct SQL for each association type:
  ```sql
  INSERT OR IGNORE INTO resource_tags (resource_id, tag_id)
  SELECT ?, tag_id FROM resource_tags WHERE resource_id IN (?)
  ```
- Same pattern for notes, groups, and other associations.

**Changes to `group_bulk_context.go` (MergeGroups):**
- Same approach: direct SQL transfers instead of loading associations into Go memory.

---

## Section 3: Memory & Scale

### 3a. Bounded LRU hash cache

**Decision:** Replace unbounded map with configurable LRU cache.

**Changes to `hash_worker/worker.go`:**
- Replace `hashCache map[uint]uint64` with an LRU cache (e.g., `hashicorp/golang-lru` or simple hand-rolled LRU).
- New config flag: `-hash-cache-size` (default: 100,000 entries).
- Remove `ensureCacheLoaded` and the `cacheLoaded` flag entirely.
- Populate cache lazily: when a hash is computed or queried, add it to the cache.
- On cache miss during similarity check, query the DB for that resource's hash.
- Invalidate cache entries when resources are deleted.
- **Bug fix:** the silent failure where `cacheLoaded = true` was set on DB error is eliminated by removing the bulk-load pattern entirely.

---

## Section 4: File System Consistency

### 4a+4b. Rotation — correct FS + new version

**Decision:** Fix alt-FS lookup and create a new version on rotation.

**Changes to `application_context/resource_media_context.go` (RotateResource):**
1. Look up the correct filesystem from `resource.StorageLocation` (same pattern as `fetchICSFromResource`).
2. After rotation, instead of overwriting the file in place:
   - Save rotated image to a temp file.
   - Call into the version system to create a new version with the rotated content.
   - The new version gets its own hash and file path.
   - The original version is preserved.
3. Preview cache invalidation stays as-is.
4. This fixes both the alt-FS issue (always uses `ctx.fs` currently) and the stale hash issue (new version = new hash).

---

## Implementation Order

1. **1a** (lock timeout) — small, isolated change. Unblocks confidence in all lock-dependent code.
2. **1c+1d** (version races) — depends on understanding the lock pattern from 1a.
3. **2a** (lightweight ByID methods) — prerequisite for 2b and 2d.
4. **2b** (batch tag SQL) — uses ByID methods from 2a.
5. **2c** (scope alignment) — independent query change.
6. **2d** (merge SQL) — uses ByID pattern from 2a.
7. **1b** (bulk delete two-phase) — larger refactor, benefits from 2a being done.
8. **3a** (LRU hash cache) — independent, can be done in parallel with others.
9. **4a+4b** (rotation) — depends on version system working correctly (1c+1d).

---

## Out of Scope

- 1e: Download manager shutdown (P3 — only matters during process termination)
- 3b: File buffering in processFileForVersion (P3 — edge case)
- 3c: MigrateResourceVersions batching (P3 — one-time migration)
