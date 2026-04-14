# mr CLI Documentation Overhaul Implementation Plan (Phase 1 + Phase 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the scaffolding for example-first, agent-friendly CLI documentation (Phase 1) and migrate the `resource` and `resources` command groups to it as a pilot (Phase 2).

**Architecture:** Cobra command Long/Example text moves from inline strings to embedded Markdown files, parsed at init time by a new `cmd/mr/helptext/` package. A new `mr docs` command group exposes `dump`, `lint`, and `check-examples` subcommands that walk the command tree and enforce a strict template. The `dump` command emits JSON for agents and Markdown for the docs-site; the `lint` rule set is gated by an allowlist so we can migrate groups incrementally without failing CI. Runnable examples tagged `# mr-doctest:` execute against an ephemeral server with per-example metadata controlling skip/tolerate/exit-code behavior.

**Tech Stack:** Go, Cobra, `embed.FS`, SQLite + FTS5 (for ephemeral server), bash + jq (for doctest), Docusaurus (for docs-site consumption, unchanged in this plan).

**Spec reference:** `docs/superpowers/specs/2026-04-14-mr-cli-docs-design.md`

**Scope of this plan:** Phase 1 (scaffolding) and Phase 2 (pilot on `resource` + `resources`). Phase 3 (migrate remaining ~20 groups) and Phase 4 (cleanup, README updates, CLAUDE.md) will get their own plans.

---

## File Structure

**New files (Phase 1):**

| Path | Responsibility |
|---|---|
| `cmd/mr/helptext/helptext.go` | Parser for Markdown help files with front matter. Exports `Help` struct and `Load` function. |
| `cmd/mr/helptext/helptext_test.go` | Unit tests for parser (valid file, missing section, malformed front matter). |
| `cmd/mr/helptext/testdata/valid.md` | Valid parser fixture. |
| `cmd/mr/helptext/testdata/missing_long.md` | Malformed parser fixture (no `# Long` section). |
| `cmd/mr/helptext/testdata/malformed_front_matter.md` | Malformed parser fixture (bad YAML). |
| `cmd/mr/commands/docs.go` | Registers `mr docs` and the `dump`, `lint`, `check-examples` subcommands. |
| `cmd/mr/commands/docs_dump.go` | Walks the command tree and emits JSON or Markdown. |
| `cmd/mr/commands/docs_lint.go` | Validates commands against template rules; allowlist-gated. |
| `cmd/mr/commands/docs_doctest.go` | Extracts and evaluates `# mr-doctest:` example blocks. |
| `cmd/mr/commands/docs_test.go` | Unit tests for lint, dump JSON shape, doctest metadata parsing. |
| `cmd/mr/commands/helptemplate.go` | Custom Cobra help template (adds "See Also" from `relatedCmds`); helper to disable flag auto-sort recursively. |
| `cmd/mr/testdata/sample.jpg` | ~2 KB test JPEG (generate via ImageMagick or use an existing one from repo). |
| `cmd/mr/testdata/sample.png` | ~1 KB test PNG. |
| `cmd/mr/testdata/sample.pdf` | Tiny PDF (can be generated or copied from `e2e/fixtures/`). |
| `cmd/mr/testdata/sample.txt` | Plain text file. |
| `cmd/mr/testdata/sample.md` | Markdown file. |
| `cmd/mr/testdata/tiny.csv` | ~50-byte CSV. |
| `cmd/mr/testdata/tiny.json` | ~50-byte JSON. |
| `cmd/mr/testdata/README.md` | Notes for the doctest runner. |

**New files added in Task 4 (docs subtree help):**

| Path | Responsibility |
|---|---|
| `cmd/mr/commands/docs_help/docs.md` | `Long` for the `docs` parent group. |
| `cmd/mr/commands/docs_help/docs_dump.md` | Long + Example + Annotations for `mr docs dump`. |
| `cmd/mr/commands/docs_help/docs_lint.md` | Long + Example + Annotations for `mr docs lint`. |
| `cmd/mr/commands/docs_help/docs_check_examples.md` | Long + Example + Annotations for `mr docs check-examples`. |

**New files (Phase 2, 35 total):**

Singular `resource` subtree (22 files) — parent + 21 subcommands:

| Path | Responsibility |
|---|---|
| `resources_help/resource.md` | `Long` for parent `resource` command group (domain overview). |
| `resources_help/resource_get.md`, `resource_edit.md`, `resource_delete.md` | Basic CRUD. |
| `resources_help/resource_edit_name.md`, `resource_edit_description.md`, `resource_edit_meta.md` | Field-scoped edits (edit-meta uses `<id> <path> <value>`). |
| `resources_help/resource_upload.md`, `resource_download.md`, `resource_preview.md` | File ingest and export. |
| `resources_help/resource_from_url.md`, `resource_from_local.md` | Server-side ingest. |
| `resources_help/resource_rotate.md`, `resource_recalculate_dimensions.md` | Transforms. |
| `resources_help/resource_versions.md`, `resource_version.md`, `resource_version_upload.md`, `resource_version_download.md`, `resource_version_restore.md`, `resource_version_delete.md`, `resource_versions_cleanup.md`, `resource_versions_compare.md` | Version family. |

Plural `resources` subtree (13 files) — parent + 12 subcommands:

| Path | Responsibility |
|---|---|
| `resources_help/resources.md` | `Long` for parent `resources` (plural) command group. |
| `resources_help/resources_list.md` | List with filters + pagination. |
| `resources_help/resources_add_tags.md`, `resources_remove_tags.md`, `resources_replace_tags.md` | Bulk tag ops on `--ids`. |
| `resources_help/resources_add_groups.md`, `resources_add_meta.md` | Bulk group and meta adds on `--ids`. |
| `resources_help/resources_delete.md`, `resources_merge.md`, `resources_set_dimensions.md` | Destructive and shape-modifying bulk ops. |
| `resources_help/resources_versions_cleanup.md`, `resources_meta_keys.md`, `resources_timeline.md` | Admin-shaped operations. |

Authoritative source for the exact subcommand set: `grep -n "cmd.AddCommand(newResource" cmd/mr/commands/resources.go`. If any subcommand is added or removed before Phase 2 begins, update Task 10's placeholder list accordingly.

**Modified files:**

| Path | Changes |
|---|---|
| `cmd/mr/main.go` | Register `mr docs` command. Apply `SortFlags(false)` recursively via helper from `helptemplate.go`. Apply custom help template. |
| `cmd/mr/commands/resources.go` | Add `//go:embed resources_help/*.md`, call `helptext.Load(...)` in each command builder, populate `Long` / `Example` / `Annotations` from it. |
| `package.json` | Add `docs-gen` script (CLI invocation). Add `docs-lint` script (calls `mr docs lint`). |
| `e2e/playwright.config.ts` | Add `cli-doctest` Playwright project. |
| `e2e/package.json` | Add `test:with-server:cli-doctest` script (runs the new project). |
| `e2e/tests/cli/cli-doctest.spec.ts` | New spec that invokes `mr docs check-examples` against the per-worker ephemeral server. |

---

## Phase 1: Scaffolding

### Task 1: Create helptext package with parser and tests

Writes the Markdown parser first (TDD: failing tests before implementation). The parser extracts YAML front matter and `# Long` / `# Example` sections; returns a `Help` struct ready to drop into Cobra.

**Files:**
- Create: `cmd/mr/helptext/helptext.go`
- Create: `cmd/mr/helptext/helptext_test.go`
- Create: `cmd/mr/helptext/testdata/valid.md`
- Create: `cmd/mr/helptext/testdata/missing_long.md`
- Create: `cmd/mr/helptext/testdata/malformed_front_matter.md`

- [ ] **Step 1: Create the valid fixture**

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

  # mr-doctest: upload, fetch, assert name
  ID=$(mr resource upload ./testdata/sample.jpg --name "sample" --json | jq -r .id)
  mr resource get $ID --json | jq -e '.name == "sample"'
```

Save as `cmd/mr/helptext/testdata/valid.md`.

- [ ] **Step 2: Create the missing-long fixture**

Same as `valid.md` but delete the entire `# Long` block (front matter + `# Example` only). Save as `testdata/missing_long.md`.

- [ ] **Step 3: Create the malformed-front-matter fixture**

```markdown
---
outputShape: Resource object
exitCodes 0 on success
---

# Long

Body.
```

Note the missing colon on `exitCodes`. Save as `testdata/malformed_front_matter.md`.

- [ ] **Step 4: Write the failing test**

```go
package helptext

import (
	"embed"
	"testing"
)

//go:embed testdata/*.md
var testFS embed.FS

func TestLoadValid(t *testing.T) {
	h := Load(testFS, "testdata/valid.md")
	if !strings.Contains(h.Long, "Get a resource by ID and print its metadata.") {
		t.Fatalf("Long missing expected content: %q", h.Long)
	}
	if !strings.Contains(h.Example, "mr resource get 42") {
		t.Fatalf("Example missing expected content: %q", h.Example)
	}
	want := map[string]string{
		"outputShape": "Resource object with id, name, tags, groups, meta",
		"exitCodes":   "0 on success; 1 on any error",
		"relatedCmds": "resource edit, resource versions, resource download",
	}
	for k, v := range want {
		if h.Annotations[k] != v {
			t.Errorf("Annotations[%q] = %q, want %q", k, h.Annotations[k], v)
		}
	}
}

func TestLoadMissingLongPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for missing # Long section")
		}
	}()
	Load(testFS, "testdata/missing_long.md")
}

func TestLoadMalformedFrontMatterPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for malformed front matter")
		}
	}()
	Load(testFS, "testdata/malformed_front_matter.md")
}
```

Save as `cmd/mr/helptext/helptext_test.go`. Add `import "strings"` at the top.

- [ ] **Step 5: Run test to verify it fails**

Run: `go test ./cmd/mr/helptext/...`
Expected: compile error: `Load` and `Help` are undefined.

- [ ] **Step 6: Implement the parser**

```go
// Package helptext parses Markdown help files used by the mr CLI's
// Cobra commands. Each file has YAML-ish front matter (key: value lines
// between `---` fences) plus named sections (# Long, # Example).
package helptext

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

// Help holds the parsed contents of a help Markdown file.
type Help struct {
	Long        string
	Example     string
	Annotations map[string]string
}

// Load reads a help Markdown file from the given embedded filesystem
// and returns its parsed Help. Load panics on any error: help files
// are validated at program startup, so errors are developer mistakes
// that should halt the binary immediately.
func Load(fsys embed.FS, path string) Help {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		panic(fmt.Errorf("helptext: reading %s: %w", path, err))
	}
	h, err := parse(string(data))
	if err != nil {
		panic(fmt.Errorf("helptext: parsing %s: %w", path, err))
	}
	return h
}

func parse(s string) (Help, error) {
	annotations := map[string]string{}
	var long, example strings.Builder
	section := ""

	scanner := bufio.NewScanner(strings.NewReader(s))
	// Increase buffer to handle long Example blocks.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	inFrontMatter := false
	sawFrontMatter := false
	lineNum := 0
	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		switch {
		case lineNum == 1 && line == "---":
			inFrontMatter = true
			sawFrontMatter = true
			continue
		case inFrontMatter && line == "---":
			inFrontMatter = false
			continue
		case inFrontMatter:
			idx := strings.Index(line, ":")
			if idx < 0 {
				return Help{}, fmt.Errorf("front matter line %d missing colon: %q", lineNum, line)
			}
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			annotations[key] = val
			continue
		}

		if strings.HasPrefix(line, "# ") {
			section = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			continue
		}

		switch section {
		case "Long":
			long.WriteString(line)
			long.WriteByte('\n')
		case "Example":
			example.WriteString(line)
			example.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return Help{}, err
	}
	if !sawFrontMatter {
		return Help{}, fmt.Errorf("missing front matter (file must start with `---`)")
	}

	longStr := strings.TrimSpace(long.String())
	exampleStr := strings.TrimRight(example.String(), "\n")
	if longStr == "" {
		return Help{}, fmt.Errorf("missing `# Long` section")
	}

	return Help{
		Long:        longStr,
		Example:     exampleStr,
		Annotations: annotations,
	}, nil
}
```

Save as `cmd/mr/helptext/helptext.go`.

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./cmd/mr/helptext/...`
Expected: PASS (3 tests: `TestLoadValid`, `TestLoadMissingLongPanics`, `TestLoadMalformedFrontMatterPanics`).

- [ ] **Step 8: Commit**

```bash
git add cmd/mr/helptext/
git commit -m "feat(cli): add helptext package for Markdown-backed Cobra help"
```

---

### Task 2: Create CLI testdata fixtures

Small fixture files used by `# mr-doctest:` blocks. Copies / minimal generation.

**Files:**
- Create: `cmd/mr/testdata/sample.jpg`
- Create: `cmd/mr/testdata/sample.png`
- Create: `cmd/mr/testdata/sample.pdf`
- Create: `cmd/mr/testdata/sample.txt`
- Create: `cmd/mr/testdata/sample.md`
- Create: `cmd/mr/testdata/tiny.csv`
- Create: `cmd/mr/testdata/tiny.json`
- Create: `cmd/mr/testdata/README.md`

- [ ] **Step 1: Find existing test fixtures in the repo to copy**

Run: `ls e2e/fixtures/ && find . -name "*.jpg" -size -10k -not -path "./node_modules/*" -not -path "./.git/*" | head`
Expected: list of existing small images. Reuse an existing ≤5 KB JPG if found.

- [ ] **Step 2: Copy or generate the image fixtures**

For each of `sample.jpg`, `sample.png`: copy a tiny existing fixture, or use ImageMagick to generate one:

```bash
convert -size 32x32 xc:red cmd/mr/testdata/sample.jpg
convert -size 32x32 xc:blue cmd/mr/testdata/sample.png
```

For `sample.pdf`: copy from `e2e/fixtures/` if a small one exists, else generate:

```bash
printf '%%PDF-1.1\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Count 0/Kids[]>>endobj\nxref\n0 3\n0000000000 65535 f\n0000000009 00000 n\n0000000054 00000 n\ntrailer<</Size 3/Root 1 0 R>>\nstartxref\n97\n%%EOF\n' > cmd/mr/testdata/sample.pdf
```

- [ ] **Step 3: Create plain-text fixtures**

```bash
echo "This is a sample text file used by mr CLI doctest blocks." > cmd/mr/testdata/sample.txt
echo "# Sample Markdown

A tiny file for doctest fixtures." > cmd/mr/testdata/sample.md
echo "id,name
1,alpha
2,beta" > cmd/mr/testdata/tiny.csv
echo '{"hello":"world"}' > cmd/mr/testdata/tiny.json
```

- [ ] **Step 4: Write README for the testdata directory**

```markdown
# mr CLI testdata

Fixtures used by `# mr-doctest:` example blocks in the mr CLI help files.

Doctest blocks run with `cwd` set to `cmd/mr/`, so examples reference
files here as `./testdata/sample.jpg`, `./testdata/sample.pdf`, etc.

Files are intentionally tiny (a few KB combined) because the doctest
runner uploads them to an ephemeral server on every CI run.
```

Save as `cmd/mr/testdata/README.md`.

- [ ] **Step 5: Verify sizes**

Run: `du -b cmd/mr/testdata/*`
Expected: each file under 10 KB; total under 30 KB.

- [ ] **Step 6: Commit**

```bash
git add cmd/mr/testdata/
git commit -m "feat(cli): add testdata fixtures for CLI doctest runner"
```

---

### Task 3: Custom help template + SortFlags helper

Extends Cobra's default help output with a "See Also" block fed from `Annotations["relatedCmds"]`. Adds a helper that disables Cobra's alphabetical flag sort recursively.

**Files:**
- Create: `cmd/mr/commands/helptemplate.go`
- Create: `cmd/mr/commands/helptemplate_test.go`

- [ ] **Step 1: Write the failing test**

```go
package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestApplyHelpCustomizationsAddsSeeAlso(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "demo",
		Short: "demo",
		Annotations: map[string]string{
			"relatedCmds": "resource edit, resource versions",
		},
	}
	ApplyHelpCustomizations(cmd)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := cmd.Help(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "See Also") {
		t.Errorf("help missing See Also block:\n%s", out)
	}
	if !strings.Contains(out, "resource edit") || !strings.Contains(out, "resource versions") {
		t.Errorf("help missing related commands:\n%s", out)
	}
}

func TestApplyHelpCustomizationsDisablesSortFlags(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	child := &cobra.Command{Use: "child"}
	root.AddCommand(child)

	ApplyHelpCustomizations(root)

	if root.Flags().SortFlags {
		t.Error("root SortFlags should be false")
	}
	if child.Flags().SortFlags {
		t.Error("child SortFlags should be false")
	}
}
```

Save as `cmd/mr/commands/helptemplate_test.go`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/mr/commands/... -run TestApplyHelpCustomizations`
Expected: compile error: `ApplyHelpCustomizations` undefined.

- [ ] **Step 3: Implement the helper**

```go
package commands

import (
	"strings"

	"github.com/spf13/cobra"
)

// helpTemplate extends Cobra's default help template with a "See Also"
// block fed from Annotations["relatedCmds"] (comma-separated).
const helpTemplate = `{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}{{with .Annotations.relatedCmds}}
See Also:
{{range split . ","}}  - mr {{trim .}}
{{end}}{{end}}`

// ApplyHelpCustomizations applies the custom help template, disables
// alphabetical flag sort, and marks help/completion commands as hidden
// from the dump/lint/doctest walker. Call once with the root command.
func ApplyHelpCustomizations(root *cobra.Command) {
	cobra.AddTemplateFunc("split", strings.Split)
	cobra.AddTemplateFunc("trim", strings.TrimSpace)
	walk(root, func(c *cobra.Command) {
		c.SetHelpTemplate(helpTemplate)
		c.Flags().SortFlags = false
		c.LocalFlags().SortFlags = false
		c.InheritedFlags().SortFlags = false
	})
}

func walk(c *cobra.Command, fn func(*cobra.Command)) {
	fn(c)
	for _, child := range c.Commands() {
		walk(child, fn)
	}
}
```

Save as `cmd/mr/commands/helptemplate.go`.

- [ ] **Step 4: Run test to verify pass**

Run: `go test ./cmd/mr/commands/... -run TestApplyHelpCustomizations`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/mr/commands/helptemplate.go cmd/mr/commands/helptemplate_test.go
git commit -m "feat(cli): custom help template with See Also + flag-sort helper"
```

---

### Task 4: `mr docs` command skeleton with full help text + register in main.go

Registers the new `mr docs` command group with three subcommands. Each subcommand stubs its implementation with `"not implemented"` but loads its full help (Long, Example, Annotations) from embedded Markdown. This lets the `docs` subtree pass the linter when it is added to the allowlist in Task 16.

**Files:**
- Create: `cmd/mr/commands/docs.go`
- Create: `cmd/mr/commands/docs_help/docs.md`
- Create: `cmd/mr/commands/docs_help/docs_dump.md`
- Create: `cmd/mr/commands/docs_help/docs_lint.md`
- Create: `cmd/mr/commands/docs_help/docs_check_examples.md`
- Modify: `cmd/mr/main.go:82` (add `rootCmd.AddCommand(commands.NewDocsCmd())` and `ApplyHelpCustomizations(rootCmd)`)

- [ ] **Step 1: Create the 4 help files**

Write each with the minimal required fields. These are meta-commands; doctest blocks do not apply (they would recurse into themselves), so rely on reference-only examples.

`docs_help/docs.md` (parent group):

```markdown
---
exitCodes: 0 on success; 1 on any error
relatedCmds: docs dump, docs lint, docs check-examples
---

# Long

Introspect and validate the mr CLI's own documentation. The `docs` subcommands
walk the command tree to emit machine-readable JSON, generate docs-site
Markdown pages, validate help text against the template rules, and execute
runnable examples.

Use `mr docs` during CLI development to keep help text consistent, and in CI
to guarantee that published documentation stays in sync with the
implementation.
```

`docs_help/docs_dump.md`:

```markdown
---
outputShape: CommandTree JSON (when --format json) or directory of Markdown files (when --format markdown)
exitCodes: 0 on success; 1 on any error
relatedCmds: docs lint, docs check-examples
---

# Long

Emit the full mr command tree with rich metadata: persistent flags, per-command
local and inherited flags, required-flag lists, positional-argument contracts,
parsed examples, and Annotations (outputShape, exitCodes, relatedCmds). JSON
output is intended for agents and tooling; Markdown output is intended for the
docs-site (`docs-site/docs/cli/`).

Cobra's built-in `help` and `completion` subcommands are skipped: they are not
user-facing and are excluded from the documented contract.

# Example

  # Emit JSON to stdout (agent-friendly)
  mr docs dump --format json

  # Emit JSON to a file
  mr docs dump --format json --output /tmp/mr-tree.json

  # Regenerate docs-site pages
  mr docs dump --format markdown --output docs-site/docs/cli/
```

`docs_help/docs_lint.md`:

```markdown
---
exitCodes: 0 if all commands pass; 1 if any fail
relatedCmds: docs dump, docs check-examples
---

# Long

Validate every user-facing command's help against the template rules defined
in the spec: Short, Long, ≥2 Examples per leaf, rich flag descriptions,
required Annotations (outputShape where applicable, exitCodes), and sensible
Short length. Missing `# mr-doctest:` examples emit warnings, not errors.

Lint is allowlist-gated during migration: only command groups explicitly added
to the allowlist are subject to the strict rules, so partial migrations do not
block CI.

# Example

  # Lint the full command tree
  mr docs lint

  # Use in CI (non-zero exit fails the build)
  mr docs lint || exit 1
```

`docs_help/docs_check_examples.md`:

```markdown
---
exitCodes: 0 if every non-skipped doctest passes its declared expectation; 1 otherwise
relatedCmds: docs lint, docs dump
---

# Long

Walks the command tree, extracts every example tagged `# mr-doctest:`, and
evaluates each block against the connected server. Per-example metadata on the
label line controls behavior: `expect-exit=N`, `tolerate=/regex/`,
`skip-on=ephemeral`, `timeout=Ns`, and `stdin=<fixture>`.

The runner pipes each block through `bash -e -o pipefail -c`, with cwd set to
`cmd/mr/` so examples can reference `./testdata/*` fixtures. Requires
`MAHRESOURCES_URL`, `bash`, and `jq` on PATH.

# Example

  # Run against a local ephemeral server
  mr docs check-examples --server http://localhost:8181 --environment=ephemeral

  # Inherit server URL from the environment
  MAHRESOURCES_URL=http://localhost:8181 mr docs check-examples --environment=ephemeral
```

- [ ] **Step 2: Write the docs.go skeleton (with helptext)**

```go
package commands

import (
	"embed"
	"fmt"

	"github.com/spf13/cobra"

	"mahresources/cmd/mr/helptext"
)

//go:embed docs_help/*.md
var docsHelpFS embed.FS

// NewDocsCmd builds the `mr docs` command group.
func NewDocsCmd() *cobra.Command {
	help := helptext.Load(docsHelpFS, "docs_help/docs.md")
	cmd := &cobra.Command{
		Use:         "docs",
		Short:       "Introspect and validate the mr CLI's own documentation",
		Long:        help.Long,
		Annotations: help.Annotations,
	}
	cmd.AddCommand(newDocsDumpCmd())
	cmd.AddCommand(newDocsLintCmd())
	cmd.AddCommand(newDocsCheckExamplesCmd())
	return cmd
}

func newDocsDumpCmd() *cobra.Command {
	help := helptext.Load(docsHelpFS, "docs_help/docs_dump.md")
	var format, output string
	cmd := &cobra.Command{
		Use:         "dump",
		Short:       "Emit the mr command tree as JSON or Markdown",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			return dumpCommandTree(cmd.Root(), format, output)
		},
	}
	cmd.Flags().StringVar(&format, "format", "",
		"Output format: `json` (stdout by default) or `markdown` (requires --output). Required.")
	cmd.Flags().StringVar(&output, "output", "",
		"Output path. Required for `markdown`; optional for `json` (stdout when omitted).")
	_ = cmd.MarkFlagRequired("format")
	return cmd
}

func newDocsLintCmd() *cobra.Command {
	help := helptext.Load(docsHelpFS, "docs_help/docs_lint.md")
	cmd := &cobra.Command{
		Use:         "lint",
		Short:       "Validate every command's help against the template",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			return lintCommandTree(cmd.Root())
		},
	}
	return cmd
}

func newDocsCheckExamplesCmd() *cobra.Command {
	help := helptext.Load(docsHelpFS, "docs_help/docs_check_examples.md")
	var server, environment string
	cmd := &cobra.Command{
		Use:         "check-examples",
		Short:       "Execute every `# mr-doctest:` example block against a live server",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			return checkExamples(cmd.Root(), server, environment)
		},
	}
	cmd.Flags().StringVar(&server, "server", "",
		"Server URL (defaults to MAHRESOURCES_URL env var, then http://localhost:8181).")
	cmd.Flags().StringVar(&environment, "environment", "",
		"Target environment label used by `skip-on=<env>` metadata. Example: `ephemeral` when targeting a seed-less in-memory server.")
	return cmd
}

// Stubs; real implementations are in docs_dump.go, docs_lint.go, docs_doctest.go.
func dumpCommandTree(root *cobra.Command, format, output string) error {
	return fmt.Errorf("docs dump: not implemented")
}
func lintCommandTree(root *cobra.Command) error {
	return fmt.Errorf("docs lint: not implemented")
}
func checkExamples(root *cobra.Command, server, environment string) error {
	return fmt.Errorf("docs check-examples: not implemented")
}
```

Save as `cmd/mr/commands/docs.go`.

- [ ] **Step 2: Register docs command in main.go**

Open `cmd/mr/main.go`. After the line `rootCmd.AddCommand(commands.NewAdminCmd(c, opts))` (currently around line 82), add two lines:

```go
rootCmd.AddCommand(commands.NewDocsCmd())

commands.ApplyHelpCustomizations(rootCmd)
```

The call to `ApplyHelpCustomizations` MUST come after all `AddCommand` calls so it walks the fully-populated tree.

- [ ] **Step 3: Build and verify docs command exists**

Run:
```bash
npm run build-cli
./mr docs --help
```

Expected: output lists `dump`, `lint`, `check-examples` subcommands.

- [ ] **Step 4: Verify help customization took effect**

Run: `./mr --help`
Expected: alphabetical flag sort is gone (flags appear in registration order).

- [ ] **Step 5: Commit**

```bash
git add cmd/mr/commands/docs.go cmd/mr/commands/docs_help/ cmd/mr/main.go
git commit -m "feat(cli): scaffold 'mr docs' command group with help text"
```

---

### Task 5: Implement `mr docs dump --format json`

Walks the command tree, extracts metadata per command, emits the JSON schema defined in the spec (persistent flags at root + per-command entries with local/inherited/required flags, args constraints, examples with parsed metadata, annotations). Cobra's built-in `help` and `completion` commands are skipped.

**Files:**
- Create: `cmd/mr/commands/docs_dump.go`
- Modify: `cmd/mr/commands/docs.go` (delete the `dumpCommandTree` stub)
- Create: `cmd/mr/commands/docs_dump_test.go`

- [ ] **Step 1: Write failing tests for JSON shape**

Test that `dumpCommandTree` on a toy root produces: expected top-level keys, skips `help`/`completion`, extracts inherited flags correctly, parses positional args from `Use`, and parses `# mr-doctest:` metadata from Example content. See the spec § `mr docs dump` for the schema; tests should assert each top-level field and each per-command field.

Write concrete table-driven tests in `cmd/mr/commands/docs_dump_test.go`. At minimum:

- `TestDumpSkipsHelpAndCompletion`: builds a root with `help` / `completion` children and asserts they are absent from the output.
- `TestDumpExtractsPersistentFlags`: sets `server`, `json`, etc. on root and asserts `persistentFlags` lists them.
- `TestDumpExtractsArgsConstraint`: Cobra's `Args` function is not directly inspectable; instead parse the `Use` string and an optional `Annotations["argsConstraint"]` override. Assert:
  - `Use: "get <id>"` yields `{constraint: "exact", n: 1, names: ["id"]}`
  - `Use: "compare <a> <b>"` yields `{constraint: "exact", n: 2, names: ["a","b"]}`
  - `Use: "list"` yields `{constraint: "none"}`
  - `Use: "set <key> [value]"` yields `{constraint: "range", min: 1, max: 2, names: ["key","value"]}`
  - `Use: "export <id> [<id>...]"` yields `{constraint: "minimum", min: 1, names: ["id"]}` (variadic marker `...` triggers minimum)
  - `Use: "export [<id>...]"` yields `{constraint: "minimum", min: 0, names: ["id"]}`
  - `Annotations["argsConstraint"] = "range:2-4"` wins over the Use inference and yields `{constraint: "range", min: 2, max: 4, names: [...]}`
- `TestDumpUsesArgsConstraintAnnotation`: when a command sets `Annotations["argsConstraint"]`, the annotation is parsed and overrides the Use-string inference.
- `TestDumpParsesDoctestMetadata`: example content with `# mr-doctest: upload and fetch, expect-exit=2, skip-on=ephemeral` is parsed into `{label: "upload and fetch", doctest: true, expectedExit: 2, skipOn: "ephemeral"}`.
- `TestDumpExtractsRequiredFlags`: flags marked required via `MarkFlagRequired` appear in `requiredFlags`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/mr/commands/... -run TestDump`
Expected: FAIL on compile (symbols don't exist yet).

- [ ] **Step 3: Implement the dumper**

Key functions (full signatures):

```go
package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type dumpRoot struct {
	Name            string        `json:"name"`
	Short           string        `json:"short"`
	Long            string        `json:"long"`
	PersistentFlags []dumpFlag    `json:"persistentFlags"`
	Commands        []dumpCommand `json:"commands"`
}

type dumpCommand struct {
	Path           string        `json:"path"`
	Short          string        `json:"short"`
	Long           string        `json:"long,omitempty"`
	Use            string        `json:"use"`
	IsGroup        bool          `json:"isGroup"`
	Args           dumpArgs      `json:"args"`
	Examples       []dumpExample `json:"examples"`
	LocalFlags     []dumpFlag    `json:"localFlags"`
	InheritedFlags []string      `json:"inheritedFlags"`
	RequiredFlags  []string      `json:"requiredFlags"`
	OutputShape    string        `json:"outputShape,omitempty"`
	ExitCodes      string        `json:"exitCodes,omitempty"`
	RelatedCmds    []string      `json:"relatedCmds,omitempty"`
}

type dumpArgs struct {
	Constraint string   `json:"constraint"`
	N          int      `json:"n,omitempty"`
	Min        int      `json:"min,omitempty"`
	Max        int      `json:"max,omitempty"`
	Names      []string `json:"names"`
}

type dumpFlag struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Default     string `json:"default"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	EnvVar      string `json:"envVar,omitempty"`
}

type dumpExample struct {
	Label        string `json:"label"`
	Command      string `json:"command"`
	Doctest      bool   `json:"doctest"`
	ExpectedExit int    `json:"expectedExit,omitempty"`
	SkipOn       string `json:"skipOn,omitempty"`
	Tolerate     string `json:"tolerate,omitempty"`
	TimeoutSec   int    `json:"timeoutSec,omitempty"`
	Stdin        string `json:"stdin,omitempty"`
}

// dumpCommandTree walks the root Cobra tree and emits JSON or Markdown.
func dumpCommandTree(root *cobra.Command, format, output string) error {
	tree := buildDump(root)
	switch format {
	case "json":
		return writeJSON(tree, output)
	case "markdown":
		if output == "" {
			return fmt.Errorf("--output is required for markdown format")
		}
		return writeMarkdown(tree, output) // implemented in Task 6
	default:
		return fmt.Errorf("invalid --format: %q (want json or markdown)", format)
	}
}

func buildDump(root *cobra.Command) dumpRoot {
	r := dumpRoot{
		Name:            root.Name(),
		Short:           root.Short,
		Long:            root.Long,
		PersistentFlags: collectFlags(root.PersistentFlags(), nil),
	}
	persistentNames := map[string]bool{}
	for _, f := range r.PersistentFlags {
		persistentNames[f.Name] = true
	}
	for _, c := range walkSkippingBuiltins(root) {
		if c == root {
			continue
		}
		r.Commands = append(r.Commands, buildDumpCommand(c, persistentNames))
	}
	return r
}

func walkSkippingBuiltins(root *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	var rec func(*cobra.Command)
	rec = func(c *cobra.Command) {
		name := c.Name()
		if name == "help" || name == "completion" {
			return
		}
		out = append(out, c)
		for _, ch := range c.Commands() {
			rec(ch)
		}
	}
	rec(root)
	return out
}

var argNameRE = regexp.MustCompile(`[<\[]([a-z0-9-]+)[>\]]`)

// parseArgsFromUse infers positional-argument constraints from the command's
// Use string. Cobra's Args validators are function values and cannot be
// introspected; the Use string plus an optional Annotations override are the
// authoritative sources.
//
// Grammar recognised:
//
//   <name>            required positional
//   [name]            optional positional
//   [<name>...]       variadic, zero or more (constraint="minimum", min=0)
//   <name> [<name>...]variadic, one or more  (constraint="minimum", min=1)
//
// A command can override the inferred result by setting
// Annotations["argsConstraint"] to one of: "none", "exact:N", "minimum:N",
// "maximum:N", "range:MIN-MAX". Use the annotation when the Use string
// cannot express the real validator (e.g. cobra.RangeArgs(2,4)).
func parseArgsFromUse(use string, annotation string) dumpArgs {
	if annotation != "" {
		if a, ok := parseArgsAnnotation(annotation); ok {
			a.Names = collectNames(use)
			return a
		}
	}

	names := collectNames(use)
	hasVariadic := strings.Contains(use, "...")
	required := 0
	optional := 0
	for _, m := range argNameRE.FindAllStringSubmatchIndex(use, -1) {
		openCh := use[m[0]]
		if openCh == '<' {
			required++
		} else {
			optional++
		}
	}

	switch {
	case hasVariadic:
		// required counts the bare `<name>` before the variadic tail;
		// Cobra's MinimumNArgs takes the required count as the floor.
		return dumpArgs{Constraint: "minimum", Min: required, Names: names}
	case required == 0 && optional == 0:
		return dumpArgs{Constraint: "none", Names: nil}
	case optional == 0:
		return dumpArgs{Constraint: "exact", N: required, Names: names}
	case required == 0:
		return dumpArgs{Constraint: "maximum", Max: optional, Names: names}
	default:
		return dumpArgs{Constraint: "range", Min: required, Max: required + optional, Names: names}
	}
}

func collectNames(use string) []string {
	matches := argNameRE.FindAllStringSubmatch(use, -1)
	var names []string
	seen := map[string]bool{}
	for _, m := range matches {
		if seen[m[1]] {
			continue // avoid duplicating `<id>` in `<id> [<id>...]`
		}
		seen[m[1]] = true
		names = append(names, m[1])
	}
	return names
}

var argsAnnotationRE = regexp.MustCompile(`^(none|exact|minimum|maximum|range)(?::(\d+)(?:-(\d+))?)?$`)

func parseArgsAnnotation(s string) (dumpArgs, bool) {
	m := argsAnnotationRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return dumpArgs{}, false
	}
	switch m[1] {
	case "none":
		return dumpArgs{Constraint: "none"}, true
	case "exact":
		n, _ := strconv.Atoi(m[2])
		return dumpArgs{Constraint: "exact", N: n}, true
	case "minimum":
		n, _ := strconv.Atoi(m[2])
		return dumpArgs{Constraint: "minimum", Min: n}, true
	case "maximum":
		n, _ := strconv.Atoi(m[2])
		return dumpArgs{Constraint: "maximum", Max: n}, true
	case "range":
		lo, _ := strconv.Atoi(m[2])
		hi, _ := strconv.Atoi(m[3])
		return dumpArgs{Constraint: "range", Min: lo, Max: hi}, true
	}
	return dumpArgs{}, false
}

func buildDumpCommand(c *cobra.Command, persistent map[string]bool) dumpCommand {
	local := collectFlags(c.LocalFlags(), persistent)
	var required []string
	for _, f := range local {
		if f.Required {
			required = append(required, f.Name)
		}
	}
	var inherited []string
	c.InheritedFlags().VisitAll(func(f *pflag.Flag) {
		if persistent[f.Name] {
			inherited = append(inherited, f.Name)
		}
	})
	related := parseRelatedCmds(c.Annotations["relatedCmds"])
	return dumpCommand{
		Path:           c.CommandPath()[len(c.Root().Name())+1:],
		Short:          c.Short,
		Long:           c.Long,
		Use:            c.Use,
		IsGroup:        c.HasSubCommands(),
		Args:           parseArgsFromUse(c.Use, c.Annotations["argsConstraint"]),
		Examples:       parseExamples(c.Example),
		LocalFlags:     local,
		InheritedFlags: inherited,
		RequiredFlags:  required,
		OutputShape:    c.Annotations["outputShape"],
		ExitCodes:      c.Annotations["exitCodes"],
		RelatedCmds:    related,
	}
}

func parseRelatedCmds(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func collectFlags(set *pflag.FlagSet, skip map[string]bool) []dumpFlag {
	var out []dumpFlag
	set.VisitAll(func(f *pflag.Flag) {
		if skip != nil && skip[f.Name] {
			return
		}
		required := false
		for _, a := range f.Annotations["cobra_annotation_bash_completion_one_required_flag"] {
			if a == "true" {
				required = true
			}
		}
		out = append(out, dumpFlag{
			Name:        f.Name,
			Type:        f.Value.Type(),
			Default:     f.DefValue,
			Description: f.Usage,
			Required:    required,
			EnvVar:      envVarFromUsage(f.Usage),
		})
	})
	return out
}

var envVarRE = regexp.MustCompile(`env: ([A-Z_][A-Z0-9_]*)`)

func envVarFromUsage(usage string) string {
	if m := envVarRE.FindStringSubmatch(usage); m != nil {
		return m[1]
	}
	return ""
}

var (
	labelLineRE = regexp.MustCompile(`^\s*#\s+(.+)$`)
	doctestRE   = regexp.MustCompile(`^mr-doctest:\s*(.+)$`)
	metaKVRE    = regexp.MustCompile(`^\s*([a-z-]+)(?:=(.*))?\s*$`)
)

func parseExamples(s string) []dumpExample {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	var out []dumpExample
	var cur *dumpExample
	var body strings.Builder
	flush := func() {
		if cur == nil {
			return
		}
		cur.Command = strings.TrimRight(body.String(), "\n")
		out = append(out, *cur)
		cur = nil
		body.Reset()
	}
	for _, line := range lines {
		if m := labelLineRE.FindStringSubmatch(line); m != nil {
			flush()
			cur = &dumpExample{}
			label := m[1]
			if dm := doctestRE.FindStringSubmatch(label); dm != nil {
				cur.Doctest = true
				label = dm[1]
			}
			cur.Label, *cur = applyExampleMetadata(label, *cur)
			continue
		}
		if cur == nil {
			continue
		}
		body.WriteString(strings.TrimPrefix(line, "  "))
		body.WriteByte('\n')
	}
	flush()
	return out
}

func applyExampleMetadata(raw string, ex dumpExample) (string, dumpExample) {
	// raw is the full label line minus the leading "mr-doctest: ".
	// Split on commas; the first segment is the human description; remaining
	// segments are key=value or bare-key metadata.
	parts := strings.Split(raw, ",")
	label := strings.TrimSpace(parts[0])
	for _, p := range parts[1:] {
		kv := metaKVRE.FindStringSubmatch(p)
		if kv == nil {
			continue
		}
		switch kv[1] {
		case "expect-exit":
			n, _ := strconv.Atoi(strings.TrimSpace(kv[2]))
			ex.ExpectedExit = n
		case "skip-on":
			ex.SkipOn = strings.TrimSpace(kv[2])
		case "tolerate":
			ex.Tolerate = strings.Trim(strings.TrimSpace(kv[2]), "/")
		case "timeout":
			v := strings.TrimSuffix(strings.TrimSpace(kv[2]), "s")
			n, _ := strconv.Atoi(v)
			ex.TimeoutSec = n
		case "stdin":
			ex.Stdin = strings.TrimSpace(kv[2])
		}
	}
	return label, ex
}

func writeJSON(tree dumpRoot, output string) error {
	var w io.Writer = os.Stdout
	if output != "" {
		f, err := os.Create(output)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(tree)
}
```

Save as `cmd/mr/commands/docs_dump.go`. Delete the `dumpCommandTree` stub from `docs.go`.

- [ ] **Step 4: Run unit tests to verify they pass**

Run: `go test ./cmd/mr/commands/... -run TestDump -v`
Expected: all `TestDump*` tests pass.

- [ ] **Step 5: Manual smoke-test**

Run:
```bash
npm run build-cli
./mr docs dump --format json | jq '.name'
```
Expected: `"mr"`.

Run: `./mr docs dump --format json | jq '.commands | length'`
Expected: number of real subcommands (should exclude `help`, `completion`, and `docs dump`/`lint`/`check-examples` because `docs` walkers treat them like regular commands: verify the count is plausible, not that help/completion are excluded).

- [ ] **Step 6: Commit**

```bash
git add cmd/mr/commands/docs.go cmd/mr/commands/docs_dump.go cmd/mr/commands/docs_dump_test.go
git commit -m "feat(cli): implement 'mr docs dump --format json'"
```

---

### Task 6: Implement `mr docs dump --format markdown`

Reuses the dump data structure from Task 5. Renders one Markdown file per leaf command into `--output <dir>`, plus an `index.md` at the group level. Does NOT generate Phase 2 help content yet: this produces output based on whatever Long/Example is already on each command (which, pre-migration, will be mostly empty).

**Files:**
- Modify: `cmd/mr/commands/docs_dump.go` (add `writeMarkdown` function and supporting templates)
- Create: `cmd/mr/commands/docs_dump_markdown_test.go`

- [ ] **Step 1: Write failing test**

Test fixture: build a minimal command tree with one group (`foo`) and one leaf (`foo bar`) with pre-set `Long` / `Example` / `Annotations`. Call `writeMarkdown` into a temp dir. Assert:

- `foo/index.md` exists and contains the group's `Long`.
- `foo/bar.md` exists and contains `# mr foo bar`, the `Long`, the "Examples" section with labeled blocks, a "Flags" table with local flags, an "Inherited global flags" section, an "Exit Codes" section, and a "See Also" section.
- `index.md` (root) exists and lists both `foo` and `foo bar` in a table.

- [ ] **Step 2: Run test (expect FAIL)** (compile error: `writeMarkdown` is an error stub).

Run: `go test ./cmd/mr/commands/... -run TestDumpMarkdown`
Expected: FAIL.

- [ ] **Step 3: Implement `writeMarkdown`**

Key functions:

Key design decisions encoded in the implementation below:

1. **Parent command pages live at `<group>/index.md`**, NOT `<group>/<group>.md`. This matches Docusaurus conventions and the Task 6 unit test's expectation.
2. **Leaf command pages live at `<group>/<leaf>.md`** with hyphens in the leaf path preserved (e.g., `resource/version-upload.md`).
3. **See Also links are computed as relative paths between the page's actual output path and the target command's output path.** A link from `resource/get.md` to `resource edit` is `./edit.md`; a link from `resource/get.md` to `group list` is `../group/list.md`. Use the `relPath` helper.

```go
func writeMarkdown(tree dumpRoot, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}

	// Build a path-to-output-file map so See Also can generate correct
	// relative links regardless of which page is being written.
	outputPath := map[string]string{} // key: command path (e.g., "resource get"); value: filesystem path
	for _, c := range tree.Commands {
		outputPath[c.Path] = commandOutputPath(c, outputDir)
	}

	if err := writeRootIndex(tree, outputDir); err != nil {
		return err
	}

	for _, c := range tree.Commands {
		target := outputPath[c.Path]
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := writeCommandPage(c, tree, target, outputPath); err != nil {
			return err
		}
	}
	return nil
}

// commandOutputPath returns the on-disk path for a dumped command.
// Parent/group commands (any command that has its own descendants in the
// tree) are written to <group>/index.md; leaves are written to
// <group>/<leaf-path-with-slashes>.md.
func commandOutputPath(c dumpCommand, outputDir string) string {
	parts := strings.Split(c.Path, " ")
	if c.IsGroup {
		return filepath.Join(append([]string{outputDir}, append(parts, "index.md")...)...)
	}
	// Leaf: <outputDir>/<top>/<rest>.md; rest has dashes preserved.
	top := parts[0]
	rest := strings.Join(parts[1:], "/") + ".md"
	return filepath.Join(outputDir, top, rest)
}

func writeRootIndex(tree dumpRoot, outputDir string) error {
	// Writes <outputDir>/index.md with Docusaurus front matter and a
	// three-column table: Command | Short | Link. Sorted by path.
	// The link targets use the same relative-path helper so rendered
	// HTML works from the site root.
	// Implementation: build the list, sort by path, render via a
	// text/template constant. No placeholders; the spec's Docusaurus
	// format is known.
}

func writeCommandPage(c dumpCommand, tree dumpRoot, targetPath string, outputPath map[string]string) error {
	// Body structure (rendered via text/template):
	//
	//   ---
	//   title: mr <path>
	//   description: <short>
	//   sidebar_label: <last path segment>
	//   ---
	//
	//   # mr <path>
	//
	//   <long>
	//
	//   ## Usage
	//
	//       mr <path> <use portion after the command name>
	//
	//   (Optional) Positional arguments section rendered from c.Args:
	//     - omit entirely when constraint == "none"
	//     - for "exact", list each name as required
	//     - for "minimum" with variadic, list the required floor + note "(variadic; one or more)"
	//     - for "range", list required then optional with a note about the bounds
	//
	//   ## Examples
	//
	//   **<label>**
	//
	//       <command>
	//
	//   ## Flags
	//
	//   (table from c.LocalFlags; if empty: "This command has no local flags.")
	//
	//   ### Inherited global flags
	//
	//   (table from tree.PersistentFlags, filtered to c.InheritedFlags)
	//
	//   ## Output
	//
	//   <outputShape, if non-empty>
	//
	//   ## Exit Codes
	//
	//   <exitCodes, always present per spec>
	//
	//   ## See Also
	//
	//   For each c.RelatedCmds entry, render `- [`mr <name>`](<rel>)` where
	//   <rel> is computed as:
	//     rel = relPath(filepath.Dir(targetPath), outputPath[name])
	//   If the related command has no outputPath entry (typo or unknown
	//   name), skip it and log a warning to stderr so the docs-gen dirty
	//   diff is stable but the author sees the problem.
}

// relPath wraps filepath.Rel with slash normalisation so Markdown links
// produced on Windows still use forward slashes.
func relPath(fromDir, toFile string) string {
	rel, err := filepath.Rel(fromDir, toFile)
	if err != nil {
		return toFile
	}
	return filepath.ToSlash(rel)
}
```

Add `IsGroup bool` to `dumpCommand` in Task 5. Populate it from `len(c.Commands()) > 0` during `buildDumpCommand`. Use `text/template` for both the index and per-command page; declare template constants in the same file. Keep template logic minimal: all computation (relative links, flag filtering) happens in Go before the template is executed.

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./cmd/mr/commands/... -run TestDumpMarkdown`
Expected: PASS.

- [ ] **Step 5: Manual smoke**

```bash
rm -rf /tmp/cli-docs
./mr docs dump --format markdown --output /tmp/cli-docs
ls /tmp/cli-docs
cat /tmp/cli-docs/resource/get.md
```
Expected: file tree is created; `resource/get.md` has the template structure even though Long/Example are currently empty.

- [ ] **Step 6: Commit**

```bash
git add cmd/mr/commands/docs_dump.go cmd/mr/commands/docs_dump_markdown_test.go
git commit -m "feat(cli): implement 'mr docs dump --format markdown'"
```

---

### Task 7: Implement `mr docs lint`

Validates each command against the template rules from the spec. Gated by an allowlist that starts containing only the `docs` command tree itself (lets us ship Phase 1 without blocking on Phase 2 content).

**Files:**
- Create: `cmd/mr/commands/docs_lint.go`
- Modify: `cmd/mr/commands/docs.go` (delete the `lintCommandTree` stub)
- Create: `cmd/mr/commands/docs_lint_test.go`

- [ ] **Step 1: Write failing tests**

- `TestLintAllowlistSkipsUnmigratedCommands`: `lintCommandTree` on a root with a partially-migrated child should exit 0 if the un-migrated child is absent from the allowlist. Inject a test-only allowlist via an unexported setter so the test is independent of the production allowlist value.
- `TestLintFailsMissingLong`: allowlisted command without `Long` returns an error reporting the command path and the missing field.
- `TestLintFailsMissingExitCodesOnGroup`: allowlisted parent-group command (one with subcommands) without `Annotations["exitCodes"]` fails lint. Confirms the rule applies to groups too, per the spec.
- `TestLintFailsShortFieldTooLong`: `Short` > 60 chars flagged.
- `TestLintFailsFlagWithoutDescription`: flag with empty `Usage` flagged.
- `TestLintFailsFewerThanTwoExamples`: leaf command with one example flagged.
- `TestLintWarnsNoDoctest`: leaf command with zero `# mr-doctest:` examples emits a warning (non-failing; captured on stderr). Lint exits 0.
- `TestLintPassesOnValidAllowlistedCommand`: fully-populated command in allowlist passes.

- [ ] **Step 2: Run tests (expect FAIL)** (compile error).

- [ ] **Step 3: Implement lint**

```go
package commands

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// allowlist names the top-level command groups whose subtrees are
// subject to strict lint rules. Phase 1 ships empty; Task 16 adds
// "docs", "resource", "resources" together once their help content is in.
// Each subsequent migration PR adds its group.
var lintAllowlist = map[string]bool{}

func lintCommandTree(root *cobra.Command) error {
	return lintCommandTreeTo(root, os.Stdout, os.Stderr)
}

func lintCommandTreeTo(root *cobra.Command, stdout, stderr io.Writer) error {
	var failures []string
	var warnings []string
	for _, c := range walkSkippingBuiltins(root) {
		if c == root {
			continue
		}
		top := strings.SplitN(c.CommandPath(), " ", 3)[1]
		if !lintAllowlist[top] {
			continue
		}
		f, w := lintCommand(c)
		failures = append(failures, f...)
		warnings = append(warnings, w...)
	}
	sort.Strings(failures)
	sort.Strings(warnings)
	for _, w := range warnings {
		fmt.Fprintln(stderr, "warning:", w)
	}
	if len(failures) > 0 {
		for _, f := range failures {
			fmt.Fprintln(stderr, "error:", f)
		}
		return fmt.Errorf("%d lint failures", len(failures))
	}
	fmt.Fprintln(stdout, "OK:", len(warnings), "warnings")
	return nil
}

func lintCommand(c *cobra.Command) (failures, warnings []string) {
	path := c.CommandPath()
	if len(c.Short) == 0 {
		failures = append(failures, fmt.Sprintf("%s: missing Short", path))
	} else if len(c.Short) > 60 {
		failures = append(failures, fmt.Sprintf("%s: Short > 60 chars (%d)", path, len(c.Short)))
	}
	if strings.TrimSpace(c.Long) == "" {
		failures = append(failures, fmt.Sprintf("%s: missing Long", path))
	} else if sentenceCount(c.Long) < 2 {
		failures = append(failures, fmt.Sprintf("%s: Long has fewer than 2 sentences", path))
	}
	// Flag descriptions
	c.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if strings.TrimSpace(f.Usage) == "" {
			failures = append(failures, fmt.Sprintf("%s: flag --%s missing description", path, f.Name))
		}
	})
	// exitCodes annotation is required on every command (spec: "all commands").
	// Default value "0 on success; 1 on any error" is acceptable.
	if c.Annotations["exitCodes"] == "" {
		failures = append(failures, fmt.Sprintf("%s: missing exitCodes annotation", path))
	}

	if !c.HasSubCommands() {
		// Leaf-only rules.
		exs := parseExamples(c.Example)
		if len(exs) < 2 {
			failures = append(failures, fmt.Sprintf("%s: fewer than 2 examples (%d)", path, len(exs)))
		}
		// Doctest is a warning, not a failure.
		hasDoctest := false
		for _, ex := range exs {
			if ex.Doctest {
				hasDoctest = true
				break
			}
		}
		if !hasDoctest {
			warnings = append(warnings, fmt.Sprintf("%s: no # mr-doctest: examples", path))
		}
	}
	return failures, warnings
}

func sentenceCount(s string) int {
	// Conservative: count period-space and trailing period.
	n := strings.Count(s, ". ")
	trimmed := strings.TrimSpace(s)
	if strings.HasSuffix(trimmed, ".") {
		n++
	}
	return n
}
```

Save as `cmd/mr/commands/docs_lint.go`. Delete the `lintCommandTree` stub from `docs.go`.

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./cmd/mr/commands/... -run TestLint`
Expected: PASS.

- [ ] **Step 5: Wire lint into Go test suite**

Create/append `cmd/mr/commands/docs_lint_main_test.go`:

```go
package commands_test

import (
	"io"
	"testing"

	"github.com/spf13/cobra"
	"mahresources/cmd/mr/commands"
)

// TestLintRealTree runs the lint against the actual production command tree
// so CI fails fast if any migrated command regresses.
func TestLintRealTree(t *testing.T) {
	root := buildProductionRoot(t) // helper that mirrors main.go's registration
	err := commands.RunLintForTest(root, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
}
```

Expose `RunLintForTest` in `docs_lint.go` as a public wrapper around `lintCommandTreeTo` to keep internals unexported. Implement `buildProductionRoot` in the test file by copying the `AddCommand` loop from `main.go`.

- [ ] **Step 6: Commit**

```bash
git add cmd/mr/commands/docs.go cmd/mr/commands/docs_lint.go cmd/mr/commands/docs_lint_test.go cmd/mr/commands/docs_lint_main_test.go
git commit -m "feat(cli): implement 'mr docs lint' with allowlist gating"
```

---

### Task 8: Implement `mr docs check-examples`

Walks the command tree, extracts every `# mr-doctest:` example, and evaluates each block against per-example metadata (exit code, tolerate regex, skip-on, timeout, stdin). Runs with `cwd=cmd/mr/` so fixtures resolve.

**Files:**
- Create: `cmd/mr/commands/docs_doctest.go`
- Modify: `cmd/mr/commands/docs.go` (delete the `checkExamples` stub)
- Create: `cmd/mr/commands/docs_doctest_test.go`

- [ ] **Step 1: Write failing unit tests**

Tests (no server needed, they mock `exec.Command` with a fake bash that echoes args / returns controlled exits):

- `TestDoctestSkipOnMatchesEnvironment`: block with `skip-on=ephemeral` is skipped when `--environment=ephemeral`, and EXECUTED when `--environment` is empty or a different value (e.g., `seeded`). Confirms the runner honours the environment flag rather than unconditionally skipping.
- `TestDoctestExpectExit`: block with `expect-exit=2` passes when bash returns exit 2.
- `TestDoctestTolerateMatchesStderr`: non-zero exit + stderr matching tolerate regex passes.
- `TestDoctestTimeoutKills`: block with `timeout=1s` that runs `sleep 5` is terminated and reported as a failure.
- `TestDoctestStdinPipe`: block with `stdin=sample.txt` pipes that file's contents to the block's stdin.

- [ ] **Step 2: Run tests (expect FAIL)** (compile error).

- [ ] **Step 3: Implement the runner**

Key structure:

```go
package commands

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func checkExamples(root *cobra.Command, serverURL, environment string) error {
	serverURL = resolveServerURL(serverURL)
	cwd, err := doctestCwd()
	if err != nil {
		return err
	}
	mrPath, err := resolveMrBinary()
	if err != nil {
		return err
	}
	env := os.Environ()
	env = append(env,
		"MAHRESOURCES_URL="+serverURL,
		"PATH="+prependPath(os.Getenv("PATH"), filepath.Dir(mrPath)),
	)

	var failures []string
	for _, c := range walkSkippingBuiltins(root) {
		for _, ex := range parseExamples(c.Example) {
			if !ex.Doctest {
				continue
			}
			if ex.SkipOn != "" && ex.SkipOn == environment {
				fmt.Printf("SKIP  %s: %s (skip-on=%s)\n", c.CommandPath(), ex.Label, ex.SkipOn)
				continue
			}
			if err := runDoctest(ex, cwd, env); err != nil {
				failures = append(failures, fmt.Sprintf("%s: %s: %v", c.CommandPath(), ex.Label, err))
				fmt.Printf("FAIL  %s: %s\n", c.CommandPath(), ex.Label)
			} else {
				fmt.Printf("PASS  %s: %s\n", c.CommandPath(), ex.Label)
			}
		}
	}
	if len(failures) > 0 {
		for _, f := range failures {
			fmt.Fprintln(os.Stderr, f)
		}
		return fmt.Errorf("%d doctest failures", len(failures))
	}
	return nil
}

func runDoctest(ex dumpExample, cwd string, env []string) error {
	timeout := 30 * time.Second
	if ex.TimeoutSec > 0 {
		timeout = time.Duration(ex.TimeoutSec) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-eo", "pipefail", "-c", ex.Command)
	cmd.Dir = cwd
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if ex.Stdin != "" {
		data, err := os.ReadFile(filepath.Join(cwd, "testdata", ex.Stdin))
		if err != nil {
			return fmt.Errorf("reading stdin fixture: %w", err)
		}
		cmd.Stdin = bytes.NewReader(data)
	}

	err := cmd.Run()
	exitCode := 0
	if ee, ok := err.(*exec.ExitError); ok {
		exitCode = ee.ExitCode()
	} else if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timed out after %s", timeout)
		}
		return err
	}

	expected := ex.ExpectedExit
	if exitCode == expected {
		return nil
	}
	if ex.Tolerate != "" {
		re, err := regexp.Compile(ex.Tolerate)
		if err != nil {
			return fmt.Errorf("invalid tolerate regex %q: %w", ex.Tolerate, err)
		}
		if re.Match(stderr.Bytes()) {
			return nil
		}
	}
	return fmt.Errorf("exit %d (want %d); stderr: %s", exitCode, expected, truncate(stderr.String(), 400))
}

func resolveServerURL(flag string) string {
	if flag != "" {
		return flag
	}
	if env := os.Getenv("MAHRESOURCES_URL"); env != "" {
		return env
	}
	return "http://localhost:8181"
}

func doctestCwd() (string, error) {
	// Walk from the current working directory up a few levels looking for
	// the `cmd/mr/testdata` subtree. Supports running from the repo root
	// (`./mr docs check-examples`) and from inside `e2e/` (CI).
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for _, rel := range []string{".", "..", "../..", "../../.."} {
		root := filepath.Clean(filepath.Join(wd, rel))
		candidate := filepath.Join(root, "cmd", "mr")
		if _, err := os.Stat(filepath.Join(candidate, "testdata")); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not locate cmd/mr/testdata from %s", wd)
}

// resolveMrBinary returns the absolute path of the currently-running `mr`
// binary. os.Executable() is reliable on all supported platforms.
func resolveMrBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe, nil // fall back to the un-resolved path
	}
	return resolved, nil
}

func prependPath(existing, dir string) string {
	sep := string(os.PathListSeparator)
	if existing == "" {
		return dir
	}
	// Avoid duplicating dir if it is already first.
	parts := strings.Split(existing, sep)
	if len(parts) > 0 && parts[0] == dir {
		return existing
	}
	return dir + sep + existing
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
```

Save as `cmd/mr/commands/docs_doctest.go`. Delete the `checkExamples` stub from `docs.go`.

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./cmd/mr/commands/... -run TestDoctest`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/mr/commands/docs.go cmd/mr/commands/docs_doctest.go cmd/mr/commands/docs_doctest_test.go
git commit -m "feat(cli): implement 'mr docs check-examples' runner"
```

---

### Task 9: Wire doctest into E2E CI scaffolding

The existing `test:with-server:cli` path is a Playwright runner (`e2e/scripts/run-tests.js` that invokes `npx playwright test --project=cli`). Ephemeral servers are started by Playwright worker fixtures, not by a standalone shell wrapper. Doctest execution therefore ships as a new Playwright spec that reuses the existing `workerServer` fixture and invokes the `mr docs check-examples` binary as a child process, assertng its exit code.

**Files:**
- Modify: `package.json` (add `docs-gen`, `docs-lint` scripts)
- Modify: `e2e/playwright.config.ts` (add a `cli-doctest` project)
- Modify: `e2e/package.json` (add `test:with-server:cli-doctest` script)
- Create: `e2e/tests/cli/cli-doctest.spec.ts` (single-test spec that invokes the runner)

- [ ] **Step 1: Read current scripts and config**

Run:
```bash
cat package.json | jq '.scripts'
cat e2e/package.json | jq '.scripts'
cat e2e/playwright.config.ts
cat e2e/scripts/run-tests.js
grep -rn "workerServer" e2e/fixtures/ e2e/helpers/ | head
```
Expected: understand how `test:with-server:cli` and the `workerServer` fixture produce an ephemeral-server URL per worker.

- [ ] **Step 2: Add docs-gen and docs-lint scripts to root package.json**

Edit `package.json`'s `scripts` to add:

```json
"docs-gen": "./mr docs dump --format markdown --output docs-site/docs/cli/",
"docs-lint": "./mr docs lint"
```

- [ ] **Step 3: Add a `cli-doctest` project to playwright.config.ts**

Mirror the existing `cli` project's settings (single-worker friendly, or whatever isolation the current `cli` project uses), but scope it to the new spec file:

```ts
// inside projects[]
{
  name: 'cli-doctest',
  testMatch: /cli-doctest\.spec\.ts/,
  // inherit server-start behavior from the cli project; do not define `use.baseURL`,
  // the spec reads the URL from the workerServer fixture directly.
},
```

Reference the existing `cli` entry in the same file for the exact fixture wiring; copy anything related to fixture setup verbatim.

- [ ] **Step 4: Write the doctest spec**

```ts
// e2e/tests/cli/cli-doctest.spec.ts
import { test, expect } from '../../fixtures/base.fixture';
import { spawnSync } from 'node:child_process';
import * as path from 'node:path';

test('every mr-doctest example passes against ephemeral server', async ({ workerServer }) => {
  const repoRoot = path.resolve(__dirname, '../../..');
  const mr = path.join(repoRoot, 'mr');

  const result = spawnSync(mr, [
    'docs',
    'check-examples',
    '--server', workerServer.url,
    '--environment', 'ephemeral',
  ], {
    cwd: repoRoot,
    encoding: 'utf-8',
    env: {
      ...process.env,
      MAHRESOURCES_URL: workerServer.url,
    },
  });

  if (result.status !== 0) {
    console.log('stdout:\n' + result.stdout);
    console.error('stderr:\n' + result.stderr);
  }
  expect(result.status, 'mr docs check-examples failed').toBe(0);
});
```

Adjust the fixture import path and the fixture property name (`workerServer.url` or similar) to match what `e2e/fixtures/base.fixture.ts` actually exposes. The one-liner `grep -rn "workerServer" e2e/fixtures/` from Step 1 gives the correct shape.

- [ ] **Step 5: Add the E2E npm script**

In `e2e/package.json`, add to `scripts`:

```json
"test:with-server:cli-doctest": "node scripts/run-tests.js test --project=cli-doctest"
```

The existing `run-tests.js` already builds the `mr` binary (line 30 in that script), so nothing extra is needed.

- [ ] **Step 6: Smoke-test locally**

```bash
cd e2e && npm run test:with-server:cli-doctest
```
Expected: Phase 1 has no `# mr-doctest:` blocks yet, so the runner reports "0 doctest blocks" and exits 0. The Playwright test passes.

- [ ] **Step 7: Commit**

```bash
git add package.json e2e/package.json e2e/playwright.config.ts e2e/tests/cli/cli-doctest.spec.ts
git commit -m "ci(cli): add Playwright-driven doctest runner against ephemeral server"
```

---

## Phase 2: Pilot on `resource` + `resources`

### Task 10: Refactor resources.go to use embedded help Markdown

Adds the `//go:embed` directive and converts every command builder in `resources.go` to `helptext.Load` the corresponding help file. Content of each `.md` file is a minimal placeholder at this step; Tasks 11-15b fill the real content in.

**Files:**
- Modify: `cmd/mr/commands/resources.go` (top of file + every `new*Cmd` function)
- Create: placeholder `cmd/mr/commands/resources_help/*.md` files, one per subcommand (35 files total: parent `resource` + 21 resource subcommands + parent `resources` + 12 resources subcommands)

- [ ] **Step 1: Create placeholder help files**

Each placeholder is a minimal valid file that passes the parser but fails the linter (because `Long` is too short). That's intentional: it unblocks the refactor; content fills in during later tasks.

For every subcommand listed below, create `cmd/mr/commands/resources_help/<name>.md`:

```markdown
---
exitCodes: 0 on success; 1 on any error
---

# Long

Placeholder.

# Example

  # Placeholder example
  mr <placeholder>
```

Filenames (35 total, matching the real subcommand set in `resources.go`):

Singular `resource` (22 files: parent + 21 subcommands):
- `resource.md`
- `resource_get.md`, `resource_edit.md`, `resource_delete.md`
- `resource_edit_name.md`, `resource_edit_description.md`, `resource_edit_meta.md`
- `resource_upload.md`, `resource_download.md`, `resource_preview.md`
- `resource_from_url.md`, `resource_from_local.md`
- `resource_rotate.md`, `resource_recalculate_dimensions.md`
- `resource_versions.md`, `resource_version.md`
- `resource_version_upload.md`, `resource_version_download.md`
- `resource_version_restore.md`, `resource_version_delete.md`
- `resource_versions_cleanup.md`, `resource_versions_compare.md`

Plural `resources` (13 files: parent + 12 subcommands):
- `resources.md`
- `resources_list.md`
- `resources_add_tags.md`, `resources_remove_tags.md`, `resources_replace_tags.md`
- `resources_add_groups.md`, `resources_add_meta.md`
- `resources_delete.md`, `resources_merge.md`
- `resources_set_dimensions.md`, `resources_versions_cleanup.md`
- `resources_meta_keys.md`, `resources_timeline.md`

Verify the full set before proceeding: run `grep -n "cmd.AddCommand(newResource" cmd/mr/commands/resources.go` and confirm the filename list matches the subcommand names (with hyphens converted to underscores).

- [ ] **Step 2: Add embed directive and import**

At the top of `cmd/mr/commands/resources.go`, just after the existing imports, add:

```go
import (
	"embed"
	// ... existing imports
	"mahresources/cmd/mr/helptext"
)

//go:embed resources_help/*.md
var resourcesHelpFS embed.FS
```

- [ ] **Step 3: Apply helptext.Load in each command builder**

For every `new*Cmd` function in the file, change the pattern from:

```go
return &cobra.Command{
    Use:   "get <id>",
    Short: "Get a resource by ID",
    Args:  cobra.ExactArgs(1),
    RunE:  ...,
}
```

to:

```go
help := helptext.Load(resourcesHelpFS, "resources_help/resource_get.md")
return &cobra.Command{
    Use:         "get <id>",
    Short:       "Get a resource by ID",
    Args:        cobra.ExactArgs(1),
    Long:        help.Long,
    Example:     help.Example,
    Annotations: help.Annotations,
    RunE:        ...,
}
```

Apply to every builder in `resources.go`. Use the filename mapping above. For the parent command (`newResourceCmd`), load `"resources_help/resource.md"` and set `Long` + `Annotations` only (no `Example` on groups).

- [ ] **Step 4: Build to confirm the refactor compiles**

Run: `npm run build-cli`
Expected: success.

- [ ] **Step 5: Smoke-test**

Run: `./mr resource get --help`
Expected: shows the placeholder Long. Fine for now.

- [ ] **Step 6: Commit**

```bash
git add cmd/mr/commands/resources.go cmd/mr/commands/resources_help/
git commit -m "refactor(cli): load resource help text from embedded Markdown"
```

---

### Task 11: Write parent + CRUD + edit-* help (7 files)

Fills the first 7 placeholder files with real content. Each file follows the structure from the spec: YAML front matter + `# Long` + `# Example` with ≥2 labeled examples (including ≥1 `# mr-doctest:` where safe).

**Files:**
- Modify: `cmd/mr/commands/resources_help/resource.md`
- Modify: `cmd/mr/commands/resources_help/resource_get.md`
- Modify: `cmd/mr/commands/resources_help/resource_edit.md`
- Modify: `cmd/mr/commands/resources_help/resource_delete.md`
- Modify: `cmd/mr/commands/resources_help/resource_edit_name.md`
- Modify: `cmd/mr/commands/resources_help/resource_edit_description.md`
- Modify: `cmd/mr/commands/resources_help/resource_edit_meta.md`

- [ ] **Step 1: Write `resource.md` (parent group Long)**

Content outline:
- Front matter: `relatedCmds: resources list, group list, tag list`. No `outputShape` or `exitCodes` on groups.
- `# Long`: 3-4 sentences covering: what a Resource is (a file with metadata); lifecycle (upload → tag/group → version → archive/delete); relationship to Groups, Notes, Tags; the plural `mr resources list` for discovery.
- No `# Example` section on groups (parent commands don't run).

Example content:

```markdown
---
exitCodes: 0 on success; 1 on any error
relatedCmds: resources list, group list, tag list
---

# Long

Resources are files stored in mahresources. A Resource has a name,
content bytes, MIME type, optional dimensions, perceptual hash, and
free-form meta JSON. Resources relate many-to-many to Tags, Notes, and
Groups, and support versioned edits (see `versions`, `version-upload`).

Use the `resource` subcommands to operate on a single resource by ID:
fetch metadata, upload a file, rotate an image, or manage its version
history. Use `resources list` to discover resources matching filters.
```

- [ ] **Step 2: Write `resource_get.md`**

Content outline:
- Front matter: `outputShape: Resource object with id, name, tags, groups, meta`; `exitCodes: 0 on success; 1 on any error`; `relatedCmds: resource edit, resource versions, resource download`.
- `# Long`: 2-3 sentences. What it does, what's in the output, how to script against it.
- `# Example`: 3 examples. (1) placeholder table fetch, (2) JSON fetch piped to `jq`, (3) `# mr-doctest:` that uploads a fixture, fetches it, asserts name.

Example content:

```markdown
---
outputShape: Resource object with id (uint), name (string), tags ([]Tag), groups ([]Group), meta (object)
exitCodes: 0 on success; 1 on any error
relatedCmds: resource edit, resource versions, resource download
---

# Long

Get a resource by ID and print its metadata. Fetches the full record
including tags, groups, resource category, owner, dimensions, hash,
and any custom meta JSON. Output is a key/value table by default; pass
the global `--json` flag to get the full record for scripting.

# Example

  # Get a resource by ID (table output)
  mr resource get 42

  # Get as JSON and extract a single field with jq
  mr resource get 42 --json | jq -r .name

  # mr-doctest: upload a fixture and round-trip the name
  ID=$(mr resource upload ./testdata/sample.jpg --name "doctest-get" --json | jq -r .id)
  mr resource get $ID --json | jq -e '.name == "doctest-get"'
```

- [ ] **Step 3: Write `resource_edit.md`**

Content outline:
- Front matter: `outputShape` empty (edit prints a success message). `exitCodes`: default. `relatedCmds: resource get, resource upload, resource versions`.
- `# Long`: 3 sentences. Describe that most fields are partial-update (empty flag = unchanged); comma-separated ID lists for `--tags`, `--groups`, `--notes`; `--meta` takes a JSON string merged on top.
- `# Example`: 3 examples. (1) rename + description update, (2) attach tags, (3) `# mr-doctest:` that creates, renames, re-fetches, asserts.

- [ ] **Step 4: Write `resource_delete.md`**

Content outline:
- Front matter: `exitCodes: 0 on success; 1 on any error` (deletion of non-existent IDs returns 1). `relatedCmds: resource get, resources list`.
- `# Long`: 2 sentences. Destructive; deletes the file from storage along with the DB row.
- `# Example`: 3 examples. (1) delete by ID, (2) delete with `--quiet`, (3) `# mr-doctest:` that uploads, deletes, asserts the follow-up `get` returns non-zero via `tolerate=/not found|404/i`.

- [ ] **Step 5: Write `resource_edit_name.md`**

Content outline:
- Front matter: `exitCodes: default`. `relatedCmds: resource edit, resource edit-description, resource edit-meta`.
- `# Long`: 2 sentences. Updates only the `name` of an existing resource. Convenient shorthand for `mr resource edit <id> --name=<value>` when name is the only change.
- `# Example`: 2 examples + 1 doctest. (1) rename by id, (2) read ID from upload pipeline + rename in one command, (3) `# mr-doctest:` uploads fixture, edit-name, fetches to verify.

- [ ] **Step 6: Write `resource_edit_description.md`**

Content outline:
- Front matter: `exitCodes: default`. `relatedCmds: resource edit, resource edit-name, resource edit-meta`.
- `# Long`: 2 sentences. Updates only the `description`. Accepts an empty string to clear the description.
- `# Example`: 2 examples + 1 doctest. (1) set description, (2) clear description with `""`, (3) `# mr-doctest:` asserts round-trip.

- [ ] **Step 7: Write `resource_edit_meta.md`**

Verify the live command signature before writing: `grep -A3 "func newResourceEditMetaCmd" cmd/mr/commands/resources.go`. The real signature is `edit-meta <id> <path> <value>` with `cobra.ExactArgs(3)`. The outline MUST match this, not a merge-object model.

Content outline:
- Front matter: `exitCodes: default`. `relatedCmds: resource edit, resources add-meta, resources meta-keys`.
- `# Long`: 3 sentences. Edits a single metadata field at a dot-separated JSON path. Takes three positional arguments: the resource ID, the path (e.g., `address.city`), and a JSON literal value (e.g., `'"Berlin"'`, `42`, `'{"nested":"obj"}'`, `'[1,2,3]'`). Creates intermediate path segments as needed and leaves sibling keys at each level untouched.
- `# Example`: 3 examples. (1) set a top-level string field: `mr resource edit-meta 5 status '"active"'` (note the JSON-quoted value); (2) set a nested number: `mr resource edit-meta 5 address.postalCode 10115`; (3) `# mr-doctest:` upload a fixture, set a single meta key via path, fetch with `--json | jq '.meta.status'`, assert the value.

- [ ] **Step 8: Run lint (allowlist still excludes resource)**

Run: `./mr docs lint`
Expected: PASS: resource isn't in the allowlist yet, so these changes don't affect the lint result.

- [ ] **Step 9: Smoke-test help rendering**

Run: `./mr resource --help`
Expected: the new Long for the parent group renders; the "See Also" block at the bottom shows `resources list`, `group list`, `tag list`.

Run: `./mr resource get --help && ./mr resource edit-meta --help`
Expected: each renders a new Long, 2-3 examples, and See Also.

- [ ] **Step 10: Commit**

```bash
git add cmd/mr/commands/resources_help/resource.md cmd/mr/commands/resources_help/resource_get.md cmd/mr/commands/resources_help/resource_edit.md cmd/mr/commands/resources_help/resource_delete.md cmd/mr/commands/resources_help/resource_edit_name.md cmd/mr/commands/resources_help/resource_edit_description.md cmd/mr/commands/resources_help/resource_edit_meta.md
git commit -m "docs(cli): write help for resource parent + CRUD + edit-* subcommands"
```

---

### Task 12: Write ingest help (upload, download, preview, from-url, from-local)

Same pattern. Each file gets front matter, Long, and Examples. Content outlines below.

**Files:**
- Modify: `cmd/mr/commands/resources_help/resource_upload.md`
- Modify: `cmd/mr/commands/resources_help/resource_download.md`
- Modify: `cmd/mr/commands/resources_help/resource_preview.md`
- Modify: `cmd/mr/commands/resources_help/resource_from_url.md`
- Modify: `cmd/mr/commands/resources_help/resource_from_local.md`

- [ ] **Step 1: `resource_upload.md`**

Outline:
- `outputShape: Resource object (ID, name)` on success.
- `relatedCmds: resource edit, resource from-url, resource from-local, resources list`.
- Long: uploads a file via multipart form (`resource` field); accepts metadata via flags; defaults the resource name to the source filename.
- Examples: (1) basic upload, (2) upload with `--owner-id` and `--meta` JSON string, (3) `# mr-doctest:` uploads `./testdata/sample.jpg` with a label, extracts the ID via jq, asserts.

- [ ] **Step 2: `resource_download.md`**

Outline:
- `outputShape` empty (writes to file). `relatedCmds: resource get, resource preview, resource version-download`.
- Long: streams resource bytes to `-o` path; default filename is `resource_<id>`.
- Examples: (1) download with explicit output path, (2) download to default filename, (3) `# mr-doctest:` uploads a fixture, downloads, asserts size via `stat`. Use `timeout=60s` for safety.

- [ ] **Step 3: `resource_preview.md`**

Outline:
- `outputShape` empty. `relatedCmds: resource download, resource recalculate-dimensions`.
- Long: downloads a server-rendered thumbnail; width/height caps via `--width`/`--height`; not all types support previews.
- Examples: (1) default preview, (2) preview with size caps, (3) `# mr-doctest: tolerate=/preview not available|no preview/i`-guarded attempt.

- [ ] **Step 4: `resource_from_url.md`**

Outline:
- `outputShape: Resource object (ID)`. `relatedCmds: resource upload, resource from-local`.
- Long: fetches a URL server-side and creates a resource; useful when you have a public asset but don't want to download locally first; `--url` is required.
- Examples: (1) basic, (2) with custom name and metadata, (3) `# mr-doctest: skip-on=ephemeral` (ephemeral servers can't reach arbitrary URLs in CI).

- [ ] **Step 5: `resource_from_local.md`**

Outline:
- Like `from-url` but from a server-local path. `--path` required. `relatedCmds: resource upload, resource from-url`.
- Examples: (1) basic, (2) with metadata, (3) `# mr-doctest: skip-on=ephemeral` (path only valid on target server).

- [ ] **Step 6: Smoke-test help output**

Run: `./mr resource upload --help && ./mr resource download --help && ./mr resource preview --help`
Expected: each renders its new Long, Examples (≥2), flags table, See Also.

- [ ] **Step 7: Commit**

```bash
git add cmd/mr/commands/resources_help/resource_upload.md cmd/mr/commands/resources_help/resource_download.md cmd/mr/commands/resources_help/resource_preview.md cmd/mr/commands/resources_help/resource_from_url.md cmd/mr/commands/resources_help/resource_from_local.md
git commit -m "docs(cli): write help for resource ingest subcommands"
```

---

### Task 13: Write transform help (rotate, recalculate-dimensions)

**Files:**
- Modify: `cmd/mr/commands/resources_help/resource_rotate.md`
- Modify: `cmd/mr/commands/resources_help/resource_recalculate_dimensions.md`

- [ ] **Step 1: `resource_rotate.md`**

Outline:
- `outputShape` empty. `relatedCmds: resource preview, resource edit`.
- Long: rotates an image resource in place; `--degrees` is required; only image resources are supported; transformation creates a new version.
- Examples: (1) 90° rotate, (2) 180° rotate, (3) `# mr-doctest: tolerate=/unexpected EOF|not supported/i` (reasons documented at `e2e/tests/cli/cli-resources.spec.ts:215`).

- [ ] **Step 2: `resource_recalculate_dimensions.md`**

Outline:
- `outputShape` empty. `relatedCmds: resource get, resource rotate`.
- Long: reads the resource file, decodes it, and updates the stored `width` / `height`; useful after manual DB edits or thumbnail regeneration.
- Examples: (1) basic call, (2) run across a batch via `mrql` pipe (reference example, no `mr-doctest:`), (3) `# mr-doctest:` that uploads a known-dimension fixture, calls recalc, asserts dims.

- [ ] **Step 3: Smoke-test**

Run: `./mr resource rotate --help`
Expected: new Long + Examples + See Also.

- [ ] **Step 4: Commit**

```bash
git add cmd/mr/commands/resources_help/resource_rotate.md cmd/mr/commands/resources_help/resource_recalculate_dimensions.md
git commit -m "docs(cli): write help for resource transform subcommands"
```

---

### Task 14: Write version-family help (8 files)

**Files:**
- Modify: `resource_versions.md`, `resource_version.md`, `resource_version_upload.md`, `resource_version_download.md`, `resource_version_restore.md`, `resource_version_delete.md`, `resource_versions_cleanup.md`, `resource_versions_compare.md`

For each, write front matter + Long + ≥2 Examples using this template:

| File | Long outline | Example outline |
|---|---|---|
| `resource_versions.md` | Lists all versions of a resource with ID, version number, size, type, optional comment, and creation time. Use as a discovery step before `version-download` or `versions-compare`. | (1) basic list, (2) pipe to `jq` to extract the newest version ID, (3) `# mr-doctest:` upload → version-upload → assert list length == 2. |
| `resource_version.md` | Fetches a single version record by its ID. Returns the same fields as `versions` in KV form. | (1) basic get, (2) JSON + jq for size, (3) `# mr-doctest:` chained with upload and version-upload. |
| `resource_version_upload.md` | Uploads a new version of an existing resource (multipart); increments the version counter; `--comment` is optional free text. | (1) basic, (2) with comment, (3) `# mr-doctest:` upload → version-upload a second fixture → assert versions count == 2. |
| `resource_version_download.md` | Downloads a specific version's bytes; use `resource download` for the current version. | (1) basic with `-o`, (2) default filename, (3) `# mr-doctest:` upload → version-upload → download v1 and v2 → assert sizes differ. |
| `resource_version_restore.md` | Restores a resource to an earlier version, creating a new version that is a copy of the target; `--resource-id` and `--version-id` are required. | (1) restore with comment, (2) restore silently, (3) `# mr-doctest:` upload → version-upload → restore to v1 → assert hash matches v1. |
| `resource_version_delete.md` | Deletes a specific version; does not delete the resource itself; fails if it would leave zero versions. | (1) basic, (2) via flag combinations (both `--resource-id` and `--version-id` required), (3) `# mr-doctest:` upload → version-upload → delete v1 → assert count == 1. |
| `resource_versions_cleanup.md` | Bulk-removes old versions by count (`--keep N`) or age (`--older-than-days N`). Use `--dry-run` to preview. | (1) keep last 5, (2) older than 30 days dry-run, (3) `# mr-doctest:` create 3 versions, cleanup `--keep 1`, assert count == 1. |
| `resource_versions_compare.md` | Compares two versions of a resource and reports size delta, whether hashes match, whether content types match, and dimension differences. | (1) basic, (2) JSON + jq to check SameHash, (3) `# mr-doctest:` upload same fixture twice, compare, assert `SameHash == true`. |

For each file, use these annotations:
- `outputShape`: where applicable (e.g., `resource_versions.md`: `Array of version objects with id, number, size, type, comment, created`).
- `exitCodes`: default.
- `relatedCmds`: sibling version-family commands + `resource get`.

- [ ] **Step 1: Write all 8 files in a batch.**

Follow the table above. The example commands use real fixtures from `cmd/mr/testdata/`; the `# mr-doctest:` blocks target an ephemeral server that lets us create-and-destroy without contamination.

- [ ] **Step 2: Smoke-test three of them**

Run: `./mr resource versions --help && ./mr resource version-upload --help && ./mr resource versions-compare --help`
Expected: each renders the new content.

- [ ] **Step 3: Commit**

```bash
git add cmd/mr/commands/resources_help/resource_version*.md cmd/mr/commands/resources_help/resource_versions*.md
git commit -m "docs(cli): write help for resource version-family subcommands"
```

---

### Task 15a: Write resources (plural) group + list + bulk-add help (7 files)

**Files:**
- Modify: `cmd/mr/commands/resources_help/resources.md`
- Modify: `cmd/mr/commands/resources_help/resources_list.md`
- Modify: `cmd/mr/commands/resources_help/resources_add_tags.md`
- Modify: `cmd/mr/commands/resources_help/resources_remove_tags.md`
- Modify: `cmd/mr/commands/resources_help/resources_replace_tags.md`
- Modify: `cmd/mr/commands/resources_help/resources_add_groups.md`
- Modify: `cmd/mr/commands/resources_help/resources_add_meta.md`

Before writing any file in this task: verify each subcommand's actual flag set and semantics. For each, run `grep -A15 "func newResources<Name>Cmd" cmd/mr/commands/resources.go`. Do NOT assume MRQL selector support, `--resources` flags, `--dry-run`, or filters unless they are present in the live code. As of writing, every bulk mutation accepts `--ids=<csv>` for the target resources.

- [ ] **Step 1: `resources.md` (parent group)**

Parent-group Long for the plural. Front matter: `exitCodes: 0 on success; 1 on any error`; `relatedCmds: resource get, group list, search, mrql`. Long covers: list-style and bulk-mutation operations vs. singular `resource` for per-item ops; filter vocabulary on `list` at a high level; pagination via global `--page` flag; the fact that most bulk-mutation commands accept a comma-separated `--ids` list to select targets, while `merge` (uses `--winner`/`--losers`) and a few admin-shaped commands have their own flag vocabulary (see each subcommand's `--help`). No MRQL-style selectors in the current CLI.

- [ ] **Step 2: `resources_list.md`**

- `outputShape: Array of resources with id, name, content type, size, dimensions, owner id, created timestamp`.
- Long: 3-4 sentences. Filter-rich list endpoint; filters combine with AND; comma-separated IDs for `--tags`/`--groups`/`--notes`; date filters expect `YYYY-MM-DD`; sort via `--sort-by=field1,-field2`.
- Examples: 4. (1) basic list, (2) filter by `--content-type=image/jpeg`, (3) filter by tags + date range with `--json | jq`, (4) `# mr-doctest:` upload two fixtures with distinct tags, list with that tag filter, assert count ≥ 2.

- [ ] **Step 3: `resources_add_tags.md`**

Flags (verified): `--ids` (required, comma-separated resource IDs), `--tags` (required, comma-separated tag IDs).

- `outputShape` empty (returns success summary). `exitCodes: default`. `relatedCmds: resources remove-tags, resources replace-tags, tag list`.
- Long: 2 sentences. Adds the given tag IDs to every resource in `--ids`; idempotent (a tag that is already present is a no-op).
- Examples: 3. (1) add tag 5 to resources 1,2,3 (`mr resources add-tags --ids=1,2,3 --tags=5`); (2) add multiple tags at once (`--tags=5,6`); (3) `# mr-doctest:` upload two fixtures, create a tag, add-tags to both, list by that tag, assert count >= 2.

- [ ] **Step 4: `resources_remove_tags.md`**

Flags (verified): `--ids` (required), `--tags` (required).

- Same front matter pattern. `relatedCmds: resources add-tags, resources replace-tags`.
- Long: 2 sentences. Removes the given tag IDs from every resource in `--ids`; idempotent.
- Examples: 3. (1) `mr resources remove-tags --ids=1,2 --tags=5`, (2) remove multiple tags at once, (3) `# mr-doctest:` add-tags then remove-tags round-trip, assert 0 matches after removal.

- [ ] **Step 5: `resources_replace_tags.md`**

Flags (verified): `--ids` (required), `--tags` (required).

- Same front matter pattern. `relatedCmds: resources add-tags, resources remove-tags`.
- Long: 3 sentences. Sets the full tag set on every resource in `--ids` to exactly the given `--tags`; any tag not in the list is removed, any tag in the list is added. Use when you want exact-state semantics instead of delta semantics.
- Examples: 3. (1) `mr resources replace-tags --ids=1 --tags=5,7`, (2) `--tags=""` (empty list clears all tags), (3) `# mr-doctest:` assert exact final tag set after replace.

- [ ] **Step 6: `resources_add_groups.md`**

Flags (verified): `--ids` (required), `--groups` (required).

- Similar front matter. `relatedCmds: resources add-tags, group list`.
- Long: 2 sentences. Adds the given group IDs to every resource in `--ids`; idempotent.
- Examples: 3. (1) `mr resources add-groups --ids=1,2 --groups=3`, (2) add multiple groups at once, (3) `# mr-doctest:` assert via `mr resource get <id> --json | jq '.groups | length >= 1'`.

- [ ] **Step 7: `resources_add_meta.md`**

Flags (verified): `--ids` (required), `--meta` (required, single JSON string).

- Similar front matter. `relatedCmds: resources meta-keys, resource edit-meta`.
- Long: 2 sentences. Takes a JSON string in `--meta` and adds its keys onto every resource in `--ids`. Server-side behavior for collisions with existing meta keys is determined by the `/v1/resources/addMeta` endpoint; do NOT claim merge-vs-replace semantics in the Long beyond what the endpoint actually does.
- Examples: 3. (1) `mr resources add-meta --ids=1,2,3 --meta='{"status":"reviewed"}'`, (2) passing multi-key JSON as a shell-quoted argument, (3) `# mr-doctest:` assert a key landed via `mr resource get <id> --json | jq '.meta.status'`.

- [ ] **Step 8: Smoke-test**

Run: `./mr resources list --help && ./mr resources add-tags --help && ./mr resources add-meta --help`
Expected: each renders the new content.

- [ ] **Step 9: Commit**

```bash
git add cmd/mr/commands/resources_help/resources.md cmd/mr/commands/resources_help/resources_list.md cmd/mr/commands/resources_help/resources_add_tags.md cmd/mr/commands/resources_help/resources_remove_tags.md cmd/mr/commands/resources_help/resources_replace_tags.md cmd/mr/commands/resources_help/resources_add_groups.md cmd/mr/commands/resources_help/resources_add_meta.md
git commit -m "docs(cli): write help for resources list + bulk-add commands"
```

---

### Task 15b: Write resources destructive + admin help (6 files)

Covers the remaining plural bulk commands.

**Files:**
- Modify: `cmd/mr/commands/resources_help/resources_delete.md`
- Modify: `cmd/mr/commands/resources_help/resources_merge.md`
- Modify: `cmd/mr/commands/resources_help/resources_set_dimensions.md`
- Modify: `cmd/mr/commands/resources_help/resources_versions_cleanup.md`
- Modify: `cmd/mr/commands/resources_help/resources_meta_keys.md`
- Modify: `cmd/mr/commands/resources_help/resources_timeline.md`

Before writing: verify each subcommand's actual flags. The reviewer caught cases in 15a where the plan invented unsupported features (MRQL, `--dry-run`, etc.). Do the same verification here.

- [ ] **Step 1: `resources_delete.md`**

Flags (verified): `--ids` (required). No `--dry-run`, no selector.

- `relatedCmds: resource delete, resources merge`. Long: 2 sentences. Bulk-deletes every resource in `--ids` (and their files); destructive and irreversible. There is no dry-run preview; run `mr resource get <id>` first if you need to confirm targets.
- Examples: 3. (1) `mr resources delete --ids=42,43`, (2) delete the output of a list query piped through `jq`: `mr resources list --json --tags=7 | jq -r '.[].id' | paste -sd, | xargs -I{} mr resources delete --ids={}`, (3) `# mr-doctest:` upload fixture, delete, follow-up `get` with `tolerate=/not found|404/i`.

- [ ] **Step 2: `resources_merge.md`**

Flags (verified): `--winner` (required, single uint), `--losers` (required, comma-separated uints). No `--dry-run`, no bulk mode.

- `relatedCmds: resource get, resources delete`. Long: 3 sentences. Merges one or more "loser" resources into a single "winner"; the winner's content is preserved while tags, groups, notes, and relations from the losers are moved onto it. The loser records and their files are deleted. Use to consolidate duplicates after perceptual-hash detection or manual review.
- Examples: 3. (1) `mr resources merge --winner=1 --losers=2,3`, (2) pipe duplicate IDs from a similarity search: `mr search ... --json | jq -r '.ids | @csv' | xargs -I{} mr resources merge --winner=1 --losers={}`, (3) `# mr-doctest:` upload three fixtures, add distinct tags to each, merge two losers into a winner, assert the winner's tag set contains the union.

- [ ] **Step 3: `resources_set_dimensions.md`**

Flags (verified): `--ids` (required), `--width` (required uint), `--height` (required uint). No MRQL.

- `relatedCmds: resource rotate, resource recalculate-dimensions`. Long: 2 sentences. Forces the stored `width`/`height` to the given values on every resource in `--ids`; useful when `recalculate-dimensions` cannot decode the file format or the stored dimensions are known to be stale. Does not transform the file bytes.
- Examples: 3. (1) `mr resources set-dimensions --ids=7 --width=1920 --height=1080`, (2) batch of IDs, (3) `# mr-doctest:` upload fixture, set known dimensions, fetch, assert.

- [ ] **Step 4: `resources_versions_cleanup.md`**

Flags (verified against `cmd/mr/commands/resources.go`): confirm the exact flag set before writing. This command mirrors the singular `resource versions-cleanup` but applies across multiple resources. Do not assume `--keep`, `--older-than-days`, or `--dry-run` are present without checking; read the `newResourcesVersionsCleanupCmd` builder.

- `relatedCmds: resource versions-cleanup, resource versions`. Long: 2 sentences summarizing actual behavior.
- Examples: 3. (1) the common case for whatever flags exist, (2) a JSON+jq piped case, (3) `# mr-doctest:` create versions, run cleanup, assert reduced count.

- [ ] **Step 5: `resources_meta_keys.md`**

Flags (verified): NONE. The live command takes no flags and has no filters; it hits `/v1/resources/meta/keys` and returns all distinct meta keys globally.

- `outputShape: Array of meta-key strings observed across all resources`. `relatedCmds: resource edit-meta, resources add-meta`. Long: 2 sentences. Lists every distinct `meta` key observed across the entire resource corpus. Useful for discovering the vocabulary of an evolving meta schema; there are no filter flags on this command (pair it with client-side `jq` filtering if you only want a subset).
- Examples: 3. (1) `mr resources meta-keys`, (2) filter with jq: `mr resources meta-keys --json | jq '.[] | select(startswith("image_"))'`, (3) `# mr-doctest:` upload fixture with `--meta='{"probe_key":1}'`, then `mr resources meta-keys --json | jq 'index("probe_key") != null'` and assert true.

- [ ] **Step 6: `resources_timeline.md`**

Outline:
- `outputShape: Timeline-series with buckets, each containing count, period_start, period_end`. `relatedCmds: timeline, resources list`. Long: 3 sentences. Aggregates resource creation/modification counts over time; flags control bucket size (day/week/month/year); filters mirror `resources list`. Used for dashboards and data-health views.
- Examples: 3. (1) month-bucketed last year, (2) filter by tag + JSON, (3) `# mr-doctest:` upload 2 fixtures and expect a bucket with count ≥ 2.

- [ ] **Step 7: Smoke-test a few**

Run: `./mr resources delete --help && ./mr resources merge --help && ./mr resources timeline --help`

- [ ] **Step 8: Commit**

```bash
git add cmd/mr/commands/resources_help/resources_delete.md cmd/mr/commands/resources_help/resources_merge.md cmd/mr/commands/resources_help/resources_set_dimensions.md cmd/mr/commands/resources_help/resources_versions_cleanup.md cmd/mr/commands/resources_help/resources_meta_keys.md cmd/mr/commands/resources_help/resources_timeline.md
git commit -m "docs(cli): write help for resources destructive + admin commands"
```

---

### Task 16: Add `docs`, `resource`, `resources` to lint allowlist

**Files:**
- Modify: `cmd/mr/commands/docs_lint.go` (the `lintAllowlist` map)

- [ ] **Step 1: Add entries**

Change:

```go
var lintAllowlist = map[string]bool{}
```

to:

```go
var lintAllowlist = map[string]bool{
	"docs":      true,
	"resource":  true,
	"resources": true,
}
```

`docs` is safe to allowlist now because Task 4 already populates Long/Example/Annotations for every `docs` subcommand.

- [ ] **Step 2: Run lint**

Run: `./mr docs lint`
Expected: PASS. If any failures, fix the corresponding `.md` file (likely: a missing 2nd example, a Long with only one sentence, a missing `outputShape`).

Expected warnings: any leaf command without a `# mr-doctest:` example. Warnings are informational; they do not fail CI.

- [ ] **Step 3: Run Go test suite**

Run: `go test ./cmd/mr/...`
Expected: PASS (TestLintRealTree from Task 7 now validates resource + resources too).

- [ ] **Step 4: Commit**

```bash
git add cmd/mr/commands/docs_lint.go
git commit -m "feat(cli): lint resource + resources help per template"
```

---

### Task 17: Run doctest against ephemeral server and fix failures

This is a verification task. Running for the first time is where brittle examples get found.

- [ ] **Step 1: Start ephemeral server**

In a separate terminal:
```bash
./mahresources -ephemeral -bind-address=:8181 -max-db-connections=2
```

- [ ] **Step 2: Run check-examples**

```bash
npm run build-cli
./mr docs check-examples --server http://localhost:8181
```
Expected: per-example PASS/FAIL lines. Non-zero exit if any FAILs.

- [ ] **Step 3: Fix failures iteratively**

For each FAIL:
1. Read the failure message (exit code, stderr snippet).
2. Decide: is the example wrong (fix the Markdown), or is the assumed tolerance missing (add `tolerate=...` or `skip-on=ephemeral`)?
3. Re-run `check-examples` after each change.

Commit fixes in small batches (one logical group per commit, e.g., "fix rotate and preview doctest tolerances").

- [ ] **Step 4: Final run**

Expected: all `# mr-doctest:` blocks PASS or SKIP. No FAIL.

- [ ] **Step 5: Commit**

If no changes from this task beyond iterative fixes, no extra commit. The iterative commits from Step 3 cover it.

---

### Task 18: Generate docs-site pages for the resource subtree

**Files:**
- Create: `docs-site/docs/cli/resource/*.md`
- Create: `docs-site/docs/cli/resources/list.md`, `docs-site/docs/cli/resources/index.md`
- Possibly: `docs-site/docs/cli/index.md` (full CLI landing, but only if feasible with Phase 2 alone; otherwise defer to Phase 3)

- [ ] **Step 1: Run docs-gen**

```bash
npm run build-cli
npm run docs-gen
```

Expected: `docs-site/docs/cli/resource/*.md` and `docs-site/docs/cli/resources/*.md` are created. Other top-level groups (tag, group, note, etc.) will also have generated files with thin Long/Example content from Phase 1's refactor: that's fine; Phase 3 will fill those in.

- [ ] **Step 2: Manually inspect resource/get.md**

Run: `cat docs-site/docs/cli/resource/get.md`
Expected: front matter; `# mr resource get` heading; the Long prose; "## Examples" with labeled blocks; "## Flags" (empty or "no local flags"); "### Inherited global flags" table; "## Output"; "## Exit Codes"; "## See Also" with markdown links.

If any section reads awkwardly, fix the generator template in `docs_dump.go`. Re-run `docs-gen` and re-inspect.

- [ ] **Step 3: Confirm old hand-written CLI pages in docs-site are not conflicting**

Run: `ls docs-site/docs/cli/ 2>/dev/null || ls docs-site/docs/`
Expected: understand whether old hand-written pages exist. If they do and overlap with generated paths (e.g., `docs-site/docs/cli/resource.md`), remove them in this task. If they're in a separate `docs-site/docs/features/` tree, defer cleanup to Phase 4.

- [ ] **Step 4: Commit generated docs**

```bash
git add docs-site/docs/cli/
git commit -m "docs(cli): generate docs-site pages for resource subtree"
```

---

### Task 19: Pilot review and capture template adjustments

Manual review task. The spec says: after the pilot, "is `mr resource get --help` actually impeccable? Adjust the template if needed."

- [ ] **Step 1: Read each `mr <resource subcommand> --help` as if new to the tool**

Run each of these and read the output top-to-bottom:

```bash
./mr resource --help
./mr resource get --help
./mr resource upload --help
./mr resource edit --help
./mr resource versions --help
./mr resource versions-compare --help
./mr resources list --help
```

- [ ] **Step 2: Note issues**

Capture a short list of concrete problems. Examples of real issues to watch for:
- Text too dense; would benefit from a blank line before Examples
- Long descriptions that bury the essential info
- Examples where the placeholder IDs aren't obvious (should be `42` or `<id>`, not arbitrary)
- Flag help that's still too terse
- See Also links pointing at non-existent commands

- [ ] **Step 3: Apply fixes**

Edit the relevant `.md` files. For template-level issues (e.g., "See Also should appear above Flags, not at the end"), edit the help template in `helptemplate.go` and the Markdown generator in `docs_dump.go`.

- [ ] **Step 4: Run full verification**

```bash
go test ./cmd/mr/...
./mr docs lint
./mr docs check-examples --server http://localhost:8181
npm run docs-gen
```
Expected: all PASS; no diff in git working tree for generated docs beyond intended changes.

- [ ] **Step 5: Commit any tweaks**

```bash
git add -u
git commit -m "docs(cli): pilot review adjustments from resource group"
```

---

## Phase 3 & Phase 4 (Outline Only: Separate Plans Later)

### Phase 3: Migrate remaining command groups

One migration task per group, following the exact Task 10-16 pattern used for `resource`:

1. Create placeholder help files; refactor the Go command file to use `helptext.Load`.
2. Write full Long + Examples per subcommand.
3. Add the group to the lint allowlist.
4. Run doctest; fix failures with metadata as needed.
5. Regenerate docs-site pages.

Priority order (most-used first):

`group` → `note` → `mrql` → `search` → `query` → `note-type` → `tag` → `category` → `resource-category` → `relation` → `relation-type` → `note-block` → `group-export` → `group-import` → `series` → `timeline` → `admin` → `job` → `log` → `plugin`

Exit criterion: all top-level groups are in the lint allowlist; `./mr docs lint` passes without any allowlist bypass (remove the allowlist map entirely in the final commit of Phase 3).

### Phase 4: Cleanup + CI integration

- Delete any hand-written CLI pages in `docs-site/docs/features/cli*` (or wherever the current CLI docs live) that are superseded by generated content.
- Wire CI checks:
  - `go test ./cmd/mr/...` already covers the lint (see Task 7).
  - New CI step: `npm run build-cli && npm run docs-gen && git diff --exit-code docs-site/docs/cli/`: fails the build if the author forgot to regenerate.
  - New CI job (in the E2E workflow): runs the ephemeral server + `mr docs check-examples`.
- Update `README.md` CLI section to point at the regenerated docs-site (instead of the old hand-written page).
- Add a short "Documentation" section to `CLAUDE.md`: "When you add or change a command or flag in `cmd/mr/commands/`, update the corresponding `<group>_help/*.md` file. CI runs `./mr docs lint` and `./mr docs check-examples`."
- Final full test sweep: `go test --tags 'json1 fts5' ./... && cd e2e && npm run test:with-server:all && npm run test:with-server:cli-doctest`.

---

## Self-Review Checklist

**Spec coverage:**

| Spec section | Covered by tasks |
|---|---|
| Goals (rich help, single source of truth, agent JSON, doctest-guaranteed examples) | Tasks 5, 6, 7, 8, 11-15 |
| Non-goals (no behavior changes; skip help/completion) | Enforced in Task 5 (walkSkippingBuiltins) |
| Template (Short/Long/Example/Flag help/Annotations) | Task 1 (parser), Task 7 (lint), Tasks 11-15b (content) |
| Flag ordering via SetSortFlags | Task 3 |
| Code organization (embed Markdown) | Task 1, Task 10 |
| `mr docs dump` | Tasks 4-6 |
| `mr docs lint` | Task 7 |
| `mr docs check-examples` + per-example metadata | Task 8 |
| Testdata fixtures | Task 2 |
| Doctest: tolerate/expect-exit/skip-on/timeout/stdin | Task 8 (runner + `--environment` flag) + Tasks 11-15b (use) |
| docs-site generation pipeline | Tasks 6, 9, 18 |
| Phase 1 phasing | Tasks 1-9 |
| Phase 2 phasing (pilot on resource + resources) | Tasks 10, 11, 12, 13, 14, 15a, 15b, 16, 17, 18, 19 |
| Phases 3 and 4 | Outlined above |

**Placeholder scan:** no "TBD" / "TODO" / "similar to Task N" / vague error-handling directives. Code blocks present for every code step. Phase 2 help content uses outlines with key bullets + example command structure per user direction ("outline acceptable").

**Type consistency:**
- `Help` struct (Task 1) matches the struct used in Task 10's refactor.
- `dumpCommand` struct (Task 5) matches the shape lint walks in Task 7 and the Markdown template consumes in Task 6.
- `dumpExample` fields (`Doctest`, `ExpectedExit`, `SkipOn`, `Tolerate`, `TimeoutSec`, `Stdin`) are used consistently in Tasks 5, 7 (lint check for `Doctest`), and 8 (runner).
- `lintAllowlist` map key is the top-level command name (e.g., `"resource"`), referenced consistently in Tasks 7 and 16.
- Filename conventions: hyphens in command names become underscores in filenames (e.g., `version-upload` → `resource_version_upload.md`). Applied consistently in Tasks 10-15.

---

## Execution

Plan complete. Ready to execute via `superpowers:subagent-driven-development` (fresh subagent per task with between-task review) or `superpowers:executing-plans` (inline batch execution with checkpoints).
