# KAN Audit Bugfixes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix all 18 in-progress security/data-integrity/stability bugs from the KAN Jira project codebase audit.

**Architecture:** Each fix is independent and targeted at specific files. Security fixes (SQL injection, XSS, credential logging) come first, then data integrity (cascade deletes, zero-ID guards), concurrency (data races, goroutine leaks), and infrastructure (graceful shutdown, Docker).

**Tech Stack:** Go (GORM, Gorilla Mux, sqlx), JavaScript (Alpine.js, Vite), Docker, SQLite/PostgreSQL

---

### Task 1: KAN-9 — Remove DSN password logging

**Files:**
- Modify: `application_context/context.go:455`

**Step 1: Fix the logging line**

At line 455, replace:
```go
fmt.Printf("DB_TYPE %v DB_DSN %v FILE_SAVE_PATH %v\n", dbType, dbDsn, cfg.FileSavePath)
```
with:
```go
fmt.Printf("DB_TYPE %v FILE_SAVE_PATH %v\n", dbType, cfg.FileSavePath)
```

**Step 2: Build to verify**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add application_context/context.go
git commit -m "fix(KAN-9): remove DSN password from startup log output"
```

---

### Task 2: KAN-12 — URL-encode error message in redirect

**Files:**
- Modify: `server/api_handlers/relation_api_handlers.go:96-101`

**Step 1: Add url import and fix the redirect**

Add `"net/url"` to the imports. Then at the redirect code (around line 96), replace:
```go
		backUrl := fmt.Sprintf(
			"/relation/new?FromGroupId=%v&ToGroupId=%v&GroupRelationTypeId=%v&Error=%v",
			editor.FromGroupId, editor.ToGroupId, editor.GroupRelationTypeId,
			err.Error(),
		)
```
with:
```go
		backUrl := fmt.Sprintf(
			"/relation/new?FromGroupId=%v&ToGroupId=%v&GroupRelationTypeId=%v&Error=%v",
			editor.FromGroupId, editor.ToGroupId, editor.GroupRelationTypeId,
			url.QueryEscape(err.Error()),
		)
```

**Step 2: Build to verify**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add server/api_handlers/relation_api_handlers.go
git commit -m "fix(KAN-12): URL-encode error message in relation redirect"
```

---

### Task 3: KAN-24 — Cap search limit

**Files:**
- Modify: `server/api_handlers/search_api_handlers.go:17`

**Step 1: Add limit cap**

Replace line 17:
```go
			Limit: int(http_utils.GetIntQueryParameter(request, "limit", 20)),
```
with:
```go
			Limit: min(int(http_utils.GetIntQueryParameter(request, "limit", 20)), 200),
```

**Step 2: Build to verify**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add server/api_handlers/search_api_handlers.go
git commit -m "fix(KAN-24): cap search limit to 200 results"
```

---

### Task 4: KAN-7 — Block javascript: protocol in renderMarkdown links

**Files:**
- Modify: `src/components/blockEditor.js:47`

**Step 1: Add URL validation to the link regex replacement**

Replace the link regex line (around line 47):
```javascript
escaped = escaped.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" class="text-blue-600 hover:underline" target="_blank" rel="noopener">$1</a>');
```
with:
```javascript
escaped = escaped.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (match, text, href) => {
    const trimmed = href.trim().toLowerCase();
    if (trimmed.startsWith('javascript:') || trimmed.startsWith('data:') || trimmed.startsWith('vbscript:')) {
        return text;
    }
    return `<a href="${href}" class="text-blue-600 hover:underline" target="_blank" rel="noopener">${text}</a>`;
});
```

**Step 2: Build JS bundle**

Run: `npm run build-js`
Expected: Builds successfully

**Step 3: Commit**

```bash
git add src/components/blockEditor.js
git commit -m "fix(KAN-7): reject javascript:/data:/vbscript: URLs in renderMarkdown"
```

---

### Task 5: KAN-8 — Replace innerHTML with safe DOM methods in bulkSelection

**Files:**
- Modify: `src/components/bulkSelection.js:324`

**Step 1: Replace innerHTML with safe DOM insertion**

Find the line (around 324):
```javascript
form.innerHTML = res;
```

Replace with a DOMParser approach:
```javascript
const parser = new DOMParser();
const doc = parser.parseFromString(res, 'text/html');
form.replaceChildren(...doc.body.childNodes);
```

Then find the nearby `Alpine.initTree(form)` call (should be right after) and keep it as-is.

**Step 2: Build JS bundle**

Run: `npm run build-js`
Expected: Builds successfully

**Step 3: Commit**

```bash
git add src/components/bulkSelection.js
git commit -m "fix(KAN-8): use DOMParser instead of innerHTML for server response"
```

---

### Task 6: KAN-5 — Fix SQL injection in FTS GetRankExpr

**Files:**
- Modify: `fts/sqlite.go:206-241`
- Modify: `fts/postgres.go:137-168`
- Modify: `fts/provider.go:33`

Note: `GetRankExpr` is currently part of the `FTSProvider` interface but NOT called anywhere. The `BuildSearchScope` methods properly use parameterized queries. The fix changes the interface to return both a SQL expression and parameters.

**Step 1: Update the FTSProvider interface**

In `fts/provider.go`, change line 33 from:
```go
	GetRankExpr(tableName string, columns []string, query ParsedQuery) string
```
to:
```go
	GetRankExpr(tableName string, columns []string, query ParsedQuery) (string, []interface{})
```

**Step 2: Fix SQLite GetRankExpr**

In `fts/sqlite.go`, replace the entire `GetRankExpr` method (lines 206-241):
```go
func (s *SQLiteFTS) GetRankExpr(tableName string, columns []string, query ParsedQuery) (string, []interface{}) {
	if query.Term == "" {
		return "0", nil
	}

	ftsTableName := tableName + "_fts"
	escapedTerm := EscapeForFTS(query.Term)

	switch query.Mode {
	case ModeFuzzy:
		return fmt.Sprintf("(1.0 / (1 + length(%s.name)))", tableName), nil

	case ModePrefix:
		terms := strings.Fields(escapedTerm)
		var matchParts []string
		for _, term := range terms {
			matchParts = append(matchParts, term+"*")
		}
		matchExpr := strings.Join(matchParts, " ")

		return fmt.Sprintf(
			"(SELECT -bm25(%s) FROM %s WHERE rowid = %s.id AND %s MATCH ?)",
			ftsTableName, ftsTableName, tableName, ftsTableName,
		), []interface{}{matchExpr}

	default:
		return fmt.Sprintf(
			"(SELECT -bm25(%s) FROM %s WHERE rowid = %s.id AND %s MATCH ?)",
			ftsTableName, ftsTableName, tableName, ftsTableName,
		), []interface{}{escapedTerm}
	}
}
```

**Step 3: Fix PostgreSQL GetRankExpr**

In `fts/postgres.go`, replace the entire `GetRankExpr` method (lines 137-168):
```go
func (p *PostgresFTS) GetRankExpr(tableName string, columns []string, query ParsedQuery) (string, []interface{}) {
	if query.Term == "" {
		return "0", nil
	}

	escapedTerm := EscapeForFTS(query.Term)

	switch query.Mode {
	case ModeFuzzy:
		return fmt.Sprintf("similarity(%s.name, ?)", tableName), []interface{}{escapedTerm}

	case ModePrefix:
		terms := strings.Fields(escapedTerm)
		var tsqueryParts []string
		for _, term := range terms {
			tsqueryParts = append(tsqueryParts, term+":*")
		}
		tsquery := strings.Join(tsqueryParts, " & ")
		return fmt.Sprintf(
			"ts_rank(%s.search_vector, to_tsquery('english', ?))",
			tableName,
		), []interface{}{tsquery}

	default:
		return fmt.Sprintf(
			"ts_rank(%s.search_vector, plainto_tsquery('english', ?))",
			tableName,
		), []interface{}{escapedTerm}
	}
}
```

**Step 4: Build to verify**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 5: Commit**

```bash
git add fts/provider.go fts/sqlite.go fts/postgres.go
git commit -m "fix(KAN-5): parameterize FTS GetRankExpr to prevent SQL injection"
```

---

### Task 7: KAN-10 — Change Category→Groups cascade to SET NULL

**Files:**
- Modify: `models/category_model.go:14`

**Step 1: Change the GORM constraint**

Replace:
```go
	Groups      []*Group `gorm:"foreignKey:CategoryId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
```
with:
```go
	Groups      []*Group `gorm:"foreignKey:CategoryId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
```

**Step 2: Build to verify**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add models/category_model.go
git commit -m "fix(KAN-10): change Category→Groups cascade to SET NULL"
```

---

### Task 8: KAN-11 — Change NoteType→Notes cascade to SET NULL

**Files:**
- Modify: `models/note_type_model.go:13`

**Step 1: Change the GORM constraint**

Replace:
```go
	Notes       []*Note `gorm:"foreignKey:NoteTypeId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
```
with:
```go
	Notes       []*Note `gorm:"foreignKey:NoteTypeId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
```

**Step 2: Build to verify**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add models/note_type_model.go
git commit -m "fix(KAN-11): change NoteType→Notes cascade to SET NULL"
```

---

### Task 9: KAN-13 — Change Group ownership cascades to SET NULL

**Files:**
- Modify: `models/group_model.go:19,27-28`

**Step 1: Change the GORM constraints**

Change the Owner self-reference (line 19) from:
```go
	Owner   *Group `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
```
to:
```go
	Owner   *Group `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
```

Change OwnNotes (line 27) from:
```go
	OwnNotes     []*Note     `gorm:"foreignKey:OwnerId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
```
to:
```go
	OwnNotes     []*Note     `gorm:"foreignKey:OwnerId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
```

Change OwnGroups (line 28) from:
```go
	OwnGroups    []*Group    `gorm:"foreignKey:OwnerId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
```
to:
```go
	OwnGroups    []*Group    `gorm:"foreignKey:OwnerId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
```

**Step 2: Build to verify**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add models/group_model.go
git commit -m "fix(KAN-13): change Group ownership cascades to SET NULL"
```

---

### Task 10: KAN-18 — Add zero-ID guard to delete handlers

**Files:**
- Modify: `server/api_handlers/resource_api_handlers.go:377-378`
- Modify: `server/api_handlers/note_api_handlers.go:86-88`
- Modify: `server/api_handlers/series_api_handlers.go:58-60`

**Step 1: Guard resource delete**

In `resource_api_handlers.go`, after the struct fill (around line 376), add a zero-ID check before the delete call:

After:
```go
		if err := tryFillStructValuesFromRequest(&query, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}
```

Add before the `DeleteResource` call:
```go
		if query.ID == 0 {
			http_utils.HandleError(fmt.Errorf("missing or invalid resource ID"), writer, request, http.StatusBadRequest)
			return
		}
```

Make sure `"fmt"` is in the imports.

**Step 2: Guard note delete**

In `note_api_handlers.go`, after `id := getEntityID(request)` (around line 86), add:
```go
		if id == 0 {
			http_utils.HandleError(fmt.Errorf("missing or invalid note ID"), writer, request, http.StatusBadRequest)
			return
		}
```

Make sure `"fmt"` is in the imports.

**Step 3: Guard series delete**

In `series_api_handlers.go`, after `id := getEntityID(request)` (around line 58), add:
```go
		if id == 0 {
			http_utils.HandleError(fmt.Errorf("missing or invalid series ID"), writer, request, http.StatusBadRequest)
			return
		}
```

Make sure `"fmt"` is in the imports.

**Step 4: Build to verify**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 5: Commit**

```bash
git add server/api_handlers/resource_api_handlers.go server/api_handlers/note_api_handlers.go server/api_handlers/series_api_handlers.go
git commit -m "fix(KAN-18): add zero-ID validation to delete handlers"
```

---

### Task 11: KAN-20 — Fix multi-file upload double-write on error

**Files:**
- Modify: `server/api_handlers/resource_api_handlers.go:149-170`

**Step 1: Remove http.Error inside loop, collect errors instead**

Replace the loop body (lines 149-170):
```go
		for i := range files {
			func(i int) {
				var res *models.Resource
				file, err := files[i].Open()

				if err != nil {
					http.Error(writer, err.Error(), http.StatusInternalServerError)
					return
				}

				defer file.Close()

				name := files[i].Filename

				res, err = effectiveCtx.AddResource(file, name, &creator)
				resources[i] = res

				if err != nil {
					errorMessages = append(errorMessages, err.Error())
				}
			}(i)
		}
```

with:
```go
		for i := range files {
			func(i int) {
				file, err := files[i].Open()

				if err != nil {
					errorMessages = append(errorMessages, err.Error())
					return
				}

				defer file.Close()

				name := files[i].Filename

				res, err := effectiveCtx.AddResource(file, name, &creator)
				resources[i] = res

				if err != nil {
					errorMessages = append(errorMessages, err.Error())
				}
			}(i)
		}
```

The key change: replaced `http.Error()` with appending to `errorMessages`, preventing the double-write. The error aggregation at line 172-177 already handles the response.

**Step 2: Build to verify**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add server/api_handlers/resource_api_handlers.go
git commit -m "fix(KAN-20): collect file open errors instead of writing response in loop"
```

---

### Task 12: KAN-19 — Fix data race on job.ctx/cancel in download queue

**Files:**
- Modify: `download_queue/job.go` (add safe getters)
- Modify: `download_queue/manager.go:394-407`

**Step 1: Add safe getter/setter for ctx and cancel on DownloadJob**

In `download_queue/job.go`, add these methods after the existing methods:

```go
// GetContext safely returns the job's context.
func (j *DownloadJob) GetContext() context.Context {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.ctx
}

// SetContext safely sets the job's context and cancel function.
func (j *DownloadJob) SetContext(ctx context.Context, cancel context.CancelFunc) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.ctx = ctx
	j.cancel = cancel
}

// Cancel safely calls the job's cancel function.
func (j *DownloadJob) Cancel() {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.cancel != nil {
		j.cancel()
	}
}
```

Make sure `"context"` is in the imports of job.go.

**Step 2: Update manager.go Resume to use safe setter**

In `download_queue/manager.go` around line 394-397, replace:
```go
	// Create a new context for the resumed download
	ctx, cancel := context.WithCancel(context.Background())
	job.ctx = ctx
	job.cancel = cancel
```
with:
```go
	// Create a new context for the resumed download
	ctx, cancel := context.WithCancel(context.Background())
	job.SetContext(ctx, cancel)
```

**Step 3: Update all direct accesses to job.ctx and job.cancel in manager.go**

Search for all direct `job.ctx`, `job.cancel` accesses in `manager.go` and replace with the safe methods. Also search for `job.cancel()` calls and replace with `job.Cancel()`.

**Step 4: Build to verify**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 5: Run existing tests**

Run: `go test ./download_queue/...`
Expected: All tests pass

**Step 6: Commit**

```bash
git add download_queue/job.go download_queue/manager.go
git commit -m "fix(KAN-19): protect job.ctx/cancel with mutex via safe accessors"
```

---

### Task 13: KAN-21 — Fix data race in cleanupOldJobs

**Files:**
- Modify: `download_queue/job.go` (add safe getter for CompletedAt)
- Modify: `download_queue/manager.go:532`

**Step 1: Add safe getter for CompletedAt**

In `download_queue/job.go`, add:

```go
// GetCompletedAt safely returns the job's completion time.
func (j *DownloadJob) GetCompletedAt() *time.Time {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.CompletedAt
}
```

**Step 2: Use the safe getter in cleanupOldJobs**

In `manager.go` at line 532, replace:
```go
		if job.CompletedAt != nil && job.CompletedAt.Before(completedCutoff) {
```
with:
```go
		if completedAt := job.GetCompletedAt(); completedAt != nil && completedAt.Before(completedCutoff) {
```

**Step 3: Build and test**

Run: `go build --tags 'json1 fts5' && go test ./download_queue/...`
Expected: Compiles and tests pass

**Step 4: Commit**

```bash
git add download_queue/job.go download_queue/manager.go
git commit -m "fix(KAN-21): use safe getter for CompletedAt in cleanupOldJobs"
```

---

### Task 14: KAN-15 — Fix goroutine leak in timeoutReader

**Files:**
- Modify: `application_context/resource_upload_context.go:95-124`

**Step 1: Refactor timeoutReader.Read to not leak goroutines**

The current design spawns a goroutine per Read() call. On timeout, the goroutine blocks forever trying to send to resultCh. Fix: use a buffered channel (capacity 1) so the goroutine can send even if nobody receives.

Replace the resultCh declaration at line 99:
```go
	resultCh := make(chan readResult, 1)
```

This is actually already a buffered channel (the exploration said `make(chan readResult, 1)`). Let me verify by re-reading — yes, line 99 has `resultCh := make(chan readResult, 1)`. The buffer of 1 means the goroutine CAN send without blocking even if Read() returns early. So the goroutine won't actually leak permanently — it will complete when the underlying reader returns.

The real issue is that the goroutine holds a reference to the buffer `p []byte`, which the caller may reuse. On timeout, the goroutine may still write to `p` after the caller received the timeout error and reused the buffer.

Fix: Copy data through the channel rather than writing directly to the caller's buffer.

Replace the Read method (around lines 86-124):
```go
func (tr *timeoutReader) Read(p []byte) (n int, err error) {
	// Check for existing error
	tr.mu.Lock()
	if tr.err != nil {
		err := tr.err
		tr.mu.Unlock()
		return 0, err
	}
	tr.mu.Unlock()

	// Use an internal buffer to avoid the goroutine writing to p after timeout
	buf := make([]byte, len(p))
	resultCh := make(chan readResult, 1)
	go func() {
		n, err := tr.reader.Read(buf)
		resultCh <- readResult{n, err}
	}()

	for {
		select {
		case result := <-resultCh:
			if result.n > 0 {
				copy(p[:result.n], buf[:result.n])
				tr.mu.Lock()
				tr.lastRead = time.Now()
				tr.mu.Unlock()
			}
			return result.n, result.err
		case <-tr.done:
			return 0, fmt.Errorf("remote server stopped sending data (idle timeout after %v)", tr.idleTimeout)
		default:
			tr.mu.Lock()
			err := tr.err
			tr.mu.Unlock()
			if err != nil {
				return 0, err
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}
```

**Step 2: Build to verify**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add application_context/resource_upload_context.go
git commit -m "fix(KAN-15): use internal buffer in timeoutReader to prevent data race on timeout"
```

---

### Task 15: KAN-23 — Fix cross-DB meta merge precedence inconsistency

**Files:**
- Modify: `application_context/resource_bulk_context.go:492-498`

**Step 1: Fix SQLite meta merge to match PostgreSQL precedence**

The intent is: merge loser's meta INTO winner, with winner's existing keys taking precedence.

- PostgreSQL `loser_meta || winner_meta`: right side (winner) wins — correct
- SQLite `json_patch(winner_meta, loser_meta)`: patch (loser) overwrites target (winner) — wrong

Fix: swap the arguments so winner's meta is the patch (overlay):

Replace line 496:
```go
			err = tx.Exec(`UPDATE resources SET meta = json_patch(meta, coalesce((SELECT meta FROM resources WHERE id = ?), '{}')) WHERE id = ?`, loser.ID, winnerId).Error
```
with:
```go
			err = tx.Exec(`UPDATE resources SET meta = json_patch(coalesce((SELECT meta FROM resources WHERE id = ?), '{}'), meta) WHERE id = ?`, loser.ID, winnerId).Error
```

This reads: "take loser's meta as base, overlay winner's meta on top" → winner's keys take precedence, matching PostgreSQL behavior.

**Step 2: Build to verify**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add application_context/resource_bulk_context.go
git commit -m "fix(KAN-23): align SQLite json_patch argument order with PostgreSQL || precedence"
```

---

### Task 16: KAN-14 — Add graceful shutdown

**Files:**
- Modify: `main.go:373`

**Step 1: Replace log.Fatal with graceful shutdown**

Replace the final line (around 373):
```go
log.Fatal(server.CreateServer(context, mainFs, context.Config.AltFileSystems).ListenAndServe())
```

with:
```go
	srv := server.CreateServer(context, mainFs, context.Config.AltFileSystems)

	// Start server in background
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Give in-flight requests up to 30 seconds to complete
	shutdownCtx, shutdownCancel := stdcontext.WithTimeout(stdcontext.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited cleanly")
```

Add required imports (if not already present):
```go
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
```

Note: since the file likely already imports `"context"` for the application context, you may need to alias the standard library context, e.g. `stdcontext "context"`. Check the existing imports.

**Step 2: Build to verify**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add main.go
git commit -m "fix(KAN-14): add graceful shutdown with signal handling"
```

---

### Task 17: KAN-16 — Run Docker container as non-root user

**Files:**
- Modify: `Dockerfile:23-35`

**Step 1: Add non-root user to the runtime stage**

In the Dockerfile runtime stage (after `FROM alpine:3.19`), add user creation and use it. Replace the runtime stage:

```dockerfile
# Stage 3: Runtime
FROM alpine:3.19
RUN apk add --no-cache sqlite-libs ca-certificates
RUN addgroup -S mahres && adduser -S mahres -G mahres
WORKDIR /app
COPY --from=go-builder /app/mahresources .
COPY --from=go-builder /app/templates ./templates
COPY --from=go-builder /app/public ./public
RUN mkdir -p /app/data /app/files && chown -R mahres:mahres /app
USER mahres
ENV DB_TYPE=SQLITE
ENV DB_DSN=/app/data/test.db
ENV FILE_SAVE_PATH=/app/files
ENV BIND_ADDRESS=0.0.0.0:8181
ENV SKIP_FTS=1
EXPOSE 8181
CMD ["./mahresources"]
```

**Step 2: Commit**

```bash
git add Dockerfile
git commit -m "fix(KAN-16): run Docker container as non-root user"
```

---

### Task 18: KAN-17 — Add visual warning for queries without read-only DB enforcement

**Files:**
- Modify: `application_context/context.go` (add method to check if readOnlyDB is truly read-only)
- Modify: `templates/partials/query_display.html` or equivalent query template (add visual warning)

**Step 1: Add IsReadOnlyEnforced method**

In `application_context/context.go`, add a method:
```go
// IsReadOnlyDBEnforced returns true if the read-only database connection
// has database-level read-only enforcement (e.g., SQLite mode=ro).
func (ctx *MahresourcesContext) IsReadOnlyDBEnforced() bool {
	if ctx.readOnlyDB == nil {
		return false
	}
	// Check if the DSN contains read-only mode indicators
	dsn := ctx.Config.DbReadOnlyDsn
	if strings.Contains(dsn, "mode=ro") {
		return true
	}
	// PostgreSQL: check if using a read-only connection
	if ctx.Config.DbType == constants.DbTypePosgres && dsn != "" && dsn != ctx.Config.DbDsn {
		return true // Separate DSN configured (assumed to be a read replica)
	}
	return false
}
```

**Step 2: Expose in template context and add warning to query view**

Find the query display template and add a warning banner when read-only is not enforced. The exact template location needs to be identified — look for query-related templates in `templates/`.

This is a visual change that requires identifying the exact template. The implementer should:
1. Check how query results are rendered (likely `templates/query_display.html` or similar)
2. Add a warning div when read-only is not enforced
3. Wire the `IsReadOnlyDBEnforced()` into the template context

**Step 3: Build and commit**

```bash
git add application_context/context.go templates/
git commit -m "fix(KAN-17): add visual warning when read-only DB is not enforced"
```

---

### Task 19: Run tests and verify all fixes

**Step 1: Build the full application**

Run: `npm run build`
Expected: CSS, JS, and Go binary all build successfully

**Step 2: Run Go unit tests**

Run: `go test ./...`
Expected: All tests pass

**Step 3: Run E2E tests**

Run: `cd e2e && npm run test:with-server`
Expected: All E2E tests pass

**Step 4: Fix any test failures**

If tests fail, investigate and fix. The cascade changes (Tasks 7-9) may require test updates if tests relied on cascade delete behavior.

**Step 5: Final commit if any test fixes needed**

```bash
git add -A
git commit -m "test: fix tests after audit bugfix batch"
```
