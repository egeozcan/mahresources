---
outputShape: Array of Note objects with ID, Name, Description, Meta, Tags, OwnerId, NoteTypeId, CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: note get, notes timeline, mrql
---

# Long

List Notes, optionally filtered. Filter flags combine with AND.
Comma-separated ID lists on `--tags` and `--groups` match any of the
given IDs. Date flags (`--created-before`, `--created-after`) expect
`YYYY-MM-DD`. The `--name` and `--description` flags match substrings.
Use `--owner-id` and `--note-type-id` to scope by owner group or note
type. Pagination is via the global `--page` flag (default page size 50).

`--mrql` applies an MRQL filter expression, with `type = "note"`
implied (the same expression the list-page filter bar accepts). It uses
the WHERE-clause grammar only — no `ORDER BY`, `LIMIT`, `GROUP BY`,
`SCOPE`, or `$name` parameters — and composes with the other filter
flags via AND. Example: `--mrql 'tags = "todo" AND created > -7d'`.

# Example

  # List all notes (first page)
  mr notes list

  # Filter by name substring and owner
  mr notes list --name meeting --owner-id 42

  # Filter by tag + date, JSON + jq
  mr notes list --tags 5 --created-after 2026-01-01 --json | jq -r '.[].Name'

  # Filter with an MRQL expression (type = "note" implied)
  mr notes list --mrql 'tags = "todo" AND created > -7d'

  # mr-doctest: create two notes with a shared tag, list by tag, assert count >= 2
  TAG=$(mr tag create --name "list-notes-$$-$RANDOM" --json | jq -r '.ID')
  ID1=$(mr note create --name "doctest-list-a-$$-$RANDOM" --json | jq -r '.ID')
  ID2=$(mr note create --name "doctest-list-b-$$-$RANDOM" --json | jq -r '.ID')
  mr notes add-tags --ids $ID1,$ID2 --tags $TAG
  mr notes list --tags $TAG --json | jq -e 'length >= 2'

  # mr-doctest: --mrql narrows notes by a tag-name filter expression
  MTAG="mrql-notes-$$-$RANDOM"
  MTID=$(mr tag create --name "$MTAG" --json | jq -r '.ID')
  NID=$(mr note create --name "doctest-mrql-$$-$RANDOM" --json | jq -r '.ID')
  mr notes add-tags --ids $NID --tags $MTID
  mr notes list --mrql "tags = \"$MTAG\"" --json | jq -e --argjson id "$NID" 'map(.ID) | index($id) != null'
