# `mr` CLI — Recipes & Verified Semantics

Scripting patterns and the full catalogue of non-obvious behavior. All claims here were verified against the mahresources source (`cmd/mr/...`, `application_context/...`, `models/...`) — where the implementation and a command's own `--help` text disagree, this file follows the **implementation**.

---

## 1. JSON output casing (the #1 footgun)

`jq` is case-sensitive and mahresources returns two different casings:

| Endpoint family | Shape | Example jq |
|---|---|---|
| Entity `get` / `create` / `list` (resource, note, group, tag, category, series, note-type, …) | **PascalCase** Go field names | `mr resources list --json \| jq -r '.[].ID'` |
| MRQL standard result | lowercase wrapper, PascalCase rows | `mr mrql '…' --json \| jq '.resources[].ID'` |
| MRQL aggregated `GROUP BY` | `.mode`, `.rows[]` (lowercase aggregate keys: `count`, `sum_fileSize`) | `mr mrql '… GROUP BY contentType COUNT()' --json \| jq '.rows'` |
| `search` | `.query`, `.total`, `.results[].id` | `mr search foo --json \| jq '.total'` |
| `jobs list` / `job` ops | `.jobs[].id`, `.status`, `.progress` | `mr jobs list --json \| jq -r '.jobs[].id'` |
| `mrql list` (saved queries) | `.id`, `.name`, `.query` | `mr mrql list --json \| jq -r '.[].id'` |
| `plugins list` | `.name`, `.enabled`, `.values` (always JSON, never a table) | `mr plugins list --json \| jq '.[] \| select(.enabled)'` |
| `group children` | lightweight nodes: `.id`, `.name`, `.categoryName`, `.childCount`, `.ownerId` | `mr group children 4 --json \| jq '.[].id'` |
| `logs list` / `log entity` | `.logs[]` with `.action`, `.entityType`, `.entityId`, `.message` (+ `.details` raw JSON, often empty) | `mr logs list --json \| jq '.logs'` |

**Rule of thumb:** a stored entity → `.ID`; a built-for-purpose API response → `.id`. When unsure, run once with `--json` and look.

---

## 2. Find → act (IDs into a bulk op)

`--quiet` prints just the ID column; collapse to a comma list and feed a bulk command.

```bash
# Add tag 5 to every JPEG created this year
mr resources list --content-type image/jpeg --created-after 2026-01-01 --quiet \
  | paste -sd, - \
  | xargs -I{} mr resources add-tags --ids {} --tags 5

# Same idea via MRQL + jq (note PascalCase .ID inside .resources)
mr mrql 'type = resource AND contentType ~ "image" AND created >= -1y' --json \
  | jq -r '.resources | map(.ID) | join(",")' \
  | xargs -I{} mr resources add-tags --ids {} --tags 5
```

`--quiet` also works on MRQL bucketed output (prints only IDs, no bucket headers).

---

## 3. Paging a list (fixed page size 50)

There is no page-size flag. Walk pages until a short/empty page:

```bash
page=1
while :; do
  out=$(mr resources list --page "$page" --quiet)
  [ -z "$out" ] && break
  echo "$out"
  page=$((page+1))
done
```

For large result sets prefer a single MRQL query with an explicit `LIMIT` (MRQL's default limit is the server's `MRQL_DEFAULT_LIMIT`, default 500).

---

## 4. Create-and-capture

Almost every `create`/`upload` prints the new entity; capture its ID with `--json`:

```bash
GID=$(mr group create --name "Trip 2026" --json | jq -r '.ID')
RID=$(mr resource upload ./a.jpg --owner-id "$GID" --json | jq -r '.ID')
NID=$(mr note create --name "log" --groups "$GID" --resources "$RID" --json | jq -r '.ID')
```

---

## 5. Background download jobs

`job submit` queues one job per URL and **returns immediately** — downloads run server-side in the background.

```bash
mr job submit --urls https://a.com/1.jpg,https://b.com/2.jpg --tags 5 --owner-id 3 --json
# -> { "queued": true, "jobs": [ { "id": "...", "url": "...", "status": "..." }, ... ] }

# Poll the queue
watch -n2 'mr jobs list --json | jq -r ".jobs[] | \"\(.status)\t\(.progressPercent)%\t\(.url)\""'
```

- Jobs live **in server memory** — a restart clears the entire queue (not persisted).
- Control: `job pause <id>`, `job resume <id>`, `job cancel <id>`, `job retry <id>`.
- `job cancel` marks the job `cancelled` but **leaves it in the queue** for inspection; it is not deleted.
- `pause` + `resume` is the non-destructive way to halt a running download.

---

## 6. Resource versions

Every resource keeps an append-only version history.

```bash
mr resource version-upload 42 ./photo_v2.jpg --comment "color corrected"   # NEW version
mr resource versions 42 --json | jq -r '.[].id'                            # list
mr resource versions-compare 42 --v1 10 --v2 11                            # sameHash/sizeDelta/…
mr resource version-restore --resource-id 42 --version-id 10               # roll back (all flags)
mr resource versions-cleanup 42 --keep 3                                   # OR --older-than-days 30
mr resource versions-cleanup 42 --older-than-days 30 --dry-run             # preview
```

Which operations create a new version:

| Operation | New version? |
|---|---|
| `version-upload` | **Yes** |
| `rotate` (image) | **Yes** |
| `recalculate-dimensions` | **No** — re-reads bytes, updates DB only |
| `set-dimensions` | **No** — forces DB values, never touches bytes |

`versions-cleanup --keep N` and `--older-than-days N` are **alternative** policies (OR), not combined.

---

## 7. Notes and note blocks

A note's body is an ordered list of **blocks** (types: `text`, `heading`, `todos`, `table`, `gallery`, `divider`, …; discover with `mr note-block types --json`). Blocks sort by a fractional **string** position (lexicographic: `"a" < "m" < "z"`), so you can insert between two blocks without renumbering.

```bash
NID=$(mr note create --name "Q2 Planning" --tags 5 --groups 10 --json | jq -r '.ID')

# Append blocks (omit --position to auto-place at the end)
mr note-block create --note-id "$NID" --type heading --content '{"text":"Goals","level":2}' --position a
TODO=$(mr note-block create --note-id "$NID" --type todos \
        --content '{"items":[{"id":"t1","text":"ship"},{"id":"t2","text":"test"}]}' --json | jq -r '.ID')

# Check off a todo: State is separate from Content and is NOT NULL — always send a JSON object
mr note-block update-state "$TODO" --state '{"checked":["t1"]}'

# Reorder by mapping block-ID -> new position (only listed blocks move; positions must stay unique)
mr note-blocks reorder --note-id "$NID" --positions '{"10":"a","11":"z"}'

# After many inserts, positions grow long; normalize them:
mr note-blocks rebalance --note-id "$NID"
```

Gotchas:
- The **first `text` block** and the note's **Description** are kept in sync — `note edit-description` updates the first text block, and editing/creating/reordering the first text block updates the Description.
- `update-state` validates against the block type; sending `null` or invalid JSON fails (the `State` column is `NOT NULL`). Send `{}` at minimum.
- `reorder` only moves the blocks you name; if a new position collides with an unmoved block you get a duplicate-position error.

---

## 8. Note sharing

```bash
mr note share 42 --json    # -> { "shareToken": "...", "shareUrl": "/s/..." }
mr note unshare 42
```

- **Idempotent:** calling `share` on an already-shared note returns the **existing** token (the implementation short-circuits when a token exists). To rotate a token, `unshare` first, then `share` again.
- Sharing requires the server to be started with `-share-port`. Absolute share URLs require `-share-public-url`; otherwise `shareUrl` is a relative `/s/<token>` path.

---

## 9. Group hierarchy, relations, export/import

```bash
# Navigate the tree (children return lightweight lowercase nodes)
mr group parents 42     # ancestor chain, root first, queried group last
mr group children 42    # direct children: {id,name,categoryName,childCount,ownerId}

# Clone copies scalar fields + tag links only — NOT child groups/resources/notes
mr group clone 42

# Typed relation between two groups (directional FROM -> TO, constrained by relation type)
mr relation-type create --name references --reverse-name referenced-by --from-category 1 --to-category 2
mr relation create --from-group-id 10 --to-group-id 20 --relation-type-id 5

# Export a subtree to a portable tar (ASYNC; waits for the job by default)
mr group export 42 43 --gzip --no-blobs --output /tmp/shell.tar.gz

# Inspect an import before applying, then apply with grafting
mr group import /tmp/trip.tar --dry-run
mr group import /tmp/trip.tar --parent-group 17
```

- Export is asynchronous: it submits a job, polls until complete, then downloads the tar. `--no-wait` returns the job ID immediately; `--poll-interval` / `--timeout` tune polling.
- Export scope/fidelity is controlled by tri-state `--include-X` / `--no-X` pairs (subtree, resources, notes, related m2m, group-relations, blobs, versions, previews, series) plus schema-def flags. Defaults: most things on, **versions and previews off**.
- The tar uses **manifest schema version 1**, a stable contract: readers reject unknown major versions; unknown top-level keys are ignored for forward compatibility.
- Import surfaces a **plan** (counts, ambiguous mappings, dangling refs, hash conflicts). With `--auto-map=false` you must supply a `--decisions` JSON file. Resources whose bytes are missing from the tar require `--acknowledge-missing-hashes`.
- `clone` is shallow — for a deep subtree copy, `export` then `import`.

---

## 10. Metadata

```bash
# Bulk merge a JSON object into Meta (create or add-meta) — deep-merge, missing keys preserved
mr resources add-meta --ids 1,2 --meta '{"status":"reviewed","priority":5}'
mr note create --name n --meta '{"severity":"high"}'

# Single field by dot-path; <value> is a JSON LITERAL (a string needs its own quotes)
mr resource edit-meta 5 location.city '"Berlin"'
mr resource edit-meta 5 rating 4
mr resource edit-meta 5 flags.archived true
```

`add-meta` deep-merges (keys in the input overwrite; absent keys are kept). `edit-meta` touches exactly the one dotted path, creating intermediate objects as needed.

---

## 11. Taxonomy CRUD pattern

`tag` / `category` / `resource-category` / `note-type` / `relation-type` all share the singular/plural shape:

```bash
mr tag create --name "photo"
mr tag get 5
mr tag edit-name 5 photos
mr tags list --name pho           # filter
mr tags merge --winner 5 --losers 6,7   # consolidate duplicates (winner ID kept, losers deleted)
mr tags delete --ids 8,9
```

`note-type create` (and `category` / `resource-category`) additionally accept the UI/schema fields `--meta-schema`, `--section-config`, `--custom-header`, `--custom-sidebar`, `--custom-summary`, `--custom-avatar`, `--custom-css`, `--custom-mrql-result` — see the `mahresources-category-designer` skill for designing those.

---

## 12. Saved searches: MRQL vs. SQL Query

| | `mr mrql save/run` | `mr query create/run` |
|---|---|---|
| Language | MRQL DSL (see `mrql.md`) | raw **read-only** SQL (writes rejected) |
| Args | `mrql save <name> <query>`, `mrql run <name-or-id>` | `query create --name --text`, `query run <id>` (positional), `query run-by-name --name <name>` |
| Discover schema | n/a | `mr query schema` (table → columns map) |
| Result keys | `.resources[]` etc. (PascalCase rows) | one object per row; keys are the SQL SELECT column names — use `as` aliases |

`mr mrql run` accepts either an ID or a name in one argument; the CLI sends both `id` and `name` query params and the **server** tries ID first, then name (so a numeric *name* still resolves). `mr mrql delete <id>` takes a numeric ID only.

```bash
mr mrql save large-files 'type = resource AND fileSize > 100mb' --description "big files"
mr mrql run large-files --json | jq '.resources[].ID'
mr query create --name counts --text 'select count(*) as n from resources' --json
mr query run-by-name --name counts --json | jq '.[0].n'
```

---

## 13. Audit log & admin

```bash
mr logs list --entity-type resource --action update --created-after 2026-01-01T00:00:00Z --json | jq '.logs'
mr log entity --entity-type resource --entity-id 42 --json | jq '.logs[] | {action,message}'
mr log get 1001

mr admin stats                                  # server health + data counts + expensive stats
mr admin settings list
mr admin settings set max_upload_size 2147483648 --reason "video workflow"   # immediate, no restart
mr admin settings reset max_upload_size
```

- Log filters use **RFC3339** timestamps (`2026-01-01T00:00:00Z`); list filters AND together and page 50 at a time.
- Runtime `admin settings` overrides persist to the DB and take effect immediately. `--reason` records an audit trail. Size values accept suffixes (`2G`, `500M`); durations use Go syntax (`30s`, `5m`).

---

## 14. Plugins

```bash
mr plugins list --json | jq -r '.[] | "\(.name)\tenabled=\(.enabled)"'
mr plugin settings my-plugin --data '{"api_key":"…"}'   # WHOLESALE replace — omitted keys are lost
mr plugin enable my-plugin                              # fails if required settings are unwritten
mr plugin disable my-plugin
mr plugin purge-data my-plugin                          # irreversible
```

`plugin settings` replaces the entire settings object (it does not merge). Enable required settings *before* enabling a plugin that declares them. `plugins list` always emits JSON.

---

## Verified semantics quick-reference

| Behavior | Truth |
|---|---|
| Page size | Fixed **50**; no page-size flag, raise `--page`. |
| `jq` casing | Entity objects PascalCase (`.ID`); API wrappers lowercase (`.id`). |
| `add-tags` / `remove-tags` | Idempotent; `remove` doesn't error on a non-attached tag. |
| `replace-tags --tags ''` | Clears all tags. |
| `--meta` vs `edit-meta` | `--meta` = JSON object merge; `edit-meta` = single dot-path, JSON-literal value. |
| `recalculate-dimensions` | DB-only, **no** new version. `rotate`/`version-upload` = new version. |
| `versions-cleanup` | `--keep` and `--older-than-days` are alternatives (OR). |
| Resource ↔ Series | Resource belongs to ≤1 series; set via `resource edit --series-id`, clear via `series remove-resource`. |
| First text block ↔ Description | Kept in sync both ways. |
| Block `State` | `NOT NULL`; send `{}` minimum; validated per block type. |
| `note share` | **Idempotent** — returns existing token; unshare to rotate. |
| Download jobs | In-memory, lost on restart. `cancel` keeps them in queue. |
| `group clone` | Shallow (scalars + tags), not children/resources/notes. |
| `merge` (resources/groups/tags) | Moves loser relationships to winner, deletes losers. Irreversible. |
| `query` (SQL) | Read-only; writes rejected; result keys = SELECT column names. |
| MRQL `category`/`noteType`/`owner` | Numeric **IDs**, not names. |
| Errors | `HTTP <code>: <msg>` to stderr, exit 1. |
