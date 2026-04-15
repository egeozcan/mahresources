---
outputShape: Object with id (uint) of the deleted saved MRQL query
exitCodes: 0 on success; 1 on any error
relatedCmds: mrql save, mrql list, mrql run
---

# Long

Delete a saved MRQL query by numeric ID. Destructive: removes the
database row for the saved query. Any downstream references (bookmarks,
dashboards, or `[mrql saved="..."]` shortcodes) must be updated
separately — the server does not rewrite them. Deleting a nonexistent
ID returns exit code 1.

Unlike `mrql run`, the delete subcommand only accepts a numeric ID; pass
`mrql list --json | jq -r '.[] | select(.name == "...") | .id'` to
resolve a name to its ID first.

# Example

  # Delete a saved query by ID
  mr mrql delete 42

  # Delete and inspect the response with jq
  mr mrql delete 42 --json | jq .

  # mr-doctest: save a query, delete it, confirm list no longer contains the id
  NAME="doctest-mrql-del-$$-$RANDOM"
  ID=$(mr mrql save "$NAME" 'type = resource' --json | jq -r '.id')
  mr mrql delete $ID
  mr mrql list --json | jq -e --argjson id "$ID" 'map(select(.id == $id)) | length == 0'
