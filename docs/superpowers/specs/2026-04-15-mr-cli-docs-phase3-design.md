# mr CLI Documentation Overhaul — Phase 3 Design (Delta)

**Date:** 2026-04-15
**Status:** Approved for planning
**Parent spec:** `docs/superpowers/specs/2026-04-14-mr-cli-docs-design.md`
**Scope:** Execution-level design for Phase 3 of the mr CLI documentation overhaul. Covers migration of the remaining command groups, the batched parallel dispatch strategy, the coordinator/subagent split, and the CI wiring that closes Phase 3. Lessons learned during Phase 2 pilot execution are captured here so Phase 3 subagents inherit them.

## Relationship to the Parent Spec

The parent spec (2026-04-14) defines the template, the `helptext` package, the `mr docs` command group, the doctest runner, and the docs-site generation pipeline. That design is unchanged. Phase 1 (scaffolding) and Phase 2 (pilot on `resource` + `resources`) shipped against it and proved the pattern end-to-end.

This delta spec covers what's needed to run Phase 3 cleanly:

- The lessons learned during Phase 2 that must inform every Phase 3 authoring task.
- The coordinator/subagent role split for parallel dispatch.
- The per-batch execution loop.
- The concrete batch composition for the ~147 remaining commands.
- The CI wiring that lands as the final batch of Phase 3.

Phase 4 (README update, CLAUDE.md documentation note, final full test sweep) is out of scope for this spec. It will be a tiny follow-up plan written after Phase 3 lands.

## Scope: What Phase 3 Migrates

Phase 2 shipped `docs`, `resource`, and `resources` (39 commands total). Phase 3 migrates the remaining **147 commands across 17 logical groups (29 singular/plural top-level entries)**. Priority order, as specified in the parent spec:

```
group → note → mrql → search → query → note-type → tag → category →
resource-category → relation → relation-type → note-block → group-export →
group-import → series → timeline → admin → job → log → plugin
```

Note: `group-export`, `group-import`, and `timeline` in the priority list are **not separate top-level commands** in the current CLI. They are:

- `group export` — a subcommand of `group` (defined in `cmd/mr/commands/group_export.go`, registered in `groups.go`).
- `group import` — a subcommand of `group` (defined in `cmd/mr/commands/group_import.go`, registered in `groups.go`).
- `<plural> timeline` — a leaf on each plural group that supports it (`groups timeline`, `notes timeline`, `queries timeline`, `tags timeline`; `resources timeline` already shipped in Phase 2). The file `cmd/mr/commands/timeline.go` holds shared helpers, not a top-level command.

They are therefore migrated **implicitly** as part of the subagent task that owns the parent group (the `group` subagent writes `group_export.md` and `group_import.md`; each plural subagent writes its `<plural>_timeline.md`). The priority list is preserved for historical reference; the batching below collapses them into their parent groups.

Command counts per logical group (verified via `./mr docs dump --format json | jq -r '.commands[] | .path' | awk '{print $1}' | sort | uniq -c`):

| Logical group | Singular | Plural | Total |
|---|---|---|---|
| `group` + `groups` | 12 | 9 | 21 |
| `note` + `notes` | 9 | 9 | 18 |
| `query` + `queries` | 9 | 3 | 12 |
| `tag` + `tags` | 6 | 5 | 11 |
| `note-block` + `note-blocks` | 7 | 4 | 11 |
| `category` + `categories` | 6 | 3 | 9 |
| `note-type` + `note-types` | 7 | 2 | 9 |
| `series` | 8 | — | 8 |
| `resource-category` + `resource-categories` | 6 | 2 | 8 |
| `relation-type` + `relation-types` | 6 | 2 | 8 |
| `job` + `jobs` | 6 | 2 | 8 |
| `plugin` + `plugins` | 5 | 2 | 7 |
| `relation` (no plural) | 5 | — | 5 |
| `mrql` | 5 | — | 5 |
| `log` + `logs` | 3 | 2 | 5 |
| `search` | 1 | — | 1 |
| `admin` | 1 | — | 1 |
| **Total** | | | **147** |

## Lessons from Phase 2

Phase 2 surfaced eight concrete lessons that every Phase 3 subagent must inherit. These go into every subagent prompt verbatim, and into the plan's risks section.

**1. Server dedupes resources globally by content hash.** Every doctest that uploads a fixture must create a unique owner group (use `$RANDOM` or a timestamp suffix) AND use a unique `--name`. Asserting on the uploaded resource's returned name is unsafe — the server may return the original resource with its original name. Assert on `.ID > 0` or run `mr resource edit` first and assert on the edited value. The 31 working doctests in `resources_help/*.md` are reference examples.

**2. `mr resource upload --json` returns a JSON array, not an object.** Use `jq -r '.[0].ID'` on upload pipelines. `get` / `version` commands return single objects (`.ID`). This is a pattern that varies by endpoint family — subagents must verify response shapes against live output, not assume.

**3. Field casing varies by endpoint.** Resource objects use PascalCase (`.ID`, `.Name`, `.Meta`, `.Tags`, `.Groups`, `.Width`, `.FileSize`). Version objects use lowercase (`.id`, `.versionNumber`, `.sameHash`). `resources meta-keys` returns `[{"key": "..."}]`, not a string array. Timeline returns `{buckets: [...]}` wrapping each bucket. Always verify against live output from a running ephemeral server, not inference from Go struct tags.

**4. `$$` (bash PID) does not expand inside single-quoted jq expressions.** Use `--arg n "$VAR"` and reference `$n` inside the jq expression, or use double quotes to bash-quote the jq string. This is a common footgun in doctest authoring.

**5. The doctest runner accumulates state within one `check-examples` run.** Each doctest block must be self-contained — create its own group, its own resources, its own tags. Many blocks run sequentially inside one `check-examples` invocation; cross-contamination is a real risk.

**6. Phase 2 surfaced a real CLI bug in `resources set-dimensions`** (was sending `{ID: [array]}` to an endpoint expecting a single uint; fix was to iterate over IDs). Similar bugs likely exist in other bulk commands. Phase 3 doctests will surface them — **fix the CLI code as part of the Phase 3 batch that discovers the bug**; do not defer or leave broken CLI in place. This is what the per-batch "fix commit" is for.

**7. docs-gen warnings about "unknown command" in See Also** are the signal that a `relatedCmds` reference points to a not-yet-allowlisted group. These warnings shrink as Phase 3 progresses and reach zero after the last batch. Do not treat early-batch warnings as failures.

**8. The Markdown generator filters doctest blocks out of published pages** (`cmd/mr/commands/docs_dump.go`). That means doctest blocks can reference `./testdata/sample.jpg` and `$RANDOM` without leaking into user-facing docs. Reference examples in help files are what users see; doctests are for CI only.

## Role Split: Coordinator and Subagents

Phase 3 executes as batched parallel dispatch. One main agent acts as **coordinator**; per-group work is delegated to short-lived parallel **subagents**.

### Coordinator owns

- `cmd/mr/commands/docs_lint.go` — appends groups to `lintAllowlist` at batch end.
- All lint and doctest validation: `./mr docs lint`, `./mr docs check-examples --environment ephemeral`.
- Docs-site regeneration: `npm run docs-gen`.
- Commits (content + fix) and git hygiene.
- Cross-group bug response: when doctest surfaces a CLI bug, coordinator either fixes it inline or dispatches a targeted debug subagent invoking `superpowers:systematic-debugging`.
- Inline migration of trivial single-command groups (`search`, `timeline`, `admin`, `group-export`, `group-import`). Dispatching a subagent for a 3-line Cobra builder is overkill.

### Subagent per group owns

- Its own `cmd/mr/commands/<group>_help/*.md` directory (creates every file).
- The corresponding `cmd/mr/commands/<group>.go` (adds `//go:embed <group>_help/*.md` and rewires every builder to `helptext.Load`).
- Its own `testdata/` additions if genuinely needed (rare; existing `cmd/mr/testdata/sample.*` usually suffices).

### Subagent does not touch

- `docs_lint.go` — owned by coordinator.
- `docs-site/` — regenerated by coordinator.
- Sibling groups' `_help/` directories — enforced via explicit forbidden-paths list in every subagent prompt.
- `helptemplate.go`, `docs.go`, `docs_dump.go`, `docs_doctest.go` — scaffolding, immutable for Phase 3.

If a subagent discovers a cross-cutting issue (e.g., a missing helper in the `helptext` package, a gap in the linter), it reports the issue in its return message and the coordinator handles the scope-widening decision. Subagents do not expand their scope.

## Per-Batch Execution Loop

Each batch runs through this nine-step loop. The coordinator executes it.

1. **Dispatch parallel subagents** in a single message (Agent tool calls batched). Each subagent gets a self-contained prompt including: group name, target files, pattern reference (`cmd/mr/commands/resources_help/` as the gold standard), the 8 Phase 2 gotchas above, the forbidden-paths list, doctest safety rules, and an expected deliverable ("report which leaves got `mr-doctest:` blocks and which were skipped with reason").

2. **Collect and spot-check** when all subagents return. Coordinator reads each group's `_help/` directory and `.go` file to confirm structural correctness: `//go:embed` directive present, every builder wired to `helptext.Load`, every help file has front-matter + `# Long` + `# Example` sections. If a subagent produced broken output, coordinator fixes inline rather than re-dispatching.

3. **Activate the allowlist.** Coordinator appends all completed groups (both singular and plural keys) to `lintAllowlist` in `docs_lint.go` in a single edit.

4. **Lint gate.** Run `go build -o mr ./cmd/mr/ && ./mr docs lint`. Any failure (missing `Long`, missing `Example`, missing `exitCodes`, etc.) → coordinator fixes inline. Re-run until lint passes with zero errors. Warnings are OK — they indicate leaves without `mr-doctest:` blocks, which is allowed under the Phase 2 parity target.

5. **Doctest gate.** Start an ephemeral server (`./mahresources -ephemeral -bind-address=:8181 -max-db-connections=2` in the background), run `./mr docs check-examples --server http://127.0.0.1:8181 --environment ephemeral`. Any failing block → coordinator investigates:
   - **Authoring bug** (wrong jq path, bad field casing, missing `--arg` escape) → coordinator fixes the help file.
   - **Real CLI bug** (like Phase 2's `set-dimensions`) → coordinator fixes the `.go` code inline if small, or dispatches a debugging subagent with `superpowers:systematic-debugging` if the scope is larger.
   Re-run the doctest until all blocks PASS or SKIP. Kill the ephemeral server.

6. **Regenerate docs-site.** `npm run build-cli && npm run docs-gen`. Commit the regenerated pages in the same commit as the source help text.

7. **Commit 1 — content.** Single commit per batch with a message like `docs(cli): migrate group, note, query, tag to embedded help (batch 1/5)`. Stages: affected `cmd/mr/commands/*_help/` directories, affected `.go` files, `docs_lint.go`, regenerated `docs-site/docs/cli/`.

8. **Commit 2 — fixes (conditional).** If any CLI bugs or doctest authoring fixes surfaced during the gate, they land in a separate commit like `fix(cli): <group> <short problem> + doctest refactor`. Keeping bug fixes in their own commit makes `git blame` on later CLI regressions informative. If nothing surfaced, commit 2 is skipped.

9. **Brief retro.** Coordinator reports a one-line summary to the user: groups migrated, doctests passing/skipped, any bugs fixed, any cross-group `relatedCmds` warnings that still remain.

## Batch Composition

Four parallel-dispatch batches plus one CI-wiring batch. Each parallel batch fires 4–5 subagents in a single message; the CI batch is coordinator-only.

### Batch 1 — heaviest, most-used entities (4 parallel subagents)

- `group` + `groups` (21 commands) — subagent must also write `group_export.md` and `group_import.md` for the `export`/`import` subcommands, and `groups_timeline.md` for the plural `timeline` leaf.
- `note` + `notes` (18 commands) — subagent must also write `notes_timeline.md`.
- `query` + `queries` (12 commands) — subagent must also write `queries_timeline.md`.
- `tag` + `tags` (11 commands) — subagent must also write `tags_timeline.md`.

**Total: 62 commands across 4 groups.** These are the highest-traffic user entities; getting their docs right early means the rest of Phase 3 has strong `relatedCmds` targets to link against.

### Batch 2 — organizational types (5 parallel subagents)

- `mrql` (5 commands)
- `note-type` + `note-types` (9 commands)
- `category` + `categories` (9 commands)
- `resource-category` + `resource-categories` (8 commands)
- `relation` (5 commands, no plural)

**Total: 36 commands across 5 groups.** Medium complexity; several are CRUD-shaped and should doctest cleanly.

### Batch 3 — relations, blocks, async state (4 parallel subagents)

- `relation-type` + `relation-types` (8 commands)
- `note-block` + `note-blocks` (11 commands)
- `series` (8 commands, no plural)
- `job` + `jobs` (8 commands)

**Total: 35 commands across 4 groups.** `job` commands need `skip-on=ephemeral` for `get`/`cancel`/etc. — no pre-existing async jobs to act on. Document the skip reason in commit message.

### Batch 4 — ops and standalones (2 parallel subagents + 2 inline)

Parallel:
- `log` + `logs` (5 commands) — needs `skip-on=ephemeral` for `log get` (no pre-existing rows); `log entity` can doctest against an ephemeral group ID.
- `plugin` + `plugins` (7 commands) — mostly reference-only; `plugins list` can doctest but `enable`/`disable`/`settings`/`purge-data` require external binaries and typically ship reference-only examples.

Coordinator inline (two single-command groups):
- `search` (1 command) — trivial; inline write of `search.md` + small edit to `search.go`.
- `admin` (1 command, sensitive) — inline write of `admin.md`; `admin` is a bulk operation with destructive potential, so doctest uses `skip-on=ephemeral` or a `tolerate=/regex/` escape.

**Total: 14 commands across 4 groups.** Closes out the per-group migration. Note: `group export`, `group import`, and `<plural> timeline` are not in this batch because they are subcommands of groups already migrated in Batch 1.

### Batch 5 — CI wiring (coordinator only)

No subagents. Four changes:

1. **Remove the allowlist from `docs_lint.go`** — simpler than inverting. `TestLintRealTree` already gates regressions by running the linter against the real command tree.
2. **CI: docs-gen dirty-tree check** — add a step that runs `npm run build-cli && npm run docs-gen && git diff --exit-code docs-site/docs/cli/`. Fails if the generated tree is stale.
3. **CI: `go test ./cmd/mr/...`** — verify this runs on every PR. If the existing CI already runs `go test ./...`, this is covered; otherwise, add a targeted job.
4. **CI: CLI doctest** — verify `cd e2e && npm run test:with-server:cli-doctest` runs on every PR. The Playwright project exists from Phase 1; CI wiring just needs to invoke it.

Verification before merging Batch 5:
- `./mr docs lint` passes with zero errors across the entire tree (allowlist removed).
- `./mr docs check-examples --environment ephemeral` passes against a fresh ephemeral server.
- `git status --porcelain docs-site/docs/cli/` is clean after a fresh `npm run docs-gen`.
- Zero "unknown command" warnings from docs-gen (all `relatedCmds` references resolve).

## Risks and Mitigations

**Risk: Subagents drift from the gold pattern.**
Mitigation: every subagent prompt includes a paragraph of exemplary content copied from `resources_help/resource_get.md` as a concrete template, plus the 8 Phase 2 gotchas inline. Coordinator spot-checks (step 2 of the per-batch loop) catch structural drift before the lint/doctest gates.

**Risk: `docs_lint.go` merge race between parallel subagents.**
Mitigation: eliminated by design. Only the coordinator edits `docs_lint.go`, once per batch, after all subagents return.

**Risk: Doctest state contamination within a batch.**
Mitigation: the runner already executes blocks sequentially (spec guarantee). Subagents are briefed that each block must be self-contained (unique owner group, unique `$RANDOM`-suffixed names). Lesson #5 is in every prompt.

**Risk: Cross-group `relatedCmds` references trigger "unknown command" warnings.**
Mitigation: expected during batches 1–3; warnings shrink as each batch lands. By end of Batch 4, zero warnings. Any remaining warning after Batch 4 indicates a typo or stale ref; coordinator fixes it in Batch 4's fix-commit.

**Risk: CLI bugs surfaced by new doctests block a batch.**
Mitigation: budgeted for. Each batch's second (fix) commit handles surfaced bugs. If a bug is too complex for inline fix, coordinator pauses the batch and dispatches a debugging subagent with `superpowers:systematic-debugging`. Batch does not commit until the fix lands.

**Risk: Subagents expand scope into shared files.**
Mitigation: every subagent prompt enumerates a forbidden-paths list (`docs_lint.go`, `docs-site/`, sibling `_help/` directories, `helptemplate.go`, `docs*.go`). Subagent task description emphasizes: "report cross-cutting issues back; do not fix them."

**Risk: 4–5 parallel subagents consume too much context in the main conversation.**
Mitigation: subagents return summaries, not full file contents. Coordinator uses the Explore/general-purpose subagent type as appropriate so return values are bounded. Spot-checks (step 2) read files directly rather than relying on subagent reports.

## Phase 3 Done Definition

- `lintAllowlist` deleted from `docs_lint.go` (or empty).
- `./mr docs lint` passes with zero errors across the full command tree.
- `./mr docs check-examples --environment ephemeral` passes against a fresh ephemeral server (all non-skipped blocks PASS or SKIP).
- `docs-site/docs/cli/` contains one generated page per command; `git status --porcelain docs-site/docs/cli/` is clean after `npm run docs-gen`.
- CI: docs-gen dirty-tree check, `go test ./cmd/mr/...`, and CLI doctest job are all wired and passing.
- Zero "unknown command" warnings from docs-gen.

## Phase 4 Handoff (Separate Plan)

Phase 4 is small and lands in its own follow-up plan. For context only:

- Delete any hand-written `docs-site/docs/features/cli*` pages that reference the old flat docs.
- README.md: update the CLI section to link to the regenerated `docs-site/docs/cli/` tree.
- CLAUDE.md: add a "CLI Documentation" note — "when you add or change a command or flag in `cmd/mr/commands/`, update the corresponding `<group>_help/*.md` file. CI runs `./mr docs lint` and `./mr docs check-examples`."
- Final full test sweep: Go unit + E2E browser + E2E CLI + CLI doctest + Postgres.

## Open Questions Resolved During Brainstorming

- **Plan file split:** Phase 3 plan covers migration + CI wiring; Phase 4 is a separate tiny follow-up plan.
- **Dispatch strategy:** Batched parallel writes (4–5 subagents per batch) + serial coordinator-owned validation. Trivial single-command groups done inline by the coordinator.
- **Batch composition:** four parallel-dispatch batches plus one coordinator-only CI batch. Groups balanced to roughly equal work per batch, heaviest groups first.
- **Design doc:** fresh delta spec (this file), not an amendment to the parent.
- **Commit strategy:** two commits per batch — one content commit, one conditional fix commit.
- **Doctest coverage target:** Phase 2 parity (~85–90%), per-command judgment. Lint warnings acceptable for genuinely un-doctestable commands.
- **Subagent scope guardrails:** explicit forbidden-paths list in every prompt. Subagents do not touch shared files; they report cross-cutting issues back to the coordinator.
- **Bug response:** CLI bugs surfaced during doctest gate are fixed in the same batch that discovered them, in the batch's fix-commit.
