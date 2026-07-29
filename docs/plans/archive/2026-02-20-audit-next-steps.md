# Codebase Audit: Next Steps Report

> Generated from a full static analysis, logic/concurrency, and performance audit of the mahresources codebase.
> Committed fixes: `cefd7ac` (15 patches across 10 files).
> This document covers the **16 remaining findings** that require architectural decisions before implementation.

---

## 1. Concurrency & Data Integrity

### 1a. `RunWithLockTimeout` releases lock while work continues (CRITICAL)

**File:** `lib/id_lock.go:296-319`

When `RunWithLockTimeout` times out, it releases the lock via `defer`, but the goroutine running `fn()` continues executing. A second caller can then acquire the same lock and run concurrently, violating mutual exclusion. This affects thumbnail generation and any ffmpeg operations.

**Decision needed:** Should the timeout:
- (a) Cancel the underlying context (requires `fn` to accept a context)
- (b) Keep the lock held until `fn` completes (no timeout guarantee)
- (c) Use context cancellation only with no lock release on timeout

---

### 1b. `BulkDeleteResources` — FS operations not rolled back on DB transaction failure (CRITICAL)

**File:** `application_context/resource_bulk_context.go:271-280`

Inside `WithTransaction`, each `DeleteResource` performs file system operations (backup copy, file deletion). If deletion N+1 fails and the transaction rolls back, the first N files are already gone from disk but their DB records are restored — leaving orphaned DB records pointing to missing files.

**Decision needed:** Should bulk delete:
- (a) Perform file operations after the transaction commits (two-phase: mark-then-sweep)
- (b) Use soft deletes in the DB first and a background cleanup worker
- (c) Accept the risk and document it

---

### 1c. Version number race condition in `UploadNewVersion` (HIGH)

**File:** `application_context/resource_version_context.go:130-133`

The `MAX(version_number)` query runs on `ctx.db` instead of the transaction `tx`. Two concurrent uploads get the same max version, creating duplicate version numbers.

**Decision needed:** Should the fix:
- (a) Move the query inside the transaction with `FOR UPDATE` locking (Postgres only)
- (b) Add a unique constraint on `(resource_id, version_number)` and retry on conflict
- (c) Use the existing `ResourceHashLock` to serialize per-resource

---

### 1d. Lazy migration in `UploadNewVersion` not transactional (HIGH)

**File:** `application_context/resource_version_context.go:109-128`

The "create v1 if none exists" migration uses `ctx.db` outside the transaction. Concurrent calls can both create v1, or a crash between v1 creation and the new version upload leaves inconsistent state.

**Decision needed:** This should likely be folded into the same transaction as the new version creation (1c), but needs validation that it doesn't conflict with the migration startup path.

---

### 1e. Download manager `Shutdown` doesn't wait for goroutines (MEDIUM)

**File:** `download_queue/manager.go:550-561`

`Shutdown()` cancels contexts and closes the `done` channel but doesn't `sync.WaitGroup.Wait()` for `processJob` goroutines. Post-shutdown, goroutines may still access shared state.

**Decision needed:** Add a `WaitGroup` for active jobs, or accept that the application is exiting and the OS will clean up?

---

## 2. N+1 Query Patterns

> Largest performance impact at scale. The CLAUDE.md notes "some deployments deal with millions of resources."

### 2a. `clause.Associations` over-preloads everywhere (HIGH)

**Files:** `tags_context.go:29`, `resource_crud_context.go:17`, `group_crud_context.go:161-184`

`GetTag` loads all Resources, Notes, Groups. `GetResource` loads 10+ associations. `GetGroup` fires 17 separate preload queries. These are called in bulk operation loops, multiplying the cost.

**Decision needed:** Two approaches:
- (a) Create lightweight `GetTagByID` / `GetResourceByID` methods that skip preloads for internal use (bulk ops, merge), keeping the full preload versions for API/template rendering
- (b) Make preloads opt-in via a functional options pattern like `GetTag(id, WithPreload("Resources"))`

---

### 2b. Bulk tag operations do individual association updates (HIGH)

**Files:** `resource_bulk_context.go:121-172`, `group_bulk_context.go:166-205`

Adding/removing tags iterates each entity and calls `Association("Tags").Append/Delete` individually — one SQL statement per entity.

**Decision needed:** Replace with batch SQL?
```sql
INSERT INTO resource_tags (resource_id, tag_id)
SELECT ?, tag_id FROM ... WHERE ...
```
This would bypass GORM's association API but dramatically reduce roundtrips.

---

### 2c. Correlated subqueries in note and group scopes (MEDIUM)

**Files:** `models/database_scopes/note_scope.go:18-24`, `group_scope.go:45-105`

Note tag filtering uses `(SELECT COUNT(*) FROM note_tags WHERE ... AND note_id = notes.id) = ?` — a correlated subquery per row. The resource scope already uses the more efficient JOIN + GROUP BY + HAVING pattern.

**Decision needed:** Align note and group scopes to match the resource scope pattern? This is straightforward but needs testing against both SQLite and PostgreSQL to ensure the query planner handles it correctly.

---

### 2d. Merge operations preload all associations just for IDs (MEDIUM)

**Files:** `resource_bulk_context.go:317-318`, `group_bulk_context.go:33-39`

Merging loads all associations of every loser entity, but only uses the IDs for raw SQL INSERTs.

**Decision needed:** Replace with direct SQL subqueries that transfer associations without loading into Go memory?
```sql
INSERT INTO winner_tags
SELECT winner_id, tag_id FROM loser_tags WHERE resource_id IN (?)
```

---

## 3. Memory & Scale

### 3a. Hash cache loads all records and grows unboundedly (HIGH)

**File:** `hash_worker/worker.go:295-318`

`ensureCacheLoaded` does `Find(&hashes)` with no LIMIT, loading every `ImageHash` row into memory. For millions of resources, this is hundreds of MB. The cache also never evicts entries for deleted/changed resources.

**Decision needed:**
- (a) Use a bounded LRU cache with a configurable max size
- (b) Eliminate the in-memory cache and query the DB for similarity comparisons (slower but constant memory)
- (c) Use a locality-sensitive hashing index (e.g., VP-tree) that can be loaded in pages
- (d) Add a configurable threshold: use in-memory cache below N records, switch to DB queries above

---

### 3b. `processFileForVersion` buffers entire file in memory (MEDIUM)

**File:** `application_context/resource_version_context.go:204-225`

`io.ReadAll(file)` loads the entire uploaded file into memory. A 500MB video upload holds 500MB in a Go slice.

**Decision needed:** Align with the `AddResource` pattern that streams to a temp file first, then hashes from disk? This is a straightforward refactor.

---

### 3c. `MigrateResourceVersions` individual queries per resource (MEDIUM)

**File:** `application_context/resource_version_context.go:692-753`

The one-time migration does 2-3 queries per resource. For 1M resources, that's 2-3M roundtrips.

**Decision needed:** Batch the INSERT and UPDATE operations (e.g., multi-row INSERT for versions, single UPDATE...FROM for current_version_id)? This is a one-time migration so urgency is lower.

---

## 4. File System Consistency

### 4a. `RotateResource` ignores alt file systems (HIGH)

**File:** `application_context/resource_media_context.go:1165`

Always uses `ctx.fs`. Resources stored in alt filesystems fail silently or operate on wrong files.

**Decision needed:** Straightforward fix — look up the correct filesystem from `resource.StorageLocation` before any file operations. Same pattern already used in `fetchICSFromResource`.

---

### 4b. `RotateResource` doesn't update resource hash after rotation (HIGH)

**File:** `application_context/resource_media_context.go:1154-1215`

After JPEG re-encoding (rotation), the file content changes but `resource.Hash` is never updated. This breaks deduplication and `CountHashReferences` (used during deletion to decide if the underlying file can be removed).

**Decision needed:** Should the rotation:
- (a) Recompute the hash and update the DB (simple but changes the resource identity)
- (b) Also create a new version (aligns with the versioning system)
- (c) Store the rotated file at a new hash-based path and update the location

Option (b) is most consistent with the existing architecture.

---

## Recommended Priority Order

| Priority | Issue | Rationale |
|----------|-------|-----------|
| **P0** | 1a (lock timeout), 1b (bulk delete FS) | Data corruption / data loss risk in production |
| **P1** | 2a (N+1 preloads), 3a (hash cache OOM) | Scale blockers for large deployments |
| **P1** | 1c+1d (version races) | Data integrity under concurrent use |
| **P2** | 4a+4b (rotation FS/hash) | Correctness for alt-FS users |
| **P2** | 2b+2c+2d (bulk query patterns) | Performance at scale |
| **P3** | 3b+3c (memory/migration) | Edge cases and one-time operations |
| **P3** | 1e (shutdown) | Only matters during process termination |
