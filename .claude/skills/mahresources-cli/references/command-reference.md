# mr CLI — Full Command Reference

Generated from `mr docs dump --format json` (authoritative: built from the live cobra command tree, examples are CI-verified doctests). Grep by command path, e.g. `resource upload`, `mrql run`, `notes add-tags`.

Every command accepts the global flags: `--server` (env `MAHRESOURCES_URL`, default `http://localhost:8181`), `--json`, `--quiet` (IDs only), `--no-header`, and `--page` (list commands, page size 50). They are omitted from each entry below.

## Command index

| Command | Summary |
|---------|---------|
| `mr resource delete` | Delete a resource by ID |
| `mr resource download` | Download a resource file |
| `mr resource edit` | Edit a resource |
| `mr resource edit-description` | Edit a resource's description |
| `mr resource edit-meta` | Edit a single metadata field by JSON path |
| `mr resource edit-name` | Edit a resource's name |
| `mr resource from-local` | Create a resource from a local server path |
| `mr resource from-url` | Create a resource from a remote URL |
| `mr resource get` | Get a resource by ID |
| `mr resource preview` | Download a scaled thumbnail of a resource |
| `mr resource recalculate-dimensions` | Recalculate resource dimensions |
| `mr resource rotate` | Rotate a resource image |
| `mr resource upload` | Upload a file as a new resource |
| `mr resource version` | Get a specific version by ID |
| `mr resource version-delete` | Delete a specific version |
| `mr resource version-download` | Download a specific version file |
| `mr resource version-restore` | Restore a resource to a previous version |
| `mr resource version-upload` | Upload a new version of a resource |
| `mr resource versions` | List versions of a resource |
| `mr resource versions-cleanup` | Clean up old versions of a resource |
| `mr resource versions-compare` | Compare two versions of a resource |
| `mr resources add-groups` | Add groups to multiple resources |
| `mr resources add-meta` | Add metadata to multiple resources |
| `mr resources add-tags` | Add tags to multiple resources |
| `mr resources delete` | Delete multiple resources |
| `mr resources list` | List resources |
| `mr resources merge` | Merge resources into a winner |
| `mr resources meta-keys` | List all unique metadata keys used across resources |
| `mr resources remove-tags` | Remove tags from multiple resources |
| `mr resources replace-tags` | Replace tags on multiple resources |
| `mr resources set-dimensions` | Set dimensions on multiple resources |
| `mr resources timeline` | Display a timeline of resource activity |
| `mr resources versions-cleanup` | Clean up old versions across resources |
| `mr series create` | Create a new series |
| `mr series delete` | Delete a series by ID |
| `mr series edit` | Edit a series |
| `mr series edit-name` | Edit a series name |
| `mr series get` | Get a series by ID |
| `mr series list` | List series |
| `mr series remove-resource` | Remove a resource from its series |
| `mr note create` | Create a new note |
| `mr note delete` | Delete a note by ID |
| `mr note edit-description` | Edit a note's description |
| `mr note edit-meta` | Edit a single metadata field by JSON path |
| `mr note edit-name` | Edit a note's name |
| `mr note get` | Get a note by ID |
| `mr note share` | Generate a share token for a note |
| `mr note unshare` | Remove the share token from a note |
| `mr notes add-groups` | Add groups to multiple notes |
| `mr notes add-meta` | Add metadata to multiple notes |
| `mr notes add-tags` | Add tags to multiple notes |
| `mr notes delete` | Delete multiple notes |
| `mr notes list` | List notes |
| `mr notes meta-keys` | List all unique metadata keys used across notes |
| `mr notes remove-tags` | Remove tags from multiple notes |
| `mr notes timeline` | Display a timeline of note activity |
| `mr note-block create` | Create a new note block |
| `mr note-block delete` | Delete a note block by ID |
| `mr note-block get` | Get a note block by ID |
| `mr note-block types` | Show available block types (text, table, calendar, etc.) |
| `mr note-block update` | Update a note block's content |
| `mr note-block update-state` | Update a note block's state |
| `mr note-blocks list` | List note blocks for a note |
| `mr note-blocks rebalance` | Rebalance note block positions |
| `mr note-blocks reorder` | Reorder note blocks |
| `mr note-type create` | Create a new note type |
| `mr note-type delete` | Delete a note type by ID |
| `mr note-type edit` | Edit a note type |
| `mr note-type edit-description` | Edit a note type's description |
| `mr note-type edit-name` | Edit a note type's name |
| `mr note-type get` | Get a note type by ID |
| `mr note-types list` | List note types |
| `mr group children` | List child groups (tree children) of a group |
| `mr group clone` | Clone a group |
| `mr group create` | Create a new group |
| `mr group delete` | Delete a group by ID |
| `mr group edit-description` | Edit a group's description |
| `mr group edit-meta` | Edit a single metadata field by JSON path |
| `mr group edit-name` | Edit a group's name |
| `mr group export` | Export one or more groups to a tar archive |
| `mr group get` | Get a group by ID |
| `mr group import` | Import a group export tar into this instance |
| `mr group parents` | List parent groups of a group |
| `mr groups add-meta` | Add metadata to multiple groups |
| `mr groups add-tags` | Add tags to multiple groups |
| `mr groups delete` | Delete multiple groups |
| `mr groups list` | List groups |
| `mr groups merge` | Merge groups into a winner |
| `mr groups meta-keys` | List all unique metadata keys used across groups |
| `mr groups remove-tags` | Remove tags from multiple groups |
| `mr groups timeline` | Display a timeline of group activity |
| `mr relation create` | Create a new group relation |
| `mr relation delete` | Delete a relation by ID |
| `mr relation edit-description` | Edit a relation's description |
| `mr relation edit-name` | Edit a relation's name |
| `mr relation-type create` | Create a new relation type |
| `mr relation-type delete` | Delete a relation type by ID |
| `mr relation-type edit` | Edit a relation type |
| `mr relation-type edit-description` | Edit a relation type's description |
| `mr relation-type edit-name` | Edit a relation type's name |
| `mr relation-types list` | List relation types |
| `mr tag create` | Create a new tag |
| `mr tag delete` | Delete a tag by ID |
| `mr tag edit-description` | Edit a tag's description |
| `mr tag edit-name` | Edit a tag's name |
| `mr tag get` | Get a tag by ID |
| `mr tags delete` | Delete multiple tags |
| `mr tags list` | List tags |
| `mr tags merge` | Merge tags into a winner |
| `mr tags timeline` | Display a timeline of tag activity |
| `mr category create` | Create a new category |
| `mr category delete` | Delete a category by ID |
| `mr category edit-description` | Edit a category's description |
| `mr category edit-name` | Edit a category's name |
| `mr category get` | Get a category by ID |
| `mr categories list` | List categories |
| `mr categories timeline` | Display a timeline of category activity |
| `mr resource-category create` | Create a new resource category |
| `mr resource-category delete` | Delete a resource category by ID |
| `mr resource-category edit-description` | Edit a resource category's description |
| `mr resource-category edit-name` | Edit a resource category's name |
| `mr resource-category get` | Get a resource category by ID |
| `mr resource-categories list` | List resource categories |
| `mr mrql delete` | Delete a saved MRQL query by ID |
| `mr mrql list` | List saved MRQL queries |
| `mr mrql run` | Run a saved MRQL query by name or ID |
| `mr mrql save` | Save a MRQL query |
| `mr query create` | Create a new query |
| `mr query delete` | Delete a query by ID |
| `mr query edit-description` | Edit a query's description |
| `mr query edit-name` | Edit a query's name |
| `mr query get` | Get a query by ID |
| `mr query run` | Run a query by ID |
| `mr query run-by-name` | Run a query by name |
| `mr query schema` | Show database table and column names for query building |
| `mr queries list` | List queries |
| `mr queries timeline` | Display a timeline of query activity |
| `mr search` | Search across all entities |
| `mr job cancel` | Cancel a job |
| `mr job pause` | Pause a job |
| `mr job resume` | Resume a job |
| `mr job retry` | Retry a failed job |
| `mr job submit` | Submit URLs for download |
| `mr jobs list` | List the download queue |
| `mr log entity` | Get log entries for a specific entity |
| `mr log get` | Get a log entry by ID |
| `mr logs list` | List log entries |
| `mr plugin disable` | Disable a plugin |
| `mr plugin enable` | Enable a plugin |
| `mr plugin purge-data` | Purge all data for a plugin |
| `mr plugin settings` | Update plugin settings (pass JSON via --data) |
| `mr plugins list` | List plugins and management info |
| `mr admin settings get` | Show a single runtime setting by key |
| `mr admin settings list` | List all runtime settings |
| `mr admin settings reset` | Remove a runtime override and revert to boot default |
| `mr admin settings set` | Override a runtime setting |
| `mr admin stats` | Show server and data statistics |
| `mr docs check-examples` | Run `# mr-doctest:` example blocks against a live server |
| `mr docs dump` | Emit the mr command tree as JSON or Markdown |
| `mr docs lint` | Validate every command's help against the template |


---

## `resource` — Upload, download, edit, or version a resource

Resources are files stored in mahresources. A Resource has a name,
content bytes, MIME type, optional dimensions, perceptual hash, and
free-form meta JSON. Resources relate many-to-many to Tags, Notes, and
Groups, and support versioned edits (see `versions`, `version-upload`).

Use the `resource` subcommands to operate on a single resource by ID:
fetch metadata, upload a file, rotate an image, or manage its version
history. Use `resources list` to discover resources matching filters.

### `mr resource delete`

Delete a resource by ID

```
mr resource delete <id>
```

Delete a resource by ID. Destructive: removes both the database row and
the stored file bytes. Deleting a nonexistent ID returns exit code 1.

**Arguments:** `<id>`

**Examples:**

```bash
# Delete a resource by ID
mr resource delete 42
# Delete and pipe the result to jq to confirm the response
mr resource delete 42 --json | jq .
```

**See also:** `mr resource get`, `mr resources list`, `mr resources delete`

### `mr resource download`

Download a resource file

```
mr resource download <id>
```

Stream a Resource's bytes to a local file. Writes to the path given by
`-o, --output`, defaulting to `resource_<id>` in the current directory.
The file content is streamed as-is from the server; no conversion is
performed.

**Arguments:** `<id>`

**Flags:**

- `--output` (string) — Output file path (default: resource_<id>)

**Examples:**

```bash
# Download to an explicit path
mr resource download 42 -o ./out.jpg
# Download to the default path (resource_42)
mr resource download 42
```

**See also:** `mr resource get`, `mr resource preview`, `mr resource version-download`

### `mr resource edit`

Edit a resource

```
mr resource edit <id>
```

Edit fields on an existing resource. Any flag left unset keeps the
existing value (partial update). Collection flags (`--tags`, `--groups`,
`--notes`) take comma-separated ID lists and replace the current set;
`--meta` takes a JSON string merged onto existing meta.

**Arguments:** `<id>`

**Flags:**

- `--name` (string) — Resource name
- `--description` (string) — Resource description
- `--tags` (string) — Comma-separated tag IDs
- `--groups` (string) — Comma-separated group IDs
- `--notes` (string) — Comma-separated note IDs
- `--owner-id` (uint) — Owner group ID
- `--meta` (string) — Meta JSON string
- `--category` (string) — Category
- `--resource-category-id` (uint) — Resource category ID
- `--original-name` (string) — Original file name
- `--original-location` (string) — Original file location
- `--width` (uint) — Width in pixels
- `--height` (uint) — Height in pixels
- `--series-id` (uint) — Series ID

**Examples:**

```bash
# Rename and update the description
mr resource edit 42 --name "renamed" --description "new description"
# Attach tags 5 and 7
mr resource edit 42 --tags 5,7
```

**See also:** `mr resource get`, `mr resource upload`, `mr resource versions`

### `mr resource edit-description`

Edit a resource's description

```
mr resource edit-description <id> <new-description>
```

Update only the description of an existing resource. Passing an empty
string clears the description. Shorthand for `mr resource edit <id> --description <value>`.

**Arguments:** `<id> <new-description>`

**Examples:**

```bash
# Set the description on resource 42
mr resource edit-description 42 "scanned contract, Q1 2026"
# Clear the description by passing an empty string
mr resource edit-description 42 ""
```

**See also:** `mr resource edit`, `mr resource edit-name`, `mr resource edit-meta`

### `mr resource edit-meta`

Edit a single metadata field by JSON path

```
mr resource edit-meta <id> <path> <value>
```

Edit a single metadata field at a dot-separated JSON path. Takes three
positional arguments: the resource ID, the path (e.g., `address.city`),
and a JSON literal value (e.g., `'"Berlin"'`, `42`, `'{"nested":"obj"}'`,
`'[1,2,3]'`). Creates intermediate path segments as needed and leaves
sibling keys at each level untouched.

**Arguments:** `<id> <path> <value>`

**Examples:**

```bash
# Set a top-level string field (note: shell-quoted JSON string)
mr resource edit-meta 5 status '"active"'
# Set a nested numeric field (creates address.postalCode if missing)
mr resource edit-meta 5 address.postalCode 10115
```

**See also:** `mr resource edit`, `mr resources add-meta`, `mr resources meta-keys`

### `mr resource edit-name`

Edit a resource's name

```
mr resource edit-name <id> <new-name>
```

Update only the name of an existing resource. Shorthand for
`mr resource edit <id> --name <value>` when name is the only change.

**Arguments:** `<id> <new-name>`

**Examples:**

```bash
# Rename resource 42
mr resource edit-name 42 "my new name"
# Rename and confirm with a follow-up get
mr resource edit-name 42 "renamed" && mr resource get 42 --json | jq -r .Name
```

**See also:** `mr resource edit`, `mr resource edit-description`, `mr resource edit-meta`

### `mr resource from-local`

Create a resource from a local server path

```
mr resource from-local
```

Create a Resource from a file already present on the server's filesystem.
Differs from `upload` (which streams bytes over HTTP) in that the server
reads the file in place. The `--path` flag is required and must resolve
on the target server. Useful for bulk-importing existing files or
deploying pre-staged assets.

**Flags:**

- `--path` (string) **(required)** — Local server path (required)
- `--name` (string) — Resource name
- `--description` (string) — Resource description
- `--tags` (string) — Comma-separated tag IDs
- `--groups` (string) — Comma-separated group IDs
- `--owner-id` (uint) — Owner group ID
- `--meta` (string) — Meta JSON string

**Examples:**

```bash
# Create from a server-local path
mr resource from-local --path /var/mahresources/incoming/photo.jpg
# With metadata
mr resource from-local --path /srv/imports/doc.pdf --name "Doc" --tags 3,7
```

**Output:** Resource object with id

**See also:** `mr resource upload`, `mr resource from-url`

### `mr resource from-url`

Create a resource from a remote URL

```
mr resource from-url
```

Create a Resource by having the server fetch a remote URL. Useful when
you have a public asset that shouldn't be proxied through your local
machine. The `--url` flag is required; the server downloads, stores, and
indexes the file. Optional `--tags` / `--groups` attach relationships at
creation.

**Flags:**

- `--url` (string) **(required)** — Remote URL (required)
- `--name` (string) — Resource name
- `--description` (string) — Resource description
- `--tags` (string) — Comma-separated tag IDs
- `--groups` (string) — Comma-separated group IDs
- `--owner-id` (uint) — Owner group ID
- `--meta` (string) — Meta JSON string
- `--file-name` (string) — Override file name

**Examples:**

```bash
# Create from a URL
mr resource from-url --url https://example.com/photo.jpg
# With metadata and groups
mr resource from-url --url https://example.com/doc.pdf --name "Paper" --meta '{"source":"arxiv"}' --groups 5
```

**Output:** Resource object with id

**See also:** `mr resource upload`, `mr resource from-local`

### `mr resource get`

Get a resource by ID

```
mr resource get <id>
```

Get a resource by ID and print its metadata. Fetches the full record
including tags, groups, resource category, owner, dimensions, hash,
and any custom meta JSON. Output is a key/value table by default; pass
the global `--json` flag to get the full record for scripting.

**Arguments:** `<id>`

**Examples:**

```bash
# Get a resource by ID (table output)
mr resource get 42
# Get as JSON and extract a single field with jq
mr resource get 42 --json | jq -r .Name
```

**Output:** Resource object with id (uint), name (string), tags ([]Tag), groups ([]Group), meta (object)

**See also:** `mr resource edit`, `mr resource versions`, `mr resource download`

### `mr resource preview`

Download a scaled thumbnail of a resource

```
mr resource preview <id>
```

Download a server-rendered thumbnail preview of a Resource. Width and
height can be capped via `-w, --width` and `--height`; without caps the
server returns its default preview size. Not every content type supports
previews (e.g., some binary formats or failed decodes).

**Arguments:** `<id>`

**Flags:**

- `--output` (string) — Output file path (default: preview_<id>)
- `--width` (uint) — Preview width
- `--height` (uint) — Preview height

**Examples:**

```bash
# Default preview
mr resource preview 42 -o preview.jpg
# Constrained to 256x256 max
mr resource preview 42 -o preview.jpg -w 256 --height 256
```

**See also:** `mr resource download`, `mr resource recalculate-dimensions`

### `mr resource recalculate-dimensions`

Recalculate resource dimensions

```
mr resource recalculate-dimensions <id>
```

Re-read an image Resource's bytes and update its stored width and
height. Useful after external file edits or when the original ingest
path failed to decode dimensions. Does not modify the file content
itself; only updates the database record.

**Arguments:** `<id>`

**Examples:**

```bash
# Recalculate dimensions for a single resource
mr resource recalculate-dimensions 42
# Pipe from a list query to bulk-recalculate
mr resources list --content-type image/jpeg --json | jq -r '.[].id' | xargs -I {} mr resource recalculate-dimensions {}
```

**See also:** `mr resource get`, `mr resource rotate`, `mr resources set-dimensions`

### `mr resource rotate`

Rotate a resource image

```
mr resource rotate <id>
```

Rotate an image Resource by the given number of degrees. Only image
Resources are supported; the rotation creates a new version on success
so the original is preserved. The `--degrees` flag is required and
typically takes 90, 180, or 270 (negative values rotate counter-
clockwise).

**Arguments:** `<id>`

**Flags:**

- `--degrees` (int) **(required)** — Rotation degrees (required)

**Examples:**

```bash
# Rotate 90 degrees clockwise
mr resource rotate 42 --degrees 90
# Rotate 180 degrees
mr resource rotate 42 --degrees 180
```

**See also:** `mr resource preview`, `mr resource edit`, `mr resource versions`

### `mr resource upload`

Upload a file as a new resource

```
mr resource upload <file>
```

Upload a local file as a new Resource. Sends the file via multipart form
to `POST /v1/resource`. The Resource's name defaults to the source
filename if `--name` is not set. Use `--meta` for a JSON blob of custom
metadata that is merged into the new record.

**Arguments:** `<file>`

**Flags:**

- `--name` (string) — Resource name
- `--description` (string) — Resource description
- `--owner-id` (uint) — Owner group ID
- `--meta` (string) — Meta JSON string
- `--category` (string) — Category
- `--content-category` (string) — Content category
- `--resource-category-id` (uint) — Resource category ID
- `--original-name` (string) — Original file name

**Examples:**

```bash
# Basic upload (name defaults to the filename)
mr resource upload ./photo.jpg
# Upload with ownership and meta JSON
mr resource upload ./photo.jpg --owner-id 3 --meta '{"camera":"Pixel"}'
```

**Output:** Resource object with id, name

**See also:** `mr resource edit`, `mr resource from-url`, `mr resource from-local`, `mr resources list`

### `mr resource version`

Get a specific version by ID

```
mr resource version <version-id>
```

Fetch metadata for a single version by its version ID. Returns the same
fields as `versions` but as a single key/value record. Useful when you
know the version ID and need its size or comment without a list call.

**Arguments:** `<version-id>`

**Examples:**

```bash
# Fetch a version by ID
mr resource version 17
# Extract size via jq
mr resource version 17 --json | jq -r .size
```

**Output:** Version object with id, number, size, type, comment, created

**See also:** `mr resource versions`, `mr resource version-download`, `mr resource version-restore`

### `mr resource version-delete`

Delete a specific version

```
mr resource version-delete
```

Delete a specific version by ID. The parent Resource is untouched. Both
`--resource-id` and `--version-id` are required. Fails if deleting would
leave the Resource with zero versions.

**Flags:**

- `--resource-id` (uint) **(required)** — Resource ID (required)
- `--version-id` (uint) **(required)** — Version ID (required)

**Examples:**

```bash
# Delete an old version
mr resource version-delete --resource-id 42 --version-id 17
# Pipe a list of old version IDs
mr resource versions 42 --json | jq -r '.[1:][].id' | xargs -I {} mr resource version-delete --resource-id 42 --version-id {}
```

**See also:** `mr resource versions`, `mr resource versions-cleanup`

### `mr resource version-download`

Download a specific version file

```
mr resource version-download <version-id>
```

Stream a specific version's bytes to a local file. Use `resource
download` to fetch the current version; this command exists to retrieve
older versions by their version ID. Output path defaults to
`version_<id>` if `-o` is not given.

**Arguments:** `<version-id>`

**Flags:**

- `--output` (string) — Output file path (default: version_<id>)

**Examples:**

```bash
# Download a version to an explicit path
mr resource version-download 17 -o old.jpg
# Default output path
mr resource version-download 17
```

**See also:** `mr resource download`, `mr resource versions`, `mr resource version`

### `mr resource version-restore`

Restore a resource to a previous version

```
mr resource version-restore
```

Restore a previous version to be the current version of a Resource.
Creates a new version that is a copy of the target (the original target
version is preserved). Both `--resource-id` and `--version-id` are
required. The optional `--comment` annotates the restore for the audit
trail.

**Flags:**

- `--resource-id` (uint) **(required)** — Resource ID (required)
- `--version-id` (uint) **(required)** — Version ID (required)
- `--comment` (string) — Restore comment

**Examples:**

```bash
# Restore with a comment
mr resource version-restore --resource-id 42 --version-id 17 --comment "revert bad edit"
# Silent restore
mr resource version-restore --resource-id 42 --version-id 17
```

**See also:** `mr resource versions`, `mr resource version`, `mr resource version-upload`

### `mr resource version-upload`

Upload a new version of a resource

```
mr resource version-upload <resource-id> <file>
```

Push a new version of an existing Resource. The new bytes replace the
current version pointer; previous versions remain accessible via their
version IDs. The `--comment` flag attaches a free-form note (useful for
"rotated 90°" or "rescanned" audit trails).

**Arguments:** `<resource-id> <file>`

**Flags:**

- `--comment` (string) — Version comment

**Examples:**

```bash
# Upload a new version
mr resource version-upload 42 ./photo_v2.jpg
# With a comment
mr resource version-upload 42 ./photo_v2.jpg --comment "color corrected"
```

**See also:** `mr resource versions`, `mr resource version`, `mr resource version-restore`

### `mr resource versions`

List versions of a resource

```
mr resource versions <resource-id>
```

List every stored version of a Resource, newest first. Columns are the
version ID, version number, size in bytes, content type, an optional
author comment, and the creation timestamp. Pass the global `--json`
flag to get the full records for scripting.

**Arguments:** `<resource-id>`

**Examples:**

```bash
# List versions (table)
mr resource versions 42
# Get the newest version's ID via jq
mr resource versions 42 --json | jq -r '.[0].id'
```

**Output:** Array of version objects with id, number, size, type, comment, created

**See also:** `mr resource version`, `mr resource version-upload`, `mr resource versions-compare`

### `mr resource versions-cleanup`

Clean up old versions of a resource

```
mr resource versions-cleanup <resource-id>
```

Bulk-delete old versions of a single Resource. Retains either the N most
recent versions (`--keep`) or deletes versions older than N days
(`--older-than-days`). Pass `--dry-run` to preview without deleting.

**Arguments:** `<resource-id>`

**Flags:**

- `--keep` (uint) — Number of versions to keep
- `--older-than-days` (uint) — Delete versions older than N days
- `--dry-run` (bool) — Preview without deleting

**Examples:**

```bash
# Keep only the last 3 versions
mr resource versions-cleanup 42 --keep 3
# Delete versions older than 90 days (preview)
mr resource versions-cleanup 42 --older-than-days 90 --dry-run
```

**See also:** `mr resource versions`, `mr resource version-delete`, `mr resources versions-cleanup`

### `mr resource versions-compare`

Compare two versions of a resource

```
mr resource versions-compare <resource-id>
```

Compare two versions of a Resource and report the size delta, whether
the content hashes match, whether the content types match, and the
dimension differences. Both `--v1` and `--v2` are required and must be
version IDs of the same Resource.

**Arguments:** `<resource-id>`

**Flags:**

- `--v1` (uint) **(required)** — First version ID (required)
- `--v2` (uint) **(required)** — Second version ID (required)

**Examples:**

```bash
# Compare two versions (table)
mr resource versions-compare 42 --v1 17 --v2 21
# Extract sameHash via jq
mr resource versions-compare 42 --v1 17 --v2 21 --json | jq -r .sameHash
```

**Output:** Comparison object with sizeDelta, sameHash, sameType, dimensionsDiff

**See also:** `mr resource versions`, `mr resource version`, `mr resource versions-cleanup`


---

## `resources` — List, merge, or bulk-edit resources

Discover and bulk-mutate Resources. The `resources` subcommands operate
on multiple Resources at once: `list` for filtered queries (with
pagination via global `--page`), `add-tags` / `remove-tags` /
`replace-tags` for bulk tag ops, `add-groups` / `add-meta` for bulk
annotation, and `delete` / `merge` for destructive operations.

Most bulk-mutation commands select targets via `--ids=<csv>`; `merge`
uses `--winner` / `--losers` instead. The current CLI does not support
MRQL selectors on bulk commands — pipe from `resources list --json | jq`
to extract IDs when you need query-based selection.

### `mr resources add-groups`

Add groups to multiple resources

```
mr resources add-groups
```

Add group IDs to every Resource listed in `--ids`. Idempotent. Both
`--ids` and `--groups` accept comma-separated unsigned integer lists
and are required.

**Flags:**

- `--ids` (string) **(required)** — Comma-separated resource IDs (required)
- `--groups` (string) **(required)** — Comma-separated group IDs (required)

**Examples:**

```bash
# Add groups 2 and 3 to resources 1
mr resources add-groups --ids 1,2 --groups 2,3
# Bulk from a list query
mr resources list --content-type image/jpeg --json | jq -r 'map(.id) | join(",")' | xargs -I {} mr resources add-groups --ids {} --groups 7
```

**See also:** `mr resources add-tags`, `mr resources add-meta`, `mr groups list`

### `mr resources add-meta`

Add metadata to multiple resources

```
mr resources add-meta
```

Add metadata keys to every Resource listed in `--ids` by passing a JSON
string via `--meta`. The server-side endpoint at
`POST /v1/resources/addMeta` determines whether this merges on top of
existing meta or replaces it — see the admin interface docs for exact
semantics. For single-resource single-key edits, use
`resource edit-meta` (dot-path syntax).

**Flags:**

- `--ids` (string) **(required)** — Comma-separated resource IDs (required)
- `--meta` (string) **(required)** — Meta JSON string (required)

**Examples:**

```bash
# Set a single key on multiple resources
mr resources add-meta --ids 1,2,3 --meta '{"status":"reviewed"}'
# Set multiple keys at once (JSON object)
mr resources add-meta --ids 1,2 --meta '{"priority":5,"owner":"alice"}'
```

**See also:** `mr resource edit-meta`, `mr resources meta-keys`, `mr resources add-tags`

### `mr resources add-tags`

Add tags to multiple resources

```
mr resources add-tags
```

Add tag IDs to every Resource listed in `--ids`. Idempotent: adding a
tag that's already attached is a no-op. Both `--ids` and `--tags`
accept comma-separated unsigned integer lists and are required.

**Flags:**

- `--ids` (string) **(required)** — Comma-separated resource IDs (required)
- `--tags` (string) **(required)** — Comma-separated tag IDs (required)

**Examples:**

```bash
# Add tag 5 to resources 1
mr resources add-tags --ids 1,2,3 --tags 5
# Add multiple tags at once
mr resources add-tags --ids 1,2,3 --tags 5,6,7
```

**See also:** `mr resources remove-tags`, `mr resources replace-tags`, `mr tags list`

### `mr resources delete`

Delete multiple resources

```
mr resources delete
```

Bulk-delete Resources. Destructive: removes both the database rows and
the stored file bytes. Target Resources are selected via `--ids` (CSV
of unsigned ints). The current CLI has no dry-run; pipe
`resources list --json` first if you need to preview targets.

**Flags:**

- `--ids` (string) **(required)** — Comma-separated resource IDs to delete (required)

**Examples:**

```bash
# Delete specific resources
mr resources delete --ids 42,43,44
# Delete the output of a filter query
mr resources list --tags 7 --json | jq -r 'map(.id) | join(",")' | xargs -I {} mr resources delete --ids {}
```

**See also:** `mr resource delete`, `mr resources merge`, `mr resources list`

### `mr resources list`

List resources

```
mr resources list
```

List Resources, optionally filtered. Filter flags combine with AND.
Comma-separated ID lists on `--tags`, `--groups`, `--notes` use the
`?Add` query parameter to match any of the given IDs. Date flags
(`--created-before`, `--created-after`) expect `YYYY-MM-DD`. Sort with
`--sort-by=field1,-field2` (prefix with `-` for descending). Pagination
via the global `--page` flag (default page size 50).

**Flags:**

- `--name` (string) — Filter by name
- `--description` (string) — Filter by description
- `--content-type` (string) — Filter by content type
- `--owner-id` (uint) — Filter by owner group ID
- `--tags` (string) — Comma-separated tag IDs to filter by
- `--groups` (string) — Comma-separated group IDs to filter by
- `--notes` (string) — Comma-separated note IDs to filter by
- `--resource-category-id` (uint) — Filter by resource category ID
- `--created-before` (string) — Filter by creation date (before)
- `--created-after` (string) — Filter by creation date (after)
- `--min-width` (uint) — Minimum width
- `--min-height` (uint) — Minimum height
- `--max-width` (uint) — Maximum width
- `--max-height` (uint) — Maximum height
- `--hash` (string) — Filter by hash
- `--original-name` (string) — Filter by original name
- `--sort-by` (string) — Comma-separated sort fields

**Examples:**

```bash
# List all resources (paged)
mr resources list
# Filter by content type
mr resources list --content-type image/jpeg
# Filter by tag + date
mr resources list --tags 5 --created-after 2026-01-01 --json | jq -r '.[].Name'
```

**Output:** Array of resources with id, name, content type, size, dimensions, owner id, created

**See also:** `mr resource get`, `mr groups list`, `mr mrql`

### `mr resources merge`

Merge resources into a winner

```
mr resources merge
```

Merge one or more "loser" Resources into a single "winner". The
winner's bytes and ID are preserved; tags, groups, notes, and relations
from the losers are moved onto the winner; the loser records and their
file bytes are then deleted. Use to consolidate duplicates after
perceptual-hash detection or manual review.

**Flags:**

- `--winner` (uint) **(required)** — Winning resource ID (required)
- `--losers` (string) **(required)** — Comma-separated loser resource IDs (required)

**Examples:**

```bash
# Merge resources 2 and 3 into winner 1
mr resources merge --winner 1 --losers 2,3
# Pipe duplicate IDs from a search
mr resources merge --winner 1 --losers $(mr resources list --hash abcd1234 --json | jq -r 'map(.id) | join(",")')
```

**See also:** `mr resource get`, `mr resources delete`, `mr search`

### `mr resources meta-keys`

List all unique metadata keys used across resources

```
mr resources meta-keys
```

List every distinct `meta` key observed across the entire Resource
corpus. Useful for discovering the vocabulary of an evolving meta
schema. The command has no filter flags in the current CLI; pair it
with client-side `jq` filtering if you only want a subset of keys.

**Examples:**

```bash
# List all meta keys
mr resources meta-keys
# Filter client-side with jq
mr resources meta-keys --json | jq '.[] | select(startswith("image_"))'
```

**Output:** Array of distinct meta key strings across the entire resource corpus

**See also:** `mr resource edit-meta`, `mr resources add-meta`

### `mr resources remove-tags`

Remove tags from multiple resources

```
mr resources remove-tags
```

Remove tag IDs from every Resource listed in `--ids`. Idempotent:
removing a tag that isn't attached is a no-op. Both `--ids` and `--tags`
accept comma-separated unsigned integer lists and are required.

**Flags:**

- `--ids` (string) **(required)** — Comma-separated resource IDs (required)
- `--tags` (string) **(required)** — Comma-separated tag IDs (required)

**Examples:**

```bash
# Remove tag 5 from resources 1
mr resources remove-tags --ids 1,2 --tags 5
# Remove multiple tags at once
mr resources remove-tags --ids 1,2,3 --tags 5,6
```

**See also:** `mr resources add-tags`, `mr resources replace-tags`, `mr tags list`

### `mr resources replace-tags`

Replace tags on multiple resources

```
mr resources replace-tags
```

Set the exact tag set on every Resource listed in `--ids` to the tags
in `--tags`. Any tag not in the list is removed; any tag in the list is
added. Use when you want exact-state semantics rather than delta
semantics. Pass `--tags ""` to clear all tags.

**Flags:**

- `--ids` (string) **(required)** — Comma-separated resource IDs (required)
- `--tags` (string) **(required)** — Comma-separated tag IDs (required)

**Examples:**

```bash
# Replace tags with exactly [5, 7]
mr resources replace-tags --ids 1 --tags 5,7
# Clear all tags from a resource
mr resources replace-tags --ids 1 --tags ""
```

**See also:** `mr resources add-tags`, `mr resources remove-tags`, `mr tags list`

### `mr resources set-dimensions`

Set dimensions on multiple resources

```
mr resources set-dimensions
```

Force the stored `width` and `height` on every Resource listed in
`--ids`. Useful when `recalculate-dimensions` cannot decode the file
format (e.g., proprietary formats) or when the stored dimensions are
known to be stale. Does not transform the file bytes; only updates the
database record. All three flags (`--ids`, `--width`, `--height`) are
required.

**Flags:**

- `--ids` (string) **(required)** — Comma-separated resource IDs (required)
- `--width` (uint) **(required)** — Width in pixels (required)
- `--height` (uint) **(required)** — Height in pixels (required)

**Examples:**

```bash
# Set dimensions on a single resource
mr resources set-dimensions --ids 7 --width 1920 --height 1080
# Batch update from a tag filter
IDS=$(mr resources list --tags 5 --json | jq -r 'map(.id) | join(",")')
mr resources set-dimensions --ids $IDS --width 800 --height 600
```

**See also:** `mr resource rotate`, `mr resource recalculate-dimensions`

### `mr resources timeline`

Display a timeline of resource activity

```
mr resources timeline
```

Display a timeline of Resource activity as an ASCII bar chart. Each
bar represents a time bucket (yearly, monthly, or weekly, controlled by
`--granularity`), and the bar height reflects the count of Resources
created in that bucket.

The chart is anchored at the `--anchor` date (default: today) and shows
`--columns` buckets backward from the anchor (default 15, max 60). All
resource-list filter flags (`--name`, `--tags`, `--groups`, etc.) apply
the same way to the timeline aggregation. Pass the global `--json` flag
to get the raw bucket data for scripting.

**Flags:**

- `--granularity` (string) (default `monthly`) — Bucket granularity: yearly, monthly, or weekly
- `--anchor` (string) — Anchor date (YYYY-MM-DD); defaults to today
- `--columns` (int) (default `15`) — Number of timeline buckets (max 60)
- `--name` (string) — Filter by name
- `--description` (string) — Filter by description
- `--content-type` (string) — Filter by content type
- `--owner-id` (uint) — Filter by owner group ID
- `--tags` (string) — Comma-separated tag IDs to filter by
- `--groups` (string) — Comma-separated group IDs to filter by
- `--notes` (string) — Comma-separated note IDs to filter by
- `--resource-category-id` (uint) — Filter by resource category ID
- `--created-before` (string) — Filter by creation date (before)
- `--created-after` (string) — Filter by creation date (after)
- `--min-width` (uint) — Minimum width
- `--min-height` (uint) — Minimum height
- `--max-width` (uint) — Maximum width
- `--max-height` (uint) — Maximum height
- `--hash` (string) — Filter by hash
- `--original-name` (string) — Filter by original name

**Examples:**

```bash
# Monthly timeline anchored at today (default)
mr resources timeline
# Weekly granularity
mr resources timeline --granularity weekly --columns 12
# Yearly timeline filtered by tag
mr resources timeline --granularity yearly --tags 5 --json
```

**See also:** `mr resources list`, `mr groups timeline`

### `mr resources versions-cleanup`

Clean up old versions across resources

```
mr resources versions-cleanup
```

Bulk-clean old Resource versions across the entire corpus. Applies the
same retention rules as the singular `resource versions-cleanup`:
`--keep N` retains the N most recent versions per resource;
`--older-than-days N` removes versions older than N days. Both filters
may be combined. Scope the operation to a single owner group with
`--owner-id`. Pass `--dry-run` to preview the count of versions that
would be removed without committing any deletes.

**Flags:**

- `--keep` (uint) — Number of versions to keep
- `--older-than-days` (uint) — Delete versions older than N days
- `--owner-id` (uint) — Filter by owner group ID
- `--dry-run` (bool) — Preview without deleting

**Examples:**

```bash
# Keep last 3 versions across all resources
mr resources versions-cleanup --keep 3
# Preview cleanup of versions older than 90 days
mr resources versions-cleanup --older-than-days 90 --owner-id 5 --dry-run
# Remove all but the latest version across the entire corpus
mr resources versions-cleanup --keep 1
```

**See also:** `mr resource versions-cleanup`, `mr resource versions`, `mr resources list`


---

## `series` — Manage resource series (list, create, edit, delete)

A Series is an ordered collection of Resources, typically used for content
that has an intrinsic sequence: a volume of a manga, a photo shoot, the
chapters of a scanned document. A Resource may belong to at most one
Series via its `SeriesId` reference, and removing that reference detaches
the Resource from the Series without deleting either.

Use the `series` subcommands to manage a series by ID: fetch it, create
a new one, rename or fully edit it, delete it, remove a resource from
its series, or list series matching filters. Series membership is
assigned on the Resource side (see `resource edit --series-id`), so to
attach a resource to a series edit the resource.

### `mr series create`

Create a new series

```
mr series create
```

Create a new series. `--name` is required. The server derives the slug
from the name at creation time; the slug never changes when the name is
later edited, so pick a name with care. On success prints a confirmation
line with the new ID; pass the global `--json` flag to emit the full
record for scripting (e.g., piping the new ID into follow-up commands).

**Flags:**

- `--name` (string) **(required)** — Series name (required)

**Examples:**

```bash
# Create a series with just a name
mr series create --name "spring-2026-photos"
# Create and capture the new ID via jq
ID=$(mr series create --name "volume-1" --json | jq -r .ID)
```

**Output:** Created Series object with ID (uint), Name (string), Slug (string), Meta (object), CreatedAt, UpdatedAt

**See also:** `mr series get`, `mr series edit-name`, `mr series list`

### `mr series delete`

Delete a series by ID

```
mr series delete <id>
```

Delete a series by ID. Destructive: removes the series row. Resources
previously attached to the series keep their bytes but have their
`SeriesId` cleared (the foreign key uses `ON DELETE SET NULL`). Deleting
a nonexistent ID returns exit code 1.

**Arguments:** `<id>`

**Examples:**

```bash
# Delete a series by ID
mr series delete 42
# Delete and pipe the result to jq to confirm the response shape
mr series delete 42 --json | jq .
```

**See also:** `mr series list`, `mr series get`, `mr series create`

### `mr series edit`

Edit a series

```
mr series edit <id>
```

Edit a series. `--name` is required on every call; `--meta` is optional
and takes a JSON string merged into the series meta. The slug is derived
from the original name at creation time and is not updated by this
command, so changing the name here leaves the slug untouched.

**Arguments:** `<id>`

**Flags:**

- `--name` (string) **(required)** — Series name (required)
- `--meta` (string) — Series metadata as JSON

**Examples:**

```bash
# Rename a series and set meta in one call
mr series edit 42 --name "volume-1-final" --meta '{"season":"fall"}'
# Rename only (meta unchanged)
mr series edit 42 --name "renamed"
```

**See also:** `mr series edit-name`, `mr series get`, `mr series list`

### `mr series edit-name`

Edit a series name

```
mr series edit-name <id> <new-name>
```

Update only the name of an existing series. Shorthand for `mr series
edit <id> --name <value>` when the name is the only change. Takes two
positional arguments: the series ID and the new name. The slug is
derived from the original name at creation time and is not changed by
this command.

**Arguments:** `<id> <new-name>`

**Examples:**

```bash
# Rename series 42
mr series edit-name 42 "volume-1-final"
# Rename and confirm with a follow-up get
mr series edit-name 42 "renamed" && mr series get 42 --json | jq -r .Name
```

**See also:** `mr series edit`, `mr series get`, `mr series list`

### `mr series get`

Get a series by ID

```
mr series get <id>
```

Get a series by ID and print its fields. Fetches the full record
including the slug, meta JSON, and the list of resources currently
attached to the series. Output is a key/value table by default; pass the
global `--json` flag to emit the raw record for scripting.

**Arguments:** `<id>`

**Examples:**

```bash
# Get a series by ID (table output)
mr series get 42
# Get as JSON and extract the name with jq
mr series get 42 --json | jq -r .Name
```

**Output:** Series object with ID (uint), Name (string), Slug (string), Meta (object), Resources ([]Resource), CreatedAt, UpdatedAt

**See also:** `mr series list`, `mr series edit`, `mr series delete`

### `mr series list`

List series

```
mr series list
```

List Series, optionally filtered by name or slug. The `--name` and
`--slug` flags do substring matching on the server. Results are
paginated via the global `--page` flag (default page size 50). Default
output is a table with ID, NAME, SLUG, and CREATED columns; pass
`--json` for the full array.

**Flags:**

- `--name` (string) — Filter by name
- `--slug` (string) — Filter by slug

**Examples:**

```bash
# List all series (first page)
mr series list
# Filter by name substring
mr series list --name volume
# JSON output piped into jq
mr series list --json | jq -r '.[].Name'
```

**Output:** Array of Series objects with ID, Name, Slug, Meta, CreatedAt, UpdatedAt

**See also:** `mr series get`, `mr series create`, `mr resources list`

### `mr series remove-resource`

Remove a resource from its series

```
mr series remove-resource <resource-id>
```

Remove a resource from its series. Takes the resource ID as a single
positional argument and clears the resource's `SeriesId`; the series
itself and the resource's bytes are preserved. To move a resource to a
different series instead of detaching it, use `resource edit
--series-id` on the resource.

**Arguments:** `<resource-id>`

**Examples:**

```bash
# Detach resource 123 from whatever series it belongs to
mr series remove-resource 123
# Detach and confirm by inspecting the resource's seriesId
mr series remove-resource 123 && mr resource get 123 --json | jq .seriesId
```

**See also:** `mr resource edit`, `mr series get`, `mr resources list`


---

## `note` — Get, create, edit, delete, or share a note

Notes are free-form text records in mahresources. A Note has a name,
description, optional meta JSON, an optional owner group, an optional
note type (template), optional start/end dates, and many-to-many links
to Tags, Resources, and Groups. A Note may also carry a share token
that exposes it at `/s/<token>` for read-only public access.

Use the `note` subcommands to operate on a single note by ID: fetch the
full record, create a new one, edit the name/description/meta fields,
toggle sharing, or delete it. Use `notes list` to discover notes
matching filters, or the bulk subcommands under `notes` to mutate many
at once.

### `mr note create`

Create a new note

```
mr note create
```

Create a new Note. Only `--name` is required; every other field is
optional. Use `--tags`, `--groups`, and `--resources` (comma-separated
unsigned integer IDs) to link the new Note to existing entities at
creation time. Use `--meta` to attach free-form JSON metadata, and
`--owner-id` / `--note-type-id` to set the owner group and note type
respectively. The created record is returned; capture `.ID` from JSON
output for use in follow-up commands.

**Flags:**

- `--name` (string) **(required)** — Note name (required)
- `--description` (string) — Note description
- `--tags` (string) — Comma-separated tag IDs
- `--groups` (string) — Comma-separated group IDs
- `--resources` (string) — Comma-separated resource IDs
- `--meta` (string) — Meta JSON string
- `--owner-id` (uint) — Owner group ID
- `--note-type-id` (uint) — Note type ID

**Examples:**

```bash
# Create a minimal note
mr note create --name "shopping list"
# Create with description
mr note create --name "meeting-notes" --description "Q2 planning" --tags 5,6 --owner-id 42
```

**Output:** Created Note object with ID (uint), Name (string), Description (string), Meta (object), Tags ([]Tag), Groups ([]Group), Resources ([]Resource)

**See also:** `mr note get`, `mr note edit-name`, `mr note edit-meta`, `mr notes list`

### `mr note delete`

Delete a note by ID

```
mr note delete <id>
```

Delete a note by ID. Destructive: removes the database row and all of
its tag/group/resource associations. Deleting a nonexistent ID returns
exit code 1 with an HTTP 404 error message.

**Arguments:** `<id>`

**Examples:**

```bash
# Delete a note by ID
mr note delete 42
# Delete and pipe the response to jq to confirm
mr note delete 42 --json | jq .
```

**See also:** `mr note get`, `mr notes delete`, `mr notes list`

### `mr note edit-description`

Edit a note's description

```
mr note edit-description <id> <new-description>
```

Update only the description of an existing note. Takes two positional
arguments: the note ID and the new description. Passing an empty
string clears the description.

**Arguments:** `<id> <new-description>`

**Examples:**

```bash
# Set the description on note 42
mr note edit-description 42 "raw brainstorm, needs polish"
# Clear the description by passing an empty string
mr note edit-description 42 ""
```

**See also:** `mr note edit-name`, `mr note edit-meta`, `mr note get`

### `mr note edit-meta`

Edit a single metadata field by JSON path

```
mr note edit-meta <id> <path> <value>
```

Edit a single metadata field at a dot-separated JSON path. Takes three
positional arguments: the note ID, the path (e.g., `address.city`),
and a JSON literal value (e.g., `'"Berlin"'`, `42`, `'{"nested":"obj"}'`,
`'[1,2,3]'`). Creates intermediate path segments as needed and leaves
sibling keys at each level untouched.

**Arguments:** `<id> <path> <value>`

**Examples:**

```bash
# Set a top-level string field (note: shell-quoted JSON string)
mr note edit-meta 5 status '"active"'
# Set a nested numeric field (creates address.postalCode if missing)
mr note edit-meta 5 address.postalCode 10115
```

**See also:** `mr note get`, `mr notes add-meta`, `mr notes meta-keys`

### `mr note edit-name`

Edit a note's name

```
mr note edit-name <id> <new-name>
```

Update only the name of an existing note. Takes two positional
arguments: the note ID and the new name. Use this when renaming is the
only change; for multi-field edits, prefer a single request via the
server API.

**Arguments:** `<id> <new-name>`

**Examples:**

```bash
# Rename note 42
mr note edit-name 42 "renamed title"
# Rename and confirm with a follow-up get
mr note edit-name 42 "final draft" && mr note get 42 --json | jq -r .Name
```

**See also:** `mr note edit-description`, `mr note edit-meta`, `mr note get`

### `mr note get`

Get a note by ID

```
mr note get <id>
```

Get a note by ID and print its metadata. Fetches the full record
including name, description, meta JSON, attached tags/groups/resources,
owner group, note type, optional start/end dates, and the share token
(when the note is currently shared). Output is a key/value table by
default; pass the global `--json` flag to get the full record for
scripting.

**Arguments:** `<id>`

**Examples:**

```bash
# Get a note by ID (table output)
mr note get 42
# Get as JSON and extract the name with jq
mr note get 42 --json | jq -r .Name
```

**Output:** Note object with ID (uint), Name (string), Description (string), Meta (object), Tags ([]Tag), Groups ([]Group), Resources ([]Resource), OwnerId (*uint), NoteTypeId (*uint), shareToken (*string, omitempty)

**See also:** `mr note create`, `mr note edit-name`, `mr notes list`

### `mr note share`

Generate a share token for a note

```
mr note share <id>
```

Generate a share token for a note, making it readable via the public
`/s/<token>` share URL without authentication. Calling `share` on a
note that is already shared rotates the token, invalidating any
previous share URL. The response contains both the raw token and the
relative share URL for convenience.

**Arguments:** `<id>`

**Examples:**

```bash
# Share note 42 and print the share URL
mr note share 42 --json | jq -r .shareUrl
# Share and capture just the token for use elsewhere
TOKEN=$(mr note share 42 --json | jq -r .shareToken)
```

**Output:** Object with shareToken (string) and shareUrl (string path beginning with /s/)

**See also:** `mr note unshare`, `mr note get`, `mr note create`

### `mr note unshare`

Remove the share token from a note

```
mr note unshare <id>
```

Remove the share token from a note, invalidating any previous share
URL. Calling `unshare` on a note that is not currently shared is a
no-op from the client's perspective but still returns success. After
unsharing, subsequent `get` responses will omit the `shareToken`
field entirely.

**Arguments:** `<id>`

**Examples:**

```bash
# Unshare note 42
mr note unshare 42
# Unshare and confirm via JSON response
mr note unshare 42 --json | jq -e '.success == true'
```

**Output:** Object with success (bool, true) on successful unshare

**See also:** `mr note share`, `mr note get`


---

## `notes` — List notes and bulk tag/group/meta operations

Discover and bulk-mutate Notes. The `notes` subcommands operate on
multiple Notes at once: `list` for filtered queries (with pagination
via global `--page`), `add-tags` / `remove-tags` for bulk tag ops,
`add-groups` / `add-meta` for bulk annotation, `delete` for destructive
bulk removal, `meta-keys` for discovering the meta-schema vocabulary,
and `timeline` for ASCII activity charts.

Bulk-mutation commands select targets via `--ids=<csv>`. The current
CLI does not support MRQL selectors on bulk commands — pipe from
`notes list --json | jq` to extract IDs when you need query-based
selection.

### `mr notes add-groups`

Add groups to multiple notes

```
mr notes add-groups
```

Add group IDs to every Note listed in `--ids`. Idempotent. Both
`--ids` and `--groups` accept comma-separated unsigned integer lists
and are required. The linked Groups appear in the Note's `Groups`
array on subsequent `get` responses.

**Flags:**

- `--ids` (string) **(required)** — Comma-separated note IDs (required)
- `--groups` (string) **(required)** — Comma-separated group IDs (required)

**Examples:**

```bash
# Add groups 2 and 3 to notes 1
mr notes add-groups --ids 1,2 --groups 2,3
# Bulk from a list query
mr notes list --tags 5 --json | jq -r '[.[].ID] | join(",")' | xargs -I {} mr notes add-groups --ids {} --groups 7
```

**See also:** `mr notes add-tags`, `mr notes add-meta`, `mr groups list`

### `mr notes add-meta`

Add metadata to multiple notes

```
mr notes add-meta
```

Add metadata keys to every Note listed in `--ids` by passing a JSON
string via `--meta`. The server-side endpoint at
`POST /v1/notes/addMeta` determines whether this merges on top of
existing meta or replaces it — see the admin interface docs for exact
semantics. For single-note single-key edits, use `note edit-meta`
(dot-path syntax).

**Flags:**

- `--ids` (string) **(required)** — Comma-separated note IDs (required)
- `--meta` (string) **(required)** — Meta JSON string (required)

**Examples:**

```bash
# Set a single key on multiple notes
mr notes add-meta --ids 1,2,3 --meta '{"status":"reviewed"}'
# Set multiple keys at once (JSON object)
mr notes add-meta --ids 1,2 --meta '{"priority":5,"owner":"alice"}'
```

**See also:** `mr note edit-meta`, `mr notes meta-keys`, `mr notes add-tags`

### `mr notes add-tags`

Add tags to multiple notes

```
mr notes add-tags
```

Add tag IDs to every Note listed in `--ids`. Idempotent: adding a tag
that's already attached is a no-op. Both `--ids` and `--tags` accept
comma-separated unsigned integer lists and are required.

**Flags:**

- `--ids` (string) **(required)** — Comma-separated note IDs (required)
- `--tags` (string) **(required)** — Comma-separated tag IDs (required)

**Examples:**

```bash
# Add tag 5 to notes 1
mr notes add-tags --ids 1,2,3 --tags 5
# Add multiple tags at once
mr notes add-tags --ids 1,2,3 --tags 5,6,7
```

**See also:** `mr notes remove-tags`, `mr notes add-groups`, `mr tags list`

### `mr notes delete`

Delete multiple notes

```
mr notes delete
```

Bulk-delete Notes. Destructive: removes database rows for every Note
listed in `--ids` along with their tag/group/resource associations.
The current CLI has no dry-run; pipe `notes list --json` first if you
need to preview targets before deleting.

**Flags:**

- `--ids` (string) **(required)** — Comma-separated note IDs to delete (required)

**Examples:**

```bash
# Delete specific notes
mr notes delete --ids 42,43,44
# Delete the output of a filter query
mr notes list --tags 7 --json | jq -r '[.[].ID] | join(",")' | xargs -I {} mr notes delete --ids {}
```

**See also:** `mr note delete`, `mr notes list`

### `mr notes list`

List notes

```
mr notes list
```

List Notes, optionally filtered. Filter flags combine with AND.
Comma-separated ID lists on `--tags` and `--groups` match any of the
given IDs. Date flags (`--created-before`, `--created-after`) expect
`YYYY-MM-DD`. The `--name` and `--description` flags match substrings.
Use `--owner-id` and `--note-type-id` to scope by owner group or note
type. Pagination is via the global `--page` flag (default page size 50).

**Flags:**

- `--name` (string) — Filter by name
- `--description` (string) — Filter by description
- `--tags` (string) — Comma-separated tag IDs to filter by
- `--groups` (string) — Comma-separated group IDs to filter by
- `--owner-id` (uint) — Filter by owner group ID
- `--note-type-id` (uint) — Filter by note type ID
- `--created-before` (string) — Filter by creation date (before)
- `--created-after` (string) — Filter by creation date (after)

**Examples:**

```bash
# List all notes (first page)
mr notes list
# Filter by name substring and owner
mr notes list --name meeting --owner-id 42
# Filter by tag + date
mr notes list --tags 5 --created-after 2026-01-01 --json | jq -r '.[].Name'
```

**Output:** Array of Note objects with ID, Name, Description, Meta, Tags, OwnerId, NoteTypeId, CreatedAt, UpdatedAt

**See also:** `mr note get`, `mr notes timeline`, `mr mrql`

### `mr notes meta-keys`

List all unique metadata keys used across notes

```
mr notes meta-keys
```

List every distinct `meta` key observed across the entire Note corpus.
Useful for discovering the vocabulary of an evolving meta schema. The
response is a JSON array of objects each shaped `{"key": "..."}`. The
command has no filter flags in the current CLI; pair it with
client-side `jq` filtering if you only want a subset of keys.

**Examples:**

```bash
# List all meta keys
mr notes meta-keys
# Filter client-side with jq
mr notes meta-keys --json | jq '.[] | select(.key | startswith("project_"))'
```

**Output:** Array of objects with key (string), one per distinct meta key observed across the entire Note corpus

**See also:** `mr note edit-meta`, `mr notes add-meta`

### `mr notes remove-tags`

Remove tags from multiple notes

```
mr notes remove-tags
```

Remove tag IDs from every Note listed in `--ids`. Idempotent: removing
a tag that isn't attached is a no-op. Both `--ids` and `--tags` accept
comma-separated unsigned integer lists and are required.

**Flags:**

- `--ids` (string) **(required)** — Comma-separated note IDs (required)
- `--tags` (string) **(required)** — Comma-separated tag IDs (required)

**Examples:**

```bash
# Remove tag 5 from notes 1
mr notes remove-tags --ids 1,2 --tags 5
# Remove multiple tags at once
mr notes remove-tags --ids 1,2,3 --tags 5,6
```

**See also:** `mr notes add-tags`, `mr notes list`, `mr tags list`

### `mr notes timeline`

Display a timeline of note activity

```
mr notes timeline
```

Display a timeline of Note activity as an ASCII bar chart. Each bar
represents a time bucket (yearly, monthly, or weekly, controlled by
`--granularity`), and the bar height reflects the count of Notes
created in that bucket.

The chart is anchored at the `--anchor` date (default: today) and
shows `--columns` buckets backward from the anchor (default 15, max
60). All note-list filter flags (`--name`, `--tags`, `--groups`,
`--owner-id`, `--note-type-id`) apply the same way to the timeline
aggregation. Pass the global `--json` flag to get the raw bucket data
for scripting.

**Flags:**

- `--granularity` (string) (default `monthly`) — Bucket granularity: yearly, monthly, or weekly
- `--anchor` (string) — Anchor date (YYYY-MM-DD); defaults to today
- `--columns` (int) (default `15`) — Number of timeline buckets (max 60)
- `--name` (string) — Filter by name
- `--description` (string) — Filter by description
- `--tags` (string) — Comma-separated tag IDs to filter by
- `--groups` (string) — Comma-separated group IDs to filter by
- `--owner-id` (uint) — Filter by owner group ID
- `--note-type-id` (uint) — Filter by note type ID
- `--created-before` (string) — Filter by creation date (before)
- `--created-after` (string) — Filter by creation date (after)

**Examples:**

```bash
# Monthly timeline anchored at today (default)
mr notes timeline
# Weekly granularity
mr notes timeline --granularity weekly --columns 12
# Yearly timeline filtered by tag
mr notes timeline --granularity yearly --tags 5 --json
```

**Output:** Object with buckets (array of {label, start, end, created, updated}) and hasMore ({left, right})

**See also:** `mr notes list`, `mr resources timeline`, `mr groups timeline`


---

## `note-block` — Get, create, update, or delete a note block

Note blocks are ordered, typed content units attached to a single Note
(similar to Notion's blocks). Each block has a type (`text`, `heading`,
`todos`, `gallery`, `references`, `table`, `calendar`, `divider`, plus
any plugin-registered types), a free-form `content` JSON payload whose
shape depends on the type, a free-form `state` JSON payload for runtime
UI/view state, and a fractional `position` string that defines its
order within the parent note.

Use the `note-block` subcommands to operate on a single block by ID:
fetch it, create a new one on a note, update its content or state,
delete it, or list the available block types. Use `note-blocks` (plural)
for per-note listing, reorder, and rebalance operations.

### `mr note-block create`

Create a new note block

```
mr note-block create
```

Create a new block attached to a Note. `--note-id` and `--type` are
required. Use `--content` to supply the block's content JSON (the exact
shape depends on the chosen type — see `note-block types` for the
default content schema of each built-in type). `--position` is optional;
when omitted the server assigns a position after the current last block.
The created record is returned; capture `.id` from JSON output for use
in follow-up commands.

**Flags:**

- `--note-id` (uint) **(required)** — Note ID (required)
- `--type` (string) **(required)** — Block type (required)
- `--content` (string) (default `{}`) — Block content JSON
- `--position` (string) — Block position

**Examples:**

```bash
# Create a text block on note 42
mr note-block create --note-id 42 --type text --content '{"text":"hello"}'
# Create a heading block with an explicit position
mr note-block create --note-id 42 --type heading --content '{"text":"Intro","level":2}' --position a
```

**Output:** Created NoteBlock object with id (uint), noteId (uint), type (string), position (string), content (object), state (object)

**See also:** `mr note-block types`, `mr note-block update`, `mr note-blocks list`

### `mr note-block delete`

Delete a note block by ID

```
mr note-block delete <id>
```

Delete a note block by ID. Destructive: removes the database row.
Deleting a nonexistent ID returns exit code 1 with an HTTP 404 error.
Deleting a block does not affect its parent Note or sibling blocks;
to remove every block on a note, delete the note itself.

**Arguments:** `<id>`

**Examples:**

```bash
# Delete a note block by ID
mr note-block delete 42
# Delete and pipe the response to jq to confirm
mr note-block delete 42 --json | jq .
```

**See also:** `mr note-block get`, `mr note-blocks list`, `mr note delete`

### `mr note-block get`

Get a note block by ID

```
mr note-block get <id>
```

Get a single note block by ID and print its fields. Fetches the full
record including the parent note ID, block type, fractional position,
content JSON, state JSON, and timestamps. Output is a key/value table
by default; pass the global `--json` flag to get the full record for
scripting.

**Arguments:** `<id>`

**Examples:**

```bash
# Get a note block by ID (table output)
mr note-block get 42
# Get as JSON and extract the block type
mr note-block get 42 --json | jq -r .type
```

**Output:** NoteBlock object with id (uint), noteId (uint), type (string), position (string), content (object), state (object), createdAt (RFC3339), updatedAt (RFC3339)

**See also:** `mr note-block update`, `mr note-block update-state`, `mr note-blocks list`

### `mr note-block types`

Show available block types (text, table, calendar, etc.)

```
mr note-block types
```

List every block type the server knows about, including built-in types
(`text`, `heading`, `todos`, `gallery`, `references`, `table`,
`calendar`, `divider`) and any types registered by active plugins. Each
entry includes `defaultContent` and `defaultState` — the canonical
empty-payload shapes you should extend when creating a block of that
type. Useful for discovering the content/state schema a given type
expects before calling `note-block create` or `note-block update`.

**Examples:**

```bash
# List all block types as a table (default)
mr note-block types
# List types as JSON and extract just the names
mr note-block types --json | jq -r '.[].type'
```

**Output:** Array of block type descriptors, each with type (string), defaultContent (object), defaultState (object), and optional plugin metadata (label, icon, description, plugin, pluginName)

**See also:** `mr note-block create`, `mr note-block update`, `mr note-block update-state`

### `mr note-block update`

Update a note block's content

```
mr note-block update <id>
```

Replace a block's `content` payload. Takes the block ID as a positional
argument and the new content as `--content` JSON. The content shape
must match the block's type (see `note-block types` for the default
content schema of each built-in type). This command does not touch the
block's `state`, `position`, or `type` — use `note-block update-state`
for state changes and `note-blocks reorder` for position changes.

**Arguments:** `<id>`

**Flags:**

- `--content` (string) **(required)** (default `{}`) — Block content JSON (required)

**Examples:**

```bash
# Update a text block's content
mr note-block update 42 --content '{"text":"new body"}'
# Update and print the updated record as JSON
mr note-block update 42 --content '{"text":"new body"}' --json | jq .
```

**Output:** Updated NoteBlock object with id (uint), noteId (uint), type (string), position (string), content (object), state (object)

**See also:** `mr note-block update-state`, `mr note-block get`, `mr note-block create`

### `mr note-block update-state`

Update a note block's state

```
mr note-block update-state <id>
```

Replace a block's `state` payload. Takes the block ID as a positional
argument and the new state as `--state` JSON. `state` is separate from
`content`: it holds runtime/UI state like which todo items are checked,
which gallery layout is selected, or a calendar's current view. The
shape depends on the block's type (see `note-block types` for default
state schemas). Sending `null` or an empty body returns an error: the
state column has a NOT NULL constraint.

**Arguments:** `<id>`

**Flags:**

- `--state` (string) **(required)** (default `{}`) — Block state JSON (required)

**Examples:**

```bash
# Mark a text block as "done" via a custom state field
mr note-block update-state 42 --state '{"done":true}'
# Check off a todo item (todos blocks use `{"checked":[itemId,...]}`)
mr note-block update-state 42 --state '{"checked":["task-1"]}'
```

**Output:** Updated NoteBlock object with id (uint), noteId (uint), type (string), position (string), content (object), state (object)

**See also:** `mr note-block update`, `mr note-block get`, `mr note-block types`


---

## `note-blocks` — List, reorder, or rebalance note blocks

Discover and reorganize the blocks attached to a Note. The `note-blocks`
subcommands operate on the full set of blocks owned by one parent note:
`list` returns every block in position order, `reorder` moves specific
blocks to new positions via an explicit `blockId -> position` map, and
`rebalance` normalizes every block's position string to clean, evenly
spaced values (useful after many reorders cause position strings to
grow long).

All commands require `--note-id` to scope to a single note. To mutate
an individual block's content, state, or type, use the singular
`note-block` subcommands.

### `mr note-blocks list`

List note blocks for a note

```
mr note-blocks list
```

List every block attached to a Note in position order. `--note-id` is
required; the server returns the full set (no pagination), ordered by
the fractional `position` string. Use this to inspect the current
layout before reordering, to dump a note's structured content to JSON
for processing, or to feed block IDs into downstream commands.

**Flags:**

- `--note-id` (uint) **(required)** — Note ID (required)

**Examples:**

```bash
# List every block on note 42 (table output)
mr note-blocks list --note-id 42
# Get blocks as JSON and extract id + position pairs
mr note-blocks list --note-id 42 --json | jq -r '.[] | [.id, .position] | @tsv'
```

**Output:** Array of NoteBlock objects with id, noteId, type, position, content, state, createdAt, updatedAt (ordered by position ascending)

**See also:** `mr note-block get`, `mr note-blocks reorder`, `mr note-blocks rebalance`

### `mr note-blocks rebalance`

Rebalance note block positions

```
mr note-blocks rebalance
```

Rewrite every block's `position` string on a note to evenly spaced,
compact values while preserving the current display order. Use this
as a cleanup step after heavy reordering, when fractional positions
have grown long (e.g. `"aaamzzz"`), or when you want a predictable
position layout before a batch of reorders. The block IDs, types,
content, and state are untouched.

**Flags:**

- `--note-id` (uint) **(required)** — Note ID (required)

**Examples:**

```bash
# Rebalance all block positions on note 42
mr note-blocks rebalance --note-id 42
# Rebalance
mr note-blocks rebalance --note-id 42
mr note-blocks list --note-id 42 --json | jq -r '.[] | [.id, .position] | @tsv'
```

**See also:** `mr note-blocks reorder`, `mr note-blocks list`

### `mr note-blocks reorder`

Reorder note blocks

```
mr note-blocks reorder
```

Move specific blocks to new positions on their parent note. `--note-id`
and `--positions` are both required. `--positions` takes a JSON object
mapping block ID (as a string key) to its new fractional `position`
string. Only the listed blocks are moved; every other block on the
note keeps its current position. Fractional positions sort
lexicographically, so `"a" < "m" < "z"` — pick new values that slot
into the desired order.

After many reorders, positions can grow long; run `note-blocks
rebalance` to normalize them.

**Flags:**

- `--note-id` (uint) **(required)** — Note ID (required)
- `--positions` (string) **(required)** — Positions JSON map (required), e.g. '{"1":"a","2":"b"}'

**Examples:**

```bash
# Move block 10 to the top and block 11 to the bottom of note 42
mr note-blocks reorder --note-id 42 --positions '{"10":"a","11":"z"}'
# Move one block between two siblings using a midpoint string
mr note-blocks reorder --note-id 42 --positions '{"10":"m"}'
```

**See also:** `mr note-blocks rebalance`, `mr note-blocks list`, `mr note-block update`


---

## `note-type` — Get, create, edit, or delete a note type

Note Types are typed schemas for Notes. A NoteType defines the shape of
a Note's metadata via a JSON Schema (`MetaSchema`) and may carry custom
rendering bits: `CustomHeader`, `CustomCSS`, `CustomSidebar`,
`CustomSummary`, `CustomAvatar`, `CustomMRQLResult`, and a `SectionConfig`
JSON toggle for which sections appear on note detail pages. Typical examples are
"Meeting Minutes", "Code Review", or "Bug Report".

Use the `note-type` subcommands to operate on a single note type by ID:
fetch it, create a new one, edit it (whole record or scoped name /
description), or delete it. Use `note-types list` to discover the
available note types and feed their IDs into `note create --note-type-id`.

### `mr note-type create`

Create a new note type

```
mr note-type create
```

Create a new note type. `--name` is required; all other fields are
optional. Pass a JSON Schema string to `--meta-schema` to constrain the
metadata shape of Notes of this type, and a JSON object to
`--section-config` to control which sections render on note detail
pages. The Custom* flags (`--custom-header`, `--custom-css`,
`--custom-sidebar`, `--custom-summary`, `--custom-avatar`,
`--custom-mrql-result`) accept raw HTML or Pongo2 template strings that
the server injects into note pages and MRQL result cards;
`--custom-css` is injected as a `<style>` block on detail and list
pages.

On success prints a confirmation line with the new ID; pass the global
`--json` flag to emit the full created record for scripting.

**Flags:**

- `--name` (string) **(required)** — Note type name (required)
- `--description` (string) — Note type description
- `--custom-header` (string) — Custom header HTML
- `--custom-css` (string) — Custom CSS injected as a <style> block on detail/list pages
- `--custom-sidebar` (string) — Custom sidebar HTML
- `--custom-summary` (string) — Custom summary HTML
- `--custom-avatar` (string) — Custom avatar HTML
- `--meta-schema` (string) — JSON Schema defining the metadata structure for notes of this type
- `--section-config` (string) — JSON controlling which sections are visible on note detail pages
- `--custom-mrql-result` (string) — Pongo2 template for rendering notes of this type in MRQL results

**Examples:**

```bash
# Create a minimal note type (name only)
mr note-type create --name "Meeting Minutes"
# Create with a JSON Schema constraining metadata
mr note-type create --name "Bug Report" \
  --meta-schema '{"type":"object","properties":{"severity":{"type":"string"}}}'
# Capture the new ID via jq for follow-up commands
NT=$(mr note-type create --name "Code Review" --json | jq -r .ID)
```

**Output:** Created NoteType with ID, Name, Description, MetaSchema, SectionConfig, CustomHeader/CSS/Sidebar/Summary/Avatar/MRQLResult, CreatedAt, UpdatedAt

**See also:** `mr note-type get`, `mr note-type edit`, `mr note-types list`

### `mr note-type delete`

Delete a note type by ID

```
mr note-type delete <id>
```

Delete a note type by ID. Destructive: removes the note type row. Notes
that referenced it keep their rows but lose the typed schema link, so
use with care on instances where Notes depend on the type's MetaSchema
for rendering. Deleting a nonexistent ID is a no-op on the server but
still returns success.

**Arguments:** `<id>`

**Examples:**

```bash
# Delete a note type by ID
mr note-type delete 42
# Delete and pipe the result to jq to confirm the response shape
mr note-type delete 42 --json | jq .
```

**See also:** `mr note-type get`, `mr note-type create`, `mr note-types list`

### `mr note-type edit`

Edit a note type

```
mr note-type edit
```

Edit a note type. `--id` is required; every other flag is optional and
only fields explicitly passed are modified (server-side PATCH
semantics). Use this command when you need to change the `MetaSchema`,
`SectionConfig`, or any of the Custom* rendering fields
(`--custom-header`, `--custom-css`, `--custom-sidebar`,
`--custom-summary`, `--custom-avatar`, `--custom-mrql-result`); the
dedicated `edit-name` / `edit-description` commands only touch those two
scoped fields. `--custom-css` is injected as a `<style>` block on detail
and list pages.

**Flags:**

- `--id` (uint) **(required)** — Note type ID (required)
- `--name` (string) — Note type name
- `--description` (string) — Note type description
- `--custom-header` (string) — Custom header HTML
- `--custom-css` (string) — Custom CSS injected as a <style> block on detail/list pages
- `--custom-sidebar` (string) — Custom sidebar HTML
- `--custom-summary` (string) — Custom summary HTML
- `--custom-avatar` (string) — Custom avatar HTML
- `--meta-schema` (string) — JSON Schema defining the metadata structure for notes of this type
- `--section-config` (string) — JSON controlling which sections are visible on note detail pages
- `--custom-mrql-result` (string) — Pongo2 template for rendering notes of this type in MRQL results

**Examples:**

```bash
# Swap the JSON Schema on note type 1
mr note-type edit --id 1 \
  --meta-schema '{"type":"object","properties":{"priority":{"type":"string"}}}'
# Update the custom summary template and confirm via list
mr note-type edit --id 1 --custom-summary "<div>{{ Note.Name }}</div>"
mr note-types list --json | jq '.[] | select(.ID == 1).CustomSummary'
```

**Output:** Updated NoteType with ID, Name, Description, MetaSchema, SectionConfig, CustomHeader/CSS/Sidebar/Summary/Avatar/MRQLResult, CreatedAt, UpdatedAt

**See also:** `mr note-type edit-name`, `mr note-type edit-description`, `mr note-type get`

### `mr note-type edit-description`

Edit a note type's description

```
mr note-type edit-description <id> <new-description>
```

Update only the description of an existing note type. Takes two
positional arguments: the note type ID and the new description.
Passing an empty string clears the description. Useful for annotating
a note type's intended use without touching its MetaSchema or rendering
fields. Returns `{"id":N,"ok":true}` on success.

**Arguments:** `<id> <new-description>`

**Examples:**

```bash
# Set a description on note type 1
mr note-type edit-description 1 "for weekly engineering standups"
# Clear the description by passing an empty string
mr note-type edit-description 1 ""
```

**See also:** `mr note-type edit-name`, `mr note-type edit`, `mr note-type get`

### `mr note-type edit-name`

Edit a note type's name

```
mr note-type edit-name <id> <new-name>
```

Update only the name of an existing note type. Takes two positional
arguments: the note type ID and the new name. Shorthand for
`mr note-type edit --id <id> --name <value>` when name is the only
change. Returns `{"id":N,"ok":true}` on success; chain with
`mr note-type get <id>` to inspect the renamed record.

**Arguments:** `<id> <new-name>`

**Examples:**

```bash
# Rename note type 1
mr note-type edit-name 1 "Team Meeting"
# Rename and confirm with a follow-up get
mr note-type edit-name 1 "renamed" && mr note-type get 1 --json | jq -r .Name
```

**See also:** `mr note-type edit-description`, `mr note-type edit`, `mr note-type get`

### `mr note-type get`

Get a note type by ID

```
mr note-type get <id>
```

Get a note type by ID and print its fields. The server has no
single-NoteType GET endpoint, so the CLI fetches the full list and
filters in-process; this is slower than a direct lookup on large
instances. The table output shows five core fields (ID, Name, Description,
Created, Updated). The `--json` flag emits the full server response,
including MetaSchema, SectionConfig, CustomHeader, CustomCSS, and other
Custom* fields.

**Arguments:** `<id>`

**Examples:**

```bash
# Get a note type by ID (table output)
mr note-type get 1
# Get as JSON and extract the name with jq
mr note-type get 1 --json | jq -r .Name
```

**Output:** Full server NoteType JSON (--json); table with ID, Name, Description, Created, Updated (default)

**See also:** `mr note-type create`, `mr note-type edit`, `mr note-types list`


---

## `note-types` — List note types

Discover Note Types, the typed schemas assigned to Notes. The
`note-types` subcommand currently exposes `list` for filtered queries
(with pagination via the global `--page` flag). Pipe `note-types list
--json` through `jq` when you need to derive IDs to feed into
`note create --note-type-id`.

Singular operations (get, create, edit, delete) live under the sibling
`note-type` command.

### `mr note-types list`

List note types

```
mr note-types list
```

List Note Types, optionally filtered by name or description. The
`--name` and `--description` flags do substring matching on the server.
Results are paginated via the global `--page` flag (default page size
50). Default output is a table with ID, NAME, DESCRIPTION, and CREATED
columns; pass `--json` for the full array including MetaSchema,
SectionConfig, and the Custom* rendering fields.

**Flags:**

- `--name` (string) — Filter by name
- `--description` (string) — Filter by description

**Examples:**

```bash
# List all note types (first page)
mr note-types list
# Filter by name substring
mr note-types list --name meeting
# JSON output piped into jq to extract just names
mr note-types list --json | jq -r '.[].Name'
```

**Output:** Array of NoteType objects with ID, Name, Description, MetaSchema, SectionConfig, CustomHeader/CSS/Sidebar/Summary/Avatar/MRQLResult, CreatedAt, UpdatedAt

**See also:** `mr note-type get`, `mr note-type create`, `mr notes list`


---

## `group` — Get, create, edit, delete, or clone a group

Groups are hierarchical collections in mahresources. A Group has a name,
description, optional meta JSON, an optional owner (the parent group),
an optional category, and many-to-many links to Resources, Notes, Tags,
and other Groups. The owner relationship forms a tree, so a Group can
also have child groups (descendants whose `OwnerId` points at this one).

Use the `group` subcommands to operate on a single group by ID: fetch
metadata, edit its name/description/meta, walk its ancestor chain or
direct children, clone it, or export/import a self-contained subtree as
a portable tar archive. Use `groups list` to discover groups matching
filters, or the bulk subcommands under `groups` to mutate many at once.

### `mr group children`

List child groups (tree children) of a group

```
mr group children <id>
```

List the direct children of a Group as lightweight tree-node records.
Each node returns `id`, `name`, `categoryName`, `childCount` (the
number of grandchildren under that child), and `ownerId`. Returns
a JSON array ordered alphabetically by name. A group with no children
returns an empty array.

Field names on tree-node responses are lowercase (`id`, `name`), not
PascalCase — unlike full Group objects returned by `group get`.

**Arguments:** `<id>`

**Examples:**

```bash
# List the direct children of group 42
mr group children 42
# Extract child IDs as CSV
mr group children 42 --json | jq -r 'map(.id) | join(",")'
```

**Output:** Array of GroupTreeNode objects with id (uint), name (string), categoryName (string), childCount (int), ownerId (uint or null)

**See also:** `mr group get`, `mr group parents`, `mr groups list`

### `mr group clone`

Clone a group

```
mr group clone <id>
```

Create a copy of an existing Group. The clone receives a new ID and
GUID but inherits the source Group's `Name`, `Description`, `Meta`,
`OwnerId`, `CategoryId`, and tag associations. Related resources,
notes, and sub-groups are NOT cloned — use `group export` + `group
import` for a deep subtree copy.

**Arguments:** `<id>`

**Examples:**

```bash
# Clone group 42
mr group clone 42
# Clone and capture the new ID with jq
NEW=$(mr group clone 42 --json | jq -r '.ID')
```

**Output:** Group object for the newly-created clone (new ID, new guid; copied Name, Description, Meta, owner/category references)

**See also:** `mr group get`, `mr group create`, `mr group export`

### `mr group create`

Create a new group

```
mr group create
```

Create a new Group. `--name` is required; all other fields are
optional. Use `--owner-id` to place the new Group under an existing
parent (forming a subtree); use `--category-id` to attach a Category;
pass a JSON blob via `--meta` for free-form custom metadata. Sends
`POST /v1/group` and returns the persisted record.

**Flags:**

- `--name` (string) **(required)** — Group name (required)
- `--description` (string) — Group description
- `--tags` (string) — Comma-separated tag IDs
- `--groups` (string) — Comma-separated group IDs
- `--meta` (string) — Meta JSON string
- `--url` (string) — URL
- `--owner-id` (uint) — Owner group ID
- `--category-id` (uint) — Category ID

**Examples:**

```bash
# Create a top-level group
mr group create --name "Trips 2026"
# Create a child group with meta and a category
mr group create --name "Berlin" --owner-id 5 --category-id 2 --meta '{"city":"Berlin"}'
```

**Output:** Group object with ID, Name, Description, Meta, OwnerId, CategoryId, CreatedAt/UpdatedAt

**See also:** `mr group get`, `mr group edit-name`, `mr group edit-description`, `mr group edit-meta`, `mr groups list`

### `mr group delete`

Delete a group by ID

```
mr group delete <id>
```

Delete a Group by ID. Destructive: removes the Group row and its
direct join-table entries (tag links, m2m relations). Owned children,
resources, and notes are orphaned (their `OwnerId` becomes null) rather
than cascaded. Use `groups delete --ids=...` for bulk deletion, or
`groups merge` to consolidate rather than destroy.

**Arguments:** `<id>`

**Examples:**

```bash
# Delete a single group
mr group delete 42
```

**See also:** `mr group create`, `mr groups delete`, `mr groups merge`

### `mr group edit-description`

Edit a group's description

```
mr group edit-description <id> <new-description>
```

Replace a Group's `Description` field. Takes the Group ID and the new
description as positional arguments. Sends `POST /v1/group/editDescription`
and returns `{id, ok}` on success. Descriptions are free-form text used
for human-readable context; for structured metadata use `edit-meta`.

**Arguments:** `<id> <new-description>`

**Examples:**

```bash
# Update the description on group 42
mr group edit-description 42 "Our summer 2026 travel photos"
```

**Output:** Status object with id (uint) and ok (bool)

**See also:** `mr group get`, `mr group edit-name`, `mr group edit-meta`

### `mr group edit-meta`

Edit a single metadata field by JSON path

```
mr group edit-meta <id> <path> <value>
```

Edit a single metadata field by JSON path. Takes three positional
arguments: the Group ID, a dot-separated path (e.g. `address.city`),
and a JSON-literal value (e.g. `'"Berlin"'`, `42`, `'[1,2,3]'`,
`'{"nested":true}'`). The server deep-merges the value at the given
path onto the existing Meta object and returns the full merged Meta
in the response.

Values must be valid JSON literals — string values need to be quoted
twice (bash single quotes around a JSON-quoted string), as in the
examples below.

**Arguments:** `<id> <path> <value>`

**Examples:**

```bash
# Set a top-level string value
mr group edit-meta 5 status '"active"'
# Set a nested field
mr group edit-meta 5 address.city '"Berlin"'
# Replace a field with an array
mr group edit-meta 5 scores '[1,2,3]'
```

**Output:** Status object with id (uint), ok (bool), and meta (object reflecting the merged Meta)

**See also:** `mr group get`, `mr groups add-meta`, `mr groups meta-keys`

### `mr group edit-name`

Edit a group's name

```
mr group edit-name <id> <new-name>
```

Replace a Group's `Name` field. Takes the Group ID and the new name
as positional arguments. Sends `POST /v1/group/editName` and returns
`{id, ok}` on success. Use `group get` afterward to view the updated
record.

**Arguments:** `<id> <new-name>`

**Examples:**

```bash
# Rename group 42
mr group edit-name 42 "Trips to Berlin"
```

**Output:** Status object with id (uint) and ok (bool)

**See also:** `mr group get`, `mr group edit-description`, `mr group edit-meta`

### `mr group export`

Export one or more groups to a tar archive

```
mr group export <id> [<id>...]
```

Export one or more Groups and their reachable entities to a portable
tar archive. Sends `POST /v1/groups/export`, polls the resulting job
until completion, then downloads the tar. Takes one or more Group IDs
as positional arguments; each ID becomes a root of the export tree.

The archive format follows the manifest schema v1 (see `archive/manifest.go`)
and is compatible with `mr group import` on any mahresources instance.
Scope and fidelity are controlled by paired `--include-*` / `--no-*`
flags (subtree, resources, notes, related, group-relations, blobs,
versions, previews, series). Schema-definition inclusion (categories,
tag defs, group-relation types) can be toggled individually or via the
`--schema-defs=all|none|selected` shortcut. Use `--gzip` to compress
the output and `--output <path>` (or `-o`) to write to a file rather
than stdout.

By default the command waits for the server-side job to finish before
downloading; pass `--no-wait` to print the job ID and exit immediately
so you can poll and download separately.

**Arguments:** `<id>...`

**Flags:**

- `--include-subtree` (bool) (default `true`) — include all descendant subgroups (default on)
- `--no-subtree` (bool) — disable --include-subtree
- `--include-resources` (bool) (default `true`) — include owned resources (default on)
- `--no-resources` (bool) — disable --include-resources
- `--include-notes` (bool) (default `true`) — include owned notes (default on)
- `--no-notes` (bool) — disable --include-notes
- `--include-related` (bool) (default `true`) — include m2m related entities (default on)
- `--no-related` (bool) — disable --include-related
- `--include-group-relations` (bool) (default `true`) — include typed group relations (default on)
- `--no-group-relations` (bool) — disable --include-group-relations
- `--include-blobs` (bool) (default `true`) — include resource file bytes (default on)
- `--no-blobs` (bool) — disable --include-blobs
- `--include-versions` (bool) (default `true`) — include resource version history (default off)
- `--no-versions` (bool) — disable --include-versions
- `--include-previews` (bool) (default `true`) — include resource previews (default off)
- `--no-previews` (bool) — disable --include-previews
- `--include-series` (bool) (default `true`) — preserve Series membership (default on)
- `--no-series` (bool) — disable --include-series
- `--include-categories-and-types` (bool) (default `true`) — include Category/NoteType/ResourceCategory defs (D1, default on)
- `--no-categories-and-types` (bool) — disable --include-categories-and-types
- `--include-tag-defs` (bool) (default `true`) — include Tag definitions (D2, default on)
- `--no-tag-defs` (bool) — disable --include-tag-defs
- `--include-group-relation-type-defs` (bool) (default `true`) — include GroupRelationType defs (D3, default on)
- `--no-group-relation-type-defs` (bool) — disable --include-group-relation-type-defs
- `--schema-defs` (string) (default `selected`) — schema-def shortcut (all|none|selected — selected defers to individual --include-*-defs flags)
- `--gzip` (bool) — gzip the output tar
- `--output` (string) — output file path (default stdout)
- `--wait` (bool) (default `true`) — wait for the job to finish before returning
- `--no-wait` (bool) — return immediately after submitting the job
- `--poll-interval` (duration) (default `1s`) — polling interval
- `--timeout` (duration) (default `30m0s`) — max total wait time
- `--related-depth` (int) — follow m2m relationships up to N hops deep (0 = off)

**Examples:**

```bash
# Export group 42 and its subtree to a tar file
mr group export 42 --output /tmp/trip-2026.tar
# Export two roots
mr group export 42 43 --gzip --no-blobs --no-related --output /tmp/shell.tar.gz
# Submit the job and print its ID without waiting
mr group export 42 --no-wait
```

**Output:** Tar archive written to stdout or --output path; when --no-wait, prints the job ID as plain text

**See also:** `mr group import`, `mr group clone`, `mr groups list`

### `mr group get`

Get a group by ID

```
mr group get <id>
```

Get a group by ID and print its metadata. Fetches the full record
including the owner chain, category, tags, and any custom Meta JSON
object. Output is a key/value table by default; pass the global `--json`
flag to get the full record for scripting (related collections such as
`Tags`, `OwnResources`, `OwnNotes`, and `OwnGroups` are included).

**Arguments:** `<id>`

**Examples:**

```bash
# Get a group by ID (table output)
mr group get 42
# Get as JSON and extract a single field with jq
mr group get 42 --json | jq -r .Name
```

**Output:** Group object with ID (uint), Name, Description, Meta (object), OwnerId, CategoryId, CreatedAt/UpdatedAt, plus related collections (Tags, OwnResources, OwnNotes, OwnGroups)

**See also:** `mr group create`, `mr group edit-name`, `mr group parents`, `mr group children`

### `mr group import`

Import a group export tar into this instance

```
mr group import <tarfile>
```

Upload a group export tar, parse it into an import plan, and optionally
apply it. Takes the path to a tar file (produced by `mr group export`
or the `/v1/groups/export` API) as its single positional argument.

The command runs a two-phase job pipeline: first a `parse` job uploads
the tar, validates the manifest schema version, and produces an
`ImportPlan` (counts, mappings, conflicts, dangling refs). Then — unless
`--dry-run` is set — an `apply` job actually creates the groups and
related entities.

Use `--dry-run` to inspect the plan without mutating state. Use
`--plan-output <file>` to save the parsed plan JSON. Use
`--parent-group <id>` to graft imported top-level groups under an
existing parent. Use `--on-resource-conflict=skip|duplicate` and
`--guid-collision-policy=merge|skip|replace` to steer conflict
resolution. For full manual control over every mapping/dangling/shell
decision, pass `--decisions <json-file>` produced from a prior dry-run.

When the server plan reports resources without bytes in the tar,
`--acknowledge-missing-hashes` is required to proceed.

**Arguments:** `<tarfile>`

**Flags:**

- `--dry-run` (bool) — Parse and print the plan without applying
- `--plan-output` (string) — Write the plan JSON to a file
- `--poll-interval` (duration) (default `1s`) — Polling interval
- `--timeout` (duration) (default `30m0s`) — Max total wait time
- `--parent-group` (uint) — Parent group ID for imported top-level groups
- `--on-resource-conflict` (string) (default `skip`) — Resource collision policy: "skip" or "duplicate"
- `--guid-collision-policy` (string) — GUID collision policy: "merge", "skip", or "replace" (default: server default = "merge")
- `--auto-map` (bool) (default `true`) — Automatically accept plan mapping suggestions
- `--acknowledge-missing-hashes` (bool) — Proceed even when some resources have no bytes
- `--decisions` (string) — Path to a decisions JSON file (overrides other flags)

**Examples:**

```bash
# Dry-run an import and print the plan
mr group import /tmp/trip-2026.tar --dry-run
# Import
mr group import /tmp/trip-2026.tar --parent-group 17
# Dry-run to JSON file for review
mr group import /tmp/trip-2026.tar --dry-run --plan-output /tmp/plan.json
```

**Output:** ImportPlan (dry-run) or ImportApplyResult object with CreatedGroups, CreatedResources, SkippedByHash, CreatedNotes, CreatedGroupIDs arrays, etc.

**See also:** `mr group export`, `mr group create`, `mr groups list`

### `mr group parents`

List parent groups of a group

```
mr group parents <id>
```

Walk up the owner chain from a Group to its top-level ancestor. Returns
an array of Group objects ordered from outermost ancestor down to the
queried Group itself (so the last element is always the group you asked
about, and root groups return a single-element array containing just
themselves). The walk is bounded to 20 levels to defend against cycles
in corrupted data.

Use this to render breadcrumbs or to detect whether a group lives under
a particular root.

**Arguments:** `<id>`

**Examples:**

```bash
# Show the ancestor chain for group 42
mr group parents 42
# Extract ancestor IDs as CSV
mr group parents 42 --json | jq -r 'map(.ID) | join(",")'
```

**Output:** Array of Group objects representing the ancestor chain (up to 20 levels deep), including the queried group itself

**See also:** `mr group get`, `mr group children`, `mr groups list`


---

## `groups` — List, merge, or bulk-edit groups

Discover and bulk-mutate Groups. The `groups` subcommands operate on
multiple Groups at once: `list` for filtered queries (with pagination
via the global `--page` flag), `add-tags` / `remove-tags` for bulk
tag ops, `add-meta` for bulk metadata merges, `delete` / `merge` for
destructive consolidation, `meta-keys` to enumerate the observed meta
vocabulary, and `timeline` for an ASCII activity chart.

Most bulk-mutation commands select targets via `--ids=<csv>`; `merge`
uses `--winner` / `--losers` instead. The current CLI does not accept
MRQL selectors on bulk commands — pipe from `groups list --json | jq`
to extract IDs when you need query-based selection.

### `mr groups add-meta`

Add metadata to multiple groups

```
mr groups add-meta
```

Merge a Meta JSON object onto multiple Groups at once. Both arguments
are required: `--ids` selects the target Groups (comma-separated) and
`--meta` is a JSON object string that is deep-merged onto each target's
existing Meta. Existing keys are overwritten by the incoming value;
keys not present in `--meta` are preserved.

To edit a single path on a single group, prefer `group edit-meta` which
takes a dotted path + JSON literal. This bulk variant is best for
stamping the same set of keys across many Groups.

**Flags:**

- `--ids` (string) **(required)** — Comma-separated group IDs (required)
- `--meta` (string) **(required)** — Meta JSON string (required)

**Examples:**

```bash
# Stamp one Meta key across three groups
mr groups add-meta --ids 10,11,12 --meta '{"reviewed":true}'
# Merge multiple keys
mr groups add-meta --ids 10 --meta '{"season":"winter","owner":"alice"}'
```

**Output:** Status object with ok (bool)

**See also:** `mr group edit-meta`, `mr groups meta-keys`, `mr group get`

### `mr groups add-tags`

Add tags to multiple groups

```
mr groups add-tags
```

Attach one or more Tags to a set of Groups in a single request. Both
arguments are comma-separated ID lists: `--ids` selects the target
Groups and `--tags` selects the Tags to add. The server merges the
requested tag links with whatever each Group already has; existing
links are unaffected, and no tag links are removed.

Verify the result by reading a target Group back with
`mr group get <id> --json | jq '.Tags'`.

**Flags:**

- `--ids` (string) **(required)** — Comma-separated group IDs (required)
- `--tags` (string) **(required)** — Comma-separated tag IDs (required)

**Examples:**

```bash
# Tag three groups with tag 5
mr groups add-tags --ids 10,11,12 --tags 5
# Add multiple tags to one group
mr groups add-tags --ids 10 --tags 5,6,7
```

**Output:** Status object with ok (bool)

**See also:** `mr groups remove-tags`, `mr group get`, `mr tags list`

### `mr groups delete`

Delete multiple groups

```
mr groups delete
```

Bulk-delete Groups. Destructive: removes each selected Group row and
its direct join-table entries (tag links, m2m relations). Owned
children, resources, and notes are orphaned (their `OwnerId` becomes
null). Targets are selected via `--ids` (CSV of unsigned ints).

The current CLI has no dry-run; pipe `groups list --json | jq` first
if you need to preview targets, or use `groups merge` to consolidate
rather than destroy.

**Flags:**

- `--ids` (string) **(required)** — Comma-separated group IDs to delete (required)

**Examples:**

```bash
# Delete specific groups
mr groups delete --ids 42,43,44
# Delete the output of a filter query
mr groups list --tags 7 --json | jq -r 'map(.ID) | join(",")' | xargs -I {} mr groups delete --ids {}
```

**See also:** `mr group delete`, `mr groups merge`, `mr groups list`

### `mr groups list`

List groups

```
mr groups list
```

List Groups, optionally filtered. Filter flags combine with AND.
Comma-separated ID lists on `--tags` and `--groups` match any of the
given IDs via the `?Add` query parameter. Date flags
(`--created-before`, `--created-after`) expect `YYYY-MM-DD`. Pagination
via the global `--page` flag (default page size 50).

Use `--owner-id=0` to restrict to root groups (no parent). The JSON
output is a flat array — use `group children <id>` for tree-structured
traversal.

**Flags:**

- `--name` (string) — Filter by name
- `--description` (string) — Filter by description
- `--tags` (string) — Comma-separated tag IDs to filter by
- `--groups` (string) — Comma-separated group IDs to filter by
- `--owner-id` (uint) — Filter by owner group ID
- `--category-id` (uint) — Filter by category ID
- `--url` (string) — Filter by URL
- `--created-before` (string) — Filter by creation date (before)
- `--created-after` (string) — Filter by creation date (after)

**Examples:**

```bash
# List all groups (paged)
mr groups list
# Filter by name prefix
mr groups list --name "Trips"
# Filter by owner and tag
mr groups list --owner-id 5 --tags 3 --json | jq -r '.[].Name'
```

**Output:** Array of Group objects with ID, Name, Description, Meta, OwnerId, CategoryId, CreatedAt/UpdatedAt

**See also:** `mr group get`, `mr group create`, `mr groups timeline`, `mr mrql`

### `mr groups merge`

Merge groups into a winner

```
mr groups merge
```

Merge one or more "loser" Groups into a single "winner". The winner's
ID and fields are preserved; tags, owned resources, owned notes, and
m2m relations from the losers are moved onto the winner; the loser
records are then deleted. Use to consolidate duplicates after manual
review or deduplication.

Both flags are required: `--winner <id>` is a single ID, and
`--losers` is a comma-separated list of IDs to merge in.

**Flags:**

- `--winner` (uint) **(required)** — Winning group ID (required)
- `--losers` (string) **(required)** — Comma-separated loser group IDs (required)

**Examples:**

```bash
# Merge groups 2 and 3 into winner 1
mr groups merge --winner 1 --losers 2,3
```

**See also:** `mr group get`, `mr groups delete`, `mr groups list`

### `mr groups meta-keys`

List all unique metadata keys used across groups

```
mr groups meta-keys
```

List every distinct `Meta` key observed across the entire Group corpus.
Useful for discovering the vocabulary of an evolving meta schema and
for building UI dropdowns of known keys. The command has no filter
flags in the current CLI; pair it with client-side `jq` filtering if
you only want a subset of keys.

The JSON shape is an array of objects with a `key` field
(`[{"key":"status"}, {"key":"owner"}]`), not a flat string array.

**Examples:**

```bash
# List all meta keys
mr groups meta-keys
# Filter client-side with jq
mr groups meta-keys --json | jq -r '.[].key | select(startswith("probe_"))'
```

**Output:** Array of objects with shape [{"key": string}] — one entry per distinct Meta key across all Groups

**See also:** `mr group edit-meta`, `mr groups add-meta`

### `mr groups remove-tags`

Remove tags from multiple groups

```
mr groups remove-tags
```

Detach one or more Tags from a set of Groups in a single request. Both
arguments are comma-separated ID lists: `--ids` selects the target
Groups and `--tags` selects the Tags to remove. Other tag links on the
targeted Groups are left untouched, and removing a tag that was never
linked is a no-op (not an error).

**Flags:**

- `--ids` (string) **(required)** — Comma-separated group IDs (required)
- `--tags` (string) **(required)** — Comma-separated tag IDs (required)

**Examples:**

```bash
# Remove tag 5 from three groups
mr groups remove-tags --ids 10,11,12 --tags 5
# Remove multiple tags from one group
mr groups remove-tags --ids 10 --tags 5,6,7
```

**Output:** Status object with ok (bool)

**See also:** `mr groups add-tags`, `mr group get`, `mr tags list`

### `mr groups timeline`

Display a timeline of group activity

```
mr groups timeline
```

Display a timeline of Group creation and update activity as an ASCII
bar chart. Each bar represents a time bucket (yearly, monthly, or
weekly, controlled by `--granularity`), and the bar height reflects
the count of Groups created in that bucket.

The chart is anchored at the `--anchor` date (default: today) and shows
`--columns` buckets backward from the anchor. All group-list filter
flags (`--name`, `--tags`, `--groups`, `--owner-id`, etc.) apply the
same way to the timeline aggregation. Pass the global `--json` flag to
get the raw bucket data for scripting — the top-level response has a
`buckets` array and a `hasMore` flag.

**Flags:**

- `--granularity` (string) (default `monthly`) — Bucket granularity: yearly, monthly, or weekly
- `--anchor` (string) — Anchor date (YYYY-MM-DD); defaults to today
- `--columns` (int) (default `15`) — Number of timeline buckets (max 60)
- `--name` (string) — Filter by name
- `--description` (string) — Filter by description
- `--tags` (string) — Comma-separated tag IDs to filter by
- `--groups` (string) — Comma-separated group IDs to filter by
- `--owner-id` (uint) — Filter by owner group ID
- `--category-id` (uint) — Filter by category ID
- `--url` (string) — Filter by URL
- `--created-before` (string) — Filter by creation date (before)
- `--created-after` (string) — Filter by creation date (after)

**Examples:**

```bash
# Monthly timeline anchored at today (default)
mr groups timeline
# Weekly granularity
mr groups timeline --granularity weekly --columns 20
# Yearly timeline anchored at 2020
mr groups timeline --granularity yearly --anchor 2020-01-01
```

**Output:** Object with buckets (array of {label, start, end, created, updated}) and hasMore (bool)

**See also:** `mr groups list`, `mr resources timeline`


---

## `relation` — Create, edit, or delete a group relation

A Relation is a typed, directional link between two Groups. It has a
`FromGroupId`, a `ToGroupId`, and a `RelationTypeId` pointing at a
`relation-type` that defines the allowed category pairing and the
relationship's semantics. Relations may also carry an optional `Name`
and `Description`.

Use the `relation` subcommands to operate on a single relation by ID:
`create` links two groups, `edit-name` and `edit-description` update
its labels, and `delete` removes the link. There is no `relation list`
or `relation get`: to read a relation back, fetch a participating group
with `mr group get <id> --json` and inspect its `Relationships` array,
or query via `mr mrql`.

### `mr relation create`

Create a new group relation

```
mr relation create
```

Create a new Relation linking two Groups with a typed relationship.
`--from-group-id`, `--to-group-id`, and `--relation-type-id` are all
required. The referenced relation-type's `FromCategory` and
`ToCategory` must match the categories of the two groups; otherwise
the server rejects the request. `--name` and `--description` are
optional labels stored on the relation itself. Sends `POST /v1/relation`
and returns the persisted record.

**Flags:**

- `--from-group-id` (uint) **(required)** — Source group ID (required)
- `--to-group-id` (uint) **(required)** — Target group ID (required)
- `--relation-type-id` (uint) **(required)** — Relation type ID (required)
- `--name` (string) — Relation name
- `--description` (string) — Relation description

**Examples:**

```bash
# Create a relation linking group 3 to group 4 with relation-type 2
mr relation create --from-group-id 3 --to-group-id 4 --relation-type-id 2
# Create a named relation with a description
mr relation create --from-group-id 3 --to-group-id 4 --relation-type-id 2 \
    --name "directed-by" --description "Kubrick directed 2001"
```

**Output:** Relation object with ID, Name, Description, FromGroupId, ToGroupId, RelationTypeId, CreatedAt/UpdatedAt

**See also:** `mr relation delete`, `mr relation edit-name`, `mr relation-type create`, `mr group get`

### `mr relation delete`

Delete a relation by ID

```
mr relation delete <id>
```

Delete a Relation by ID. Destructive: removes the link row entirely.
The two groups and the relation-type are unaffected. Deleting a
nonexistent ID returns exit code 1. To confirm the removal, re-fetch
either participating group with `mr group get <id> --json` and check
that the relation no longer appears in its `Relationships` array.

**Arguments:** `<id>`

**Examples:**

```bash
# Delete relation 7
mr relation delete 7
# Delete and pipe the result to jq
mr relation delete 7 --json | jq .
```

**Output:** Status object with id

**See also:** `mr relation create`, `mr relation edit-name`, `mr group get`

### `mr relation edit-description`

Edit a relation's description

```
mr relation edit-description <id> <new-description>
```

Replace a Relation's `Description` field. Takes the relation ID and
the new description as positional arguments; pass an empty string to
clear. Sends `POST /v1/relation/editDescription` and returns
`{id, ok}` on success. There is no `relation get`: to verify, re-fetch
a participating group with `mr group get <id> --json` and read the
description from its `Relationships` array.

**Arguments:** `<id> <new-description>`

**Examples:**

```bash
# Set the description on relation 7
mr relation edit-description 7 "confirmed by archival records"
# Clear the description by passing an empty string
mr relation edit-description 7 ""
```

**Output:** Status object with id (uint) and ok (bool)

**See also:** `mr relation create`, `mr relation edit-name`, `mr group get`

### `mr relation edit-name`

Edit a relation's name

```
mr relation edit-name <id> <new-name>
```

Replace a Relation's `Name` field. Takes the relation ID and the new
name as positional arguments. Sends `POST /v1/relation/editName` and
returns `{id, ok}` on success. There is no `relation get`: to verify,
re-fetch a participating group with `mr group get <id> --json` and
read the name from its `Relationships` array.

**Arguments:** `<id> <new-name>`

**Examples:**

```bash
# Rename relation 7
mr relation edit-name 7 "directed-by"
# Rename and confirm via the source group
mr relation edit-name 7 "produced-by" && \
    mr group get 3 --json | jq -r '.Relationships[] | select(.ID == 7) | .Name'
```

**Output:** Status object with id (uint) and ok (bool)

**See also:** `mr relation create`, `mr relation edit-description`, `mr group get`


---

## `relation-type` — Create, edit, or delete a relation type

A RelationType (`GroupRelationType`) defines the typed link allowed
between two Categories of Groups. Each relation-type has a `Name`
(e.g., "references", "contains", "depends-on"), an optional
`Description`, an optional `ReverseName` for reading the link
backwards, and references to `FromCategory` and `ToCategory`. When a
Relation is created with `mr relation create --relation-type-id <id>`,
the server enforces that the source group belongs to `FromCategory`
and the target group belongs to `ToCategory`.

Use the `relation-type` subcommands to operate on a single relation
type by ID: `create` defines a new typed link, `edit` updates any
field, `edit-name` and `edit-description` are scoped updates, and
`delete` removes the type. There is no `relation-type get`: to read a
relation-type back, use `mr relation-types list --name <substring>`
and filter by ID in jq, or fetch the full list.

### `mr relation-type create`

Create a new relation type

```
mr relation-type create
```

Create a new RelationType defining a typed link between two Categories.
`--name` is required. `--from-category` and `--to-category` take
Category IDs (not names); when set, the server enforces that relations
of this type link groups of those categories. `--description` is
free-form text shown in UIs. `--reverse-name` stores a readable label
for traversing the link in the opposite direction. Sends `POST
/v1/relationType` and returns the persisted record.

**Flags:**

- `--name` (string) **(required)** — Relation type name (required)
- `--description` (string) — Relation type description
- `--reverse-name` (string) — Reverse relation name
- `--from-category` (uint) — From category ID
- `--to-category` (uint) — To category ID

**Examples:**

```bash
# Create a basic relation type between two category IDs
mr relation-type create --name "references" --from-category 1 --to-category 2
# Create with a description and reverse-name
ID=$(mr relation-type create --name "depends-on" --description "A depends on B" \
    --reverse-name "depended-on-by" --from-category 1 --to-category 2 --json | jq -r '.ID')
```

**Output:** RelationType object with ID, Name, Description, FromCategoryId, ToCategoryId, BackRelationId, CreatedAt/UpdatedAt

**See also:** `mr relation-type edit`, `mr relation-types list`, `mr relation create`, `mr category create`

### `mr relation-type delete`

Delete a relation type by ID

```
mr relation-type delete <id>
```

Delete a RelationType by ID. Destructive: removes the type row
entirely. Existing Relations that reference this type may be orphaned
or cascade-deleted depending on the server's foreign-key configuration;
inspect affected groups with `mr group get <id> --json` after a
delete. Deleting a nonexistent ID returns exit code 1.

**Arguments:** `<id>`

**Examples:**

```bash
# Delete relation-type 5
mr relation-type delete 5
# Delete and pipe the result to jq
mr relation-type delete 5 --json | jq .
```

**Output:** Status object with id

**See also:** `mr relation-type create`, `mr relation-types list`, `mr relation delete`

### `mr relation-type edit`

Edit a relation type

```
mr relation-type edit
```

Edit fields on an existing RelationType. `--id` is required; any other
flag left unset keeps the existing value (partial update). `--name`
and `--description` replace those fields; `--reverse-name` replaces
the reverse label. `--from-category` and `--to-category` rewire the
allowed category pairing; use with caution, as existing relations
using this type may become inconsistent. Sends `POST
/v1/relationType/edit` and returns the full updated record.

**Flags:**

- `--id` (uint) **(required)** — Relation type ID (required)
- `--name` (string) — Relation type name
- `--description` (string) — Relation type description
- `--reverse-name` (string) — Reverse relation name
- `--from-category` (uint) — From category ID
- `--to-category` (uint) — To category ID

**Examples:**

```bash
# Rename a relation type and update its description
mr relation-type edit --id 5 --name "referenced-by" --description "backward link"
# Rewire the target category (relation-type 5 now points to category 7)
mr relation-type edit --id 5 --to-category 7
```

**Output:** RelationType object with ID, Name, Description, FromCategoryId, ToCategoryId, BackRelationId, CreatedAt/UpdatedAt

**See also:** `mr relation-type edit-name`, `mr relation-type edit-description`, `mr relation-types list`

### `mr relation-type edit-description`

Edit a relation type's description

```
mr relation-type edit-description <id> <new-description>
```

Replace a RelationType's `Description` field. Takes the relation-type
ID and the new description as positional arguments; pass an empty
string to clear. Shorthand for `mr relation-type edit --id <id>
--description <value>`. Sends `POST /v1/relationType/editDescription`
and returns `{id, ok}`. There is no `relation-type get`: to verify,
re-read with `mr relation-types list --name <substring>` and inspect
the `.Description` field in jq.

**Arguments:** `<id> <new-description>`

**Examples:**

```bash
# Set the description on relation-type 5
mr relation-type edit-description 5 "references another record"
# Clear the description by passing an empty string
mr relation-type edit-description 5 ""
```

**Output:** Status object with id (uint) and ok (bool)

**See also:** `mr relation-type edit`, `mr relation-type edit-name`, `mr relation-types list`

### `mr relation-type edit-name`

Edit a relation type's name

```
mr relation-type edit-name <id> <new-name>
```

Replace a RelationType's `Name` field. Takes the relation-type ID and
the new name as positional arguments. Shorthand for `mr relation-type
edit --id <id> --name <value>` when name is the only change. Sends
`POST /v1/relationType/editName` and returns `{id, ok}` on success.
There is no `relation-type get`: to verify, re-read with
`mr relation-types list --name <substring>` and match the ID in jq.

**Arguments:** `<id> <new-name>`

**Examples:**

```bash
# Rename relation-type 5
mr relation-type edit-name 5 "references"
# Rename and confirm via a filtered list
mr relation-type edit-name 5 "contains" && \
    mr relation-types list --name "contains" --json | jq -r '.[] | select(.ID == 5) | .Name'
```

**Output:** Status object with id (uint) and ok (bool)

**See also:** `mr relation-type edit`, `mr relation-type edit-description`, `mr relation-types list`


---

## `relation-types` — List relation types

Discover RelationTypes. The `relation-types` group currently exposes
only `list` for paginated, filterable reads. Use `relation-type`
(singular) for create/edit/delete operations on a specific type.

List results power downstream workflows: pipe `relation-types list
--json` into jq to pick an ID by name, then pass it to `mr relation
create --relation-type-id <id>` when linking two groups.

### `mr relation-types list`

List relation types

```
mr relation-types list
```

List RelationTypes, optionally filtered. `--name` and `--description`
do substring matches on those fields. Pagination via the global
`--page` flag (default page size 50). Use the JSON output to feed
scripted workflows: look up a type ID by name and pass it to
`mr relation create --relation-type-id <id>`.

**Flags:**

- `--name` (string) — Filter by name
- `--description` (string) — Filter by description

**Examples:**

```bash
# List all relation types (paged)
mr relation-types list
# Filter by name substring
mr relation-types list --name references
# JSON output + jq to extract the ID for a known name
mr relation-types list --name "depends-on" --json | jq -r '.[0].ID'
```

**Output:** Array of relation types with ID, Name, Description, FromCategoryId, ToCategoryId, CreatedAt

**See also:** `mr relation-type create`, `mr relation-type edit`, `mr relation create`, `mr categories list`


---

## `tag` — Get, create, edit, or delete a tag

Tags are lightweight labels attached to Resources, Notes, and Groups.
A Tag has a name and optional description; the name is the user-visible
handle. Tags are the primary way to categorize content across entity
types and are commonly used as filter selectors in list and timeline
commands.

Use the `tag` subcommands to operate on a single tag by ID: fetch it,
create a new one, rename or redescribe it, or delete it. Use
`tags list` to discover tags and `tags merge` to fold a tag's
relationships into another.

### `mr tag create`

Create a new tag

```
mr tag create
```

Create a new tag. `--name` is required and must be unique; `--description`
is optional free-form text. On success prints a confirmation line with
the new ID; pass the global `--json` flag to emit the full record for
scripting (e.g., piping the new ID into follow-up commands).

**Flags:**

- `--name` (string) **(required)** — Tag name (required)
- `--description` (string) — Tag description

**Examples:**

```bash
# Create a tag with just a name
mr tag create --name "urgent"
# Create with a description and capture the ID via jq
ID=$(mr tag create --name "archived" --description "archived items" --json | jq -r .ID)
```

**Output:** Created Tag object with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt

**See also:** `mr tag get`, `mr tag edit-name`, `mr tags list`

### `mr tag delete`

Delete a tag by ID

```
mr tag delete <id>
```

Delete a tag by ID. Destructive: removes the tag row and detaches it
from any Resources, Notes, or Groups it was attached to (the related
entities themselves are preserved). Deleting a nonexistent ID is a
no-op on the server but still returns success.

**Arguments:** `<id>`

**Examples:**

```bash
# Delete a tag by ID
mr tag delete 42
# Delete and pipe the result to jq to confirm the response shape
mr tag delete 42 --json | jq .
```

**See also:** `mr tags delete`, `mr tag get`, `mr tag create`

### `mr tag edit-description`

Edit a tag's description

```
mr tag edit-description <id> <new-description>
```

Update the description of an existing tag. Takes two positional
arguments: the tag ID and the new description. Passing an empty string
clears the description. Useful for annotating tags used across many
resources without renaming them.

**Arguments:** `<id> <new-description>`

**Examples:**

```bash
# Set a description on tag 42
mr tag edit-description 42 "used for Q1 2026 scans"
# Clear the description by passing an empty string
mr tag edit-description 42 ""
```

**See also:** `mr tag edit-name`, `mr tag get`, `mr tags list`

### `mr tag edit-name`

Edit a tag's name

```
mr tag edit-name <id> <new-name>
```

Update the name of an existing tag. Takes two positional arguments: the
tag ID and the new name. The name must remain unique across tags; the
server rejects duplicates. To rename and verify in one step, chain with
`mr tag get <id> --json`.

**Arguments:** `<id> <new-name>`

**Examples:**

```bash
# Rename tag 42
mr tag edit-name 42 "important"
# Rename and confirm with a follow-up get
mr tag edit-name 42 "renamed" && mr tag get 42 --json | jq -r .Name
```

**See also:** `mr tag edit-description`, `mr tag get`, `mr tags list`

### `mr tag get`

Get a tag by ID

```
mr tag get <id>
```

Get a tag by ID and print its fields. The server has no single-tag GET
endpoint, so the CLI fetches the full tag list and filters in-process;
on large instances this is slower than a direct lookup would be. Output
is a key/value table by default; pass the global `--json` flag to emit
the raw record for scripting.

**Arguments:** `<id>`

**Examples:**

```bash
# Get a tag by ID (table output)
mr tag get 42
# Get as JSON and extract the name with jq
mr tag get 42 --json | jq -r .Name
```

**Output:** Tag object with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt

**See also:** `mr tag create`, `mr tag edit-name`, `mr tags list`


---

## `tags` — List, merge, or bulk-delete tags

Discover and bulk-manage Tags. The `tags` subcommands operate across
multiple tags: `list` for filtered queries (with pagination via global
`--page`), `merge` for folding one or more tags into a single winner,
`delete` for bulk removal, and `timeline` for an activity histogram.

Selection for destructive commands is by ID: `merge` uses
`--winner` / `--losers`, `delete` uses `--ids`. Pipe `tags list --json`
through `jq` when you need to derive IDs from a filter.

### `mr tags delete`

Delete multiple tags

```
mr tags delete
```

Bulk-delete Tags. Destructive: removes the tag rows and detaches them
from any Resources, Notes, or Groups they were attached to (the related
entities themselves are preserved). Target tags are selected via
`--ids` (CSV of unsigned ints). The current CLI has no dry-run; pipe
`tags list --json` first if you need to preview targets.

**Flags:**

- `--ids` (string) **(required)** — Comma-separated tag IDs to delete (required)

**Examples:**

```bash
# Delete specific tags
mr tags delete --ids 42,43,44
# Delete all tags matching a name filter
mr tags delete --ids $(mr tags list --name "obsolete-" --json | jq -r 'map(.ID) | join(",")')
```

**See also:** `mr tag delete`, `mr tags merge`, `mr tags list`

### `mr tags list`

List tags

```
mr tags list
```

List Tags, optionally filtered by name or description. The `--name` and
`--description` flags do substring matching on the server. Results are
paginated via the global `--page` flag (default page size 50). Default
output is a table with ID, NAME, DESCRIPTION, and CREATED columns; pass
`--json` for the full array.

**Flags:**

- `--name` (string) — Filter by name
- `--description` (string) — Filter by description

**Examples:**

```bash
# List all tags (first page)
mr tags list
# Filter by name substring
mr tags list --name urgent
# JSON output piped into jq
mr tags list --json | jq -r '.[].Name'
```

**Output:** Array of Tag objects with ID, Name, Description, CreatedAt, UpdatedAt

**See also:** `mr tag get`, `mr tags timeline`, `mr resources list`

### `mr tags merge`

Merge tags into a winner

```
mr tags merge
```

Merge one or more "loser" tags into a single "winner". The winner's ID
and name are preserved; Resources, Notes, and Groups previously tagged
with any loser are re-tagged with the winner; the loser tag rows are
then deleted. Use to consolidate duplicate or redundant tags (e.g.,
`photo` and `photos`) without losing associations.

**Flags:**

- `--winner` (uint) **(required)** — Winning tag ID (required)
- `--losers` (string) **(required)** — Comma-separated loser tag IDs (required)

**Examples:**

```bash
# Merge tags 2 and 3 into winner 1
mr tags merge --winner 1 --losers 2,3
# Merge the result of a filter
mr tags merge --winner 1 --losers $(mr tags list --name "dup-" --json | jq -r 'map(.ID) | join(",")')
```

**See also:** `mr tags delete`, `mr tags list`, `mr resources merge`

### `mr tags timeline`

Display a timeline of tag activity

```
mr tags timeline
```

Display a timeline of Tag activity as an ASCII bar chart. Each bar
represents a time bucket (yearly, monthly, or weekly, controlled by
`--granularity`), and the bar height reflects the count of Tags created
in that bucket.

The chart is anchored at the `--anchor` date (default: today) and shows
`--columns` buckets backward from the anchor (default 15, max 60). The
`--name` and `--description` filter flags apply the same substring
matching as `tags list`. Pass the global `--json` flag to get the raw
bucket data for scripting.

**Flags:**

- `--granularity` (string) (default `monthly`) — Bucket granularity: yearly, monthly, or weekly
- `--anchor` (string) — Anchor date (YYYY-MM-DD); defaults to today
- `--columns` (int) (default `15`) — Number of timeline buckets (max 60)
- `--name` (string) — Filter by name
- `--description` (string) — Filter by description

**Examples:**

```bash
# Monthly timeline anchored at today (default)
mr tags timeline
# Weekly granularity
mr tags timeline --granularity weekly --columns 12
# Yearly timeline anchored at a specific date
mr tags timeline --granularity yearly --anchor 2020-01-01 --json
```

**Output:** Object with buckets ([]{label, start, end, created, updated})

**See also:** `mr tags list`, `mr resources timeline`


---

## `category` — Get, create, edit, or delete a group category

Categories are labels that classify Groups (distinct from ResourceCategory
which labels Resources). A Category has a name, optional description, and
optional presentation fields (CustomHeader, CustomCSS, CustomSidebar,
CustomSummary, CustomAvatar, CustomMRQLResult) plus a MetaSchema JSON that
Groups assigned to this category inherit for structured metadata.

Use the `category` subcommands to operate on a single Category by ID:
fetch it, create a new one, rename or redescribe it, or delete it. Use
`categories list` to discover categories and `categories timeline` to
view creation activity over time.

### `mr category create`

Create a new category

```
mr category create
```

Create a new Category. `--name` is required; `--description` is optional
free-form text. The optional `--custom-header`, `--custom-css`,
`--custom-sidebar`, `--custom-summary`, `--custom-avatar`, and
`--custom-mrql-result` flags accept template or HTML strings applied to
Groups assigned to this category. `--custom-css` is injected as a
`<style>` block on detail and list pages. `--meta-schema` and
`--section-config` take JSON strings
controlling structured metadata and which sections render on group
detail pages. On success prints a confirmation line with the new ID;
pass the global `--json` flag to emit the full record for scripting.

**Flags:**

- `--name` (string) **(required)** — Category name (required)
- `--description` (string) — Category description
- `--custom-header` (string) — Custom header HTML
- `--custom-css` (string) — Custom CSS injected as a <style> block on detail/list pages
- `--custom-sidebar` (string) — Custom sidebar HTML
- `--custom-summary` (string) — Custom summary HTML
- `--custom-avatar` (string) — Custom avatar HTML
- `--meta-schema` (string) — Meta schema JSON
- `--section-config` (string) — JSON controlling which sections are visible on group detail pages for this category
- `--custom-mrql-result` (string) — Pongo2 template for rendering groups of this category in MRQL results

**Examples:**

```bash
# Create a category with just a name
mr category create --name "Project"
# Create with a description and capture the ID via jq
ID=$(mr category create --name "Location" --description "Places you know about" --json | jq -r .ID)
```

**Output:** Created Category object with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt

**See also:** `mr category get`, `mr category edit-name`, `mr categories list`

### `mr category delete`

Delete a category by ID

```
mr category delete <id>
```

Delete a Category by ID. Destructive: removes the category row. Groups
previously assigned to this category become uncategorized (the group
records themselves are preserved). Deleting a nonexistent ID is a no-op
on the server but still returns success.

**Arguments:** `<id>`

**Examples:**

```bash
# Delete a category by ID
mr category delete 42
# Delete and pipe the result to jq to confirm the response shape
mr category delete 42 --json | jq .
```

**See also:** `mr category get`, `mr category create`, `mr categories list`

### `mr category edit-description`

Edit a category's description

```
mr category edit-description <id> <new-description>
```

Update the description of an existing Category. Takes two positional
arguments: the category ID and the new description. Passing an empty
string clears the description. Useful for annotating categories with
guidance about what Groups belong under them without renaming.

**Arguments:** `<id> <new-description>`

**Examples:**

```bash
# Set a description on category 42
mr category edit-description 42 "places and venues"
# Clear the description by passing an empty string
mr category edit-description 42 ""
```

**See also:** `mr category edit-name`, `mr category get`, `mr categories list`

### `mr category edit-name`

Edit a category's name

```
mr category edit-name <id> <new-name>
```

Update the name of an existing Category. Takes two positional arguments:
the category ID and the new name. The name must remain unique across
categories; the server rejects duplicates. To rename and verify in one
step, chain with `mr category get <id> --json`.

**Arguments:** `<id> <new-name>`

**Examples:**

```bash
# Rename category 42
mr category edit-name 42 "Projects"
# Rename and confirm with a follow-up get
mr category edit-name 42 "renamed" && mr category get 42 --json | jq -r .Name
```

**See also:** `mr category edit-description`, `mr category get`, `mr categories list`

### `mr category get`

Get a category by ID

```
mr category get <id>
```

Get a category by ID and print its fields. The server has no single-category
GET endpoint, so the CLI fetches the full category list and filters
in-process; on large instances this is slower than a direct lookup would be.
Output is a key/value table by default; pass the global `--json` flag to
emit the raw record for scripting.

**Arguments:** `<id>`

**Examples:**

```bash
# Get a category by ID (table output)
mr category get 42
# Get as JSON and extract the name with jq
mr category get 42 --json | jq -r .Name
```

**Output:** Category object with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt

**See also:** `mr category create`, `mr category edit-name`, `mr categories list`


---

## `categories` — List group categories

Discover and inspect Categories. The `categories` subcommands operate
across multiple categories: `list` for filtered queries (with pagination
via the global `--page` flag) and `timeline` for an ASCII histogram of
category creation activity.

The CLI has no bulk-mutate variants for categories; use the singular
`category` commands (`create`, `delete`, `edit-name`, `edit-description`)
and pipe `categories list --json` through `jq` when you need to derive
IDs from a filter.

### `mr categories list`

List categories

```
mr categories list
```

List Categories, optionally filtered by name or description. The `--name`
and `--description` flags do substring matching on the server. Results
are paginated via the global `--page` flag (default page size 50).
Default output is a table with ID, NAME, DESCRIPTION, and CREATED
columns; pass `--json` for the full array.

**Flags:**

- `--name` (string) — Filter by name
- `--description` (string) — Filter by description

**Examples:**

```bash
# List all categories (first page)
mr categories list
# Filter by name substring
mr categories list --name Project
# JSON output piped into jq
mr categories list --json | jq -r '.[].Name'
```

**Output:** Array of Category objects with ID, Name, Description, CreatedAt, UpdatedAt

**See also:** `mr category get`, `mr categories timeline`, `mr groups list`

### `mr categories timeline`

Display a timeline of category activity

```
mr categories timeline
```

Display a timeline of Category activity as an ASCII bar chart. Each bar
represents a time bucket (yearly, monthly, or weekly, controlled by
`--granularity`), and the bar height reflects the count of Categories
created in that bucket.

The chart is anchored at the `--anchor` date (default: today) and shows
`--columns` buckets backward from the anchor (default 15, max 60). The
`--name` and `--description` filter flags apply the same substring
matching as `categories list`. Pass the global `--json` flag to get the
raw bucket data for scripting.

**Flags:**

- `--granularity` (string) (default `monthly`) — Bucket granularity: yearly, monthly, or weekly
- `--anchor` (string) — Anchor date (YYYY-MM-DD); defaults to today
- `--columns` (int) (default `15`) — Number of timeline buckets (max 60)
- `--name` (string) — Filter by name
- `--description` (string) — Filter by description

**Examples:**

```bash
# Monthly timeline anchored at today (default)
mr categories timeline
# Weekly granularity
mr categories timeline --granularity weekly --columns 12
# Yearly timeline anchored at a specific date
mr categories timeline --granularity yearly --anchor 2020-01-01 --json
```

**Output:** Object with buckets ([]{label, start, end, created, updated})

**See also:** `mr categories list`, `mr tags timeline`, `mr resources timeline`


---

## `resource-category` — Get, create, edit, or delete a resource category

A ResourceCategory is a taxonomy label attached to Resources. It has a
name, optional description, and a range of optional presentation fields
(custom header, CSS, sidebar, summary, avatar, MRQL result template) plus a
MetaSchema and SectionConfig used to shape resource detail pages for
resources in this category. Resource categories are distinct from
Categories, which label Groups.

Use the `resource-category` subcommands to operate on a single category
by ID: fetch it, create a new one, rename or redescribe it, or delete
it. Use `resource-categories list` to discover categories matching
filters.

### `mr resource-category create`

Create a new resource category

```
mr resource-category create
```

Create a new resource category. `--name` is required; all other flags
are optional, including a plain `--description`, presentation
fields (`--custom-header`, `--custom-css`, `--custom-sidebar`,
`--custom-summary`, `--custom-avatar`, `--custom-mrql-result`) and
structural fields (`--meta-schema`, `--section-config`). `--custom-css`
is injected as a `<style>` block on detail and list pages. On success
prints a confirmation line with the new ID; pass the global `--json`
flag to emit the full record for scripting.

**Flags:**

- `--name` (string) **(required)** — Resource category name (required)
- `--description` (string) — Resource category description
- `--custom-header` (string) — Custom header HTML
- `--custom-css` (string) — Custom CSS injected as a <style> block on detail/list pages
- `--custom-sidebar` (string) — Custom sidebar HTML
- `--custom-summary` (string) — Custom summary HTML
- `--custom-avatar` (string) — Custom avatar HTML
- `--meta-schema` (string) — Meta schema JSON
- `--section-config` (string) — JSON controlling which sections are visible on resource detail pages for this category
- `--custom-mrql-result` (string) — Pongo2 template for rendering resources of this category in MRQL results

**Examples:**

```bash
# Create a resource category with just a name
mr resource-category create --name "Photos"
# Create with a description and capture the ID via jq
ID=$(mr resource-category create --name "Scans" --description "scanned documents" --json | jq -r .ID)
```

**Output:** Created ResourceCategory object with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt

**See also:** `mr resource-category get`, `mr resource-category edit-name`, `mr resource-categories list`

### `mr resource-category delete`

Delete a resource category by ID

```
mr resource-category delete <id>
```

Delete a resource category by ID. Destructive: removes the resource
category row. Resources that reference this category remain but lose
their category association. Deleting a nonexistent ID may still return
success at the server level.

**Arguments:** `<id>`

**Examples:**

```bash
# Delete a resource category by ID
mr resource-category delete 42
# Delete and pipe the result to jq to inspect the response
mr resource-category delete 42 --json | jq .
```

**See also:** `mr resource-category get`, `mr resource-category create`, `mr resource-categories list`

### `mr resource-category edit-description`

Edit a resource category's description

```
mr resource-category edit-description <id> <new-description>
```

Update the description of an existing resource category. Takes two
positional arguments: the resource category ID and the new description.
Passing an empty string clears the description. Useful for annotating
categories used across many resources without renaming them.

**Arguments:** `<id> <new-description>`

**Examples:**

```bash
# Set a description on resource category 42
mr resource-category edit-description 42 "high-resolution scans"
# Clear the description by passing an empty string
mr resource-category edit-description 42 ""
```

**See also:** `mr resource-category edit-name`, `mr resource-category get`, `mr resource-categories list`

### `mr resource-category edit-name`

Edit a resource category's name

```
mr resource-category edit-name <id> <new-name>
```

Update the name of an existing resource category. Takes two positional
arguments: the resource category ID and the new name. The name should
remain unique across resource categories. To rename and verify in one
step, chain with `mr resource-category get <id> --json`.

**Arguments:** `<id> <new-name>`

**Examples:**

```bash
# Rename resource category 42
mr resource-category edit-name 42 "Photos"
# Rename and confirm with a follow-up get
mr resource-category edit-name 42 "renamed" && mr resource-category get 42 --json | jq -r .Name
```

**See also:** `mr resource-category edit-description`, `mr resource-category get`, `mr resource-categories list`

### `mr resource-category get`

Get a resource category by ID

```
mr resource-category get <id>
```

Get a resource category by ID and print its fields. The server has no
single-resource-category GET endpoint, so the CLI fetches the full list
and filters in-process; on large instances this is slower than a direct
lookup would be. Output is a key/value table by default; pass the
global `--json` flag to emit the raw record for scripting.

**Arguments:** `<id>`

**Examples:**

```bash
# Get a resource category by ID (table output)
mr resource-category get 42
# Get as JSON and extract the name with jq
mr resource-category get 42 --json | jq -r .Name
```

**Output:** ResourceCategory object with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt

**See also:** `mr resource-category create`, `mr resource-category edit-name`, `mr resource-categories list`


---

## `resource-categories` — List resource categories

Discover ResourceCategories. The `resource-categories` subcommand group
currently exposes `list` for filtered queries against the full set of
resource categories, with pagination via the global `--page` flag.

Resource categories are the per-Resource taxonomy (compare `categories`
for per-Group). Use `resource-categories list --json | jq` to derive
IDs for scripting, and `resource-category` for single-category CRUD.

### `mr resource-categories list`

List resource categories

```
mr resource-categories list
```

List Resource Categories, optionally filtered by name or description.
The `--name` and `--description` flags do substring matching on the
server. Results are paginated via the global `--page` flag (default
page size 50). Default output is a table with ID, NAME, DESCRIPTION,
and CREATED columns; pass `--json` for the full array.

**Flags:**

- `--name` (string) — Filter by name
- `--description` (string) — Filter by description

**Examples:**

```bash
# List all resource categories (first page)
mr resource-categories list
# Filter by name substring
mr resource-categories list --name photos
# JSON output piped into jq
mr resource-categories list --json | jq -r '.[].Name'
```

**Output:** Array of ResourceCategory objects with ID, Name, Description, CreatedAt, UpdatedAt

**See also:** `mr resource-category get`, `mr resource-category create`, `mr resources list`


---

## `mrql` — Execute and manage MRQL queries

For the complete DSL syntax reference (operators, fields, GROUP BY, SCOPE, traversal), see the [MRQL Reference](https://egeozcan.github.io/mahresources/features/mrql-reference) docs-site page.

MRQL (Mahresources Query Language) is a small DSL for querying the
mahresources data model across Resources, Notes, and Groups. A single
expression selects an entity type and applies filters, scope, ordering,
limit, and optional `GROUP BY` aggregations — for example
`type = resource AND tags = "photo"` or
`type = resource GROUP BY contentType COUNT()`.

The top-level `mrql` command executes a one-off query supplied as a
positional argument, via `-f <file>`, or on stdin with `-`. Use the
subcommands to manage saved queries: `save` to register a named query,
`list` to discover them, `run` to execute a saved query by name or ID,
and `delete` to remove one. Saved MRQL queries differ from SQL-based
`query` records (see `query run`): MRQL is the high-level DSL, whereas
`query` executes raw read-only SQL.

### `mr mrql delete`

Delete a saved MRQL query by ID

```
mr mrql delete <id>
```

Delete a saved MRQL query by numeric ID. Destructive: removes the
database row for the saved query. Any downstream references (bookmarks,
dashboards, or `[mrql saved="..."]` shortcodes) must be updated
separately — the server does not rewrite them. Deleting a nonexistent
ID returns exit code 1.

Unlike `mrql run`, the delete subcommand only accepts a numeric ID; pass
`mrql list --json | jq -r '.[] | select(.name == "...") | .id'` to
resolve a name to its ID first.

**Arguments:** `<id>`

**Examples:**

```bash
# Delete a saved query by ID
mr mrql delete 42
# Delete and inspect the response with jq
mr mrql delete 42 --json | jq .
```

**Output:** Object with id (uint) of the deleted saved MRQL query

**See also:** `mr mrql save`, `mr mrql list`, `mr mrql run`

### `mr mrql list`

List saved MRQL queries

```
mr mrql list
```

List saved MRQL queries. Pagination is controlled via the global
`--page` flag (default page size 50). Use the global `--json` flag to
retrieve the raw array for scripting; the default table output shows
ID, name, description (truncated), and creation timestamp.

To execute a listed query, use `mrql run <name-or-id>`. To inspect the
stored MRQL text itself, use `mrql list --json` and extract the `.query`
field — there is no dedicated `mrql get` subcommand.

**Examples:**

```bash
# List all saved MRQL queries (first page)
mr mrql list
# JSON + jq: print each saved query's id
mr mrql list --json | jq -r '.[] | "\(.id)\t\(.name)\t\(.query)"'
```

**Output:** Array of saved MRQL query objects with id, name, query, description, createdAt, updatedAt

**See also:** `mr mrql save`, `mr mrql run`, `mr mrql delete`

### `mr mrql run`

Run a saved MRQL query by name or ID

```
mr mrql run <name-or-id>
```

Execute a saved MRQL query by name or numeric ID. The argument is tried
as an ID first and then as a name, so a name that happens to be numeric
can still be resolved. Returns the same shape as a one-off `mrql` call:
either a standard result with an `entityType` plus the matching entity
arrays, or — for `GROUP BY` queries — a grouped result with `mode`
(`aggregated` or `bucketed`) and `rows` / `groups`.

Pagination and shaping flags (`--limit`, `--buckets`, `--offset`, plus
the global `--page`) apply to the stored query exactly as they would to
an inline `mrql` invocation. Pass `--render` to request server-side
template rendering via the `CustomMRQLResult` template. A missing ID or
name returns HTTP 404.

This is distinct from `query run`, which executes SQL-backed Query
records rather than MRQL DSL expressions.

**Arguments:** `<name-or-id>`

**Flags:**

- `--limit` (int) — Items per bucket for GROUP BY, or total items for regular queries
- `--buckets` (int) — Groups per page for bucketed GROUP BY queries
- `--offset` (int) — Bucket offset for cursor-based GROUP BY pagination
- `--render` (bool) — Request server-side template rendering via CustomMRQLResult

**Examples:**

```bash
# Run a saved query by ID
mr mrql run 42
# Run by name with bucketed GROUP BY pagination
mr mrql run "resources-by-type" --buckets 5
# Run and extract result ids with jq
mr mrql run "recent-photos" --json | jq -r '.resources[].ID'
```

**Output:** MRQL result object with entityType (string) and resources/notes/groups arrays, or a grouped result with mode + rows/groups for GROUP BY queries

**See also:** `mr mrql save`, `mr mrql list`, `mr query run`

### `mr mrql save`

Save a MRQL query

```
mr mrql save <name> <query>
```

Save a named MRQL query for later reuse. Takes two positional arguments:
`<name>` (a unique label) and `<query>` (the MRQL text). The optional
`--description` flag attaches a human-readable note. The query text is
validated at save time — malformed MRQL returns HTTP 400 with a parse
error pointing at the offending token, and the record is not persisted.

The created record is returned; capture `.id` from JSON output to run
or delete the query in follow-up commands. Saved queries can be executed
by ID or by name via `mrql run`.

**Arguments:** `<name> <query>`

**Flags:**

- `--description` (string) — Description for the saved query

**Examples:**

```bash
# Save a simple named query
mr mrql save "recent-photos" 'type = resource AND tags = "photo"'
# Save with a description
mr mrql save "resources-by-type" 'type = resource GROUP BY contentType COUNT()' --description "Resource count per content type"
```

**Output:** Created saved MRQL query object with id (uint), name (string), query (string), description (string), createdAt, updatedAt

**See also:** `mr mrql list`, `mr mrql run`, `mr mrql delete`


---

## `query` — Get, create, run, or delete a saved query

A Query is a saved, named search definition. Queries store SQL text
(with optional template interpolation) that can be re-executed on
demand against the mahresources database. Each Query has an ID, name,
description, the SQL Text itself, and an optional Template. Queries
are read-only: `run` executes against a read-only database handle and
returns rows as JSON objects.

Use the `query` subcommands to operate on a single query by ID:
`create` to register new SQL, `get` to fetch metadata, `edit-name` /
`edit-description` to update fields, `run` / `run-by-name` to execute,
and `schema` to inspect the available tables and columns when
authoring query text. Use `queries list` to discover existing queries.

### `mr query create`

Create a new query

```
mr query create
```

Create a new saved query. Requires `--name` (unique label) and
`--text` (the SQL body). `--template` is optional and lets you embed
a Pongo2 template that receives the query's result rows for custom
rendering in the web UI. Query Text runs against a read-only handle
when executed; writes to the database via `query run` are rejected.

**Flags:**

- `--name` (string) **(required)** — Query name (required)
- `--text` (string) **(required)** — Query text/SQL (required)
- `--template` (string) — Query template

**Examples:**

```bash
# Create a minimal query
mr query create --name "count-resources" --text "select count(*) as n from resources"
# Create with a template for custom display
mr query create --name "recent-notes" --text "select id, name from notes order by created_at desc limit 10" --template "{{ rows|length }} rows"
```

**Output:** Created query object with ID, Name, Text, Template, Description, CreatedAt, UpdatedAt

**See also:** `mr query get`, `mr query run`, `mr query delete`, `mr queries list`

### `mr query delete`

Delete a query by ID

```
mr query delete <id>
```

Delete a saved query by ID. Destructive: removes the database row
for the query. Any downstream references (saved dashboards, bookmarks)
should be updated separately. Deleting a nonexistent ID returns exit
code 1.

**Arguments:** `<id>`

**Examples:**

```bash
# Delete a query by ID
mr query delete 42
# Delete and pipe the result to jq to confirm
mr query delete 42 --json | jq .
```

**See also:** `mr query get`, `mr query create`, `mr queries list`

### `mr query edit-description`

Edit a query's description

```
mr query edit-description <id> <value>
```

Update the description of an existing saved query. Passing an empty
string clears the description. Description is metadata only and does
not affect execution.

**Arguments:** `<id> <value>`

**Examples:**

```bash
# Set the description on query 42
mr query edit-description 42 "Counts resources grouped by content type"
# Clear the description by passing an empty string
mr query edit-description 42 ""
```

**See also:** `mr query get`, `mr query edit-name`, `mr queries list`

### `mr query edit-name`

Edit a query's name

```
mr query edit-name <id> <value>
```

Update the name of an existing saved query. Query names are used by
`query run-by-name`, so renaming a query breaks callers that reference
it by the old name. Shorthand for a direct field update; does not
modify the query Text or Template.

**Arguments:** `<id> <value>`

**Examples:**

```bash
# Rename query 42
mr query edit-name 42 "count-resources-v2"
# Rename and confirm with a follow-up get
mr query edit-name 42 "renamed" && mr query get 42 --json | jq -r .Name
```

**See also:** `mr query get`, `mr query edit-description`, `mr queries list`

### `mr query get`

Get a query by ID

```
mr query get <id>
```

Get a saved query by ID and print its metadata. Fetches the full
record including Name, Text (the SQL), Template, Description, and
created/updated timestamps. Output is a key/value table by default;
pass the global `--json` flag to get the full record for scripting.

**Arguments:** `<id>`

**Examples:**

```bash
# Get a query by ID (table output)
mr query get 42
# Get as JSON and extract the SQL text
mr query get 42 --json | jq -r .Text
```

**Output:** Query object with ID (uint), Name (string), Text (string), Template (string), Description (string), CreatedAt, UpdatedAt

**See also:** `mr query run`, `mr query edit-name`, `mr query edit-description`, `mr queries list`

### `mr query run`

Run a query by ID

```
mr query run <id>
```

Execute a saved query by ID and return the rows as JSON. The query
runs against a read-only database handle: any attempt to write
(INSERT/UPDATE/DELETE/DDL) is rejected. Column names in the result
come verbatim from the SELECT list, so use explicit column aliases
(`select count(*) as n ...`) to produce predictable keys.

Returns `400 Bad Request` if the SQL fails to execute and `404 Not
Found` if the given ID does not exist. For templated queries, the
request body/form values are bound as named SQL parameters.

**Arguments:** `<id>`

**Examples:**

```bash
# Run a query by ID and print the raw JSON array
mr query run 42
# Run and extract the first row's count column with jq
mr query run 42 --json | jq '.[0].n'
```

**Output:** Array of row objects; each object's keys are the query's selected column names

**See also:** `mr query run-by-name`, `mr query schema`, `mr query get`

### `mr query run-by-name`

Run a query by name

```
mr query run-by-name
```

Execute a saved query by its unique `Name` instead of its numeric
ID. Same semantics as `query run`: read-only handle, 400 on SQL
errors, 404 when the name does not resolve. Useful in scripts where
the ID is not known ahead of time but the name is a stable contract.

Renaming a query via `query edit-name` invalidates callers that
pointed at the old name, so prefer `query run <id>` for
long-running integrations.

**Flags:**

- `--name` (string) **(required)** — Query name (required)

**Examples:**

```bash
# Run by name
mr query run-by-name --name "count-resources"
# Run by name and extract the count column
mr query run-by-name --name "count-resources" --json | jq '.[0].n'
```

**Output:** Array of row objects; each object's keys are the query's selected column names

**See also:** `mr query run`, `mr query get`, `mr queries list`

### `mr query schema`

Show database table and column names for query building

```
mr query schema
```

List every database table and its columns, for use as a reference
when authoring query Text. The response is a single JSON object whose
keys are table names and whose values are arrays of column name
strings. Both user-facing tables (e.g. `resources`, `notes`,
`groups`) and internal FTS/virtual tables appear in the output.

Handy as a quick discovery tool before writing a new saved query or
MRQL expression.

**Examples:**

```bash
# Dump the full schema as JSON
mr query schema
# List only the column names of the `resources` table
mr query schema --json | jq -r '.resources[]'
```

**Output:** Object mapping table name (string) to an array of column names (string[])

**See also:** `mr query create`, `mr query run`, `mr mrql`


---

## `queries` — List saved queries

Discover and summarize saved Queries. The `queries` subcommands
operate on the collection: `list` returns queries (paged via the
global `--page` flag, optionally filtered by `--name`), and `timeline`
aggregates query creation and update activity into an ASCII bar chart.

To execute a query, use `query run <id>` or `query run-by-name --name
<name>` from the singular `query` subtree.

### `mr queries list`

List queries

```
mr queries list
```

List saved Queries, optionally filtered by name. Pagination is
controlled via the global `--page` flag (default page size 50). The
`--name` flag does a substring match on query names (SQL `LIKE`
under the hood). Use the global `--json` flag to retrieve the raw
array of query records for scripting; the default table output
truncates long Name/Description cells for readability.

**Flags:**

- `--name` (string) — Filter by name

**Examples:**

```bash
# List all queries (first page)
mr queries list
# Filter by a name substring
mr queries list --name "count"
# JSON + jq: print each query's ID and name
mr queries list --json | jq -r '.[] | "\(.ID)\t\(.Name)"'
```

**Output:** Array of query objects with ID, Name, Text, Template, Description, CreatedAt, UpdatedAt

**See also:** `mr query get`, `mr query run`, `mr queries timeline`

### `mr queries timeline`

Display a timeline of query activity

```
mr queries timeline
```

Display a timeline of saved-Query activity as an ASCII bar chart.
Each bar represents a time bucket (yearly, monthly, or weekly,
controlled by `--granularity`), with bar height reflecting the count
of queries created in that bucket.

The chart is anchored at the `--anchor` date (default: today) and
shows `--columns` buckets backward from the anchor (default 15, max
60). Pass `--name` to filter by query name substring. Pass the
global `--json` flag to get the raw bucket data for scripting.

**Flags:**

- `--granularity` (string) (default `monthly`) — Bucket granularity: yearly, monthly, or weekly
- `--anchor` (string) — Anchor date (YYYY-MM-DD); defaults to today
- `--columns` (int) (default `15`) — Number of timeline buckets (max 60)
- `--name` (string) — Filter by name

**Examples:**

```bash
# Monthly timeline anchored at today (default)
mr queries timeline
# Weekly granularity
mr queries timeline --granularity weekly --columns 12
# Yearly timeline as JSON
mr queries timeline --granularity yearly --json
```

**Output:** Object with buckets array (each bucket has label, start, end, created, updated) and hasMore (left, right)

**See also:** `mr queries list`, `mr resources timeline`, `mr groups timeline`


---

## `search` — Search across all entities

Search across resources, notes, and groups using the server's full-text index. Results are ranked by FTS5 score; the response reports the total number of matches so callers can decide whether to broaden the query or page.

Use `--types` to restrict to a comma-separated subset of entity types (e.g. `--types resources,notes`). Use `--limit` to cap the number of rows returned (default 20). The query string supports FTS5 syntax — phrase queries with double-quoted tokens, boolean operators, and prefix matching with `*`.

### `mr search`

Search across all entities

```
mr search <query>
```

Search across resources, notes, and groups using the server's full-text index. Results are ranked by FTS5 score; the response reports the total number of matches so callers can decide whether to broaden the query or page.

Use `--types` to restrict to a comma-separated subset of entity types (e.g. `--types resources,notes`). Use `--limit` to cap the number of rows returned (default 20). The query string supports FTS5 syntax — phrase queries with double-quoted tokens, boolean operators, and prefix matching with `*`.

**Arguments:** `<query>`

**Flags:**

- `--types` (string) — Comma-separated entity types to search (e.g. resources,notes)
- `--limit` (int) (default `20`) — Maximum number of results

**Examples:**

```bash
# Simple keyword search across all entities
mr search "invoice"
# Restrict to resources only
mr search "invoice" --types resources --json
# Cap results and pipe into jq to read the total
mr search "report" --limit 5 --json | jq '.total'
```

**Output:** Search response {query (string), total (int), results (array of {id, type, name, score, description, url, extra})}

**See also:** `mr mrql run`, `mr resources list`, `mr notes list`, `mr groups list`


---

## `job` — Submit, cancel, pause, or retry a download job

A download job fetches a remote URL and stores the result as a new
Resource. Each submission creates one job per URL; the server downloads
in the background while the queue tracks progress, pause/resume, and
retry state. Jobs are ephemeral — they live in server memory and do not
persist across restarts.

Use the `job` subcommands to operate on a single job by ID: `submit`
new URLs, `cancel` an active job, `pause` / `resume` an in-flight
transfer, or `retry` a failed one. Use `jobs list` to discover IDs and
check current statuses.

### `mr job cancel`

Cancel a job

```
mr job cancel <id>
```

Stop an active download job. Cancel only works while the job is still
in progress (pending, downloading, or processing); the server rejects
cancellation of jobs that have already finished, been cancelled, or are
paused. On success the server marks the job `cancelled` and leaves it
in the queue for inspection.

Use `jobs list` to see which jobs are eligible — any job with a status
other than pending, downloading, or processing cannot be cancelled.

**Arguments:** `<id>`

**Examples:**

```bash
# Cancel a specific job
mr job cancel a1b2c3d4
# Pipe through jq to cancel every active job
mr jobs list --json | jq -r '.jobs[] | select(.status == "downloading" or .status == "pending") | .id' | xargs -I {} mr job cancel {}
```

**Output:** Object with status set to "cancelled"

**See also:** `mr job submit`, `mr job pause`, `mr jobs list`

### `mr job pause`

Pause a job

```
mr job pause <id>
```

Suspend an in-flight download without cancelling it. Pause only works
while the job is pending or downloading; the server rejects pause
requests against finished, cancelled, or already-paused jobs. The
background goroutine stops after the current chunk and the job stays
in the queue with status `paused` until you call `job resume`.

Generic jobs (group exports, imports) cannot be paused — their runners
are not re-entrant. Pause is intended for long URL fetches.

**Arguments:** `<id>`

**Examples:**

```bash
# Pause a specific job
mr job pause a1b2c3d4
# Pause every job currently downloading
mr jobs list --json | jq -r '.jobs[] | select(.status == "downloading") | .id' | xargs -I {} mr job pause {}
```

**Output:** Object with status set to "paused"

**See also:** `mr job resume`, `mr job cancel`, `mr jobs list`

### `mr job resume`

Resume a job

```
mr job resume <id>
```

Restart a previously paused download job. Resume only works against
jobs currently in the `paused` state — jobs that are pending, running,
finished, or cancelled return an error. The server opens a fresh HTTP
request, resets the progress counters, and marks the job `pending`;
the background worker picks it up on the next scheduler tick.

Because the server does not keep partial bytes across pauses, resume
effectively restarts the download from the beginning.

**Arguments:** `<id>`

**Examples:**

```bash
# Resume a specific paused job
mr job resume a1b2c3d4
# Resume every paused job in one pass
mr jobs list --json | jq -r '.jobs[] | select(.status == "paused") | .id' | xargs -I {} mr job resume {}
```

**Output:** Object with status set to "resumed"

**See also:** `mr job pause`, `mr job cancel`, `mr jobs list`

### `mr job retry`

Retry a failed job

```
mr job retry <id>
```

Re-queue a failed or cancelled download job for another attempt.
Retry only works against jobs in the `failed` or `cancelled` state;
the server rejects retry on jobs that are still active, paused, or
already completed. The existing job's ID is reused — progress, error
message, and completion times are cleared, then the worker re-runs the
original URL fetch.

Useful when a transient network error blew up the first attempt.
Persistent failures need an updated URL, which means calling
`job submit` fresh rather than `job retry`.

**Arguments:** `<id>`

**Examples:**

```bash
# Retry a specific failed job
mr job retry a1b2c3d4
# Retry every failed job in the queue
mr jobs list --json | jq -r '.jobs[] | select(.status == "failed") | .id' | xargs -I {} mr job retry {}
```

**Output:** Object with status set to "retrying"

**See also:** `mr job submit`, `mr jobs list`, `mr job cancel`

### `mr job submit`

Submit URLs for download

```
mr job submit
```

Submit one or more URLs to the download queue. The server creates one
job per URL and immediately begins fetching in the background; this
command returns as soon as the jobs are queued, not when downloads
finish. Use `--urls` with a comma-separated list; attach tags, groups,
an owner, or a custom name with the remaining flags.

Downloaded content becomes a new Resource once the fetch succeeds. Watch
progress with `jobs list` or the `/v1/download/events` SSE stream.

**Flags:**

- `--urls` (string) **(required)** — Comma-separated URLs to download (required)
- `--tags` (string) — Comma-separated tag IDs
- `--groups` (string) — Comma-separated group IDs
- `--name` (string) — Job name
- `--owner-id` (uint) — Owner group ID

**Examples:**

```bash
# Queue a single download
mr job submit --urls https://example.com/photo.jpg
# Queue multiple URLs with tags and an owner group
mr job submit --urls https://a.example.com/a.jpg,https://b.example.com/b.jpg --tags 5,7 --owner-id 3
```

**Output:** Object with queued=true and a jobs array containing each created job's id, url, and initial status

**See also:** `mr jobs list`, `mr job cancel`, `mr job retry`


---

## `jobs` — View the download job queue

The download queue is the server's in-memory list of URL download jobs. Each
job tracks a source URL, a status (pending, downloading, paused, completed,
failed, cancelled), progress counters, and the resulting Resource ID once
finished.

The plural `jobs` command group exposes read-only views of the queue. Use
`jobs list` for a full snapshot of every job the server is tracking. For
lifecycle controls (submit, pause, resume, retry, cancel) on a single job,
use the singular `job` subcommands.

### `mr jobs list`

List the download queue

```
mr jobs list
```

Return a snapshot of every job the server is currently tracking,
including pending, running, paused, finished, and failed ones. The
response is a single object whose `jobs` key is an array ordered by
submission time. Each entry exposes enough detail to drive CLI
dashboards, pause/resume decisions, or cleanup scripts.

The queue lives in server memory; a restart empties it. Pagination is
not supported — the full list is returned in one response.

**Examples:**

```bash
# Show every job (human-readable)
mr jobs list
# Filter to still-running jobs and pull just their URLs
mr jobs list --json | jq -r '.jobs[] | select(.status == "downloading" or .status == "pending") | .url'
```

**Output:** Object with a jobs array; each entry has id, url, status, progress, totalSize, progressPercent, createdAt, and optional error, startedAt, completedAt, resourceId, source

**See also:** `mr job submit`, `mr job cancel`, `mr job retry`


---

## `log` — View a log entry or entity history

The activity log is an append-only record of create, update, and delete
events the server writes as resources, notes, groups, tags, and other
entities change. Each entry captures a level, action, entity type and
ID, a human message, the request path, and a timestamp — the raw JSON
uses lowercase keys (`id`, `level`, `action`, `entityType`, etc.), not
the PascalCase shape used elsewhere in the API.

Use the `log` subcommands to inspect single rows. `log get <id>` fetches
one entry by its numeric ID. `log entity --entity-type=X --entity-id=Y`
returns every entry for one specific entity, newest first. For a broad
query across the whole system, use `logs list`.

### `mr log entity`

Get log entries for a specific entity

```
mr log entity
```

Fetch every log entry recorded for one specific entity. Both
`--entity-type` (e.g. `group`, `resource`, `note`, `tag`) and
`--entity-id` are required. The response is the same paginated wrapper
`logs list` returns, so the `logs` array contains the actual rows and
pagination is controlled by the global `--page` flag.

This is the reliable way to discover a log row's ID from code: create
or touch an entity, then query its history to get the `id` value used
by `log get`. The action field (`create`, `update`, `delete`, `system`)
lets scripts filter to just the events they care about.

**Flags:**

- `--entity-type` (string) **(required)** — Entity type (required)
- `--entity-id` (uint) **(required)** — Entity ID (required)

**Examples:**

```bash
# List every log entry for group 42
mr log entity --entity-type=group --entity-id=42
# Pull only the actions for one resource
mr log entity --entity-type=resource --entity-id=7 --json | jq -r '.logs[].action'
```

**Output:** Paginated wrapper with logs (array of entries), totalCount, page, perPage; each entry has id, level, action, entityType, entityId, entityName, message, requestPath, createdAt (lowercase keys)

**See also:** `mr logs list`, `mr log get`

### `mr log get`

Get a log entry by ID

```
mr log get <id>
```

Get a single log entry by its numeric ID and print its fields. Output
is a key/value table by default; pass the global `--json` flag to emit
the raw record for scripting. Note that log entries use lowercase JSON
keys (`id`, `level`, `action`, `entityType`, `entityId`, `message`,
`createdAt`) rather than the PascalCase names most other mahresources
entities use.

Log IDs are discovered via `logs list` or `log entity`; they are not
stable across fresh databases, so doctests create an entity first and
then look up the triggered row.

**Arguments:** `<id>`

**Examples:**

```bash
# Get a log entry by ID (table output)
mr log get 42
# Get as JSON and extract the action field with jq
mr log get 42 --json | jq -r .action
```

**Output:** Log entry object with id (uint), level, action, entityType, entityId, entityName, message, requestPath, createdAt (all lowercase keys)

**See also:** `mr logs list`, `mr log entity`


---

## `logs` — List and filter audit log entries

The plural `logs` command group reads the server's activity log across
the whole system rather than a single entry. It exposes filtered,
paginated listings so scripts can audit changes, inspect recent
deletes, or build dashboards. Only read operations are provided — the
log is append-only and the server writes it automatically as entities
change.

Use `logs list` with the filter flags (`--level`, `--action`,
`--entity-type`, `--entity-id`, `--message`, `--created-before`,
`--created-after`) to narrow the result set. For single-row lookups
use the singular `log` subcommands (`log get`, `log entity`).

### `mr logs list`

List log entries

```
mr logs list
```

List log entries across the whole system, optionally filtered. Filter
flags combine with AND. `--level` accepts `info`, `warning`, or
`error`; `--action` accepts `create`, `update`, `delete`, or `system`.
`--entity-type` and `--entity-id` scope results to a single entity
kind or row, while `--message` does a substring match. Date filters
(`--created-before`, `--created-after`) expect RFC3339 strings such
as `2026-04-15T00:00:00Z`.

Pagination uses the global `--page` flag with a fixed page size of 50.
The response wraps the `logs` array with `totalCount`, `page`, and
`perPage` so scripts can walk the full result set. JSON output uses
lowercase keys throughout — match them exactly when building jq
filters.

**Flags:**

- `--level` (string) — Filter by level (info/warning/error)
- `--action` (string) — Filter by action (create/update/delete/system)
- `--entity-type` (string) — Filter by entity type
- `--entity-id` (uint) — Filter by entity ID
- `--message` (string) — Filter by message
- `--created-before` (string) — Filter by created before (RFC3339)
- `--created-after` (string) — Filter by created after (RFC3339)

**Examples:**

```bash
# List recent log entries (first page, table output)
mr logs list
# Filter to deletions only
mr logs list --action delete --json | jq -r '.logs[] | "\(.entityType) \(.entityId) \(.message)"'
# Filter by entity type and a date window
mr logs list --entity-type group --created-after 2026-01-01T00:00:00Z --json
```

**Output:** Paginated wrapper with logs (array of entries), totalCount, page, perPage; each entry has id, level, action, entityType, entityId, entityName, message, requestPath, createdAt (lowercase keys)

**See also:** `mr log get`, `mr log entity`, `mr admin`


---

## `plugin` — Enable, disable, or configure a plugin

Plugins are server-side extensions that register shortcodes, hook into
entity lifecycle events, or inject custom UI into the mahresources web
interface. Each plugin is identified by a unique name and reports its
version, human description, an `enabled` flag, and an optional settings
schema (a list of `{name, type, label, default}` descriptors the plugin
will read at runtime).

Use the `plugin` subcommands to operate on one plugin at a time by
name: `enable` / `disable` toggle activation, `settings` writes the
plugin's configuration values, and `purge-data` wipes the plugin's
persisted state. Use `plugins list` to discover the names of installed
plugins and inspect their current enablement and stored settings.

### `mr plugin disable`

Disable a plugin

```
mr plugin disable <name>
```

Disable an installed plugin by name. Once disabled, the plugin stops
contributing shortcodes, hooks, and UI injections, but its stored
settings values and persisted KV data are preserved (use `plugin
purge-data` to remove the KV data). Disabling a plugin that is
already disabled is idempotent and returns `ok`.

**Arguments:** `<name>`

**Examples:**

```bash
# Disable a plugin by name
mr plugin disable my-plugin
# Disable and confirm via the JSON response
mr plugin disable my-plugin --json | jq -e '.enabled == false'
```

**Output:** Object with name, enabled=false, and ok=true on success

**See also:** `mr plugin enable`, `mr plugin purge-data`, `mr plugins list`

### `mr plugin enable`

Enable a plugin

```
mr plugin enable <name>
```

Enable an installed plugin by name. Once enabled, the plugin's
registered shortcodes, event hooks, and UI injections become active on
the server until a matching `plugin disable` call runs. Enabling a
plugin that declares required settings will fail until those settings
have been written via `plugin settings`. Enabling an already-enabled
or unknown plugin name returns a non-zero exit code and an error
message from the server.

**Arguments:** `<name>`

**Examples:**

```bash
# Enable a plugin by name
mr plugin enable my-plugin
# Enable and confirm via the JSON response
mr plugin enable my-plugin --json | jq -e '.enabled == true'
```

**Output:** Object with name, enabled=true, and ok=true on success

**See also:** `mr plugin disable`, `mr plugin settings`, `mr plugins list`

### `mr plugin purge-data`

Purge all data for a plugin

```
mr plugin purge-data <name>
```

Purge all key/value data a plugin has written through the plugin KV
API. Destructive: wipes every row the plugin has persisted in its
private KV tables. The plugin itself stays installed and its stored
settings values (written via `plugin settings`) are preserved. The
plugin must be disabled first; calling `purge-data` on an enabled
plugin returns a non-zero exit code.

This is the reset button for plugin KV state; use it when a plugin's
stored data is corrupt, stale, or no longer needed. There is no
confirmation prompt and no undo.

**Arguments:** `<name>`

**Examples:**

```bash
# Purge all KV data for a plugin by name
mr plugin purge-data my-plugin
# Purge and confirm the JSON response
mr plugin purge-data my-plugin --json | jq -e '.ok == true'
```

**Output:** Object with name and ok=true on success

**See also:** `mr plugin disable`, `mr plugin settings`, `mr plugins list`

### `mr plugin settings`

Update plugin settings (pass JSON via --data)

```
mr plugin settings <name>
```

Write configuration values for an installed plugin. Pass the values as
a JSON object via the required `--data` flag; keys must match the
`name` fields declared in the plugin's settings descriptor (see the
`settings` array on `plugins list`). The server stores the decoded
object as the plugin's persisted values and returns `ok=true` on
success.

This command replaces the stored values wholesale — keys omitted from
the `--data` payload are not preserved. Run `plugins list --json` to
inspect the current `values` object before writing a new one.

**Arguments:** `<name>`

**Flags:**

- `--data` (string) **(required)** (default `{}`) — Plugin settings as JSON (required)

**Examples:**

```bash
# Update a plugin's banner text
mr plugin settings my-plugin --data '{"banner_text":"Hello from CLI"}'
# Write multiple settings in one call
mr plugin settings my-plugin --data '{"banner_text":"Hi","show_banner":true}'
```

**Output:** Object with name and ok=true on success

**See also:** `mr plugin enable`, `mr plugin purge-data`, `mr plugins list`


---

## `plugins` — List installed plugins

Discover and inspect the plugins installed on the mahresources server.
The plural `plugins` command group is read-only: use `plugins list` for
a full snapshot of every plugin the server knows about, including its
current `enabled` state and any stored setting values. For lifecycle
controls (enable, disable, configure, purge) on a single plugin, use
the singular `plugin` subcommands.

### `mr plugins list`

List plugins and management info

```
mr plugins list
```

Return every plugin installed on the server, regardless of whether it
is currently enabled. The response is a single array ordered by plugin
name. Each entry includes the plugin's `name`, `version`,
`description`, an `enabled` boolean, and a `settings` descriptor
array (or `null` when the plugin declares no settings). When a plugin
has stored configuration values, a `values` object is also present
keyed by setting name.

Plugin management info has a variable shape depending on what each
plugin reports, so `plugins list` always emits JSON; piping through
`jq` is the expected usage pattern.

**Examples:**

```bash
# Show every installed plugin as JSON
mr plugins list
# Print just the names of enabled plugins
mr plugins list | jq -r '.[] | select(.enabled == true) | .name'
```

**Output:** Array of plugins; each entry has name, version, description, enabled, settings (nullable array of setting descriptors), and an optional values object holding stored configuration values

**See also:** `mr plugin enable`, `mr plugin disable`, `mr plugin settings`


---

## `admin` — Server administration commands

Server administration commands. The default subcommand is `stats`, which prints a full health and data overview. The `settings` subgroup lets you view and change runtime configuration overrides without restarting the server.

Run `mr admin stats --help` for the full stats flags, or `mr admin settings --help` for the settings subcommands.

### `mr admin settings get`

Show a single runtime setting by key

```
mr admin settings get <key>
```

Show a single runtime setting by key. The output includes the effective current value, the boot-time default, whether an override is active, and when it was last changed.

Pass `--json` to emit the raw JSON object for scripting.

**Arguments:** `<key>`

**Examples:**

```bash
# Show max_upload_size in table form
mr admin settings get max_upload_size
# Get as JSON and extract the current value
mr admin settings get max_upload_size --json | jq -r .current
```

**Output:** Single setting object with key, label, group, type, current, bootDefault, overridden, updatedAt, reason

**See also:** `mr admin settings list`, `mr admin settings set`, `mr admin settings reset`

### `mr admin settings list`

List all runtime settings

```
mr admin settings list
```

List all runtime-editable settings with their current value, boot default, override status, and last-updated timestamp. Overridden settings show the effective value alongside the original boot default so you can see what changed.

Pass `--json` to emit the raw JSON array for scripting or to inspect fields like `minNumeric`, `maxNumeric`, and `allowZero`.

**Examples:**

```bash
# Show all settings in a table
mr admin settings list
# Emit raw JSON for scripting
mr admin settings list --json
```

**Output:** Array of setting objects with key, label, group, type, current, bootDefault, overridden, updatedAt, reason

**See also:** `mr admin settings get`, `mr admin settings set`, `mr admin settings reset`

### `mr admin settings reset`

Remove a runtime override and revert to boot default

```
mr admin settings reset <key>
```

Remove a runtime override and revert the setting to its boot-time default. The command prints the post-reset view so you can confirm the current value is back to the default.

Use `--reason` to record why the override was removed; the reason is stored in the database alongside the reset timestamp.

**Arguments:** `<key>`

**Flags:**

- `--reason` (string) — Free-text note recorded in the audit log

**Examples:**

```bash
# Reset max_upload_size to its boot default
mr admin settings reset max_upload_size
# Reset with a reason for the audit log
mr admin settings reset mrql_query_timeout --reason "back to default after testing"
```

**Output:** Setting object after reset with key, label, group, type, current (equals bootDefault), bootDefault, overridden (false), updatedAt, reason

**See also:** `mr admin settings set`, `mr admin settings get`, `mr admin settings list`

### `mr admin settings set`

Override a runtime setting

```
mr admin settings set <key> <value>
```

Override a runtime setting. The override persists to the database and takes effect on the next use of the setting — no restart required. The command prints the updated setting view so you can confirm the new value.

Size values accept suffix notation (e.g., `1G`, `500M`, `2048K`). Duration values use Go's time.ParseDuration format (`30s`, `5m`, `2h`). Use `--reason` to record why the change was made; the reason is stored in the database and shown by `mr admin settings get`.

**Arguments:** `<key> <value>`

**Flags:**

- `--reason` (string) — Free-text note recorded in the audit log

**Examples:**

```bash
# Set max_upload_size to 2 GB
mr admin settings set max_upload_size 2147483648 --reason "increase for video workflow"
# Set mrql query timeout
mr admin settings set mrql_query_timeout 30s
```

**Output:** Updated setting object with key, label, group, type, current, bootDefault, overridden, updatedAt, reason

**See also:** `mr admin settings reset`, `mr admin settings get`, `mr admin settings list`

### `mr admin stats`

Show server and data statistics

```
mr admin stats
```

Show administrative statistics about the running server and its data. By default the command fetches three sections — server health (uptime, memory, DB connections), data counts (entity totals), and expensive stats that require full-table scans (hash collisions, dangling references). Together they give a one-page picture of instance size and health.

Use `--server-only` to fetch just the server health block, or `--data-only` to fetch just the data counts — useful for lightweight monitoring that skips the expensive scans. Neither flag is required; when both are unset the command fetches all three sections.

**Flags:**

- `--server-only` (bool) — Show server stats only
- `--data-only` (bool) — Show data stats only

**Examples:**

```bash
# Full admin stats (human-readable, three sections)
mr admin
# Server health only
mr admin --server-only --json
# Data counts only
mr admin --data-only
```

**Output:** Combined stats object {serverStats, dataStats, expensiveStats} in JSON mode; three sectioned tables in human mode

**See also:** `mr resources versions-cleanup`, `mr jobs list`, `mr logs list`


---

## `docs` — Introspect and validate the mr CLI's own documentation

Introspect and validate the mr CLI's own documentation. The `docs` subcommands
walk the command tree to emit machine-readable JSON, generate docs-site
Markdown pages, validate help text against the template rules, and execute
runnable examples.

Use `mr docs` during CLI development to keep help text consistent, and in CI
to guarantee that published documentation stays in sync with the
implementation.

### `mr docs check-examples`

Run `# mr-doctest:` example blocks against a live server

```
mr docs check-examples
```

Walks the command tree, extracts every example tagged `# mr-doctest:`, and
evaluates each block against the connected server. Per-example metadata on the
label line controls behavior: `expect-exit=N`, `tolerate=/regex/`,
`skip-on=ephemeral`, `timeout=Ns`, and `stdin=<fixture>`.

The runner pipes each block through `bash -e -o pipefail -c`, with cwd set to
`cmd/mr/` so examples can reference `./testdata/*` fixtures. Requires
`MAHRESOURCES_URL`, `bash`, and `jq` on PATH.

**Flags:**

- `--environment` (string) — Target environment label used by `skip-on=<env>` metadata. Example: `ephemeral` when targeting a seed-less in-memory server.

**Examples:**

```bash
# Run against a local ephemeral server
mr docs check-examples --server http://localhost:8181 --environment=ephemeral
# Inherit server URL from the environment
MAHRESOURCES_URL=http://localhost:8181 mr docs check-examples --environment=ephemeral
```

**See also:** `mr docs lint`, `mr docs dump`

### `mr docs dump`

Emit the mr command tree as JSON or Markdown

```
mr docs dump
```

Emit the full mr command tree with rich metadata: persistent flags, per-command
local and inherited flags, required-flag lists, positional-argument contracts,
parsed examples, and Annotations (outputShape, exitCodes, relatedCmds). JSON
output is intended for agents and tooling; Markdown output is intended for the
docs-site (`docs-site/docs/cli/`).

Cobra's built-in `help` and `completion` subcommands are skipped: they are not
user-facing and are excluded from the documented contract.

**Flags:**

- `--format` (string) **(required)** — Output format: json (stdout by default) or markdown (requires --output). Required.
- `--output` (string) — Output `path`. Required for markdown; optional for json (stdout when omitted).
- `--help` (bool) — help for dump

**Examples:**

```bash
# Emit JSON to stdout (agent-friendly)
mr docs dump --format json
# Emit JSON to a file
mr docs dump --format json --output /tmp/mr-tree.json
# Regenerate docs-site pages
mr docs dump --format markdown --output docs-site/docs/cli/
```

**Output:** CommandTree JSON (when --format json) or directory of Markdown files (when --format markdown)

**See also:** `mr docs lint`, `mr docs check-examples`

### `mr docs lint`

Validate every command's help against the template

```
mr docs lint
```

Validate every user-facing command's help against the template rules defined
in the spec: Short, Long, ≥2 Examples per leaf, rich flag descriptions,
required Annotations (outputShape where applicable, exitCodes), and sensible
Short length. Missing `# mr-doctest:` examples emit warnings, not errors.

Lint is allowlist-gated during migration: only command groups explicitly added
to the allowlist are subject to the strict rules, so partial migrations do not
block CI.

**Examples:**

```bash
# Lint the full command tree
mr docs lint
# Use in CI (non-zero exit fails the build)
mr docs lint || exit 1
```

**See also:** `mr docs dump`, `mr docs check-examples`

