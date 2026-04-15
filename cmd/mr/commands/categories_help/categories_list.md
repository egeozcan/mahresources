---
outputShape: Array of Category objects with ID, Name, Description, CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: category get, categories timeline, groups list
---

# Long

List Categories, optionally filtered by name or description. The `--name`
and `--description` flags do substring matching on the server. Results
are paginated via the global `--page` flag (default page size 50).
Default output is a table with ID, NAME, DESCRIPTION, and CREATED
columns; pass `--json` for the full array.

# Example

  # List all categories (first page)
  mr categories list

  # Filter by name substring
  mr categories list --name Project

  # JSON output piped into jq
  mr categories list --json | jq -r '.[].Name'

  # mr-doctest: create a uniquely named category and assert the filter returns at least one match
  NAME="list-test-$$-$RANDOM"
  mr category create --name "$NAME" --json > /dev/null
  mr categories list --name "$NAME" --json | jq -e 'length >= 1'
