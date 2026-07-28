# CLI E2E Test Suite Design

## Overview

Comprehensive end-to-end tests for the `mr` CLI binary, covering all 15 entity types and ~230 test cases. Tests run against an automatically-launched ephemeral server using the existing Playwright infrastructure.

## Architecture

### Location & Framework

- **Test files:** `e2e/tests/cli/*.spec.ts`
- **Framework:** Playwright test runner (no browser context needed)
- **Project:** New `cli` project in `e2e/playwright.config.ts`
- **Helper:** `e2e/helpers/cli-runner.ts` — wraps `child_process.execSync`
- **Fixture:** `e2e/fixtures/cli.fixture.ts` — exposes `cli: CliRunner` to tests

### Server & Binary Management

The existing `e2e/scripts/run-tests.js` is extended to:

1. Build the CLI binary (`go build -o mr ./cmd/mr/`) alongside the server binary
2. Pass `CLI_PATH` environment variable (absolute path to `mr`) to Playwright
3. All other behavior unchanged — same port detection, health check, ephemeral server flags, graceful shutdown

### npm Scripts

New scripts in `e2e/package.json`:

```json
{
  "test:cli": "playwright test --project=cli",
  "test:with-server:cli": "node scripts/run-tests.js test --project=cli"
}
```

## CliRunner Helper

```typescript
// e2e/helpers/cli-runner.ts

interface CliResult {
  stdout: string;
  stderr: string;
  exitCode: number;
}

class CliRunner {
  constructor(
    private binaryPath: string,  // path to ./mr binary
    private serverUrl: string    // e.g. http://localhost:8182
  ) {}

  // Run command, return raw result (doesn't throw on non-zero exit)
  run(...args: string[]): CliResult

  // Run command, throw if non-zero exit code
  runOrFail(...args: string[]): CliResult

  // Run with --json flag, parse stdout as JSON, throw on failure
  runJson<T = any>(...args: string[]): T

  // Run expecting failure (non-zero exit), throw if it succeeds
  runExpectError(...args: string[]): CliResult
}
```

Key behaviors:
- Automatically prepends `--server <serverUrl>` to every command
- `runJson` appends `--json` and parses output — primary assertion method. Wraps `JSON.parse` in a try-catch that includes raw stdout in the error message for debugging
- `runExpectError` for error-case tests (invalid ID, missing entity)
- 30s timeout per command
- Captures stdout and stderr separately
- CLI binary does not need `json1 fts5` build tags (no SQLite dependency)

### Implementation Note: execSync Error Handling

`child_process.execSync` throws on non-zero exit codes. The `run()` method must wrap `execSync` in a try-catch and extract `error.status` (exit code), `error.stderr`, and `error.stdout` from the thrown exception to construct the `CliResult`. This is critical — without it, every non-zero exit will throw instead of returning a result.

### Retry Logic

The CLI binary has no built-in retry for SQLite "database is locked" errors. When the ephemeral server returns HTTP 500 due to lock contention, the CLI command fails. The `CliRunner` includes retry logic: on specific error patterns in stderr/stdout (e.g., "database is locked", "SQLITE_BUSY"), re-run the command up to 3 times with exponential backoff (500ms, 1s, 2s). This mirrors the existing browser E2E `ApiClient.withRetry` pattern.

### Fixture

```typescript
// e2e/fixtures/cli.fixture.ts
import { test as base } from '@playwright/test';

export const test = base.extend<{ cli: CliRunner }>({
  cli: async ({}, use) => {
    const cli = new CliRunner(
      process.env.CLI_PATH || path.resolve(__dirname, '../../mr'),
      process.env.BASE_URL || 'http://localhost:8181'
    );
    await use(cli);
  },
});
```

## Playwright Config Changes

New project in `e2e/playwright.config.ts`:

```typescript
{
  name: 'cli',
  testDir: './tests/cli',
  use: {},  // no browser needed
}
```

Workers: 2 locally (reduced from default 4 to avoid SQLite lock contention — CLI has no built-in retry), 1 in CI. No dependency on other projects — runs independently of browser test projects. Must explicitly set `use: {}` to avoid inheriting `devices['Desktop Chrome']` from the default project config.

## Test Organization

### Test Files

| File | Commands Covered | Approx Tests |
|---|---|---|
| `cli-tags.spec.ts` | tag get/create/delete/edit-name/edit-description, tags list/merge/delete | 15 |
| `cli-notes.spec.ts` | note get/create/delete/edit-name/edit-description/share/unshare, notes list/add-tags/remove-tags/add-groups/add-meta/delete/meta-keys | 22 |
| `cli-groups.spec.ts` | group get/create/delete/edit-name/edit-description/parents/children/clone, groups list/add-tags/remove-tags/add-meta/delete/merge/meta-keys | 22 |
| `cli-resources.spec.ts` | resource get/edit/delete/edit-name/edit-description/upload/download/preview/from-url/from-local/rotate/recalculate-dimensions, resources list/add-tags/remove-tags/replace-tags/add-groups/add-meta/delete/merge/set-dimensions/meta-keys | 30 |
| `cli-resource-versions.spec.ts` | resource versions/version/version-upload/version-download/version-restore/version-delete/versions-cleanup/versions-compare, resources versions-cleanup | 12 |
| `cli-categories.spec.ts` | category get/create/delete/edit-name/edit-description, categories list | 8 |
| `cli-resource-categories.spec.ts` | resource-category get/create/delete/edit-name/edit-description, resource-categories list | 8 |
| `cli-note-types.spec.ts` | note-type get/create/edit/delete/edit-name/edit-description, note-types list | 10 |
| `cli-note-blocks.spec.ts` | note-block get/create/update/update-state/delete/types, note-blocks list/reorder/rebalance | 12 |
| `cli-relations.spec.ts` | relation create/delete/edit-name/edit-description | 6 |
| `cli-relation-types.spec.ts` | relation-type create/edit/delete, relation-types list | 6 |
| `cli-queries.spec.ts` | query get/create/delete/edit-name/edit-description/run/run-by-name/schema, queries list | 12 |
| `cli-series.spec.ts` | series get/create/edit/delete/remove-resource/list | 8 |
| `cli-jobs.spec.ts` | job submit/cancel/pause/resume/retry, jobs list | 8 |
| `cli-logs.spec.ts` | log get/entity, logs list (with filters) | 8 |
| `cli-search.spec.ts` | search (with --types, --limit) | 5 |
| `cli-plugins.spec.ts` | plugin enable/disable/settings/purge-data, plugins list | 7 |
| `cli-output-formats.spec.ts` | --json/--quiet/--no-header/--page across tags, notes, resources | 12 |
| `cli-error-handling.spec.ts` | invalid IDs, missing flags, server unreachable, non-existent entities | 15 |
| `cli-global-flags.spec.ts` | --server flag, MAHRESOURCES_URL env var | 4 |

**Total: ~230 tests**

## Test Patterns

### Lifecycle Pattern

Most entity test files follow this structure:

```
1. Create entity → capture ID from JSON output
2. Get entity by ID → verify fields match
3. Edit name → verify updated
4. Edit description → verify updated
5. List entities → verify created entity appears
6. Delete entity → verify success
7. Get deleted entity → verify error
```

### Data Isolation

- Each test creates its own data with unique names (test title + suffix)
- No shared state between tests — fully self-contained
- Allows parallel execution within the `cli` project
- List assertions must filter by the unique name prefix (e.g., `tags list --name "cli-test-xyz"`) rather than asserting on total list length, to avoid interference from parallel tests

### Assertion Strategy

- **Happy path:** `cli.runJson()` → assert on specific fields (ID, Name, Description)
- **Error cases:** `cli.runExpectError()` → assert on exitCode and stderr content
- **Table output:** `cli.runOrFail()` → string matching on column headers and row values
- **Quiet mode:** Assert stdout contains only IDs (one per line, no headers)

### Dependency Setup

Tests needing prerequisite entities (e.g., relations need groups + relation-type) create them via the CLI itself. This tests the full chain rather than using the API client as a backdoor.

### Cleanup

Best-effort cleanup in `test.afterEach` / `test.afterAll`. Ephemeral server means missed cleanup isn't fatal, but cleanup avoids list pollution affecting later assertions.

### File Operations (Resources)

- Test files from existing `e2e/test-assets/` directory
- Upload tests use a small text file and a small image
- Download tests write to a temp directory, verify file contents match

### Known Limitations

- **`resource from-local`**: Requires a path accessible to the *server* process. In ephemeral mode with in-memory filesystem, the server has no pre-existing local files. This command is tested by first uploading a file (which gives it a server-side path), but if the server's filesystem is purely in-memory, there may be no path to reference. If untestable, skip with a comment explaining why.
- **`resource from-url`**: Server needs to fetch from a URL. Tests use the server's own URL to download an already-uploaded resource (self-referential download), avoiding dependency on external URLs.
- **`resource upload` response shape**: The upload API may return an array. The CLI currently unmarshals as a single object. Tests should verify the JSON output structure and expose any parsing issues.

## List Filter Tests

Each entity's test file includes tests for its list command's filter flags. These are tested within the entity file, not in a separate file.

Representative filter tests:
- `tags list --name "partial"` — verify only matching tags returned
- `notes list --tags 1,2` — verify filtering by tag IDs
- `notes list --owner-id X` — verify owner filter
- `notes list --created-before / --created-after` — verify date range filters
- `groups list --category-id X` — verify category filter
- `resources list --content-type "text/plain"` — verify content type filter
- `logs list --level error --action create` — verify log filters

Each list filter test creates 2-3 entities, applies a filter that should match only one, and verifies the filtered results.

## Error Handling Tests

`cli-error-handling.spec.ts` covers cross-cutting error scenarios:

### Invalid Arguments
- Missing required flags (e.g., `tag create` without `--name`) — Cobra exits non-zero with usage hint
- Invalid ID format (e.g., `tag get abc`) — "invalid ID" error
- Wrong positional arg count — Cobra validation

### Non-existent Entities
- `tag get 999999` — "not found" error
- `note get 999999` — API error
- `resource download 999999` — error

### Server Connectivity
- `--server http://localhost:1` — connection refused error
- Malformed server URL — error

### Edge Cases
- Empty `--name ""`
- Very long names/descriptions
- Special characters in names (quotes, unicode, newlines)
- `--json` combined with `--quiet`

## Output Format Tests

`cli-output-formats.spec.ts` uses tags, notes, and resources as representative entities:

- **Default (table):** Verify column headers present, data rows formatted
- **`--json`:** Verify valid JSON with expected fields
- **`--quiet`:** Verify only IDs printed (one per line)
- **`--no-header`:** Verify table output without column header row
- **`--page`:** Verify pagination works (create enough entities, check page 0 vs page 1). Note: CLI defaults page to 1 (from main.go). Verify both `--page 0` and `--page 1` return results to validate the pagination interface.
- **`--quiet` on single-entity commands:** Verify behavior of `--quiet` with `get` commands. `PrintSingle` does not handle `Quiet` mode — this tests and documents the current behavior.

## Global Flag Tests

`cli-global-flags.spec.ts`:

- `--server` flag overrides default URL
- `MAHRESOURCES_URL` environment variable works
- `--server` flag takes precedence over env var
- Default `http://localhost:8181` when neither set

## New Files Summary

| File | Purpose |
|---|---|
| `e2e/helpers/cli-runner.ts` | CliRunner class |
| `e2e/fixtures/cli.fixture.ts` | Playwright fixture exposing `cli` |
| `e2e/tests/cli/cli-tags.spec.ts` | Tag command tests |
| `e2e/tests/cli/cli-notes.spec.ts` | Note command tests |
| `e2e/tests/cli/cli-groups.spec.ts` | Group command tests |
| `e2e/tests/cli/cli-resources.spec.ts` | Resource command tests |
| `e2e/tests/cli/cli-resource-versions.spec.ts` | Resource version tests |
| `e2e/tests/cli/cli-categories.spec.ts` | Category command tests |
| `e2e/tests/cli/cli-resource-categories.spec.ts` | Resource category tests |
| `e2e/tests/cli/cli-note-types.spec.ts` | Note type command tests |
| `e2e/tests/cli/cli-note-blocks.spec.ts` | Note block command tests |
| `e2e/tests/cli/cli-relations.spec.ts` | Relation command tests |
| `e2e/tests/cli/cli-relation-types.spec.ts` | Relation type command tests |
| `e2e/tests/cli/cli-queries.spec.ts` | Query command tests |
| `e2e/tests/cli/cli-series.spec.ts` | Series command tests |
| `e2e/tests/cli/cli-jobs.spec.ts` | Job command tests |
| `e2e/tests/cli/cli-logs.spec.ts` | Log command tests |
| `e2e/tests/cli/cli-search.spec.ts` | Search command tests |
| `e2e/tests/cli/cli-plugins.spec.ts` | Plugin command tests |
| `e2e/tests/cli/cli-output-formats.spec.ts` | Output format tests |
| `e2e/tests/cli/cli-error-handling.spec.ts` | Error handling tests |
| `e2e/tests/cli/cli-global-flags.spec.ts` | Global flag tests |
