# Installable agent skills

Skills in this directory follow the [open agent skills](https://github.com/vercel-labs/skills)
format: a directory containing a `SKILL.md` with YAML frontmatter, plus any supporting
reference files.

## Install

```bash
# Just the MRQL skill (recommended)
npx skills add https://github.com/egeozcan/mahresources/tree/master/skills/mahresources-mrql

# Or from the repo root, selecting by name
npx skills add egeozcan/mahresources --skill mahresources-mrql
```

Add `-g` to install globally (user-level) rather than into the current project, and
`-a <agent>` to target a specific agent. Preview without installing with `--list`:

```bash
npx skills add https://github.com/egeozcan/mahresources/tree/master/skills/mahresources-mrql --list
```

Note that `npx skills add egeozcan/mahresources` with no `--skill` also discovers the
project-local skills under `.claude/skills/`, which are tailored to development *of* this
repository rather than to *using* a mahresources server.

## Skills

| Skill | Purpose |
|---|---|
| [`mahresources-mrql`](./mahresources-mrql/) | Query a mahresources server with MRQL through the `mr` CLI |

## Maintaining them

`mahresources-mrql/references/language.md` is **generated** and carries a do-not-edit
header. Its language sections come from `docs-site/docs/features/mrql-reference.md` and
its CLI section from the live Cobra tree, so a new field or flag reaches the skill
without anyone retyping it:

```bash
npm run build-cli && npm run skills-gen
```

Three gates keep the rest honest, all in CI:

- `mrql/reference_docs_test.go` and `application_context/mrql_reference_docs_test.go`
  fail if the reference page and the Go source disagree about which fields exist or what
  the guardrail maximums are.
- The `cli-docs-fresh` job regenerates and diffs `skills/`, so a stale committed copy
  fails the build.
- The `cli-doctest` job runs every fenced `bash` block in `SKILL.md` and
  `references/recipes.md` against an ephemeral server
  (`mr docs check-examples --files`). A block that cannot run standalone opts out with
  `# mr-doctest: skip, <reason>`.

Edit `SKILL.md` and `references/recipes.md` directly. Edit the docs-site page, not the
generated reference.
