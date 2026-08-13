---
name: mahresources-mrql
description: "Query a mahresources server with MRQL through the `mr` CLI: one-off queries, saved queries, EXPLAIN, CSV/JSON export, and the `--mrql` list filter. Use whenever a task mentions `mr mrql`, MRQL syntax, or pulling Resources, Notes, or Groups out of a mahresources instance from a shell."
---

# MRQL via the `mr` CLI

MRQL (Mahresources Query Language) is a server-side DSL for querying a mahresources
instance across three entity types: **Resources** (files), **Notes** (text), and
**Groups** (hierarchical collections). `mr mrql` is the CLI front end for it.

Reach for MRQL instead of `mr resources list` / `mr notes list` whenever the task needs
boolean logic, cross-entity search, relative dates, metadata predicates, group-subtree
scoping, hierarchy traversal, or `GROUP BY` aggregation.

## Before the first query

1. **Binary.** `mr --help` must work. If the repo is checked out, build it with
   `go build -o mr ./cmd/mr`.
2. **Server URL.** Defaults to `http://localhost:8181`. Override with `--server <url>`
   or the `MAHRESOURCES_URL` environment variable.
3. **Auth.** Only needed if the server runs with `-auth`. Either `mr auth login`
   (mints and stores a personal API token) or set `MR_TOKEN=<token>`. `MR_TOKEN`
   always overrides the stored credentials file; `MR_TOKEN_FILE` relocates that file.
4. **Smoke test.** Confirm connectivity and shape before building a real query:

   ```bash
   mr mrql 'type = resource LIMIT 1' --json
   ```

   Exit code is `0` on success and `1` on any error, for every subcommand.

## Command surface

| Command | Purpose |
|---|---|
| `mr mrql '<query>'` | Execute a one-off query |
| `mr mrql -f query.mrql` / `echo '<query>' \| mr mrql -` | Query from a file or stdin |
| `mr mrql explain '<query>'` | Print the SQL it *would* run, without executing |
| `mr mrql export '<query>'` | Stream results as CSV or JSON |
| `mr mrql save <name> '<query>'` | Persist a named query |
| `mr mrql list` | List saved queries |
| `mr mrql run <name-or-id>` | Execute a saved query |
| `mr mrql delete <id>` | Delete a saved query (numeric ID only, destructive) |
| `mr resources\|notes\|groups list --mrql '<filter>'` | Filter a list command with a bare MRQL filter expression |

**Global flags** (any subcommand): `--server <url>`, `--json`, `--quiet` (IDs only),
`--no-header`, `--page <n>` (page size 50 for list commands).

**Query flags.** `--limit`, `--buckets`, `--offset` and `--param` are shared by `mrql`,
`mrql run` and `mrql export`; the rest are noted per command below.

| Flag | Meaning |
|---|---|
| `--limit <n>` | Total results, or **items per bucket** for a bucketed `GROUP BY` |
| `--buckets <n>` | Buckets per page (bucketed `GROUP BY` only) |
| `--offset <n>` | Bucket offset for cursor-based `GROUP BY` paging |
| `--param name=value` | Bind a `$name` placeholder (repeatable) |
| `--render` | Populate `renderedHTML` via `CustomMRQLResult` templates (`mrql` and `mrql run` only) |
| `-f, --file <path>` | Read the query from a file (`mrql`, `explain`, `export`) |
| `--saved <name-or-id>` | Operate on a saved query (`explain`, `export` only) |
| `--format csv\|json`, `-o <file>` | `export` only; CSV is the default |
| `--description <text>` | `save` only |

## Query shape

```
[type = "resource|note|group" AND] <conditions>
  [SCOPE <group-id-or-name>]
  [GROUP BY <field>[, <field>...] [<aggregates>] [HAVING <aggregate-conditions>]]
  [ORDER BY <field> [ASC|DESC] | RANDOM() | RANK]
  [LIMIT <n>] [OFFSET <n>]
```

Omitting `type =` runs a **cross-entity** query over all three types, restricted to
the fields common to all of them.

```bash
mr mrql 'type = resource AND tags = "photo" AND created > -30d ORDER BY created DESC LIMIT 20'
mr mrql 'type = resource GROUP BY contentType COUNT() ORDER BY count DESC'
mr mrql 'type = note AND TEXT ~ "kubernetes migration" ORDER BY RANK LIMIT 10'
mr mrql 'name ~ "budget*"'    # cross-entity
```

Full syntax (every field, operator, traversal form, aggregate and guardrail) is in
`references/language.md`. Task-shaped worked examples are in `references/recipes.md`.

## Working with the output

Three response shapes, distinguished by `mode`:

```jsonc
// standard (no GROUP BY). A zero-match query is exactly the envelope, with no
// "resources" key at all: {"entityType":"resource","default_limit_applied":true,"applied_limit":500}
{"entityType": "resource", "resources": [...], "default_limit_applied": true, "applied_limit": 500}

// aggregated GROUP BY (has aggregate functions)
{"entityType": "resource", "mode": "aggregated", "columns": [...], "rows": [{"contentType": "image/png", "count": 42}]}

// bucketed GROUP BY (no aggregate functions)
{"entityType": "resource", "mode": "bucketed", "keyColumns": [...],
 "groups": [{"key": {...}, "items": [...]}], "totalGroups": 150, "nextOffset": 10}
```

Read `columns` / `keyColumns` for column order, because a JSON object carries none. A bucket's
`key` may hold extra entries `keyColumns` does not name (a relation-keyed bucket also
gets `<field>_id` so two same-named groups stay apart); those print after the named ones.

**Key casing is not uniform.** Entities inside `resources` / `notes` / `groups` are the
GORM models, whose core columns carry no JSON tag and so serialize **PascalCase**:
`.ID`, `.Name`, `.CreatedAt`, `.UpdatedAt`. Saved-query objects from `mrql save` and
`mrql list` are tagged and serialize lowercase: `.id`, `.name`, `.query`,
`.description`. `jq` on the wrong casing silently yields `null`, not an error.

**Empty collections are omitted, not empty.** `resources`, `notes`, `groups` and `rows`
are `omitempty`, so a query that matches nothing has no such key at all and `jq
'.resources[]'` fails with "Cannot iterate over null". Use the optional form (`.resources[]?`)
or a default (`.rows // []`) in any pipeline that must survive a zero-result query.

```bash
mr mrql 'type = resource AND fileSize > 100mb' --json | jq -r '.resources[]?.ID'
mr mrql 'type = resource AND tags = "photo"' --quiet          # IDs only, no jq needed
mr mrql 'type = resource GROUP BY contentType COUNT()' --json | jq -r '.rows[]? | "\(.contentType)\t\(.count)"'
mr mrql list --json | jq -r '.[] | "\(.id)\t\(.name)\t\(.query)"'
```

`--quiet` prints the first table column, which is the ID. A cross-entity query therefore
merges resource, note and group IDs into one undifferentiated list. Use `--json` there.

## Patterns that work well for agents

- **Explain before you run anything expensive.** `mr mrql explain '<query>'` returns the
  SQL without touching data, resolves `SCOPE`, and shows whether the default `LIMIT` was
  applied. It is the cheapest way to confirm a query means what you think.
- **Parameterize instead of interpolating.** Build the query once with `$name`
  placeholders and bind with `--param name=value`. Binding is value-level and
  injection-safe; string interpolation into query text is not.

  ```bash
  mr mrql 'type = resource AND tags = $tag AND created > $since' --param tag=photo --param since=-7d
  ```

  A supplied value coerces like a typed literal (`-7d`, `10mb`, `NOW()`). Force a
  string with embedded quotes: `--param n='"42"'`. Every placeholder must be supplied;
  unknown ones are rejected; names are case-sensitive.
- **Always set an explicit `LIMIT`** when exploring. Without one the server applies its
  configured default (500 in the standard binary). Deployments hold millions of
  resources. An explicit `LIMIT` or `OFFSET` above 10,000 is rejected outright rather
  than truncated, and the cap is the same for `mrql export`, so page for anything larger.
- **Export for the format, not for the volume.** `mr mrql export '<query>' --format csv -o
  out.csv` streams CSV or JSON straight to a file, but it runs under the same bounds as an
  ordinary query: the 10,000 cap still applies, and a query with no `LIMIT` still gets the
  server default. Widen `LIMIT` (up to the cap) or page to get more rows. CSV requires a
  single entity type; use `--format json` for cross-entity results.
- **Save a query you will run more than twice**, then run it by name:
  `mr mrql save daily-photos 'type = resource AND tags = "photo" AND created > -1d'`
  followed by `mr mrql run daily-photos --json`. `mrql run` resolves the argument as an
  ID first, then as a name.
- **Filtering an existing list command** is often simpler than a full query:
  `mr resources list --mrql 'tags = "vacation" AND created > -30d'`. That grammar is the
  filter expression *only*: `ORDER BY`, `LIMIT`, `OFFSET`, `GROUP BY`, `SCOPE`, `$name`
  parameters and an explicit `type` are all rejected there.

## Pitfalls

- **`category` and `noteType` are numeric IDs, and a name there fails silently.**
  `category = "Photos"` is accepted and matches nothing, so it looks like "no results"
  rather than a mistake. Write `category = 3`, or go through the group
  (`owner.category = 3`, `SCOPE "Photos"`). `owner` and `parent` are the exception: they
  take an ID *or* a group name, so `owner = "Project Alpha"` works.
- **`~` is not a regex.** It is a case-insensitive substring/wildcard match (`*`, `?`),
  auto-wrapped in `*…*` when the value contains no wildcard. Real POSIX regex is
  `~*` / `!~*`, and those are **PostgreSQL-only**: they error on SQLite.
- **`ancestors.` / `descendants.` exclude the base entity.** A resource sitting directly
  in "Archive" does *not* match `ancestors.name = "Archive"`. Write
  `owner.name = "Archive" OR ancestors.name = "Archive"` to include it.
- **`LIMIT` means per-bucket in bucketed `GROUP BY`**, not total. Page buckets with
  `--buckets` / `--offset`, and items within a bucket with the query's own `LIMIT`.
- **`mr mrql delete` takes only a numeric ID.** Resolve a name first:
  `mr mrql list --json | jq -r '.[] | select(.name == "my-query") | .id'`.
- **`ORDER BY RANK` needs full-text search.** Exactly one `TEXT ~` predicate, a single
  entity type, no `GROUP BY`, and a server not started with `-skip-fts`.
- **Saved MRQL queries are not saved SQL queries.** `mr mrql run` executes MRQL;
  `mr query run` executes raw read-only SQL Query records. Different commands, different
  languages.

## References

- `references/language.md`: complete MRQL syntax. Fields per entity type, operators,
  relation counts, dates, `SCOPE`, `GROUP BY`/`HAVING`/date buckets, traversal,
  similarity search, ordering keys, parameters, guardrails.
- `references/recipes.md`: task-to-query cookbook with runnable shell examples.
