# Bulk resource uploads

The create-resource form posts every selected file in one multipart body, which
the server buffers with `ParseMultipartForm`. Above a threshold that is minutes
of a page that looks hung, no per-file outcome and no cancel — and one
`MaxBytesReader` budget for the whole batch, so exceeding it wastes the entire
transfer. Above `upload_widget_file_threshold` files **or**
`upload_widget_size_threshold` bytes, `src/components/resourceUpload.js`
intercepts the submit and sends one request per file instead,
`upload_concurrency` at a time.

- **The drop/paste modal uses the same setting.** `src/components/pasteUpload.js`
  runs a worker pool of `upload_concurrency` over one snapshot of the pending
  items; the modal partial carries the value as `data-upload-concurrency`
  (default 3 if absent). Folder groups are created once per run: workers that
  need the same folder await one shared creation promise (`folderRuns`), so
  parallel files never make duplicate groups and a failed folder is reported
  once. `drop-upload.spec.ts` pins both (peak in-flight above 1 and within the
  setting, one group POST for eight files).
- **The endpoint is unchanged.** `POST /v1/resource` still accepts many files per
  request and still loops them; the split is a browser-side decision. Below the
  threshold, or with JavaScript unavailable, the native post is what happens — so
  the no-JS path, the API, the CLI and every existing create-form test are
  untouched, and the rarely-exercised path is the *new* one, not the old one.
- **XMLHttpRequest, not fetch**, because `fetch` has no upload-progress event.
  It is the only XHR in `src/`, so it is outside `src/csrf.js`'s `window.fetch`
  wrapper and sets `X-CSRF-Token` itself. `csrfToken()` therefore lives in
  `src/utils/csrfToken.js`, separate from `csrf.js`, whose import installs the
  wrapper and a document listener — importing that from a vitest suite would run
  both. It also sets `Accept: application/json`, without which the endpoint
  answers a 302 that XHR follows silently.
- **The payload is snapshotted once**, as `[...new FormData(form).entries()]`
  minus `resource`, and replayed per file. `FormData(form)` reproduces native
  submission exactly: it skips the autocompleters' `disabled` empty sentinels and
  picks up the light-DOM hidden input `schema-form-mode` appends for `Meta`.
  Taxonomy travels as ids (creatable selectors persist the entity first), so
  replaying is idempotent.
- **The submit handler is registered on `document`, not on the form.**
  `schema-form-mode` registers its own submit listener on that same form and
  calls `preventDefault()` + `stopPropagation()` when the Meta schema fails
  validation. It is created inside an `x-if`, so it connects *after* Alpine has
  wired the form — and listeners on one element fire in registration order, so an
  `@submit` there would run first and upload the batch past a visible validation
  error. Listening on an ancestor makes `stopPropagation()` do what it says; the
  `event.defaultPrevented` check that remains is a second line, not the
  mechanism. `TestPhantom`-style reasoning applies to the guard too: the e2e that
  covers it uploads eleven files against a required-field schema and asserts zero
  requests, and it was verified failing with the handler moved back onto the form.
- **`max_upload_size` becomes a per-file bound** under the widget, which is the
  point rather than a regression — it bounds a *request*, and a request is now
  one file. Each file is pre-checked in the browser, because the server's own
  answer is a raw `MaxBytesError` string at HTTP 400, not worth transferring a
  gigabyte to receive.
- **Only in-flight and failed files get a row.** A batch can be hundreds of
  files; completed ones collapse to a count. Progress is aggregated over
  **bytes**, not files completed, or one 4 GB file among nine small ones would
  sit at 0% and jump to 90%.
- **Deterministic failures are excluded from "Retry failed".** Every 4xx except
  the codes that mean "try again" (408, 423, 425, 429) answers identically
  however many times the same bytes are sent — a 409 duplicate, a 413 the browser
  refused on size, a 400 the server could not decode. A duplicate's row links to
  the resource it collided with instead, which is the only useful action left.
  Transport failures and 5xx stay retryable.
- **A partial batch does not navigate.** Full success of a single file goes to
  that resource, mirroring the server's own single-file redirect; a full batch
  goes to `/group?id=<owner>`, or to `/resources` when no owner was chosen — the
  server's own multi-file redirect is `/group?id=0` there, which is a dead page.
- **The three settings are runtime-only** — no flag, no env var, no
  `MahresourcesConfig` field, following `hash_backfill_paused`. They govern
  browser behaviour on one page, so a restart to change one would be the wrong
  shape. They are read through context accessors rather than `Settings()`, since
  a context built from a bare config would publish 0, which reads as "every
  selection crosses the threshold" and "start no workers".

**`AddResource` had to be made concurrency-safe first.** It opened a *deferred*
transaction, made its first statement a read (the content-hash lookup), copied
the entire file body, and only then wrote. In WAL, promoting a read snapshot to a
write after another connection has committed returns `SQLITE_BUSY_SNAPSHOT`, for
which SQLite **does not invoke the busy handler** — so `busy_timeout` did not
apply, nothing retried, and the caller got HTTP 500 on bytes already written to
disk. The sequential loop hid this completely: within one request goroutine, two
`AddResource` transactions never overlap. It is now three phases —

1. the hash existence check, outside any transaction (the per-hash idlock is what
   guarantees the dedup invariant, and it is released only after the winner
   commits, so an autocommit read sees it; the collision branches take their own
   short transactions in `mergeIntoExistingResource`, which validate their
   association ids **inside** theirs — see below);
2. the filesystem write, outside any transaction (an orphan file on a later
   failure is a state that already existed, and the `Stat`-then-reuse branch
   already handles it);
3. `insertUploadedResource`, whose **first statement is a write**.

A read that fails for any reason other than `gorm.ErrRecordNotFound` is
**returned, not treated as "no such content"**: falling through on a transient
failure would persist a second row for content that already exists.

**Deduplication is process-local, and a unique index on `hash` cannot fix that.**
`Resource.Hash` carries a plain `gorm:"index"` and the per-hash lock is in
memory, so two processes sharing one database hold two locks, can both read "not
found" for the same bytes, and both insert. A partial unique index over
non-empty hashes was built for exactly this, and the test suite proved it
unsound: **version uploads legitimately give two resources the same hash.**
`AddResourceVersion` updates `resources.hash` to the new version's hash
(`resource_version_context.go:184-196`) and does not dedupe, so resource 1 can
version-upload content X while resource 2 is later created from the file that
still hashes to whatever 1 used to hold — and then version-uploaded to X too.
Eight `mr resource version-*` doctests plus `resource-versioning.spec.ts` fail
on the index within one shared server, which is how this was found rather than
reasoned about.

So "one resource per content hash" is a rule `AddResource` applies **at create
time**, not an invariant the schema can hold. Closing the cross-process race
needs a claim keyed on hash with its own lifetime — a distributed lock, with
stale-claim recovery — not a constraint. That has not been built; the race
requires a multi-process deployment *and* two simultaneous uploads of identical
new content, and `TestAddResource_ConcurrentSameHashOnWAL` pins the in-process
guarantee under the production SQLite configuration.

**Deduplication counts only what the uploader can see.** The phase-1 lookup runs
on the caller's handle, so for a group-limited principal the scope callback
confines it to their subtree. Content held only by resources outside it is new
content to them: they get a resource of their own, inside their subtree, over the
same file when both are on one storage location (the path is content-addressed,
so the `Stat` branch reuses it; another alternative filesystem gets its own copy). The
collision branches can therefore only write to, or report, a resource the caller
may see; anything else is a write outside the subtree plus an answer about what
the library holds there. Background downloads get the same rule because the
worker binds the submitter's account as it stands once the body is copied
(`WithActorUserID` and `RebindSubmitter`, through
`principalForPluginActor`, so a deleted, disabled or unreadable account, or one
whose role no longer writes, binds deny-all). It used to bind the actor id alone, unscoped, and a scoped user's
download attached their group to a resource they could not open and reported it
as created. Two rows over one file make the reference count a question about
every row: `CountHashReferences` counts through `unscopedDB()`. Bound to a scoped
deleter it saw none of the other subtree's rows, and a resource with no version
rows (from before versioning, or kept by `-skip-version-migration`) lost its file
when the scoped copy was deleted. Every removal goes through
`removeIfUnreferenced`, which counts **after the delete commits, under the
per-hash upload lock**, and nothing counts inside the delete's transaction any
more. A count inside it misses what commits later: on Postgres two deletes of the
last two rows over one file each saw the other's row and both kept the file. A
count outside the lock misses an upload of the same bytes that landed between the
commit and the unlink and reused the file about to go. The four version writers
(upload, rotate, crop, trim) do not take that lock, so the second gap stays open
for them. `AddResource` releases the lock at its commit, before its synchronous
after-create hooks: a hook that deletes a resource over those bytes, or uploads
them again, would otherwise wait on its own upload forever. **The consequence is
accepted dedup semantics, not a defect:** an upload of the same bytes that runs
while those hooks run is answered with the committed row (the collision branches
merge onto it or report it), and if a hook then deletes that row, the second
upload's answer names a resource that no longer exists. That is the outcome of
deduplicating onto any resource that is deleted a moment later, which dedup has
always allowed; the state stays consistent, because the removal counts under the
lock and so never leaves a row without its file or a file without a row. Holding
the lock across the hooks instead would run arbitrary plugin code inside the dedup
critical section and deadlock the hooks above. `resource_upload_scope_test.go` and its `_pg` twin pin all of this.

**The collision branches validate their association ids *inside* their
transaction**, and handle contention by retrying (`withUploadTxRetry`) rather
than by becoming write-first. Hoisting those reads out was tried and is wrong:
`Association.Append` upserts its target, so a group deleted between the check and
the append is **recreated as a blank stub** — and no foreign key objects, because
by then the row exists again. The different-owner branch validates the owner id
for the same reason; master validated nothing there at all.

Placement alone is not enough on Postgres. A `COUNT` inside a READ COMMITTED
transaction is still check-then-act: the delete commits and is immediately
visible. `ValidateAndLockAssociationIDs` therefore takes `SELECT ... FOR UPDATE`
on the rows it validated, so the deleter waits for the transaction to end. The
clause is Postgres-only — SQLite serializes writers already and rejects the
syntax, which also means **SQLite cannot exhibit this bug and cannot test it**:
`TestDeleteRacingAValidatedGroupIsRefusedPG` is a Postgres test, and it
resurrects a blank group the moment the lock clause is removed.

**Phase 3 holds the writer lock from its `Begin()`** (see [SQLite transactions take the writer lock at
BEGIN](sqlite-transactions.md)), so a read inside it can no longer be
refused by a commit landing first; what keeps phases 1 and 2 outside it now is
that the lock is not held across the file copy.
`TestAddResource_ConcurrentDistinctHashes` (and its constrained-pool twin, which
mirrors the e2e harness's `-max-db-connections=2`) covers the concurrent shape.
Under the old deferred `BEGIN`, 6–7 of 8 concurrent distinct-file uploads failed
with a read first, and a residue survived even with the INSERT first: 31 failed
attempts in 400 uploads at concurrency 4 over HTTP, 166–178 in 1,000 at
concurrency 8. With the lock taken at BEGIN, before any statement is prepared,
that measured 0 in 4,400 uploads.
Phase 3 is still retried a bounded number of times on `isLockContentionError`,
for a `busy_timeout` that expires under a long write elsewhere; that is safe
because a failed attempt rolled back and the file is already on disk, and
`res.ID` is reset each attempt or a re-run `Save` would be an UPDATE of a row the
rollback removed.
