---
outputShape: Array of saved MRQL query objects with id, name, query, description, createdAt, updatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: mrql save, mrql run, mrql delete
---

# Long

List saved MRQL queries. Pagination is controlled via the global
`--page` flag (default page size 50). Use the global `--json` flag to
retrieve the raw array for scripting; the default table output shows
ID, name, description (truncated), and creation timestamp.

To execute a listed query, use `mrql run <name-or-id>`. To inspect the
stored MRQL text itself, use `mrql list --json` and extract the `.query`
field — there is no dedicated `mrql get` subcommand.

# Example

  # List all saved MRQL queries (first page)
  mr mrql list

  # JSON + jq: print each saved query's id, name, and MRQL text
  mr mrql list --json | jq -r '.[] | "\(.id)\t\(.name)\t\(.query)"'

  # mr-doctest: save a uniquely-named query and verify list contains it
  NAME="doctest-mrql-list-$$-$RANDOM"
  mr mrql save "$NAME" 'type = resource' --json >/dev/null
  mr mrql list --json | jq -e --arg n "$NAME" 'map(select(.name == $n)) | length >= 1'
