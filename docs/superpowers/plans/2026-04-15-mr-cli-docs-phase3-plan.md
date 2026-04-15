# mr CLI Documentation Overhaul — Phase 3 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate the remaining 147 `mr` CLI commands across 17 logical groups to the embedded-Markdown help-text pattern established in Phase 2, then wire lint/doctest/docs-gen checks into CI so the docs stay in sync.

**Architecture:** Batched parallel dispatch. The main agent acts as **coordinator** and owns validation + shared files (`docs_lint.go`, docs-site regen, commits). Each batch fires 4–5 parallel **subagents** that each own one logical group (`<group>_help/*.md` + `<group>.go` rewire). After a batch's subagents return, the coordinator runs lint + doctest, fixes any CLI bugs surfaced, commits content, then commits any fixes separately. Five batches total (four parallel + one CI-only).

**Tech Stack:** Go, Cobra, `embed.FS`, SQLite + FTS5 (for ephemeral server), bash + jq (for doctest), Playwright (for doctest CI runner), GitHub Actions.

**Spec reference:** `docs/superpowers/specs/2026-04-15-mr-cli-docs-phase3-design.md` (parent: `docs/superpowers/specs/2026-04-14-mr-cli-docs-design.md`).

**Scope of this plan:** Phase 3 — migration of 147 commands across 17 logical groups AND CI wiring (docs-gen dirty-tree check, CLI doctest job). Phase 4 (README update, CLAUDE.md note, final full test sweep) is a separate tiny follow-up plan.

---

## File Structure

### New `<group>_help/` directories (one Markdown file per command — 147 files total)

Per-batch breakdown (directory paths relative to `cmd/mr/commands/`):

**Batch 1 — heaviest entities (62 files):**

| Directory | Files | Parent group |
|---|---|---|
| `groups_help/` | `group.md`, `group_get.md`, `group_create.md`, `group_delete.md`, `group_edit_name.md`, `group_edit_description.md`, `group_edit_meta.md`, `group_parents.md`, `group_children.md`, `group_clone.md`, `group_export.md`, `group_import.md` (parent + 11 subcommands = 12), plus `groups.md`, `groups_list.md`, `groups_add_tags.md`, `groups_remove_tags.md`, `groups_add_meta.md`, `groups_delete.md`, `groups_merge.md`, `groups_meta_keys.md`, `groups_timeline.md` (parent + 8 subcommands = 9) | `group` + `groups` (21 files) |
| `notes_help/` | `note.md`, `note_get.md`, `note_create.md`, `note_delete.md`, `note_edit_name.md`, `note_edit_description.md`, `note_edit_meta.md`, `note_share.md`, `note_unshare.md` (9), plus `notes.md`, `notes_list.md`, `notes_add_tags.md`, `notes_remove_tags.md`, `notes_add_groups.md`, `notes_add_meta.md`, `notes_delete.md`, `notes_meta_keys.md`, `notes_timeline.md` (9) | `note` + `notes` (18 files) |
| `queries_help/` | `query.md`, `query_get.md`, `query_create.md`, `query_delete.md`, `query_edit_name.md`, `query_edit_description.md`, `query_run.md`, `query_run_by_name.md`, `query_schema.md` (9), plus `queries.md`, `queries_list.md`, `queries_timeline.md` (3) | `query` + `queries` (12 files) |
| `tags_help/` | `tag.md`, `tag_get.md`, `tag_create.md`, `tag_delete.md`, `tag_edit_name.md`, `tag_edit_description.md` (6), plus `tags.md`, `tags_list.md`, `tags_merge.md`, `tags_delete.md`, `tags_timeline.md` (5) | `tag` + `tags` (11 files) |

**Batch 2 — organizational types (36 files):**

| Directory | Files | Parent group |
|---|---|---|
| `mrql_help/` | `mrql.md`, `mrql_save.md`, `mrql_list.md`, `mrql_run.md`, `mrql_delete.md` | `mrql` (5 files, no plural) |
| `note_types_help/` | `note_type.md`, `note_type_get.md`, `note_type_create.md`, `note_type_delete.md`, `note_type_edit.md`, `note_type_edit_name.md`, `note_type_edit_description.md` (7), plus `note_types.md`, `note_types_list.md` (2) | `note-type` + `note-types` (9 files) |
| `categories_help/` | `category.md`, `category_get.md`, `category_create.md`, `category_delete.md`, `category_edit_name.md`, `category_edit_description.md` (6), plus `categories.md`, `categories_list.md`, `categories_timeline.md` (3) | `category` + `categories` (9 files) |
| `resource_categories_help/` | `resource_category.md`, `resource_category_get.md`, `resource_category_create.md`, `resource_category_delete.md`, `resource_category_edit_name.md`, `resource_category_edit_description.md` (6), plus `resource_categories.md`, `resource_categories_list.md` (2) | `resource-category` + `resource-categories` (8 files) |
| `relations_help/` | `relation.md`, `relation_get.md`, `relation_create.md`, `relation_delete.md`, `relation_list.md` | `relation` (5 files, no plural) |

**Batch 3 — relations, blocks, series, jobs (35 files):**

| Directory | Files | Parent group |
|---|---|---|
| `relation_types_help/` | `relation_type.md`, `relation_type_get.md`, `relation_type_create.md`, `relation_type_delete.md`, `relation_type_edit_name.md`, `relation_type_edit_description.md` (6), plus `relation_types.md`, `relation_types_list.md` (2) | `relation-type` + `relation-types` (8 files) |
| `note_blocks_help/` | `note_block.md`, `note_block_get.md`, `note_block_create.md`, `note_block_update.md`, `note_block_update_state.md`, `note_block_delete.md`, `note_block_types.md` (7), plus `note_blocks.md`, `note_blocks_list.md`, `note_blocks_reorder.md`, `note_blocks_rebalance.md` (4) | `note-block` + `note-blocks` (11 files) |
| `series_help/` | `series.md`, `series_get.md`, `series_create.md`, `series_edit.md`, `series_delete.md`, `series_edit_name.md`, `series_remove_resource.md`, `series_list.md` | `series` (8 files, no plural distinction) |
| `jobs_help/` | `job.md`, `job_submit.md`, `job_cancel.md`, `job_pause.md`, `job_resume.md`, `job_retry.md` (6), plus `jobs.md`, `jobs_list.md` (2) | `job` + `jobs` (8 files) |

**Batch 4 — ops, plugins, standalones (14 files):**

| Directory | Files | Parent group |
|---|---|---|
| `logs_help/` | `log.md`, `log_get.md`, `log_entity.md` (3), plus `logs.md`, `logs_list.md` (2) | `log` + `logs` (5 files) |
| `plugins_help/` | `plugin.md`, `plugin_enable.md`, `plugin_disable.md`, `plugin_settings.md`, `plugin_purge_data.md` (5), plus `plugins.md`, `plugins_list.md` (2) | `plugin` + `plugins` (7 files) |
| `search_help/` | `search.md` | `search` (1 file) |
| `admin_help/` | `admin.md` | `admin` (1 file) |

**Authoritative source for exact subcommand names:** run `./mr docs dump --format json | jq -r '.commands[] | .path'` against a fresh build of `./mr`. The file lists above were generated from that output on 2026-04-15. If a command is added or removed before Phase 3 begins, update the relevant subagent's brief to match.

### Modified Go files

Each batch modifies the parent group's `.go` file to (a) add `//go:embed <group>_help/*.md`, (b) rewire every `cobra.Command` builder to call `helptext.Load(...)` and use `help.Long`, `help.Example`, `help.Annotations`.

| Batch | Go files modified |
|---|---|
| Batch 1 | `groups.go`, `notes.go`, `queries.go`, `tags.go`, `group_export.go` (subcommand of group — rewired inside the `group` subagent's task), `group_import.go` (same) |
| Batch 2 | `mrql.go`, `note_types.go`, `categories.go`, `resource_categories.go`, `relations.go` |
| Batch 3 | `relation_types.go`, `note_blocks.go`, `series.go`, `jobs.go` |
| Batch 4 | `logs.go`, `plugins.go`, `search.go`, `admin.go` |

Note: `timeline.go` holds shared helpers and does not define any top-level `cobra.Command`. Its helpers are called from `groups.go`, `notes.go`, `queries.go`, `tags.go` (and already from `resources.go`). The plural `timeline` subcommand wiring lives in each plural `.go` file, so `<plural>_timeline.md` is added during that plural's subagent task.

### Coordinator-modified files (all batches)

- `cmd/mr/commands/docs_lint.go` — append groups to `lintAllowlist` at end of each batch; delete the allowlist entirely in Batch 5.
- `docs-site/docs/cli/**` — regenerated by `npm run docs-gen`, committed alongside source changes.
- `.github/workflows/ci.yml` (Batch 5) — add docs-gen dirty-tree check + CLI doctest job.

---

## Reference Card: The 8 Phase 2 Gotchas

Every subagent prompt includes this block verbatim. Coordinator also references it when fixing authoring bugs.

**1. Server dedupes resources globally by content hash.** Every doctest that uploads a fixture must create a unique owner group (use `$RANDOM` or a timestamp suffix) AND use a unique `--name`. Asserting on the uploaded resource's returned name is unsafe — the server may return the original resource with its original name. Assert on `.ID > 0` or run `mr resource edit` first and assert on the edited value.

**2. `mr resource upload --json` returns a JSON array, not an object.** Use `jq -r '.[0].ID'` on upload pipelines. `get` / `version` commands return single objects (`.ID`).

**3. Field casing varies by endpoint.** Resource objects use PascalCase (`.ID`, `.Name`, `.Meta`, `.Tags`, `.Groups`, `.Width`, `.FileSize`). Version objects use lowercase (`.id`, `.versionNumber`, `.sameHash`). `resources meta-keys` returns `[{"key": "..."}]`, not a string array. Timeline returns `{buckets: [...]}` wrapping each bucket. **Verify against live output from a running ephemeral server**, not inference from Go struct tags.

**4. `$$` (bash PID) does not expand inside single-quoted jq expressions.** Use `--arg n "$VAR"` and reference `$n` inside the jq expression, or use double quotes to bash-quote the jq string.

**5. The doctest runner accumulates state within one `check-examples` run.** Each doctest block must be self-contained — create its own group, its own resources, its own tags.

**6. Fix CLI bugs surfaced by doctests in the same batch** that discovers them. Do not leave broken CLI in place.

**7. docs-gen warnings about "unknown command" in See Also** are expected until every group is migrated. They shrink per batch and reach zero by end of Batch 4.

**8. The Markdown generator filters doctest blocks** from published docs. Reference examples (without `# mr-doctest:` label) are what users see in `docs-site/docs/cli/`.

---

## Reusable Subagent Brief Template

Every parallel subagent dispatch uses this template. Fill in the `{{GROUP}}`, `{{GO_FILE}}`, `{{HELP_DIR}}`, `{{COMMANDS}}` placeholders per dispatch.

````markdown
You are migrating the `mr` CLI's `{{GROUP}}` command group to the embedded-Markdown help-text pattern established in Phase 2. Your scope is precisely:

- Create `cmd/mr/commands/{{HELP_DIR}}/` and write one `.md` file per command listed below.
- Modify `cmd/mr/commands/{{GO_FILE}}` to add `//go:embed {{HELP_DIR}}/*.md` and rewire every `cobra.Command` builder to call `helptext.Load(...)`.

### Commands to document

{{COMMANDS}}

### Gold-standard pattern

Read these four files and mirror their structure exactly:
- `cmd/mr/commands/resources_help/resource.md` — parent-group `Long` (domain overview).
- `cmd/mr/commands/resources_help/resource_get.md` — leaf with doctest.
- `cmd/mr/commands/resources_help/resource_upload.md` — leaf with multi-step doctest.
- `cmd/mr/commands/resources_help/resources_list.md` — plural list with filter flags.

Then read `cmd/mr/commands/resources.go` lines 76–145 to see how `//go:embed resources_help/*.md`, `var resourcesHelpFS embed.FS`, and `help := helptext.Load(resourcesHelpFS, "resources_help/<file>.md")` are wired into each builder.

### Per-file structure (front matter + Long + Example)

```markdown
---
outputShape: <one line describing JSON shape; omit field if command emits no data>
exitCodes: 0 on success; 1 on any error
relatedCmds: <command 1>, <command 2>, <command 3>
---

# Long

<2+ sentences explaining what the command does, when to use it, and any
non-obvious behavior. Describe positional-arg contract in prose.>

# Example

  # <human label>
  mr <group> <cmd> <args>

  # <another human label>
  mr <group> <cmd> <other args> --json

  # mr-doctest: <human description of what the block verifies>
  <self-contained bash that creates its own state and asserts via jq>
```

### Rules

1. Every leaf command gets ≥2 `# <label>` reference examples.
2. Every command (leaf or parent group) gets `exitCodes` front-matter; the default `"0 on success; 1 on any error"` matches the current `os.Exit(1)` behavior in `cmd/mr/main.go`.
3. `outputShape` is required for commands that emit data (most `get`, `list`, `create`, `edit-*` leaves). Omit for destructive/void commands that return no data.
4. `relatedCmds` is comma-separated, optional but encouraged. Use full paths like `resource get` (no leading `mr`).
5. Parent-group files (e.g., `{{GROUP}}.md`) get a `Long` explaining the entity (what it is, lifecycle, key relationships). No `Example` required on parent groups; omit `# Example` section entirely.
6. For every leaf where it's meaningful, include one `# mr-doctest:` block. See the 8 gotchas below. If a command genuinely cannot be doctested against an ephemeral server, skip the doctest and record the reason in your final report (not in the Markdown file).
7. In the Go file: import `"mahresources/cmd/mr/helptext"` and `"embed"`. Add `//go:embed {{HELP_DIR}}/*.md` and `var {{GROUP_CAMEL}}HelpFS embed.FS` at package scope. In each `new*Cmd(...)` function, insert `help := helptext.Load({{GROUP_CAMEL}}HelpFS, "{{HELP_DIR}}/<filename>.md")` before the `&cobra.Command{...}` literal, and set `Long: help.Long`, `Example: help.Example`, `Annotations: help.Annotations`.

### 8 Phase 2 gotchas (verbatim — paste this block into the subagent brief when dispatching)

**1. Server dedupes resources globally by content hash.** Every doctest that uploads a fixture must create a unique owner group (use `$RANDOM` or a timestamp suffix) AND use a unique `--name`. Asserting on the uploaded resource's returned name is unsafe — the server may return the original resource with its original name. Assert on `.ID > 0` or run `mr resource edit` first and assert on the edited value.

**2. `mr resource upload --json` returns a JSON array, not an object.** Use `jq -r '.[0].ID'` on upload pipelines. `get` / `version` commands return single objects (`.ID`).

**3. Field casing varies by endpoint.** Resource objects use PascalCase (`.ID`, `.Name`, `.Meta`, `.Tags`, `.Groups`, `.Width`, `.FileSize`). Version objects use lowercase (`.id`, `.versionNumber`, `.sameHash`). `resources meta-keys` returns `[{"key": "..."}]`, not a string array. Timeline returns `{buckets: [...]}` wrapping each bucket. **Verify against live output from a running ephemeral server**, not inference from Go struct tags.

**4. `$$` (bash PID) does not expand inside single-quoted jq expressions.** Use `--arg n "$VAR"` and reference `$n` inside the jq expression, or use double quotes to bash-quote the jq string.

**5. The doctest runner accumulates state within one `check-examples` run.** Each doctest block must be self-contained — create its own group, its own resources, its own tags.

**6. Fix CLI bugs surfaced by doctests in the same batch** that discovers them. Do not leave broken CLI in place.

**7. docs-gen warnings about "unknown command" in See Also** are expected until every group is migrated. They shrink per batch and reach zero by end of Batch 4.

**8. The Markdown generator filters doctest blocks** from published docs. Reference examples (without `# mr-doctest:` label) are what users see in `docs-site/docs/cli/`.

### Forbidden paths

You must NOT modify any of:
- `cmd/mr/commands/docs_lint.go` (owned by coordinator).
- `cmd/mr/commands/docs_dump.go`, `docs_doctest.go`, `docs.go`, `helptemplate.go`, `helpers.go` (scaffolding).
- Any other group's `<group>_help/` directory or `<group>.go` file.
- `docs-site/` (regenerated by coordinator).
- `cmd/mr/testdata/` (shared fixtures; additions are rare and go through the coordinator).

If you discover a bug in CLI code that is out of your group's scope, report it in your final message; do not fix it.

### Deliverable

When you finish, return a message containing:

1. The list of `.md` files you created (with full paths).
2. The Go file you modified and a one-line summary of the changes.
3. For each leaf command: whether you added a `# mr-doctest:` block. If not, why not (e.g., "requires pre-existing async job state", "destructive on shared state", "requires external binaries").
4. Any CLI bugs or cross-cutting issues you noticed and explicitly did not fix.
5. Whether `go build -o mr ./cmd/mr/` succeeds on your worktree. (Run this yourself before returning.)

Do NOT run `./mr docs lint` or `./mr docs check-examples` — those are the coordinator's jobs. Your job ends when your group's files compile.
````

---

## Task 0: Pre-flight verification

Before dispatching any subagents, verify the repository state and that Phase 2 is correctly landed.

**Files:** no changes in this task — verification only.

- [ ] **Step 1: Confirm clean working tree.**

Run: `git status --short`
Expected: only the stray files mentioned in the session start (`test.db-shm`, `test.db-wal`, the old plan drafts under `docs/superpowers/plans/2026-04-12-*.md`, `docs/superpowers/plans/2026-04-14-import-guid-fixes-plan.md`). No modified tracked files.

If there are other modified tracked files, stop and ask the user what to do.

- [ ] **Step 2: Build the current `mr` binary.**

Run: `npm run build-cli`
Expected: produces `./mr` at repo root, exit 0.

- [ ] **Step 3: Verify Phase 2 lint passes.**

Run: `./mr docs lint`
Expected: `OK: N warnings` on stdout where `N` is some integer (warnings are fine). Exit 0.

If this fails, stop. The Phase 2 baseline is broken and Phase 3 cannot start from a broken baseline.

- [ ] **Step 4: Count commands and confirm the 147 target.**

Run:
```bash
./mr docs dump --format json | jq -r '.commands[] | .path' | awk '{print $1}' | sort | uniq -c | sort -rn
```
Expected: output lists 32 top-level groups with counts. Sum of all non-{`resource`, `resources`, `docs`} counts = 147.

- [ ] **Step 5: Commit the stray old plan drafts and database journals (optional).**

If the session-start's stray files are old/unused, the user can decide separately. Do NOT touch them as part of this plan.

---

## Task 1: Batch 1 — heaviest entities (`group`, `note`, `query`, `tag`)

Batch 1 migrates the four highest-traffic groups. This task has 7 sub-steps: dispatch, spot-check, allowlist, lint gate, doctest gate, commit content, commit fix.

**Files created this task:**
- `cmd/mr/commands/groups_help/` (21 files)
- `cmd/mr/commands/notes_help/` (18 files)
- `cmd/mr/commands/queries_help/` (12 files)
- `cmd/mr/commands/tags_help/` (11 files)

**Files modified this task:**
- `cmd/mr/commands/groups.go`, `notes.go`, `queries.go`, `tags.go`
- `cmd/mr/commands/group_export.go`, `group_import.go` (inside the `group` subagent's scope)
- `cmd/mr/commands/docs_lint.go` (coordinator)
- `docs-site/docs/cli/**` (regenerated)

- [ ] **Step 1: Dispatch 4 parallel subagents.**

Send ONE message with FOUR `Agent` tool calls using `subagent_type: "general-purpose"`. Each uses the template from the "Reusable Subagent Brief Template" section, with these fills:

| Subagent | `{{GROUP}}` | `{{GO_FILE}}` | `{{HELP_DIR}}` | Command count |
|---|---|---|---|---|
| 1 | `group`/`groups` | `groups.go` (+ `group_export.go`, `group_import.go`) | `groups_help` | 21 |
| 2 | `note`/`notes` | `notes.go` | `notes_help` | 18 |
| 3 | `query`/`queries` | `queries.go` | `queries_help` | 12 |
| 4 | `tag`/`tags` | `tags.go` | `tags_help` | 11 |

**Important for subagent 1 (group/groups):** explicitly enumerate in the brief that the `group export` and `group import` subcommands are defined in `group_export.go` and `group_import.go` respectively (not `groups.go`), and both need the `helptext.Load` rewiring. The `//go:embed groups_help/*.md` and `var groupsHelpFS embed.FS` declaration goes in `groups.go`; `group_export.go` and `group_import.go` reference `groupsHelpFS` (same package).

Expected: 4 subagent messages return with their deliverables (see brief template for deliverable format).

- [ ] **Step 2: Spot-check subagent returns.**

For each of the 4 groups, read at minimum:
- The parent help file (`groups_help/group.md`, `notes_help/note.md`, `queries_help/query.md`, `tags_help/tag.md`) — confirm it has front matter + `# Long` section (no `# Example` expected on parents).
- One leaf file with a doctest (e.g., `groups_help/group_get.md`) — confirm it has front matter + `# Long` + `# Example` including a `# mr-doctest:` block.
- The modified `.go` file — grep for `helptext.Load` and confirm every `new*Cmd` function has a `help := helptext.Load(...)` line.

Run:
```bash
grep -c "helptext.Load" cmd/mr/commands/groups.go cmd/mr/commands/notes.go cmd/mr/commands/queries.go cmd/mr/commands/tags.go cmd/mr/commands/group_export.go cmd/mr/commands/group_import.go
```
Expected: each file shows a count matching the number of `new*Cmd` functions in it (see `grep -c "^func new.*Cmd(" <file>` for the expected count).

If any subagent produced a structurally broken file, fix it inline (don't re-dispatch). Common fixes: missing front matter, `# Long` section missing, `helptext.Load` path typo, missing `//go:embed` directive.

- [ ] **Step 3: Append Batch 1 groups to `lintAllowlist` in `docs_lint.go`.**

Open `cmd/mr/commands/docs_lint.go` and edit the `lintAllowlist` map. Use `replace_all: false` and match the exact block.

Before:
```go
var lintAllowlist = map[string]bool{
	"docs":      true,
	"resource":  true,
	"resources": true,
}
```

After:
```go
var lintAllowlist = map[string]bool{
	"docs":      true,
	"resource":  true,
	"resources": true,
	"group":     true,
	"groups":    true,
	"note":      true,
	"notes":     true,
	"query":     true,
	"queries":   true,
	"tag":       true,
	"tags":      true,
}
```

- [ ] **Step 4: Build + lint gate.**

Run: `npm run build-cli && ./mr docs lint`

Expected: exit 0, output `OK: N warnings` (warnings are fine). Any `error:` lines = fail.

If `go build` fails: fix the Go file issue inline (most common: typo in `helptext.Load` path, missing import, `var <name>HelpFS embed.FS` not declared).

If `./mr docs lint` fails with `error:` lines: fix the referenced help file inline. The common errors are:
- `<path>: missing Short` — add a `Short:` string in the Go builder.
- `<path>: missing Long` — add a `# Long` section to the Markdown file.
- `<path>: fewer than 2 Example entries` — add a second `# <label>` block to the Markdown.
- `<path>: missing exitCodes annotation` — add `exitCodes: 0 on success; 1 on any error` to front matter.

Re-run until exit 0.

- [ ] **Step 5: Doctest gate (open-ended fix loop).**

Start an ephemeral server in the background:
```bash
./mahresources -ephemeral -bind-address=:8181 -max-db-connections=2 &
SERVER_PID=$!
sleep 1
```

If `./mahresources` is not built, run `go build --tags 'json1 fts5'` first (the Go server binary, separate from the CLI binary).

Run the doctest:
```bash
./mr docs check-examples --server http://127.0.0.1:8181 --environment ephemeral
```

Expected: exit 0. Each `# mr-doctest:` block reports PASS or SKIP. Any FAIL blocks the batch.

**Failure triage:**

1. **Authoring bug** (most common — wrong jq path, field casing wrong, `$$` in single quotes, missing `--arg`): fix the `.md` help file inline. Refer to the 8 gotchas above.

2. **CLI bug** (rarer but real — see Phase 2's `set-dimensions` precedent): investigate the `.go` code. If the fix is small (≤20 lines), fix inline. If larger, dispatch a debugging subagent:
   ```
   Agent(
     description="Fix <group> <command> CLI bug",
     subagent_type="general-purpose",
     prompt="<concrete repro, error message, expected vs actual behavior, file paths>. Use superpowers:systematic-debugging. Do not modify any help text Markdown."
   )
   ```

3. **Environment issue** (server died, port conflict): kill leftover server, re-run.

Re-run `./mr docs check-examples` after every fix until exit 0.

Kill the server:
```bash
kill $SERVER_PID 2>/dev/null; wait $SERVER_PID 2>/dev/null
```

- [ ] **Step 6: Regenerate docs-site and commit content (Commit 1/2).**

Run:
```bash
npm run build-cli && npm run docs-gen
git status --porcelain docs-site/docs/cli/
```

Expected: `npm run docs-gen` exits 0 with some `warning: unknown command "resource X"` lines (expected per gotcha #7 — groups not yet migrated in later batches). `git status` shows new/modified files under `docs-site/docs/cli/`.

Stage and commit:
```bash
git add cmd/mr/commands/groups_help cmd/mr/commands/notes_help cmd/mr/commands/queries_help cmd/mr/commands/tags_help
git add cmd/mr/commands/groups.go cmd/mr/commands/notes.go cmd/mr/commands/queries.go cmd/mr/commands/tags.go
git add cmd/mr/commands/group_export.go cmd/mr/commands/group_import.go
git add cmd/mr/commands/docs_lint.go
git add docs-site/docs/cli/
git status --short
```

Confirm `git status --short` shows the expected files staged with `A` or `M` markers. Then commit:

```bash
git commit -m "$(cat <<'EOF'
docs(cli): migrate group, note, query, tag to embedded help (batch 1/5)

Adds embedded Markdown help text for the group, note, query, and tag
command groups (62 commands, 62 new help files). Rewires each cobra
builder to call helptext.Load. Appends the 8 migrated top-level keys to
lintAllowlist. Regenerates docs-site/docs/cli/ with the new pages.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 7: Commit fixes (Commit 2/2, conditional).**

If Step 5 surfaced CLI bugs that you fixed, those changes should still be staged (or unstaged if you forgot). Check:

```bash
git status --short
git diff
```

If any `cmd/mr/commands/*.go` or other non-help files have uncommitted changes from the fix loop, commit them separately:

```bash
git add <changed files>
git commit -m "$(cat <<'EOF'
fix(cli): <short description of bug fixed> (batch 1/5)

<1-3 sentence explanation of what was broken and how the fix works.
Reference the doctest that surfaced it.>

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

If nothing to fix, skip this commit.

Report retro to user: "Batch 1/5 complete: group, note, query, tag migrated (62 commands). N doctests passing, M skipped. <any notable bugs fixed>. <any remaining cross-group warnings>."

---

## Task 2: Batch 2 — organizational types (`mrql`, `note-type`, `category`, `resource-category`, `relation`)

Same 7-step structure as Task 1. Five parallel subagents this time.

**Files created this task:**
- `cmd/mr/commands/mrql_help/` (5 files)
- `cmd/mr/commands/note_types_help/` (9 files)
- `cmd/mr/commands/categories_help/` (9 files)
- `cmd/mr/commands/resource_categories_help/` (8 files)
- `cmd/mr/commands/relations_help/` (5 files)

**Files modified this task:**
- `cmd/mr/commands/mrql.go`, `note_types.go`, `categories.go`, `resource_categories.go`, `relations.go`
- `cmd/mr/commands/docs_lint.go`
- `docs-site/docs/cli/**`

- [ ] **Step 1: Dispatch 5 parallel subagents.**

Send ONE message with FIVE `Agent` tool calls.

| Subagent | `{{GROUP}}` | `{{GO_FILE}}` | `{{HELP_DIR}}` | Command count |
|---|---|---|---|---|
| 1 | `mrql` | `mrql.go` | `mrql_help` | 5 |
| 2 | `note-type`/`note-types` | `note_types.go` | `note_types_help` | 9 |
| 3 | `category`/`categories` | `categories.go` | `categories_help` | 9 |
| 4 | `resource-category`/`resource-categories` | `resource_categories.go` | `resource_categories_help` | 8 |
| 5 | `relation` (no plural) | `relations.go` | `relations_help` | 5 |

**Note for subagent 5 (relation):** although the file is `relations.go` (plural file name), the only top-level command group is `relation` (singular). There is no `relations` parent command.

- [ ] **Step 2: Spot-check subagent returns.**

Same pattern as Task 1 Step 2. Read parent help file + one leaf with doctest + the modified Go file for each of the 5 groups. Fix structural issues inline.

- [ ] **Step 3: Append Batch 2 groups to `lintAllowlist`.**

Edit `cmd/mr/commands/docs_lint.go`. Append these 8 keys alphabetically within the existing block (or at the end — the linter doesn't care about ordering):

```go
	"mrql":                true,
	"note-type":           true,
	"note-types":          true,
	"category":            true,
	"categories":          true,
	"resource-category":   true,
	"resource-categories": true,
	"relation":            true,
```

- [ ] **Step 4: Build + lint gate.**

`npm run build-cli && ./mr docs lint` — fix iteratively until exit 0.

- [ ] **Step 5: Doctest gate.**

Same as Task 1 Step 5: start ephemeral server, run `./mr docs check-examples`, fix until exit 0, kill server.

- [ ] **Step 6: Regenerate and commit content (Commit 1/2).**

```bash
npm run build-cli && npm run docs-gen
git add cmd/mr/commands/mrql_help cmd/mr/commands/note_types_help cmd/mr/commands/categories_help cmd/mr/commands/resource_categories_help cmd/mr/commands/relations_help
git add cmd/mr/commands/mrql.go cmd/mr/commands/note_types.go cmd/mr/commands/categories.go cmd/mr/commands/resource_categories.go cmd/mr/commands/relations.go
git add cmd/mr/commands/docs_lint.go
git add docs-site/docs/cli/
git commit -m "$(cat <<'EOF'
docs(cli): migrate mrql, note-type, category, resource-category, relation (batch 2/5)

Adds embedded Markdown help for organizational-type command groups
(36 commands, 36 new help files). Same pattern as batch 1.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 7: Commit fixes (Commit 2/2, conditional).**

Same pattern as Task 1 Step 7.

Report retro: "Batch 2/5 complete: mrql, note-type, category, resource-category, relation migrated (36 commands). N doctests passing, M skipped. ..."

---

## Task 3: Batch 3 — relations, blocks, series, jobs (`relation-type`, `note-block`, `series`, `job`)

Same 7-step structure. Four parallel subagents.

**Files created this task:**
- `cmd/mr/commands/relation_types_help/` (8 files)
- `cmd/mr/commands/note_blocks_help/` (11 files)
- `cmd/mr/commands/series_help/` (8 files)
- `cmd/mr/commands/jobs_help/` (8 files)

**Files modified this task:**
- `cmd/mr/commands/relation_types.go`, `note_blocks.go`, `series.go`, `jobs.go`
- `cmd/mr/commands/docs_lint.go`
- `docs-site/docs/cli/**`

- [ ] **Step 1: Dispatch 4 parallel subagents.**

Send ONE message with FOUR `Agent` tool calls.

| Subagent | `{{GROUP}}` | `{{GO_FILE}}` | `{{HELP_DIR}}` | Command count |
|---|---|---|---|---|
| 1 | `relation-type`/`relation-types` | `relation_types.go` | `relation_types_help` | 8 |
| 2 | `note-block`/`note-blocks` | `note_blocks.go` | `note_blocks_help` | 11 |
| 3 | `series` (no plural distinction) | `series.go` | `series_help` | 8 |
| 4 | `job`/`jobs` | `jobs.go` | `jobs_help` | 8 |

**Special notes for subagent 4 (job):**

The `job get`, `job cancel`, `job pause`, `job resume`, `job retry` commands all operate on a specific async job ID. On an ephemeral server, there are no pre-existing jobs. The subagent should either:
- Use `skip-on=ephemeral` on these doctests, OR
- Use `mr job submit --type hash-worker` (or whatever job type is cheapest to submit) to create a job ID, then act on it within the same block. Check `cmd/mr/commands/jobs.go` for valid job types before authoring.

Either approach is fine; document the choice in the subagent's final report.

- [ ] **Step 2: Spot-check subagent returns.**

Same pattern.

- [ ] **Step 3: Append Batch 3 groups to `lintAllowlist`.**

Append these 7 keys:
```go
	"relation-type":  true,
	"relation-types": true,
	"note-block":     true,
	"note-blocks":    true,
	"series":         true,
	"job":            true,
	"jobs":           true,
```

- [ ] **Step 4: Build + lint gate.** Same as prior batches.

- [ ] **Step 5: Doctest gate.** Same pattern. Expect some `skip-on=ephemeral` blocks in the `job` group.

- [ ] **Step 6: Regenerate and commit content.**

```bash
npm run build-cli && npm run docs-gen
git add cmd/mr/commands/relation_types_help cmd/mr/commands/note_blocks_help cmd/mr/commands/series_help cmd/mr/commands/jobs_help
git add cmd/mr/commands/relation_types.go cmd/mr/commands/note_blocks.go cmd/mr/commands/series.go cmd/mr/commands/jobs.go
git add cmd/mr/commands/docs_lint.go
git add docs-site/docs/cli/
git commit -m "$(cat <<'EOF'
docs(cli): migrate relation-type, note-block, series, job (batch 3/5)

Adds embedded Markdown help for relation-type, note-block, series, and
job command groups (35 commands, 35 new help files).

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 7: Commit fixes (conditional).**

Same pattern.

Report retro: "Batch 3/5 complete: relation-type, note-block, series, job migrated (35 commands). ..."

---

## Task 4: Batch 4 — ops, plugins, standalones (`log`, `plugin`, `search`, `admin`)

Two parallel subagents + two inline migrations (search, admin are single-command groups).

**Files created this task:**
- `cmd/mr/commands/logs_help/` (5 files)
- `cmd/mr/commands/plugins_help/` (7 files)
- `cmd/mr/commands/search_help/search.md` (1 file, inline)
- `cmd/mr/commands/admin_help/admin.md` (1 file, inline)

**Files modified this task:**
- `cmd/mr/commands/logs.go`, `plugins.go`, `search.go`, `admin.go`
- `cmd/mr/commands/docs_lint.go`
- `docs-site/docs/cli/**`

- [ ] **Step 1: Dispatch 2 parallel subagents.**

Send ONE message with TWO `Agent` tool calls.

| Subagent | `{{GROUP}}` | `{{GO_FILE}}` | `{{HELP_DIR}}` | Command count |
|---|---|---|---|---|
| 1 | `log`/`logs` | `logs.go` | `logs_help` | 5 |
| 2 | `plugin`/`plugins` | `plugins.go` | `plugins_help` | 7 |

**Special notes for subagent 1 (log):** `log get` needs a specific log row ID; on an ephemeral server there are no pre-existing rows. Use `skip-on=ephemeral` for `log get`. `log entity <entity-type> <id>` can likely doctest by creating a group or tag and then querying its log (if the ephemeral server records activity — verify by inspection; if not, use `skip-on=ephemeral`). `logs list` can doctest cleanly.

**Special notes for subagent 2 (plugin):** `plugin enable/disable/settings/purge-data` all require external plugin binaries which the ephemeral server does not have. Ship reference-only examples for these four; use `skip-on=ephemeral` if a runnable form is still desired for non-ephemeral runs. `plugins list` may return an empty list on ephemeral (doctest with `jq -e 'type == "array"'`).

- [ ] **Step 2: Spot-check subagent returns.**

Same pattern.

- [ ] **Step 3: Inline migration of `search` (single-command leaf).**

`mr search <query>` is a leaf with exactly one positional argument (`cobra.ExactArgs(1)`) and two local flags (`--types`, `--limit`). Response shape is `{Total: int, Results: [{ID, Type, Name, Score, Description}]}` — verify the JSON field casing by running `./mr search foo --json | jq '.'` against a running ephemeral server before authoring the doctest.

Create `cmd/mr/commands/search_help/search.md`:

```markdown
---
outputShape: Search response with Total (int) and Results (array of {ID, Type, Name, Score, Description})
exitCodes: 0 on success; 1 on any error
relatedCmds: mrql run, resources list, notes list, groups list
---

# Long

Search across resources, notes, and groups using the server's full-text
search index. Results are scored and ranked; the response also reports the
total number of matches so callers can page or decide whether to broaden
the query.

Use `--types` to restrict to a comma-separated subset of entity types
(e.g., `--types resources,notes`). Use `--limit` to cap the number of
results (default 20).

# Example

  # Simple keyword search across all entities
  mr search "invoice"

  # Restrict to resources only, JSON output
  mr search "invoice" --types resources --json

  # Cap results and pipe into jq to count hits
  mr search "report" --limit 5 --json | jq '.Total'

  # mr-doctest: create a uniquely-named group and confirm search returns a response
  NAME="doctest-search-$$-$RANDOM"
  mr group create --name "$NAME" --json > /dev/null
  mr search "$NAME" --json | jq -e '.Total >= 0 and (.Results | type == "array")'
```

Then modify `cmd/mr/commands/search.go`:
- Add `"embed"` and `"mahresources/cmd/mr/helptext"` to imports (alongside existing imports around lines 1–10).
- Add `//go:embed search_help/*.md` and `var searchHelpFS embed.FS` at package scope (before `NewSearchCmd`).
- In `NewSearchCmd`, immediately before `cmd := &cobra.Command{`, add: `help := helptext.Load(searchHelpFS, "search_help/search.md")`. Set `Long: help.Long`, `Example: help.Example`, `Annotations: help.Annotations` inside the struct literal.

Verify: `grep -n "helptext.Load" cmd/mr/commands/search.go` returns one line.

- [ ] **Step 4: Inline migration of `admin` (single-command leaf).**

`mr admin` is a read-only stats command with `RunE` and two local flags (`--server-only`, `--data-only`). It emits either server stats, data stats, or a combined `{serverStats, dataStats, expensiveStats}` object in JSON mode. Safe to doctest — it touches no writable state.

Create `cmd/mr/commands/admin_help/admin.md`:

```markdown
---
outputShape: Combined stats object {serverStats, dataStats, expensiveStats} in JSON mode; three sectioned tables in human mode
exitCodes: 0 on success; 1 on any error
relatedCmds: resources versions-cleanup, jobs list, logs list
---

# Long

Show administrative statistics about the server and its data. By default
this command fetches three stat sections: server health (uptime, memory,
DB connections), data counts (entity totals), and expensive stats (counts
that require full-table scans like total hash collisions).

Use `--server-only` to fetch just the server health block, or `--data-only`
to fetch just the data counts — useful for lightweight monitoring that
doesn't trigger the expensive scans. Neither flag is required; when both
are unset the command fetches all three sections.

# Example

  # Show full admin stats (human-readable, three sections)
  mr admin

  # Server health only, JSON output
  mr admin --server-only --json

  # Data counts only
  mr admin --data-only

  # mr-doctest: fetch combined stats and assert the response shape
  mr admin --json | jq -e '.serverStats and .dataStats and .expensiveStats'
```

Then modify `cmd/mr/commands/admin.go`:
- Add `"embed"` and `"mahresources/cmd/mr/helptext"` to imports.
- Add `//go:embed admin_help/*.md` and `var adminHelpFS embed.FS` at package scope (before `NewAdminCmd`).
- In `NewAdminCmd`, immediately before `cmd := &cobra.Command{` (around line 157), add: `help := helptext.Load(adminHelpFS, "admin_help/admin.md")`. Set `Long: help.Long`, `Example: help.Example`, `Annotations: help.Annotations` inside the struct literal.

Verify: `grep -n "helptext.Load" cmd/mr/commands/admin.go` returns one line.

- [ ] **Step 5: Append Batch 4 groups to `lintAllowlist`.**

Append these 6 keys:
```go
	"log":     true,
	"logs":    true,
	"plugin":  true,
	"plugins": true,
	"search":  true,
	"admin":   true,
```

After this edit, `lintAllowlist` contains all 32 top-level groups (3 from Phase 2 + 29 from Phase 3).

- [ ] **Step 6: Build + lint gate.** Same as prior batches.

- [ ] **Step 7: Doctest gate.** Same pattern.

**Critical verification for end-of-Batch-4:** `./mr docs dump --format markdown --output /tmp/docs-cli-probe/ 2>&1 | grep "unknown command" | wc -l` should output `0`. Every `relatedCmds` reference now resolves because every group is migrated. If the count is non-zero, inspect the stderr for the offending references and fix them inline in the relevant help file.

- [ ] **Step 8: Regenerate and commit content (Commit 1/2).**

```bash
npm run build-cli && npm run docs-gen
git add cmd/mr/commands/logs_help cmd/mr/commands/plugins_help cmd/mr/commands/search_help cmd/mr/commands/admin_help
git add cmd/mr/commands/logs.go cmd/mr/commands/plugins.go cmd/mr/commands/search.go cmd/mr/commands/admin.go
git add cmd/mr/commands/docs_lint.go
git add docs-site/docs/cli/
git commit -m "$(cat <<'EOF'
docs(cli): migrate log, plugin, search, admin (batch 4/5)

Adds embedded Markdown help for the last four groups (14 commands, 14
new help files). log and plugin via parallel subagents; search and admin
migrated inline since they are single-command groups. After this batch
every user-facing top-level group is in the lintAllowlist.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 9: Commit fixes (Commit 2/2, conditional).**

Same pattern.

Report retro: "Batch 4/5 complete: log, plugin, search, admin migrated (14 commands). All 147 Phase 3 commands migrated. Zero `unknown command` warnings. ..."

---

## Task 5: Batch 5 — CI wiring (coordinator only)

No subagents. Four changes: delete lintAllowlist, add docs-gen dirty-tree CI check, verify `go test ./cmd/mr/...` runs, wire CLI doctest job.

**Files modified this task:**
- `cmd/mr/commands/docs_lint.go` (delete allowlist)
- `.github/workflows/ci.yml` (add docs-gen check + CLI doctest job)

- [ ] **Step 1: Delete `lintAllowlist` from `docs_lint.go`.**

Open `cmd/mr/commands/docs_lint.go`. Two changes:

1. Delete the entire `lintAllowlist` variable block (and the `SetLintAllowlistForTest` helper if the existing tests don't require it — check `grep -n "SetLintAllowlistForTest" cmd/mr/commands/*.go` first). If tests still use it, keep the helper but change its semantics: `SetLintAllowlistForTest(nil)` means "allowlist is off, lint everything."

Actually the simpler approach: keep the variable but default it to `nil` (empty map). Replace:

Before:
```go
var lintAllowlist = map[string]bool{
	"docs":      true,
	"resource":  true,
	...
}
```

After:
```go
// lintAllowlist was used during phased migration to gate strict lint
// rules to already-migrated groups. Phase 3 completed migration of every
// top-level group, so the allowlist is now empty; the lint function
// treats an empty/nil allowlist as "validate everything".
var lintAllowlist map[string]bool
```

2. Update `lintCommandTreeTo` in the same file. Find the block:

```go
if !lintAllowlist[top] {
    continue
}
```

Replace with:

```go
if lintAllowlist != nil && !lintAllowlist[top] {
    continue
}
```

This preserves test compatibility: tests that call `SetLintAllowlistForTest(map[string]bool{...})` still scope lint to their allowlist; production runs with `lintAllowlist == nil` validate everything.

- [ ] **Step 2: Run full-tree lint + doctest verification.**

```bash
npm run build-cli
./mr docs lint
```
Expected: exit 0, `OK: N warnings`. If any `error:` lines appear, stop and fix them — these are gaps from earlier batches that the allowlist was hiding.

```bash
./mahresources -ephemeral -bind-address=:8181 -max-db-connections=2 &
SERVER_PID=$!
sleep 1
./mr docs check-examples --server http://127.0.0.1:8181 --environment ephemeral
kill $SERVER_PID 2>/dev/null; wait $SERVER_PID 2>/dev/null
```
Expected: exit 0.

```bash
npm run docs-gen
git status --porcelain docs-site/docs/cli/
```
Expected: `git status` is empty under `docs-site/docs/cli/` (regenerated tree matches committed tree). If there are changes, they're from the allowlist-removal having no effect on docs-gen (which writes all allowlisted groups) — but if allowlist is now nil, docs-gen writes everything. Inspect, stage, and include in Step 7's commit.

Also verify `warning: unknown command` count:
```bash
./mr docs dump --format markdown --output /tmp/phase3-probe/ 2>&1 | grep -c "unknown command"
```
Expected: `0`.

- [ ] **Step 3: Verify `go test ./cmd/mr/...` coverage.**

```bash
grep -n "cmd/mr" .github/workflows/ci.yml
```

`.github/workflows/ci.yml` already runs `go test --tags 'json1 fts5' ./...` (see line 23). `./...` includes `./cmd/mr/...`, so CLI tests already run on every PR. No change needed for this step; just verify the CI workflow hasn't been modified to exclude the CLI package.

Run the test suite locally to confirm:
```bash
go test --tags 'json1 fts5' ./cmd/mr/...
```
Expected: all tests pass, including `TestLintRealTree` in `cmd/mr/commands/docs_lint_main_test.go` which now validates the full tree (no allowlist).

- [ ] **Step 4: Add docs-gen dirty-tree CI check.**

Edit `.github/workflows/ci.yml`. After the existing `test` job, add a new job:

```yaml
  cli-docs-fresh:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - name: Install build dependencies
        run: sudo apt-get update && sudo apt-get install -y gcc libsqlite3-dev

      - name: Build CLI
        run: go build -o mr ./cmd/mr/

      - name: Regenerate docs-site/docs/cli/
        run: ./mr docs dump --format markdown --output docs-site/docs/cli/

      - name: Fail if regenerated docs differ from committed
        run: |
          if ! git diff --quiet -- docs-site/docs/cli/; then
            echo "::error::docs-site/docs/cli/ is out of sync with CLI help text."
            echo "Run 'npm run build-cli && npm run docs-gen' locally and commit the result."
            git diff --stat -- docs-site/docs/cli/
            exit 1
          fi

      - name: Run lint
        run: ./mr docs lint
```

- [ ] **Step 5: Wire the CLI doctest CI job.**

The Playwright project `cli-doctest` already exists (`e2e/playwright.config.ts` has it from Phase 1, and `e2e/package.json` has the `test:with-server:cli-doctest` script). What's missing is a CI workflow step that runs it.

Add this job to `.github/workflows/ci.yml`:

```yaml
  cli-doctest:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: npm
          cache-dependency-path: e2e/package-lock.json

      - name: Install build dependencies
        run: sudo apt-get update && sudo apt-get install -y gcc libsqlite3-dev

      - name: Build server
        run: go build --tags 'json1 fts5' -o mahresources

      - name: Build CLI
        run: go build -o mr ./cmd/mr/

      - name: Install root deps
        run: npm ci

      - name: Install E2E deps
        working-directory: e2e
        run: npm ci

      - name: Install Playwright browsers
        working-directory: e2e
        run: npx playwright install --with-deps chromium

      - name: Run CLI doctest against ephemeral server
        working-directory: e2e
        run: npm run test:with-server:cli-doctest
```

- [ ] **Step 6: Test the new CI jobs locally.**

Simulate the `cli-docs-fresh` job:
```bash
go build -o mr ./cmd/mr/
./mr docs dump --format markdown --output docs-site/docs/cli/
git diff --quiet -- docs-site/docs/cli/ && echo "CLEAN" || echo "DIRTY — investigate"
./mr docs lint
```
Expected: `CLEAN` + `OK: N warnings`.

Simulate the `cli-doctest` job (requires server binary):
```bash
go build --tags 'json1 fts5' -o mahresources
cd e2e && npm run test:with-server:cli-doctest
```
Expected: Playwright exits 0. The spec at `e2e/tests/cli/cli-doctest.spec.ts` starts an ephemeral server, runs `mr docs check-examples`, and asserts exit 0.

If either simulation fails, fix before committing the CI workflow changes.

- [ ] **Step 7: Commit Batch 5.**

```bash
git add cmd/mr/commands/docs_lint.go
git add .github/workflows/ci.yml
# Include any docs-site changes from Step 2 if allowlist removal caused drift:
git add docs-site/docs/cli/ 2>/dev/null || true
git commit -m "$(cat <<'EOF'
ci(cli): remove lint allowlist + wire docs-fresh & doctest jobs (batch 5/5)

Closes Phase 3. docs_lint.go's allowlist is now nil (lint validates every
command). Adds two CI jobs: cli-docs-fresh (fails on docs-site drift) and
cli-doctest (runs mr docs check-examples against an ephemeral server via
the existing Playwright harness).

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

Report: "Batch 5/5 complete: allowlist removed, CI wired. Phase 3 done. 147 commands migrated, doctest + docs-fresh gates active on PRs."

---

## Task 6: Phase 3 final verification

After all 5 batches land, run a full verification sweep.

- [ ] **Step 1: Clean rebuild.**

```bash
go build --tags 'json1 fts5' -o mahresources
go build -o mr ./cmd/mr/
```

Expected: both binaries build, exit 0.

- [ ] **Step 2: Lint passes across full tree.**

```bash
./mr docs lint
```
Expected: exit 0, `OK: N warnings`. No `error:` lines.

- [ ] **Step 3: Doctest passes across full tree.**

```bash
./mahresources -ephemeral -bind-address=:8181 -max-db-connections=2 &
SERVER_PID=$!
sleep 1
./mr docs check-examples --server http://127.0.0.1:8181 --environment ephemeral
kill $SERVER_PID 2>/dev/null; wait $SERVER_PID 2>/dev/null
```
Expected: exit 0.

- [ ] **Step 4: docs-gen produces clean tree.**

```bash
./mr docs dump --format markdown --output docs-site/docs/cli/
git diff --quiet -- docs-site/docs/cli/ && echo "CLEAN" || echo "DIRTY"
```
Expected: `CLEAN`. If dirty, investigate and commit the drift as a trailing fix commit.

- [ ] **Step 5: Go tests pass.**

```bash
go test --tags 'json1 fts5' ./cmd/mr/...
```
Expected: all pass, including `TestLintRealTree`.

- [ ] **Step 6: CLI E2E tests still pass.**

```bash
cd e2e && npm run test:with-server:cli
```
Expected: existing CLI spec suite still passes (no regressions from help-text changes).

- [ ] **Step 7: Postgres smoke.**

```bash
go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1
```
Expected: all pass (requires Docker running).

If all 7 steps green, Phase 3 is done. Final report to user: "Phase 3 complete: 147 commands migrated across 5 batches, lint + doctest + docs-gen-fresh CI wired, all tests passing. Ready for Phase 4 planning."

---

## Risks & Recovery

### Risk: A subagent produces invalid Go that prevents `go build`.

**Recovery:** the coordinator's Step 2 spot-check uses `grep -c "helptext.Load"` on each modified `.go` file before proceeding. If the count is wrong or the file fails to parse, the coordinator reads the file directly and fixes the typo (missing import, wrong path, bad struct literal). Do not re-dispatch the subagent — inline fix is faster.

### Risk: Doctest gate enters an infinite fix loop.

**Recovery:** after 3 rounds of fix-and-retry on the same command, stop and ask the user. If the bug is genuinely complex (e.g., a server-side API change needed), the coordinator can either (a) mark that doctest as `skip-on=ephemeral` with a `# TODO: fix <description>` comment in the help file and file a follow-up issue, or (b) pause the batch and hand off to the user. Do not silently disable doctests without recording the skip reason.

### Risk: Parallel subagents produce conflicting edits to `docs_lint.go`.

**Recovery:** cannot happen by design — subagents are forbidden from touching `docs_lint.go` (listed in every brief's forbidden-paths section). If a subagent did modify it anyway, the coordinator reverts that file before appending allowlist entries: `git checkout HEAD -- cmd/mr/commands/docs_lint.go`.

### Risk: `docs-site/docs/cli/` stays dirty after `npm run docs-gen` despite fresh build.

**Recovery:** inspect the diff. Common causes: timestamp in generated pages (if any), ordering of `relatedCmds` that differs from committed output. If the diff is structurally meaningful, that's a Phase 3 bug — investigate. If the diff is cosmetic (whitespace, ordering), commit it as the fix commit of whichever batch surfaced it.

### Risk: CI `cli-doctest` job times out or flakes.

**Recovery:** the CLI doctest Playwright spec uses the per-worker ephemeral server fixture from Phase 1 (`workerServer` in `e2e/fixtures/cli.fixture.ts`). If flaky, investigate whether a specific doctest block is slow (add `timeout=Ns` to its metadata) or whether the ephemeral server needs a warmup period (add a `sleep 1` after the server spawn in the spec). Do not lower CI's overall test timeout; fix the specific slow block.

---

## Appendix: Helpful Commands

```bash
# Full build (server + CLI + frontend)
npm run build

# Just CLI
npm run build-cli

# Start ephemeral server in background
./mahresources -ephemeral -bind-address=:8181 -max-db-connections=2 &

# Regenerate docs-site from CLI help
npm run docs-gen

# Lint the CLI help tree
./mr docs lint

# Run doctest against running server
./mr docs check-examples --server http://127.0.0.1:8181 --environment ephemeral

# Dump CLI tree as JSON for inspection
./mr docs dump --format json | jq .

# Count commands per top-level group
./mr docs dump --format json | jq -r '.commands[] | .path' | awk '{print $1}' | sort | uniq -c | sort -rn

# Run Go unit tests (CLI only)
go test --tags 'json1 fts5' ./cmd/mr/...

# Run CLI E2E (browser-driven Playwright spec)
cd e2e && npm run test:with-server:cli

# Run CLI doctest E2E (new in Phase 1)
cd e2e && npm run test:with-server:cli-doctest
```
