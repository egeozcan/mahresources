# Jobs drawer with live stats, per-job graphs, and a plugin stats API

## Context

The canonical Job Center (`68d1c313`) replaced the old download cockpit, and the user experience got worse in three ways:

1. **No progress, speed or ETA in the panel.** `templates/partials/jobPanel.tpl` renders title, state, kind and commands, and never reads `job.progress`. The Job Service stores progress (`jobs.Progress{Phase, Completed, Total, Unit, Message, ETA}`, written through `Service.UpdateProgress` in `jobs/service.go:535`), but a progress tick is not a Job Event. The only stream the UI reads, `/v1/jobs/events?version=2` (`server/api_handlers/job_event_handlers.go:105`), carries durable events only. So progress never reaches any page live. The old cockpit (last complete at `6c907a20`: `src/components/downloadCockpit.js`, `templates/partials/downloadCockpit.tpl`) computed speed in the browser from the legacy stream's 500 ms `updated` events.
2. **Only 5 rows per group.** `PANEL_LIMIT = 5` is hard-coded (`src/components/jobPanel.js:20`), times 3 state groups. The `download_cockpit_limit` setting (default 10) is documented in CLAUDE.md as "finished rows; active work never capped", but nothing reads it (`DownloadCockpitLimit()` has no callers, and the setting spec calls it "legacy").
3. **Popover instead of a drawer.** The old cockpit was a full-height drawer that slid in from the right.

The user also wants something new: a generic way for any job to publish **additional stats and a graph**, available to plugins as well. Nothing like per-job metrics or a history of values exists today.

Decisions made with the user:
- Plugins publish stats from Lua **and** from command runs, through a stdout line protocol.
- A graph covers the **whole lifetime** of a job, using a bounded ring that halves its resolution when full and is stored on the Job row.

## Design

### 1. Data model (`jobs/`, `models/job_model.go`)

- **`jobs.Metric`** `{Key, Label, Value float64, Total *float64, Unit, Graph bool}`, added to `jobs.Progress` as `Metrics []Metric`.
  - Validated in `validateProgress`, with the new ceilings next to the existing ones (`jobs/types.go:366-376`):
    - at most 8 metrics, and at most 3 with `Graph`
    - key must match `^[a-z0-9_-]{1,40}$` and be unique
    - label up to 60 bytes, unit up to 20 bytes
    - values must be finite and not negative
  - Validation errors return `ErrInvalidProgress`.
- **Built-in units.** The UI formats `bytes`, `items`, `percent`, `seconds` and `ms`. Any other unit is shown as a plain number followed by the unit text.
- **`jobs.ProgressSeries`** `{IntervalMs int64, Points []SeriesPoint}`, where `SeriesPoint{At (unix ms), Completed *float64, Rate *float64, Values map[string]float64}`. `Values` holds only the metrics marked `Graph`. A pure function `appendSample(series, now, progress)` in a new `jobs/progress_series.go` builds it:
  - The interval starts at 1 s. A tick arriving sooner than one interval after the last point only updates the snapshot columns and does not add a point.
  - `Rate` is the change in `Completed` divided by the elapsed time since the previous point. It is recorded only when the unit is unchanged and `Completed` did not go down. Pausing leaves a gap on the time axis. A resumed or retried Job starts a fresh rate.
  - When the series would grow past 120 points, adjacent pairs are merged (values averaged, the later timestamp kept) and `IntervalMs` doubles. The whole lifetime therefore always fits in 120 points, about 6 KB of JSON.
- **Derived speed and ETA.** `DeriveRate(series)` returns the rate averaged over the last ~5 s of points. If the executor supplied no ETA, an estimate is `(Total - Completed) / rate`. Both are computed once in Go, so the JSON API, the SSE frame and the `/jobs` template agree.
- **New columns** on `models.Job`, all nullable and added by AutoMigrate, so no writer-epoch bump is needed:
  - `ProgressMetrics types.JSON`
  - `ProgressSeries types.JSON`
  - `ProgressUpdatedAt *time.Time`
- **Writes.**
  - `Service.UpdateProgress` already loads the row before its guarded UPDATE. It now also writes `progress_metrics`, and `progress_series` only when a point was actually appended (at most once per interval), plus `progress_updated_at`.
  - `applyFinalProgress` appends a final point, so a finished Job keeps its graph.
  - Keep the invariant noted in `UpdateProgress` that the transaction's first statement is the write. The series is computed from the pre-read row, outside the transaction. The executor is the only writer, and a point lost to a race costs one sample, nothing more.
  - Retention needs no change: the series lives on the row and goes when the row goes.

### 2. Live transport

- **New read** `Service.LiveProgress(deps, access, since time.Time, limit)` in `jobs/query.go`. It selects nonterminal Jobs where `progress_updated_at > since`, filtered through the existing `jobQuery(db, access)` visibility scope, the same one that gates `PublishedEvents`. It returns id, version, progress, metrics and the last series point. The result is capped (100 rows).
- **Canonical SSE.** Once caught up, each 1 s poll in `GetCanonicalJobEventsHandler` also sends one `event: job-progress` frame per changed Job. Like `job-caught-up`, the frame has **no SSE `id`**, because it is not a durable event and must never move the delivery cursor. The high-water mark starts at the stream's start time, so a reconnect simply resumes live frames. Payload: `{jobId, version, progress: JobProgressResponse, point}`.
- **REST.**
  - `JobProgressResponse` (`server/api_handlers/job_handlers.go:431`) gains `metrics`, `rate` and `etaEstimated`, and `eta` falls back to the estimate.
  - `series` is included on `GET /v1/jobs/{id}`, and on `GET /v1/jobs` only with `include=progressSeries`, so `/jobs` pages stay small. The drawer asks for it.
  - Update `routes_openapi.go` and regenerate the spec.

### 3. Producers

- **Downloads** (`application_context/job_download_adapter.go:670`).
  - Always report `Completed` as bytes once any have arrived. `Total` is reported only when known. Today nothing is reported when `TotalSize <= 0`, so HLS and chunked transfers show no speed.
  - When `PhaseTotal > 0` (HLS segments), add the metric `segments{value, total, unit:"items"}`. This also closes the gap where HLS counters never reached the durable Job.
- **Queue-backed Kinds** (`queueJobProgress`, `application_context/job_queue_bridge.go:951`). Keep the primary measure. When both counters exist, add the secondary one as a metric (export items next to bytes). Extend `sameProgress` to compare metrics.
- **Plugin Lua** (`plugin_system/manager.go:1568`, `mah.job_progress`).
  - Accept a table as the second argument: `mah.job_progress(job_id, {percent=, completed=, total=, unit=, message=, metrics={ {key=, label=, value=, total=, unit=, graph=} }})`.
  - The positional form `(job_id, percent, message)` still works.
  - Same capability gate (`actions` or `jobs`): it is a reporter on a Job the plugin already owns and grants no new power, so neither `CompareGrants` nor the capability-gate test changes.
  - Invalid shapes raise a Lua error that names the field.
  - `HostJobSink.Progress(percent, message)` becomes `Progress(HostProgress)`, a struct carrying the same fields.
  - `ActionJob` (plus `Snapshot()` and `ProjectedActionJob`) stores the latest metrics and counts.
  - The 200 ms throttle stays latest-wins. A throttled update is marked pending and flushed at the next notify or in `settleActionJob`, so the last metrics are never dropped.
  - `pluginActionSink.Progress` (`application_context/job_plugin_action_adapter.go:887`) runs `safeText` redaction over the message **and** metric labels and units. When the plugin supplies counts, it maps them to `Completed`/`Total`/`Unit` instead of percent, so plugins also get rate and ETA.
- **Plugin command runs** (`plugin_commands/runner_unix.go:319`).
  - Wrap the stdout drain in a line filter. A line of the form `::mah-progress {json}` uses the same JSON shape as the Lua table.
  - The line buffer is bounded at 8 KB. A longer line passes through to the tail as ordinary output and is not parsed. Invalid JSON also passes through, and one warning is recorded.
  - Recognized lines are removed from the output tail and redacted with the run's existing `commandOutputSecrets`.
  - They are delivered through a new `Progress.Report(ProgressReport)` method, a local struct in `plugin_commands` that keeps the package independent of `jobs`.
  - `commandProgress` in `application_context/plugin_command_runtime.go` holds the claimed `jobs.Execution`. It mirrors reports with `execution.Progress`, throttled to 250 ms latest-wins with a final flush, and still calls `SetPhaseProgress` for the legacy queue row.
  - Only stdout is parsed, not stderr. `runner_windows.go` gets the same filter.

### 4. Frontend

- **Shared helpers**, new `src/components/jobProgress.js`:
  - `formatQuantity(value, unit)`, reusing the byte formatter style from the old cockpit
  - `formatRate`, `formatEta`, `progressPercent`
  - `sparklinePath(points, accessor, w, h)`, which scales the x axis by time rather than by index
  - `mergeLivePoint(series, point)`
  - `graphSummary(...)`, the accessible text
  - `jobCenter.js`'s existing `progressText`/`progressValue` move onto these helpers.
- **Drawer** (`templates/partials/jobPanel.tpl`, `src/components/jobPanel.js`):
  - Full-height right drawer: `fixed right-0 top-0 bottom-0 w-full max-w-md`, slide transition disabled under `prefers-reduced-motion`. Keep the teleport, `x-trap`, Escape, backdrop click, Cmd/Ctrl+Shift+D and `jobs-panel-open`.
  - Sections: **Needs attention**, **Active**, **Finished**, each with a heading.
  - Each row shows: title link, state, kind, phase and message; a progress bar (`role="progressbar"`, with `aria-valuenow` omitted when indeterminate); "12.3 MB of 40 MB · 42%"; speed; "about 14 s left"; metric chips as a `<dl>`; and an inline SVG sparkline per graphed series. The default series is `rate` when the Job has `Completed`, plus up to 3 plugin series. Each sparkline is `role="img"` with an `aria-label` summary (current, peak and average over the span).
  - Live `job-progress` frames update rows in place. They never trigger the list refetch, and they are **never announced**. Only lifecycle transitions keep announcing, as they do today.
  - Limits:
    - Active and attention groups are fetched with `limit=50`, so active work is practically uncapped. This keeps the per-row `loadAdvertisedCommands` fan-out bounded.
    - Finished rows use `download_cockpit_limit`, published as `<meta name="x-jobs-panel-finished-limit">` from `base.tpl`.
    - The footer says "Showing N of M finished" and links to All jobs.
  - Un-deprecate the setting in `application_context/runtime_setting_spec.go:266` and in the `main.go:203` flag help, and wire `DownloadCockpitLimit()` into the base template context.
  - Delete the dead `.cockpit-trigger*` rules in `public/index.css:162-190`.
- **Job detail page** (`templates/displayJob.tpl`, `jobCenter.js`): a larger graph, a metrics table, and speed and ETA, updated live from `job-progress` frames for that Job.
- **`/jobs` cards** (`templates/partials/job.tpl`, `JobRowProgress` in `job_template_context.go:601`): add the speed and ETA text from the Go-derived values. No graph there.
- Run `npm run build-js` and `npm run build-css`, then commit the `public/dist` changes, staging explicit paths.

### 5. Docs

- `docs-site/docs/features/plugin-lua-api.md` and `plugin-actions.md`: the `mah.job_progress` table form, metric bounds, graph flag and a worked example.
- The plugin commands page: the `::mah-progress` protocol, with its bounds, stripping and redaction.
- `docs-site/docs/features/job-system.md`: metrics, series and compaction, and the `job-progress` SSE frame with its no-id contract.
- `download-queue.md` and `configuration/runtime-settings.md`: the panel limit.
- CLAUDE.md: the config row for `-download-cockpit-limit`, and a short "Job progress metrics" note (ticks are not events, frames carry no id, series compaction).
- Do not overwrite `docs/todo.md`. It is a parsed ledger. The working plan copy goes to `docs/plans/2026-09-25-jobs-drawer-live-stats.md`.

## Execution order (TDD, one lane per bullet)

1. `jobs`: metric validation, `appendSample`/compaction/`DeriveRate` (pure unit tests first), the columns, `UpdateProgress`/`applyFinalProgress` persistence, and `LiveProgress` with a visibility test (owner vs other user vs admin). Add PG variants alongside the existing `*_pg_test.go` files.
2. Transport: the response fields, `include=progressSeries`, and the SSE `job-progress` frame. The handler test asserts: no `id:` line, no frame for an invisible Job, no cursor movement.
3. Producers: download and HLS mirror tests; the queue-bridge metrics; Lua `job_progress` table form (validation errors, throttle flush at settle, redaction of labels); the command line filter (split writes, over-long lines, invalid JSON, secret redaction, stripped from the tail).
4. Frontend: vitest for `jobProgress.js` and the drawer (group limits, finished limit from the meta tag, frame merge without refetch or announcement). Update the existing `jobPanel.test.ts` assertions for `limit=5` and the popover markup.
5. E2E:
   - A drawer spec: a download from a throttled local server shows a bar, speed and a sparkline.
   - A test plugin (`e2e/test-plugins`) whose action reports metrics with `graph=true`, and one whose command prints `::mah-progress` lines.
   - An axe check of the open drawer in `e2e/tests/accessibility/job-center-a11y.spec.ts`.
   - Update the panel selectors in `ws10-global-chrome.spec.ts`, `job-center.spec.ts` and the other panel specs as needed.
6. Docs, OpenAPI regeneration, `./mr docs lint` if any CLI output changes.

## Verification

- `go test --tags 'json1 fts5' ./...`
- Postgres: `go test --tags 'json1 fts5 postgres' ./jobs/... ./mrql/... ./server/api_tests/... -count=1`
- `npm test` (vitest)
- Rebuild the binary (`npm run build`), because E2E reuses `./mahresources`. Then run `cd e2e && npm run test:with-server:all` and `npm run test:with-server:postgres`. Read `.last-run.json` and a redirected log, never a piped tail.
- `./scripts/css-scan-test.sh` after the CSS and template changes.
- Manual check with the playwright-cli skill against an ephemeral server:
  - Start a large download and open the drawer with Cmd/Ctrl+Shift+D. The bar, bytes, speed and ETA move each second and the sparkline grows.
  - Reload mid-download: the graph comes back from the stored series.
  - Run a test-plugin action that reports metrics, and check the chips and graph.
  - Check the finished limit against the `download_cockpit_limit` setting.
- Review each lane with ask-pi as it lands, until two consecutive rounds come back clean.
