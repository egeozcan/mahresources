# mr CLI Documentation Overhaul: Design

**Date:** 2026-04-14
**Status:** Approved for planning
**Scope:** Comprehensive, example-first, agent-friendly documentation for every command, subcommand, and flag of the `mr` CLI. Generate the `docs-site` CLI pages from CLI help. Enforce the template with a linter and verify copy-paste examples work via a doctest runner.

## Goals

- `mr <anything> --help` shows a short line, a full description, 2+ labeled examples, rich flag help, and a "See Also" block. Every in-scope command (all user-facing commands except Cobra's built-in `help` and `completion`).
- A new user can type `mr resource --help` and, without prior context, understand what a resource is, what operations exist, and have copy-pasteable examples for the common flows.
- An AI agent can call `mr docs dump --format json` once and get the full command tree, persistent flags, local flags, inherited-flag references, positional-arg constraints, required-flag lists, examples, and output contracts in a structured form.
- `docs-site/docs/cli/` is generated from the CLI itself. One source of truth, no drift.
- CI fails if any command is missing required help fields.
- Copy-paste examples tagged `mr-doctest` are evaluated in CI against an ephemeral server. Non-skipped blocks must match their declared exit-code expectation (default `0`); blocks marked `skip-on=ephemeral` are not run on ephemeral runs.

## Non-Goals

- No custom ANSI theming beyond Cobra defaults.
- No behavioral changes to any command. Purely additive documentation. In particular: no changes to positional-argument contracts, no new flags, no changes to existing flag names or defaults, and no changes to exit codes.
- No translation to other languages.
- No per-version or changelog notes in help text.
- Cobra's built-in `help` and `completion` subcommands are out of scope. They are not documented in the CLI rewrite, not checked by the linter, and not doctested. The walker that drives `mr docs dump`, `lint`, and `check-examples` skips them.

## Current State

The `mr` CLI uses Cobra. There are 23 top-level commands and roughly 100 subcommands. Today every command has only a `Short` one-liner. No `Long` descriptions, no `Example` fields, and flag descriptions are often minimal (e.g., `"Meta JSON string"`). External CLI documentation in `docs-site/` is hand-written and drifts from the code. E2E CLI tests cover behavior but do not validate help text.

## Template

Every command's Cobra definition conforms to this shape. The linter enforces it.

```go
cmd := &cobra.Command{
    Use:   "get <id>",
    Short: "Get a resource by ID",
    Args:  cobra.ExactArgs(1),
    Long: `Get a resource by ID and print its metadata.

Fetches a single resource with its tags, groups, categories, and custom meta
fields. The resource ID is required as a positional argument.

Output is a table by default; pass the global --json flag for the full resource
record suitable for scripting.`,

    Example: `  # Get a resource by ID (table output)
  mr resource get 42

  # Get as JSON for scripting
  mr resource get 42 --json

  # Pipe into jq to extract a field
  mr resource get 42 --json | jq -r .name

  # mr-doctest: upload, fetch, assert name
  ID=$(mr resource upload ./testdata/sample.jpg --name "sample" --json | jq -r .id)
  mr resource get $ID --json | jq -e '.name == "sample"'`,

    Annotations: map[string]string{
        "outputShape":  "Resource object with id (uint), name (string), tags ([]Tag), groups ([]Group), meta (object)",
        "exitCodes":    "0 on success; 1 on any error (not-found, network, or invalid flags)",
        "relatedCmds":  "resource edit, resource versions, resource download",
    },
}
```

Note: the sample preserves the current positional-only signature (`<id>` plus `cobra.ExactArgs(1)`) and the current single-bucket exit-code behavior (`1` for any error). The overhaul does not change either. Documentation reflects the implementation as it exists today; it does not propose new invocation paths.

### Template Rules (enforced by linter)

| Field | Required for | Notes |
|---|---|---|
| `Short` | every command | ≤60 chars, imperative ("Get…", "Create…") |
| `Long` | every command | ≥2 sentences; explains when to use it and any non-obvious behavior |
| `Example` | every leaf command | ≥2 labeled examples; lines starting with `# ` are labels |
| `Example` with `mr-doctest:` label | recommended, not required | The linter emits a warning (not an error) when a leaf command has zero `mr-doctest:` examples. Authors decide per command whether a runnable example is meaningful. See the "Example Execution (doctest)" section for details. |
| Flag description | every flag | Purpose, format/type hint, default behavior when omitted; required flags say so explicitly |
| `Annotations["outputShape"]` | commands that emit data | One line describing the JSON shape |
| `Annotations["exitCodes"]` | all commands | `"0 on success; 1 on any error"` is the default and matches current behavior (see `cmd/mr/main.go`, which calls `os.Exit(1)` for any `Execute()` error). Override only if a command has demonstrably different exit behavior. This overhaul does not introduce new exit codes. |
| `Annotations["relatedCmds"]` | optional but encouraged | Comma-separated sibling or parent commands |
| Parent-group `Long` | every command group (`resource`, `group`, etc.) | Domain-model overview: entity, lifecycle, key concepts |

A custom Cobra help template renders a "See Also" block from `Annotations["relatedCmds"]` in `--help` output.

### Flag Ordering

Cobra renders flags alphabetically by default. We override this so required flags appear first, then optional, then global. Implemented via `cmd.Flags().SetSortFlags(false)` and explicit registration order.

## Code Organization

Long prose and examples live in embedded Markdown files. Flag help stays inline in Go.

```
cmd/mr/commands/
├── resources.go
├── resources_help/
│   ├── resource.md           # Long for parent `resource` group
│   ├── resource_get.md       # Long + Example + Annotations for `resource get`
│   ├── resource_upload.md
│   └── ...
├── groups.go
├── groups_help/
│   └── ...
```

Each `.md` file uses front matter for annotations and named sections for prose:

```markdown
---
outputShape: Resource object with id, name, tags, groups, meta
exitCodes: 0 on success; 1 on any error
relatedCmds: resource edit, resource versions, resource download
---

# Long

Get a resource by ID and print its metadata.

Fetches a single resource with its tags, groups, categories, and custom meta
fields. The resource ID is required as a positional argument.

# Example

  # Get a resource by ID (table output)
  mr resource get 42

  # Get as JSON for scripting
  mr resource get 42 --json

  # mr-doctest: upload, fetch, assert name
  ID=$(mr resource upload ./testdata/sample.jpg --name "sample" --json | jq -r .id)
  mr resource get $ID --json | jq -e '.name == "sample"'
```

Usage in Go:

```go
//go:embed resources_help/*.md
var resourceHelpFS embed.FS

func newResourceGetCmd(c *client.Client, opts *output.Options) *cobra.Command {
    help := helptext.Load(resourceHelpFS, "resources_help/resource_get.md")
    return &cobra.Command{
        Use:         "get <id>",
        Short:       "Get a resource by ID",
        Args:        cobra.ExactArgs(1),
        Long:        help.Long,
        Example:     help.Example,
        Annotations: help.Annotations,
        RunE:        /* unchanged */ nil,
    }
}
```

Flag help for commands that do have flags stays inline with the `.Flags()` calls (e.g., `resource edit` keeps its `cmd.Flags().StringVar(&name, "name", "", "New resource name (defaults to unchanged)")` lines).

### The `helptext` Package

New package at `cmd/mr/helptext/`. About 50-100 lines. Parses the Markdown front matter and named sections. Returns a struct:

```go
type Help struct {
    Long        string
    Example     string
    Annotations map[string]string
}

func Load(fs embed.FS, path string) Help
```

If the file is missing or malformed, `Load` panics at init time (fail loudly during startup; these are developer errors, not runtime conditions).

## The `mr docs` Command Group

New top-level command group for introspecting and validating the CLI's own documentation.

### `mr docs dump --format <json|markdown> [--output <path>]`

Emits the entire command tree with rich metadata. `--format` is required and accepts `json` or `markdown`. There is no `--json` shorthand. Default output target is stdout for `json`; `markdown` requires `--output <dir>`.

**JSON schema (per command):**

```json
{
  "name": "mr",
  "short": "CLI for mahresources",
  "long": "...",
  "persistentFlags": [
    { "name": "server", "type": "string", "default": "http://localhost:8181", "description": "mahresources server URL (env: MAHRESOURCES_URL)", "envVar": "MAHRESOURCES_URL" },
    { "name": "json", "type": "bool", "default": "false", "description": "Output raw JSON" },
    { "name": "no-header", "type": "bool", "default": "false", "description": "Omit table headers" },
    { "name": "quiet", "type": "bool", "default": "false", "description": "Only output IDs" },
    { "name": "page", "type": "int", "default": "1", "description": "Page number for list commands (default page size: 50)" }
  ],
  "commands": [
    {
      "path": "resource get",
      "short": "Get a resource by ID",
      "long": "Get a resource by ID and print its metadata...",
      "use": "get <id>",
      "args": {
        "constraint": "exact",
        "n": 1,
        "names": ["id"],
        "description": "Resource ID (numeric)"
      },
      "examples": [
        { "label": "Get a resource by ID (table output)", "command": "mr resource get 42", "doctest": false },
        { "label": "upload, fetch, assert name", "command": "ID=$(mr...)...", "doctest": true, "expectedExit": 0 }
      ],
      "localFlags": [],
      "inheritedFlags": ["server", "json", "no-header", "quiet", "page"],
      "requiredFlags": [],
      "outputShape": "Resource object with id, name, tags, groups, meta",
      "exitCodes": "0 on success; 1 on any error",
      "relatedCmds": ["resource edit", "resource versions", "resource download"]
    }
  ]
}
```

**Schema notes:**

- `persistentFlags` at the root level describes global flags inherited by every subcommand. They are not repeated per command; each command lists them by name in `inheritedFlags` so consumers can look them up.
- `localFlags` holds flags defined on that specific command. Each flag object has `name`, `type`, `default`, `description`, and `required` (boolean, derived from `cmd.MarkFlagRequired`).
- `requiredFlags` is a redundant convenience list of flag names that have `required: true`. Agents can use it as a quick check.
- `args` describes positional-argument constraints. `constraint` is one of `"exact"`, `"minimum"`, `"maximum"`, `"range"`, `"none"`. `n` is the count (or `min`/`max` for range). `names` is the ordered list of placeholder names parsed from `Use` (e.g., `get <id>` yields `["id"]`; `set <key> <value>` yields `["key", "value"]`). `description` is omitted by default; the positional-argument contract is expected to be explained in the command's `Long` prose. The field exists for future use if we add a dedicated `# Args` section to the help Markdown.
- `examples` entries carry an optional `expectedExit` integer (default `0`). Entries tagged with `# mr-doctest:` also accept `skip-on=ephemeral`, `expect-exit=N`, `tolerate=/regex/`, `timeout=Ns`, and `stdin=<file>` metadata keys on the label line (comma-separated after the human description). See the doctest section below.

**Markdown mode** writes one file per command into `--output <dir>`, structured as `docs-site/docs/cli/resource/get.md`. Generated pages render local flags, inherited flags (in a collapsible section), positional-arg contract, and the examples table.

### `mr docs lint`

Walks the command tree and validates every command against the template rules. Exits 0 if all pass; exits 1 with a grouped-by-command report on failure. Runs in CI.

### `mr docs check-examples [--server <url>]`

Extracts every `mr-doctest`-labeled example and evaluates it against the server according to its per-example metadata (see "Example Execution (doctest)" below):

- Skipped when `skip-on=ephemeral` is set and the runner is targeting an ephemeral seed.
- Executed as `bash -e -o pipefail -c "$block"` with `timeout` applied (default 30s, overridable).
- Optional stdin is piped from `stdin=<file>` if set.
- Exit code is evaluated against `expect-exit=N` (default `0`). If the actual exit does not match, the block fails unless `tolerate=/regex/` is set and stderr matches the regex, in which case the block passes.

Requires `MAHRESOURCES_URL`, `bash`, and `jq` on `PATH`. Runs in CI within the existing E2E ephemeral-server scaffolding. Exits 0 if every non-skipped block passes its evaluation.

## Example Execution (doctest)

Examples labeled `# mr-doctest:` are extracted and executed in CI. Examples without the tag are reference-only (can contain placeholders like `42`, `<id>`, `<name>`).

### Per-example Metadata

The `# mr-doctest:` label line accepts optional metadata keys, comma-separated after the description, in the form `key=value` or bare flags:

```markdown
# mr-doctest: upload, fetch, assert name
ID=$(mr resource upload ./testdata/sample.jpg --name "sample" --json | jq -r .id)
mr resource get $ID --json | jq -e '.name == "sample"'

# mr-doctest: rotate may fail on simple images, tolerate=/unexpected EOF|not supported/
mr resource rotate $ID --degrees 90 || true

# mr-doctest: seeded-only, skip-on=ephemeral
mr log get 1 --json
```

**Supported keys:**

| Key | Purpose |
|---|---|
| `expect-exit=N` | Assert exit code equals `N` instead of `0`. |
| `tolerate=/regex/` | If exit is non-zero, match stderr against regex; pass if it matches. Fails otherwise. |
| `skip-on=ephemeral` | Skip this block when running against an ephemeral seed. Useful for commands that need pre-existing data (e.g., `log get`, `job get`). |
| `timeout=Ns` | Override the default 30s per-block timeout. |
| `stdin=<file>` | Pipe the given fixture file into the block's stdin. |

This metadata lives in the label line so Markdown rendering of the examples stays readable. The doctest runner parses the label; the Markdown docs-site generator strips metadata keys from the displayed label and just shows the human description.

### Test Data

A new `cmd/mr/testdata/` directory holds fixture files (approximately 10 files, a few KB total):

- `sample.jpg`, `sample.png`, `sample.pdf`, `sample.txt`, `sample.md`
- `tiny.csv`, `tiny.json`

Doctest blocks run with `cwd` set to `cmd/mr/` so examples reference files as `./testdata/sample.jpg`.

### Guardrails

- Doctest blocks should be ≤5 lines typically. Long workflows belong in regular E2E tests.
- Each block is self-contained: creates what it needs, no assumed prior state.
- Destructive examples (`delete`) run last per command, against IDs they created themselves.
- Blocks run sequentially (not parallel) to eliminate SQLite contention against the shared ephemeral server.

### Opt-out Mechanism

Per-command opt-out is handled purely by per-example metadata plus the absence of `mr-doctest:` labels. There is no separate allowlist in code. If a command has no examples tagged `mr-doctest:`, no doctest runs for it.

The linter therefore does not require `mr-doctest:` for every leaf command. The rule is:

- Every leaf command requires ≥2 `Example` entries.
- Commands whose behavior can be exercised against an ephemeral seed should include ≥1 `mr-doctest:` example. Judgment call per command.
- Commands that cannot meaningfully doctest (e.g., `plugin`, which requires external binaries; `job get`, which needs a pre-existing async job; `log get`, which needs a pre-existing log row) ship reference-only examples and use `skip-on=ephemeral` if they want to include a runnable form for non-ephemeral environments.

This replaces the earlier "allowlist in `docs.go`" proposal. The per-example metadata is strictly more expressive and matches the pattern already used in the existing CLI E2E tests (e.g., `cli-resources.spec.ts:215` tolerates known errors for `resource rotate`; `cli-note-blocks.spec.ts` and `cli-queries.spec.ts` have similar patterns). Authors of doctest blocks should mirror those existing tolerances.

### Linter Rule Update

The linter's leaf-command requirements:

1. Every leaf command has ≥2 `Example` entries (unchanged).
2. Every leaf command has `outputShape` annotation if it emits data, otherwise omit.
3. Every leaf command has an `exitCodes` annotation (default text is acceptable).
4. `mr-doctest:` examples are not universally required. The linter emits a warning (not an error) for any leaf command with zero `mr-doctest:` blocks, so authors have to actively decide whether to add one.

## Docs-site Generation

Generated Markdown is committed to the repository. Docusaurus builds without needing the `mr` binary at CI time.

### Pipeline

```
mr docs dump --format markdown --output docs-site/docs/cli/
  ↓
docs-site/docs/cli/
├── index.md                    # Landing page: searchable command table
├── resource/
│   ├── index.md                # Group overview from resource.md `# Long`
│   ├── get.md
│   ├── upload.md
│   └── ...
├── group/
│   └── ...
└── ...
```

### Per-command Page Template

```markdown
---
title: mr resource get
description: Get a resource by ID
sidebar_label: get
---

# mr resource get

Get a resource by ID and print its metadata.

Fetches a single resource with its tags, groups, categories, and custom meta
fields. The resource ID is required as a positional argument.

## Usage

    mr resource get <id>

Positional arguments:

- `<id>` (required): Resource ID (numeric). Exactly one argument.

## Examples

**Get a resource by ID (table output)**

    mr resource get 42

**Get as JSON for scripting**

    mr resource get 42 --json

## Flags

This command has no local flags.

### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: `MAHRESOURCES_URL`) |
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--quiet` | bool | `false` | Only output IDs |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |

## Output

Resource object with id, name, tags, groups, meta.

## Exit Codes

0 on success; 1 on any error.

## See Also

- [`mr resource edit`](./edit.md)
- [`mr resource versions`](./versions.md)
- [`mr resource download`](./download.md)
```

### Integration

- New npm script in root `package.json`: `"docs-gen": "./mr docs dump --format markdown --output docs-site/docs/cli/"`. The CLI binary is `./mr`, produced by the existing `npm run build-cli` script (`go build -o mr ./cmd/mr/`). The server binary `./mahresources` is unrelated to CLI docs.
- A CI check runs `npm run build-cli && npm run docs-gen` and fails if the working tree is dirty. Ensures authors who change help text also regenerate docs.
- Existing hand-written CLI pages in `docs-site/` are deleted and replaced.
- The Docusaurus sidebar is extended with a small `sidebars.js` fragment defining the CLI section. The index page contains a searchable table of all commands.

## CI Integration

New checks:

1. **`go test` runs `mr docs lint`** as part of a unit test (`cmd/mr/commands/docs_test.go`). Fast, no server needed.
2. **`cmd/mr/helptext/` has unit tests** for the parser: valid file, missing section, malformed front matter.
3. **Doctest runs in E2E CLI job**: `e2e/` package gets a new script `test:with-server:cli-doctest` that starts an ephemeral server and runs `mr docs check-examples`. Parallel to existing CLI E2E tests.
4. **Docs-site regeneration check**: CI runs `npm run build-cli && npm run docs-gen` and fails if `git status --porcelain` is non-empty under `docs-site/docs/cli/`.

## Execution Plan

### Phase 1: Scaffolding

1. Create `cmd/mr/helptext/` package. Parser + tests.
2. Add `cmd/mr/commands/docs.go` with `dump`, `lint`, `check-examples` subcommands.
3. Implement `mr docs dump --format json` against the current command tree (pre-migration; most fields empty).
4. Implement `mr docs lint` with an allowlist that limits validation to migrated commands. Initially empty.
5. Add custom Cobra help template rendering "See Also" from `relatedCmds`.
6. Add a small helper that disables Cobra's alphabetical flag sort and applies it to the root and all subcommands (`SetSortFlags(false)` recursively). Actual flag reordering within each command happens during that command's migration in Phase 2 and Phase 3.
7. Add `cmd/mr/testdata/` fixture files.
8. Implement `mr docs check-examples` runner (bash-based, ephemeral server).

### Phase 2: Pilot on `resource`

9. Create `cmd/mr/commands/resources_help/*.md` for every subcommand (19 files: parent + 18 subcommands).
10. Update `resources.go` to load help from embedded Markdown.
11. Write rich `Long`, ≥2 `Example` entries per command, ≥1 `mr-doctest` block per non-excluded leaf.
12. Fill in `outputShape`, `exitCodes`, `relatedCmds` annotations.
13. Add `resource` to the lint allowlist; make `mr docs lint` pass for it.
14. Make `mr docs check-examples` pass for the `resource` group.
15. Implement `mr docs dump --format markdown --output <dir>`; generate `docs-site/docs/cli/resource/*.md`.
16. Review the pilot: is `mr resource get --help` actually impeccable? Adjust the template if needed.

### Phase 3: Fill-in

17. Migrate the remaining command groups in priority order: `group` → `note` → `mrql` → `search` → `query` → `note-type` → `tag` → `category` → `resource-category` → `relation` → `relation-type` → `note-block` → `group-export` → `group-import` → `series` → `timeline` → `admin` → `job` → `log` → `plugin`.
18. Remove the lint allowlist. Every command must pass.
19. Regenerate the full `docs-site/docs/cli/` tree. Delete old hand-written CLI pages.
20. Wire the docs-gen dirty-tree check into CI.
21. Wire `mr docs lint` and `mr docs check-examples` into CI.

### Phase 4: Polish

22. Update `README.md` CLI section to link to the regenerated docs-site.
23. Add a "Documentation" section to `CLAUDE.md`: "when you add a command or flag, add/update the `_help/*.md` file; CI enforces this."
24. Run the full test suite (Go unit + E2E browser + E2E CLI + CLI doctest + Postgres) and confirm green.

### PR Strategy

- **PR 1**: Phase 1 + Phase 2. Ships infrastructure and the `resource` pilot together. Proves the pattern end-to-end on the most complex group.
- **PR 2**: Phase 3 as a single PR. Large but mechanical. Reviewers see consistency across groups.
- **PR 3**: Phase 4 cleanup.

## Risks and Mitigations

**Risk: Generated docs-site files create noisy diffs.**
Mitigation: Accept the noise. Reviewers can see exactly how help renders. The dirty-tree check forces the regen to happen with the code change, so reviewers always review both together.

**Risk: Doctest runner is flaky under SQLite contention.**
Mitigation: Use the same ephemeral-server scaffolding the existing CLI E2E uses, including `-max-db-connections=2`. Doctest blocks run sequentially, not in parallel, to eliminate contention entirely.

**Risk: Markdown help files diverge from the Cobra definitions (e.g., flag renamed in Go but help file still mentions the old name).**
Mitigation: The linter checks that every flag has a description; flag descriptions stay inline in Go, co-located with the flag registration. Only Long and Example prose lives in Markdown, and those rarely mention specific flag names. When they do, CI catches drift because the doctest executes real commands.

**Risk: The 18-subcommand `resource` group's Markdown files become unwieldy.**
Mitigation: One file per subcommand, short and focused. Files never exceed ~200 lines. Front matter + two sections (`# Long`, `# Example`) is deliberately simple.

**Risk: Contributors forget to add `_help/*.md` when adding a new command.**
Mitigation: `mr docs lint` runs in CI and fails with a clear "command X missing Long/Example" message. `CLAUDE.md` documents the workflow.

## Open Questions Resolved During Brainstorming and Review

- Tone: example-first, technically complete, serves humans and agents.
- Source of truth: CLI help; docs-site generated.
- Phasing: one big sweep with a linter (blend of approaches A and D).
- Machine-readable help: `mr docs dump --format json` emits the full tree. There is no `--json` shorthand.
- Examples: hybrid. Reference examples use placeholders; runnable examples tagged `mr-doctest:` with optional per-example metadata (`expect-exit`, `tolerate`, `skip-on`, `timeout`, `stdin`).
- Docs-site files: committed, not generated at build time.
- Behavior: the overhaul is strictly additive. No positional-arg contract changes, no new flags, no flag renames, no exit-code changes. The sample template and all generated docs reflect the current implementation.
- Exit codes: current `mr` exits `1` for any error (see `cmd/mr/main.go`). Documentation reflects that. No new codes introduced by this work.
- CLI binary: `./mr`, built by `npm run build-cli`. Not `./mahresources` (which is the server binary).
- Cobra built-ins (`help`, `completion`): out of scope. Walker skips them; they are not linted, documented, or doctested.
- Doctest opt-out: per-example metadata on the label line, no separate code allowlist. `mr-doctest:` is not required for every leaf command; the linter emits a warning (not an error) when missing.
