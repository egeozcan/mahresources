---
outputShape: Array of Group objects with ID, Name, Description, Meta, OwnerId, CategoryId, CreatedAt/UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: group get, group create, groups timeline, mrql
---

# Long

List Groups, optionally filtered. Filter flags combine with AND.
Comma-separated ID lists on `--tags` and `--groups` match any of the
given IDs via the `?Add` query parameter. Date flags
(`--created-before`, `--created-after`) expect `YYYY-MM-DD`. Pagination
via the global `--page` flag (default page size 50).

Use `--owner-id=0` to restrict to root groups (no parent). The JSON
output is a flat array — use `group children <id>` for tree-structured
traversal.

# Example

  # List all groups (paged)
  mr groups list

  # Filter by name prefix
  mr groups list --name "Trips"

  # Filter by owner and tag, JSON + jq
  mr groups list --owner-id 5 --tags 3 --json | jq -r '.[].Name'

  # mr-doctest: create a group, list with matching name filter, assert at least one match
  NAME="doctest-list-$$-$RANDOM"
  ID=$(mr group create --name "$NAME" --json | jq -r '.ID')
  mr groups list --name "$NAME" --json | jq -e 'length >= 1'
