---
name: mahresources-cli
description: Operate a mahresources server from the command line with the `mr` CLI — list, create, edit, tag, and link Resources, Notes, and Groups; run MRQL queries and full-text search; manage download jobs, resource versions, series, note blocks, note sharing, taxonomy (tags/categories/note types/relations), plugins, group export/import, audit logs, and admin settings. Use whenever a task involves the `mr` binary or driving a mahresources instance over its HTTP API from a shell.
---

# mahresources CLI (`mr`)

`mr` is a [cobra](https://github.com/spf13/cobra)-based command-line client for a **mahresources** server (a personal-information-management app that stores Resources, Notes, Groups, and their relationships). The CLI is a **thin HTTP client**: every subcommand maps to a `/v1/...` JSON endpoint on the server, and all business logic lives server-side. There is no local state.

Use this skill to drive mahresources from a shell or a script. For the exhaustive per-command reference (every flag, arg, example), read `references/command-reference.md`. For the query language, read `references/mrql.md`. For multi-step scripting patterns and the full gotcha list, read `references/recipes.md`.

## Getting the binary

The CLI is built from `cmd/mr`. In this repo:

```bash
go build --tags 'json1 fts5' -o mr ./cmd/mr   # produces ./mr
```

It talks to a running server. There is **no authentication** (mahresources is private-network software), so any reachable server accepts commands.

## Connecting to a server

| How | Value |
|-----|-------|
| `--server <url>` flag | per-command override |
| `MAHRESOURCES_URL` env var | session default |
| built-in default | `http://localhost:8181` |

Flag beats env beats default. Example: `mr --server http://box:9000 resources list` or `export MAHRESOURCES_URL=http://box:9000`.

## Global flags (work on every command)

| Flag | Effect |
|------|--------|
| `--json` | Print the raw server JSON (pretty-printed) instead of a table. **Use this for scripting.** |
| `--quiet` | Print only the first column of each row (the ID). Good for piping IDs into `xargs`. |
| `--no-header` | Omit table headers in normal mode. |
| `--page <n>` | Page number for `list` commands. **Page size is fixed at 50** — there is no page-size flag; raise `--page` to walk pages. |

On any error the CLI writes `HTTP <code>: <message>` to **stderr** and exits **1**; success exits **0**. Output goes to stdout.

## The entity model (mental map)

```
Resource ── files: bytes + metadata + thumbnails + version history; belongs to ≤1 Series
Note     ── text content + metadata + ordered "blocks" (text/heading/todos/…)
Group    ── hierarchical container; OWNS resources/notes/child groups (a tree via owner-id)

Many-to-many links (attach with comma-separated ID lists):
  Resource ↔ Tags, Notes, Groups
  Note     ↔ Tags, Groups, Resources
  Group    ↔ Tags, Resources, Notes, other Groups (+ typed GroupRelations)

Taxonomy / labels (1 → many):
  Tag              → freely attached to resources/notes/groups (many-to-many)
  Category         → classifies a Group       (group.category)
  ResourceCategory → classifies a Resource    (resource.resource-category-id)
  NoteType         → classifies a Note        (note.note-type-id); can carry a MetaSchema

Saved searches:
  MRQL query  → the mahresources query DSL (mr mrql …)  — see references/mrql.md
  Query       → raw, READ-ONLY SQL with template params (mr query …)
```

`owner-id` is the parenting/ownership field everywhere: a resource/note/group "belongs to" the group whose ID is its `owner-id`. Groups form a tree through it (`group parents` / `group children` navigate it).

## Singular vs. plural commands — the core pattern

Almost every entity family has a **singular** command (operates on one entity) and a **plural** command (lists + bulk operations). Learn it once and it generalizes:

| Singular (`resource`, `note`, `group`, `tag`, `category`, `series`, …) | Plural (`resources`, `notes`, `groups`, `tags`, …) |
|---|---|
| `get <id>`, `create`, `delete <id>` | `list` (filterable, paged) |
| `edit <id>`, `edit-name`, `edit-description`, `edit-meta` | `add-tags`, `remove-tags`, `replace-tags` |
| entity-specific verbs (`upload`, `rotate`, `share`, `clone`, …) | `add-meta`, `add-groups`, `merge`, `delete`, `timeline` |

Example: `mr note get 5` (one note) vs `mr notes list --tags 3` (filtered list) vs `mr notes add-tags --ids 5,6 --tags 3` (bulk).

## Universal conventions

- **IDs are unsigned integers.** Linking/selection flags (`--tags`, `--groups`, `--notes`, `--resources`, `--ids`, `--losers`, …) take a **comma-separated list of IDs**: `--tags 5,6,7`. Whitespace is trimmed; empty entries skipped.
- **`--meta '<json-object>'`** attaches/merges free-form metadata into an entity's `Meta` field on `create` and bulk `add-meta`. It is a JSON **object** string.
- **`<entity> edit-meta <id> <path> <value>`** edits a single metadata field by dot-path. `<value>` is a **JSON literal**, so a string value must include its own quotes: `mr resource edit-meta 5 location.city '"Berlin"'`. Intermediate objects are created; siblings are untouched.
- **Bulk tag semantics:** `add-tags` = set union (idempotent); `remove-tags` = set difference (idempotent, no error if a tag wasn't attached); `replace-tags` = exact set (`--tags ''` clears all tags).
- **`merge --winner <id> --losers <ids>`** (on resources/groups/tags) moves every relationship from the losers onto the winner, then deletes the loser records. Irreversible.

## Output JSON shapes — the #1 scripting footgun

Two different casings come back depending on the endpoint, and `jq` is case-sensitive:

- **Entity objects** (from `get`/`create`/`list` on resources, notes, groups, tags, …) serialize with **Go field names → PascalCase**: `.ID`, `.Name`, `.Description`, `.CreatedAt`, `.UpdatedAt`, `.Meta`, `.Tags`, `.Groups`. So `mr resources list --json | jq -r '.[].ID'` — **`.ID`, not `.id`**.
- **Purpose-built API wrappers** use **lowercase/camelCase**: MRQL (`.resources[]`, `.notes[]`, `.groups[]`, `.rows`, `.mode`), search (`.total`, `.results[].id`), jobs (`.jobs[].id`, `.status`), saved MRQL list (`.id`, `.name`), plugins (`.name`, `.enabled`), `group children` (`.id`, `.name`, `.childCount`), logs (`.logs[].action`, `.entityId`).
  - Note: entity rows *inside* an MRQL result are still PascalCase — `mr mrql '…' --json | jq '.resources[].ID'`.

When unsure, run the command with `--json` once and look before writing the `jq` path.

## Discoverability

- `mr --help` lists all command groups; `mr <group> --help` and `mr <group> <cmd> --help` show flags, args, and examples.
- `mr docs dump --format json` prints the **entire** command tree (every flag, arg, example, output shape) as machine-readable JSON — the authoritative source. `references/command-reference.md` in this skill is a rendered snapshot of it.

## High-value workflows

**Find then act (IDs → bulk op):**
```bash
# Tag every JPEG created this year
mr resources list --content-type image/jpeg --created-after 2026-01-01 --quiet \
  | paste -sd, - \
  | xargs -I{} mr resources add-tags --ids {} --tags 5
```

**Upload a file and attach it:**
```bash
ID=$(mr resource upload ./photo.jpg --owner-id 3 --meta '{"camera":"Pixel"}' --json | jq -r '.ID')
```

**Background-download URLs and watch them:**
```bash
mr job submit --urls https://a.com/1.jpg,https://b.com/2.jpg --tags 5 --owner-id 3
mr jobs list --json | jq -r '.jobs[] | "\(.status)\t\(.url)"'
```

**Query with MRQL (see `references/mrql.md`):**
```bash
mr mrql 'type = resource AND tags = "photo" AND fileSize > 10mb ORDER BY created DESC LIMIT 20'
```

## Critical gotchas (read `references/recipes.md` for the full list)

- **`jq` paths:** entity objects are PascalCase (`.ID`), wrappers are lowercase. See above.
- **`note share` is idempotent.** Re-sharing an already-shared note returns the **existing** token; to rotate, `note unshare` then `note share` again.
- **`recalculate-dimensions` does NOT create a new version** (DB-only). `rotate` and `version-upload` DO create new versions.
- **Download jobs are in-memory** and lost on server restart; they are not persisted.
- **`job cancel`** marks the job `cancelled` but leaves it in the queue for inspection — it is not removed.
- **Page size is fixed at 50.** Walk pages with `--page`.
- A resource belongs to **at most one Series**; membership is set on the resource (`resource edit <id> --series-id N`), not on the series.
- The first **text** note-block and the note's **Description** stay in sync — editing one updates the other.

## Reference files

- `references/command-reference.md` — every command, flag, argument, example, and output shape (rendered from `mr docs dump`). Grep it by command path, e.g. `resource upload`, `mrql run`, `groups merge`.
- `references/mrql.md` — the MRQL query language: fields per entity type, operators, dates, GROUP BY, SCOPE, traversal, and the `mr mrql` / `mr query` / `mr search` CLI.
- `references/recipes.md` — scripting recipes (jq pipelines, async polling, export/import, traversal) and the complete catalogue of verified semantics and gotchas.
