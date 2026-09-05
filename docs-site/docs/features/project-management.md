---
sidebar_position: 100
title: Project Management
---

# Project Management

A bundled plugin that turns Mahresources' own groups and notes into a project
tracker: multiple projects, epics under them, and tasks with a status
workflow, priority and due date. Four views — a kanban board, a filterable
backlog, a dashboard and a due-date timeline — are served on one plugin page,
with the board's drag-and-drop and keyboard controls both wired to the same
server-side ordering.

The plugin adds **no tables**. Its domain entities are host entities:

| Concept | Host entity |
|---|---|
| Project | Group in the **PM Project** category |
| Epic | Group in the **PM Epic** category, owned by its project |
| Task | Note of the **PM Task** type, owned by its epic (or by the project directly) |
| Status / priority / order | Task `meta` keys (`status`, `priority`, `order`) |
| Due / start | The note's native, indexed `EndDate` / `StartDate` fields |
| Label | A host global `Tag` on the note |

Tasks stay first-class: they appear in search, MRQL, mass edit, group
export/import and RBAC subtree scoping exactly like any other note. The plugin
uses each taxonomy's native `Custom*` template injection points to make those
host surfaces project-aware: PM Projects, PM Epics and PM Tasks gain distinct
avatars, compact list summaries, contextual detail headers and footers, list
introductions, hover content and complete MRQL result cards. The ordinary note
detail page still owns editing and the native metadata form; the plugin adds
status/priority badges and links back to the owning epic, project and board.

## Setup

Enabling the plugin is not enough: `init()` runs on every boot with no
principal, so provisioning the taxonomy is an explicit operator gesture. On
the plugin page, press **Set up Project Management**, or call the endpoint:

```bash
curl -X POST http://localhost:8181/v1/plugins/project-management/api/setup
```

Setup find-or-creates the **PM Project** and **PM Epic** categories and the
**PM Task** note type, installs their meta schemas and custom-template slots,
and caches the ids in the plugin's key-value store. It writes empty slots and
upgrades values that exactly match an older bundled default; any other value is
treated as an operator customization and survives a re-run. Presentation CSS
is appended when its version marker is absent, so custom rules are preserved
while a partial or older install is repaired. Because creating categories
requires the **admin** role, setup is refused with 403 for any non-admin
account (with auth off every request is an administrator and setup always
works).

The ids are cached per plugin in `mah.kv` (`cfg_taxonomy`). Purging the
plugin's data removes the cache: re-run setup to repoint the views.

### Synthetic example data

For a local playground that exercises all four views, run the repository's
project-management seeder against a development server:

```bash
./scripts/seed-project-management.sh seed http://localhost:8181
```

It creates three clearly named `[PM Demo]` projects (including an empty one),
six epics (including one orphaned epic), 15 tasks, two PM-specific content
blocks on the active delivery task, and seven `pm-demo/*` labels.
The tasks cover every default status and priority, project-level and epic-owned
work, the configured default status, one deliberately status-less core-created
task, completed and unfinished past-due work, and every timeline bucket. Dates
are relative to the run's anchor date; pass a third argument such as
`2026-09-04` to make the exact date values reproducible.

The script accepts `MR_TOKEN` for an auth-enabled server and requires an admin
principal because it may enable the plugin and runs setup. It refuses a
non-loopback target unless `MAH_PM_DEMO_ALLOW_REMOTE=1` is set. It expects the
default status and priority identifiers (custom display labels are fine) and
refuses a customized workflow before creating sample rows. Created row IDs and
names are stored as `.pm-demo-state` in the repository root; reset checks both
before deleting, so it does not match or sweep other projects by prefix:

```bash
./scripts/seed-project-management.sh reset http://localhost:8181
# Or explicitly reset and recreate the same fixture:
./scripts/seed-project-management.sh reseed http://localhost:8181 2026-09-04
```

Reset removes only the tracked demo projects, epics, tasks and labels. It keeps
the plugin enabled and leaves the shared PM taxonomy created by setup in place.

## Creating a project

A project is a group in the **PM Project** category, created the ordinary way
— `/group/new`, the API, the CLI. Its optional `meta` fields are `key`
(a short code, e.g. `DEV`) and `target_date`.

From the plugin page, open the project, then **+ New epic** to add epics and
**+ New task** (per board column, or from the backlog) to add tasks. Tasks are
owned by an epic when one is chosen and by the project otherwise.

## The board

Columns are the configured statuses (defaults: `backlog`, `todo`,
`in_progress`, `blocked`, `done`). Each column is one plain GET against the
host's own `/v1/notes` endpoint — the one read surface that returns a task's
tags and dates — filtered by note type, the project's subtree and one status,
sorted by the `meta.order` key. A column that fills its 50-row page offers
**Load more**.

Every card carries three controls besides drag-and-drop:

- **↑ / ↓** move the card within its column;
- a **Move to…** select moves it to another status, landing at that column's end;
- drag-and-drop places it between specific neighbours.

All three call the plugin's `POST .../api/task/move` endpoint, which validates
the target status, computes a new lexicographic `order` key between its
neighbours (a port of the ordering scheme note blocks use) and writes the note
inside one `mah.db.transaction`. When two neighbours' keys are so close that
no key exists between them, the endpoint rebalances that column — bounded,
because a column is at most a few hundred cards. Ordering survives reloads and
re-sorts, and a move never reverts a concurrent edit: the handler re-reads the
task inside the transaction and passes its tags and groups back through
explicitly, because the host's note write clears and re-appends associations.

The a11y story mirrors the block editor: drag is an enhancement, never the
only way. Moves announce to an `aria-live` region and focus stays on the moved
card's control.

## Statuses and priorities

Both are configurable in the plugin's settings:

| Setting | Shape | Default |
|---|---|---|
| `statuses` | JSON array of names | `["backlog","todo","in_progress","blocked","done"]` |
| `status_labels` | JSON object name → label | `{"backlog":"Backlog", ...}` |
| `priorities` | JSON array of names | `["low","medium","high","urgent"]` |
| `priority_labels` | JSON object name → label | `{"low":"Low", ...}` |
| `default_status` | string | `todo` |
| `done_status` | string | The status treated as completed by the progress bar, overdue and dashboard numbers. | `done` |

Status and priority names must match `^[A-Za-z0-9_-]+$` (they travel into MRQL
conditions and CSS attribute selectors); anything else in a settings list is
ignored with a warning in the activity log.

Every surface uses the same **effective status** rule: a missing or empty task
status means `default_status`. That rule drives board placement, badges,
status-aware avatars, progress, dashboard totals and overdue treatment. A
past-due task is overdue only while its effective status is not `done_status`.

Status and priority colours are code-level (they feed both the taxonomy
schemas' `x-color` and the rendered pills, so they must agree). Unknown
statuses and priorities are refused by the mutation endpoints with a 400
naming the offending value, never silently accepted.

## Dates

Tasks use the note's native `StartDate` and `EndDate`. The host's write path
parses exactly `YYYY-MM-DDTHH:MM` and silently stores **NULL** for anything
else, so the plugin validates every date it receives and rejects anything that
is not a real calendar datetime in `YYYY-MM-DDTHH:MM` form (an optional whole
`:SS` suffix is accepted and truncated) — RFC3339 zones, fractional seconds and
impossible dates like `2026-02-30` are refused rather than let the value vanish.

## The four views

- **Board** — one column per status, described above.
- **Backlog** — every task in the project/container, filterable by status,
  priority, epic and tag.
- **Dashboard** — counts from the plugin's `api/stats` endpoint (total, done,
  overdue, due this week, by status, by priority) plus a per-epic progress
  list.
- **Timeline** — tasks bucketed by due date: Overdue, Today, Tomorrow, Next 7
  days, Later, No due date, with done-status tasks kept in a separate Completed
  bucket so a finished past-due task is never called overdue.

Views are client-rendered from `/v1/notes`, so they carry tag chips and stay
off the plugin's VM lock; `stats` aggregates with one MRQL `GROUP BY` query per
dimension. Overdue counts come from `endDate < NOW()` (server clock) unless the
page supplies its own `now` parameter; the *due this week* number needs the
page-supplied week bounds, because MRQL has no date arithmetic.

The container is chosen from the URL: `/plugins/project-management/board?project=N`
for a project subtree, `?epic=N` for a single epic (used for epics orphaned by
a deleted project group).

## Embedded shortcodes

| Shortcode | Where it renders | What it shows |
|---|---|---|
| `view-links` | PM Project / PM Epic `CustomHeader` | Links to the four views for the group's project |
| `progress` | PM Project / PM Epic `CustomHeader` | Done/total bar for the group's subtree |
| `task-list` | Optional custom templates | The group's most recently updated tasks; not installed in the default epic header because Own Entities already lists them |
| `task-badges` | PM Task summary and MRQL slots | Status and priority pills |
| `task-controls` | PM Task header, including wide display | Status, due date and owning epic controls; badges for read-only viewers |
| `mini-board` | PM Project / PM Epic Own Entities | Counts and up to five tasks per status |
| `task-avatar` | PM Task list cards | A glyph and colour derived from effective status |
| `task-date` | PM Task summary, hover and MRQL slots | Due date plus overdue state for unfinished tasks |
| `group-summary` | PM Project / PM Epic detail, summary and MRQL slots | Status, project key and target date |
| `entity-context` | PM Task / PM Epic detail and MRQL slots | Links to the owning epic/project and the relevant board |

All output is `mah.html_escape`d: plugin output is re-run through the
shortcode processor, so a task name would otherwise execute as a shortcode.

## Native entity presentation

Setup gives all three PM taxonomies a consistent host-native presentation:

| Entity | Detail page | Lists and hover cards | MRQL result |
|---|---|---|---|
| PM Project | Workspace header with project facts, view links and progress | `P` avatar, status/key/target summary and filtered-list introduction | Named project card with facts and a direct Board link |
| PM Epic | Epic header with facts and progress; project/board context appears once in the footer | `E` avatar, status/target/project summary and filtered-list introduction | Named epic card with project and board context |
| PM Task | Task header with editable status, priority, due date and owner; owner/project/board context appears once in the footer | Status-aware avatar, status/priority/due/owner summary and filtered-list introduction | Named task card with badges, overdue-aware due date and owner/project/board context |

These are ordinary Category and Note Type templates, not special cases in the
core UI. Editing a `Custom*` field replaces the bundled value for that one
taxonomy, and later setup runs leave that customization alone.

The storage kind remains `note` or `group` for routes and APIs, while native
detail titles, global and full search, screen-reader search announcements, and
dashboard activity use the taxonomy display name (`PM Task`, `PM Epic`, or
`PM Project`). Owner/project names used by list, hover and MRQL templates are
batch-loaded by the host, so those summaries do not issue a Lua DB lookup per
card.

PM Task templates are enabled on public note shares during the plugin's one-time
presentation migration. The share server deliberately suppresses plugin
shortcodes and queries, so an anonymous share receives the safe Task identity
and CSS, a host-rendered status fallback and read-only priority metadata. It
never executes plugin Lua or exposes project context. Scoped guests see plain
PM badges when an operator allows the plugin, and no PM edit controls.

## PM Task content blocks

The plugin registers five note blocks, filtered to the dynamically provisioned
**PM Task** note type:

| Block | Content |
|---|---|
| **Acceptance criteria** (`plugin:project-management:acceptance-criteria`) | One observable outcome per line plus an optional verification method |
| **Status update** (`plugin:project-management:status-update`) | A progress summary with optional next step and blocker |
| **Subtasks** (`plugin:project-management:subtasks`) | Up to 100 `{id,label,task_id?}` items; checked IDs live in block state. Promote a row to a real PM Task under the same owner. Retrying promotion returns the same task. |
| **Dependencies** (`plugin:project-management:dependencies`) | `blocked_by` and `blocks` note-ID lists (50 each), resolved names and status pills. References unavailable to the viewer show no name. |
| **Time log** (`plugin:project-management:time-log`) | Estimate hours and up to 200 dated entries with hours and notes; total-versus-estimate progress. |

All blocks validate their JSON content, escape every authored value and offer
view and edit renderers through the standard note block editor. On a first
install setup registers them as soon as it creates the PM Task type; on later
boots the cached type id restores the same filtered registrations during
plugin initialization.
The editor renders their friendly labels and batches all plugin-block HTML for
one note and mode into a single request. The block's content uses the host card
as its visual container, avoiding nested borders and padding.

## Capabilities

`db:read`, `db:write`, `render`, `pages`, `api`, `kv`, `actions`, `hooks`, `schedule`.
The plugin makes no outbound requests and registers no global injections.

### Upgrading to 1.1.0

The added actions, hooks and schedule widen the plugin's grants. An existing
installation refuses to load the new version until an administrator re-enables it
on **/plugins/manage**. Then run **Set up** once to upgrade the presentation.
Setup records `cfg_presentation_v2`, recognizes exact prior bundled templates,
and preserves operator-authored templates and section configuration. Repeating
setup is safe. CSS has one source in `plugin.lua`; setup installs it into the
three taxonomies, and the plugin pages use that same definition.

## Working from native pages

Task detail and wide-display pages support status, due-date and owning-epic
changes. Priority, project key and project target date use the host's schema-aware
`[meta editable=true]` editor. PM custom elements use the same validated handlers
as the board; successful writes refresh other PM controls and badges for the task.
The native metadata panel, timestamps and owner sections can remain stale until
reload. The page announces that limitation after saving.

Task actions also appear in detail, card and bulk surfaces: set status, set
priority, set/clear due date, or move to an epic/project using the host entity
picker. Project and epic actions create a task or open the board. Select tasks
on `/notes?noteTypeId=N` (or `noteTypeIds=N`) to run a bulk action, up to **50**
tasks at once. Each task commits separately: earlier successes survive a later
failure, and mixed selections return individual refusals.

List actions narrow only on a taxonomy the list actually constrains. An
unfiltered Notes list still offers PM actions; checking each selected task remains
necessary when the action runs. An unknown content type on a resource list does
not hide content-type-filtered actions.

The four views have distinct URLs: `/plugins/project-management/board`,
`/backlog`, `/dashboard` and `/timeline` under the same prefix. Their menu entries
appear in the host's **Plugins** dropdown. Existing `?view=` links still work.

## Status defaults and rollups

Before native task creates and updates, a pure hook stamps a missing or empty
status with `default_status`. It never allocates an order key; the first explicit
PM move does that under the existing ordering locks.

The `rollup` schedule runs every **10 minutes**, skipping overlap. It writes
`pm_open`, `pm_done`, `pm_overdue`, `pm_next_due`, `pm_counts`, `pm_subtasks`,
`pm_subtasks_done` and `pm_rollup_at` into project/epic metadata, preserving other
keys. Card summaries read these values without per-card MRQL queries. Mini-board
counts and progress use the stored counts, falling back to a query before the
first rollup. Run the schedule manually from plugin management for an immediate
reconciliation.

The schedule performs a complete reconciliation, including batched subtask
reads. This deliberately covers missed after-hooks, native mass edits, block
state edits and deleted tasks even when no dirty marker survives. It costs more
than a timestamp-only sweep on large installations; counts may lag by ten
minutes. Group and task scans use keyset pagination rather than capped offsets.

## Known limits

- **Tags are global and flat.** Two projects sharing a label share one tag; a
  `project/label` convention is the only workaround.
- **Group-scoped users cannot see the plugin at all by default.** The per-plugin
  "allow limited users" toggle on `/plugins/manage` must be set first. The
  accounts you would most want confined to one project are the ones the deny
  hides the tool from. Note that their `mah.db` access stays bound to their own
  subtree either way.
- **No JavaScript, no views.** The shortcodes and the native note pages are
  the read-only fallback.
- **50 rows per request per column**, and no total count — a column fetches a
  page and offers Load more.
- Project, epic, backlog, dashboard and timeline lists page until their full
  result is loaded; only board columns stay incremental through **Load more**.
- **Deleting a project orphans its epics** (`owner_id` SET NULL), and deleting
  an epic orphans its tasks. The project picker surfaces an *Unassigned*
  section listing orphaned epics so their tasks stay reachable.
- A task created through the **core note form** receives `default_status` while
  the plugin is enabled, with no order key until its first PM move. With the
  plugin disabled, missing status still displays as `default_status` on re-enable.
- **Ordering writes serialize among the plugin's own endpoints** through
  per-task and per-column lock rows (they are the plugin's only writers that
  touch the order key). The host's core note form edits a task the same way it
  edits any other note — outside those locks, exactly as a plugin page never
  sees core-form writes on other entities.
