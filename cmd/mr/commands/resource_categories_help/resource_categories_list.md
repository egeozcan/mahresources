---
outputShape: Array of ResourceCategory objects with ID, Name, Description, CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: resource-category get, resource-category create, resources list
---

# Long

List Resource Categories, optionally filtered by name or description.
The `--name` and `--description` flags do substring matching on the
server. Results are paginated via the global `--page` flag (default
page size 50). Default output is a table with ID, NAME, DESCRIPTION,
and CREATED columns; pass `--json` for the full array.

# Example

  # List all resource categories (first page)
  mr resource-categories list

  # Filter by name substring
  mr resource-categories list --name photos

  # JSON output piped into jq
  mr resource-categories list --json | jq -r '.[].Name'

  # mr-doctest: create a uniquely named resource category, list filtered by name, assert count >= 1
  NAME="rc-list-$$-$RANDOM"
  mr resource-category create --name "$NAME" --json > /dev/null
  mr resource-categories list --name "$NAME" --json | jq -e 'length >= 1'
