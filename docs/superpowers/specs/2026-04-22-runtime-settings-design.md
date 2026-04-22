# Runtime Settings Design

**Date:** 2026-04-22
**Status:** Design (awaiting implementation plan)

## Problem

Mahresources settings are bound once at startup from flags and environment variables into `application_context.MahresourcesConfig`. Any change — raising `MaxUploadSize`, tightening `MRQLQueryTimeout`, pointing `SharePublicURL` at a new host — requires a process restart. Operators on live private-network deployments want to adjust a small, well-chosen set of limits without downtime.

## Scope

Runtime-editable settings are limited to the subset where the running code either already reads the value each time it is used, or can be refactored to do so with a small local change. Pool sizes, worker counts, DB pool limits, bind addresses, file paths, and ephemeral-mode flags are explicitly out of scope — they require a restart and would be actively misleading in a "runtime" UI.

Eleven settings are in scope:

| Key                          | Type       | Current flag / env                              |
| ---                          | ---        | ---                                             |
| `max_upload_size`            | `int64`    | `-max-upload-size` / `MAX_UPLOAD_SIZE`          |
| `max_import_size`            | `int64`    | `-max-import-size` / `MAX_IMPORT_SIZE`          |
| `mrql_default_limit`         | `int`      | `-mrql-default-limit` / `MRQL_DEFAULT_LIMIT`    |
| `mrql_query_timeout`         | `duration` | `-mrql-query-timeout` / `MRQL_QUERY_TIMEOUT`    |
| `export_retention`           | `duration` | `-export-retention` / `EXPORT_RETENTION`        |
| `remote_connect_timeout`     | `duration` | `-remote-connect-timeout` / `REMOTE_CONNECT_TIMEOUT` |
| `remote_idle_timeout`        | `duration` | `-remote-idle-timeout` / `REMOTE_IDLE_TIMEOUT`  |
| `remote_overall_timeout`     | `duration` | `-remote-overall-timeout` / `REMOTE_OVERALL_TIMEOUT` |
| `share_public_url`           | `string`   | `-share-public-url` / `SHARE_PUBLIC_URL`        |
| `hash_similarity_threshold`  | `int`      | `-hash-similarity-threshold` / `HASH_SIMILARITY_THRESHOLD` |
| `hash_ahash_threshold`       | `uint64`   | `-hash-ahash-threshold` / `HASH_AHASH_THRESHOLD` |

`CleanupLogsDays` was considered and excluded: it only runs once at boot, so a runtime override would be cosmetic and misleading.

## Architecture

A new `RuntimeSettings` service owns runtime-editable values. `MahresourcesConfig` stays immutable after boot and becomes the source of boot-time defaults. A clean boundary results: `Config` = boot-only; `Settings` = runtime-editable.

```
                    ┌───────────────────────────┐
                    │   MahresourcesConfig      │   (immutable after boot)
                    │   boot flags / env        │
                    └───────────┬───────────────┘
                                │ captured once at startup
                                ▼
                    ┌───────────────────────────┐
                    │    RuntimeSettings        │◄──── runtime_settings table
                    │    cache + RWMutex        │      (persisted overrides)
                    │    defaults map           │
                    │    spec/bounds map        │
                    └───────────┬───────────────┘
                                │ typed getters (MaxUploadSize(), …)
                                ▼
       handlers, workers, download manager, MRQL executor
```

## Data Model

New table `runtime_settings`:

| Column       | Type          | Notes                                                           |
| ---          | ---           | ---                                                             |
| `key`        | VARCHAR PK    | Stable machine key (e.g. `max_upload_size`)                     |
| `value_json` | TEXT NOT NULL | Discriminated JSON envelope: `{"type":"int64","value":2147483648}` |
| `reason`     | TEXT          | Optional free-text note supplied on the form (nullable)         |
| `updated_at` | TIMESTAMP     | GORM auto-managed                                               |

Absent row means "no override, use boot default." JSON envelope supports the four types needed for bucket A (`int64`, `int`, `duration`, `string`) and extends to new types without schema migration.

Every successful `Set` or `Reset` writes one row to the existing `log_entries` table with `entity_type = "runtime_setting"`, the key and old→new value in `message`, and the request remote address as `actor`. This reuses existing admin-action log infrastructure.

## Service API

New file `application_context/runtime_settings.go`:

```go
type RuntimeSettings struct {
    db       *gorm.DB
    mu       sync.RWMutex
    cache    map[string]any          // decoded typed values
    defaults map[string]any          // boot-time flag values
    specs    map[string]SettingSpec  // type, bounds, display metadata
    logger   Logger
}

// Typed getters — one per scoped setting
func (s *RuntimeSettings) MaxUploadSize() int64
func (s *RuntimeSettings) MaxImportSize() int64
func (s *RuntimeSettings) MRQLDefaultLimit() int
func (s *RuntimeSettings) MRQLQueryTimeout() time.Duration
func (s *RuntimeSettings) ExportRetention() time.Duration
func (s *RuntimeSettings) RemoteConnectTimeout() time.Duration
func (s *RuntimeSettings) RemoteIdleTimeout() time.Duration
func (s *RuntimeSettings) RemoteOverallTimeout() time.Duration
func (s *RuntimeSettings) SharePublicURL() string
func (s *RuntimeSettings) HashSimilarityThreshold() int
func (s *RuntimeSettings) HashAHashThreshold() uint64

// Generic write / reset — operate on any registered key
func (s *RuntimeSettings) Set(key, rawValue, reason, actor string) error
func (s *RuntimeSettings) Reset(key, reason, actor string) error

// Introspection for the admin UI and CLI
func (s *RuntimeSettings) List() []SettingView
```

`SettingSpec` carries display metadata and validation bounds so the startup self-validator and the HTTP handler use the same numbers. `Set` parses the raw value via the spec, runs the bounds check, writes the DB row, updates the cache under write-lock, and writes the log_entries row. The DB write and cache update are ordered so a DB error leaves the cache consistent.

`appContext.Settings` returns the service. `Config` stays immutable after boot.

## Boot Sequence

In `main.go`, after `AutoMigrate` and before workers start:

1. `context.Settings = application_context.NewRuntimeSettings(db, logger, buildSpecs(), buildDefaults(cfg))`.
   `buildDefaults(cfg)` snapshots every in-scope flag value.
2. `context.Settings.Load()` reads every row from `runtime_settings` and decodes into the cache.
3. For each overridden key, compare `defaults[key]` to `cache[key]`. Emit one warning line per divergence:
   `WARN runtime_setting "max_upload_size" override (2147483648) differs from boot flag (1073741824)`.
4. Workers and handlers start after `Load()` returns. From here on, reads go through `Settings.X()`.

Persisted value fails bounds at boot (hand-edited DB): log an error, drop the key from cache (fall back to boot default), continue. Bad row stays in DB so ops can see it; a successful `Set` via the UI overwrites.

## Live-Reread Refactors

Three in-scope settings cache their value in a long-lived struct today. Each needs a small local change so the running code re-reads on next use.

**MRQL query timeout.** `application_context/mrql_context.go` declares a package-level `var MRQLQueryTimeout time.Duration` with five callsites (lines 152, 194, 426, 769, 820) plus the assignment in `main.go`. Remove the package var and replace each callsite with `appContext.Settings.MRQLQueryTimeout()`. All five call paths already have an `appContext` in scope.

**Download manager timeouts.** `download_queue/manager.go` has two constructors (`NewDownloadManager`, `NewDownloadManagerWithConfig`) that accept a `TimeoutConfig` struct, stored on the manager and read at download start. Add a `TimeoutProvider` interface (`ConnectTimeout() / IdleTimeout() / OverallTimeout()`). Change the manager to hold a provider and call the getters per download. To keep the two existing test callsites (`manager_test.go:1009`, `1063`) small, add a `StaticTimeouts(TimeoutConfig) TimeoutProvider` adapter — tests keep passing a zero `TimeoutConfig{}` via the adapter; production passes `appContext.Settings`.

**Hash worker similarity thresholds.** `hash_worker.Config` holds `SimilarityThreshold` and `AHashThreshold`. Replace both fields with getter callbacks `SimilarityThresholdFn func() int` and `AHashThresholdFn func() uint64`. The worker reads them on each pair comparison.

The remaining in-scope settings (`MaxUploadSize`, `MaxImportSize`, `MRQLDefaultLimit`, `ExportRetention`, `SharePublicURL`) are already read per-use from `appContext.Config` and switch trivially to `appContext.Settings.X()` at their callsites.

## HTTP API

Under the existing `/v1/admin` namespace:

| Method   | Path                              | Body / Params                        | Response                                                                                   |
| ---      | ---                               | ---                                  | ---                                                                                        |
| `GET`    | `/v1/admin/settings`              | —                                    | Array of `SettingView`: `{key, label, description, group, type, current, bootDefault, overridden, updatedAt, reason}` |
| `PUT`    | `/v1/admin/settings/{key}`        | `{value: string, reason?: string}`   | Updated `SettingView`                                                                      |
| `DELETE` | `/v1/admin/settings/{key}`        | `{reason?: string}`                  | `SettingView` with `current = bootDefault`, `overridden = false`                           |

Handlers in new `server/api_handlers/admin_settings_handlers.go`. OpenAPI metadata in `server/routes_openapi.go` so the generated spec stays in sync.

Value is sent as a string and parsed server-side via the type spec — uniform wire format, works for both form-encoded (HTML page) and JSON (scripts, CLI).

Errors: 400 with `{error: "value out of bounds: must be >= 1024"}` on bad input; 404 on unknown key.

## UI

New page at `/admin/settings`, template `templates/adminSettings.tpl`, context provider `server/template_handlers/template_context_providers/admin_settings_template_context.go`. Matches the existing `admin_overview`, `admin_shares`, etc. pattern.

Layout:

- Single page, grouped sections with `<h2>` per group: **Uploads**, **Queries**, **Remote downloads**, **Sharing**, **Deduplication**, **Exports**.
- Each setting is one row: label, help text, input appropriate to type (duration input accepts `30s`, `5m`, `2h`; byte input accepts `1G`, `500M`), boot default shown inline, "Override" badge when non-default, optional reason text input, per-row **Save** button (no bulk form — narrower blast radius and cleaner log_entries rows).
- Per-row **Reset** button for overridden settings. Inline confirmation via Alpine `x-data="{confirm:false}"` — no JS modal.
- Success flash: `Saved — took effect at HH:MM:SS`.
- Collapsible "boot-only settings" section at the bottom, showing the other flags (DSN masked) with "Requires restart" badges, for reference.
- Link added to admin overview page navigation.

## Validation and Bounds

Each `SettingSpec` carries min/max for numeric and duration types and an optional regex for strings. Bounds live in one place, used by both the API handler and the startup self-validator.

| Setting                     | Bounds                                                                 |
| ---                         | ---                                                                    |
| `max_upload_size`           | ≥ 1 KiB, ≤ 1 TiB. `0` allowed (unlimited — current behaviour)         |
| `max_import_size`           | ≥ 1 MiB, ≤ 1 TiB                                                       |
| `mrql_default_limit`        | ≥ 1, ≤ 100000                                                          |
| `mrql_query_timeout`        | ≥ 100ms, ≤ 5m                                                          |
| `export_retention`          | ≥ 1m, ≤ 30d                                                            |
| `remote_connect_timeout`    | ≥ 1s, ≤ 10m                                                            |
| `remote_idle_timeout`       | ≥ 1s, ≤ 1h                                                             |
| `remote_overall_timeout`    | ≥ 10s, ≤ 24h                                                           |
| `share_public_url`          | Empty OR valid `http(s)://…` URL (parsed via `url.Parse`)              |
| `hash_similarity_threshold` | ≥ 0, ≤ 64                                                              |
| `hash_ahash_threshold`      | ≥ 0, ≤ 64                                                              |

No double-confirmation on submit — the UI shows `Min: 1 KiB, Max: 1 TiB` inline under the field, and bad input returns an inline error.

## Precedence

DB override always wins. Flag/env is the seed used on first boot and the target of the per-row reset button. When a boot flag is explicitly set and differs from the DB override, one warning line per key is emitted at startup (Boot Sequence, step 3) so operators are not silently ignored.

## Audit

Every successful `Set` and `Reset` writes one row to `log_entries`:

- `entity_type = "runtime_setting"`
- `entity_id` = stable per-key synthetic ID (key is carried in the message)
- `action = "update"` or `"reset"`
- `message = "max_upload_size: 1073741824 → 2147483648 (reason: increase for video workflow)"`
- `actor` = request `RemoteAddr` (there is no authentication on the system)

Failed writes (bounds rejection, DB error) are not logged as entries; they surface in application logs only.

## CLI

`cmd/mr/commands/admin.go` becomes a command group. The existing stats behaviour moves to `mr admin stats`. Bare `mr admin` stays as an alias for `mr admin stats` to avoid breaking existing users.

New `settings` subgroup:

| Command                                                  | HTTP                              | Behaviour                                                                                |
| ---                                                      | ---                               | ---                                                                                      |
| `mr admin settings list`                                 | `GET /v1/admin/settings`          | Table by default, `--json` for raw. Columns: key, group, current, boot default, overridden, updated-at. |
| `mr admin settings get <key>`                            | `GET /v1/admin/settings` + filter | Single-key output. Nonzero exit on unknown key.                                          |
| `mr admin settings set <key> <value> [--reason <text>]`  | `PUT /v1/admin/settings/{key}`    | Prints post-update view. Bounds errors surface as nonzero exit + stderr message.         |
| `mr admin settings reset <key> [--reason <text>]`        | `DELETE /v1/admin/settings/{key}` | Prints post-reset view.                                                                  |

New help files under `cmd/mr/commands/admin_help/`:

- `admin.md` — group-level help (repositioned from current stats help).
- `admin_stats.md` — stats subcommand help (content from current `admin.md`).
- `admin_settings.md` — settings group help.
- `admin_settings_list.md`, `admin_settings_get.md`, `admin_settings_set.md`, `admin_settings_reset.md` — one per subcommand.

Each follows the existing `helptext.Load` pattern with `## Examples` blocks that `./mr docs check-examples` executes. The `set` and `reset` examples use a safe reversible key (`max_upload_size`) and clean up after themselves so the doctest is idempotent.

## Docs Site

**New page `docs-site/docs/configuration/runtime-settings.md`:**

- Explain the boot-flag → DB-override model and precedence.
- Enumerate the 11 scoped settings: key, type, bounds, default source (flag name + env var), description, "takes effect on".
- Note the `log_entries` audit trail.
- Screenshot of `/admin/settings`, captured via the `retake-screenshots` skill after the UI lands.
- Link to the `mr admin settings` CLI reference.

**Updates to existing pages:**

- `docs-site/docs/configuration/overview.md` — short "Runtime vs. boot-only settings" section linking to the new page.
- `docs-site/docs/cli/` — existing flat `admin.md` replaced by a `cli/admin/` subdirectory matching the convention used by other multi-command groups (e.g. `cli/resource/`). Contains `index.md` (group overview), `stats.md` (current admin content), and `settings.md` (settings subgroup reference — `list`, `get`, `set`, `reset` with worked examples, flag tables, exit codes).

## Concurrency and Safety

- `RuntimeSettings` cache reads take the RWMutex read-lock; writes take the write-lock.
- Cache update is ordered after DB commit so a DB error leaves the cache unchanged.
- Getters return value types (`int64`, `time.Duration`, etc.), never pointers, so callers cannot race on returned data.
- `defaults` and `specs` maps are populated at construction and not mutated after, so they are read lock-free.

## Testing Plan

**Unit tests — `application_context/runtime_settings_test.go`:**

- `Load()` empty DB → cache equals defaults; `overridden` flags false on every view.
- `Load()` with seeded row → cache returns override; `overridden` true.
- `Load()` with a row that fails bounds → error logged, key dropped from cache, getter returns boot default.
- `Set()` valid → cache updated, DB row written, log_entry row written, returns new view.
- `Set()` out-of-bounds → error returned, no cache/DB/log mutation.
- `Set()` unknown key → error returned.
- `Reset()` removes DB row, returns boot default, writes log_entry with `action=reset`.
- Concurrent `Set()` + `Get()` under `-race` — RWMutex correctness.
- Typed getters for all 11 keys return the right type and default.

**Go API tests — `server/api_tests/admin_settings_test.go`:**

- `GET /v1/admin/settings` returns all specs, `current = default` when DB empty.
- `PUT /v1/admin/settings/max_upload_size` valid → 200; subsequent `GET` shows override, `updated_at`, `reason`.
- `PUT` out-of-bounds → 400 with bounds message, no state change.
- `PUT` unknown key → 404.
- `DELETE` on overridden key → reverts, subsequent `GET` shows boot default, `overridden = false`.
- After `PUT max_upload_size=1KiB`, a 2KiB upload is rejected — end-to-end proof the override reaches the hot path.

**Boot-conflict test — `application_context/runtime_settings_boot_test.go`:**

- Seed `runtime_settings` row for `max_upload_size=X`. Boot `NewRuntimeSettings` with `defaults[max_upload_size]=Y`. Assert the captured logger received one WARN line naming both values.

**Live-reread refactor tests:**

- MRQL: run a query with `MRQLQueryTimeout` override and verify the timeout is honored. Reuse or adapt an existing MRQL timeout test if one exists; otherwise add a focused test against a query that exceeds the configured timeout.
- Download manager: construct with a `TimeoutProvider`, swap the provider's return value mid-test, verify the next download uses the new value. Local `httptest` server — no real network.
- Hash worker: set `HashSimilarityThreshold` override, feed a pair whose distance is between old and new threshold, verify the comparison decision matches the override.

**E2E browser tests — `e2e/tests/admin-settings.spec.ts`:**

- Page loads; all 11 settings render in their groups; boot defaults visible.
- Change `max_upload_size` to `1MB`, save, flash appears, "Override" badge appears, `updated_at` populated.
- Reset the same setting, override badge disappears.
- Submit out-of-bounds value — inline error shown, nothing persisted (verified by navigating away and back).
- Boot-only section lists expected keys with "Requires restart" badges.
- Accessibility — `e2e/tests/accessibility/admin-settings.a11y.spec.ts` passes axe.

**CLI E2E tests — `e2e/tests/cli/`:**

- `admin-settings-list.spec.ts` — basic output, `--json` format.
- `admin-settings-set-reset.spec.ts` — set reflects in `list`, reset reverts.
- `admin-settings-bounds.spec.ts` — out-of-bounds exits nonzero with useful stderr.

**Docs tests (gating):**

- `./mr docs lint` — passes with new help files in place.
- `./mr docs check-examples` — the `## Examples` blocks in new help files execute cleanly against a live server.

**Postgres pass — per CLAUDE.md finishing-features rule:**

```
go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1
cd e2e && npm run test:with-server:postgres
```

## Out of Scope

- Worker-tunable values that need re-poll or signaling (`HashPollInterval`, `ThumbPollInterval`, `HashBatchSize`, `ThumbBatchSize`, `VideoThumbnailTimeout`, `VideoThumbnailLockTimeout`) — candidates for a future iteration.
- Resizing worker pools at runtime (`HashWorkerCount`, `ThumbWorkerCount`, `VideoThumbnailConcurrency`, `MaxJobConcurrency`, `HashCacheSize`) — invasive, deferred.
- Settings that fundamentally require a restart (DB type/DSN, bind addresses, share port, file save path, alt filesystems, memory/ephemeral mode, seed paths, plugin path, `MaxDBConnections`, `SkipFTS`). These are shown read-only in the UI's boot-only collapsible section for reference.
- Export/import of the `runtime_settings` table as part of group export tarballs — this is operational config, not content.
- Multi-admin history view beyond the existing `log_entries` surface.

## Open Questions

None blocking. Implementation plan can proceed.
