---
outputShape: Array of Series objects with ID, Name, Slug, Meta, CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: series get, series create, resources list
---

# Long

List Series, optionally filtered by name or slug. The `--name` and
`--slug` flags do substring matching on the server. Results are
paginated via the global `--page` flag (default page size 50). Default
output is a table with ID, NAME, SLUG, and CREATED columns; pass
`--json` for the full array.

# Example

  # List all series (first page)
  mr series list

  # Filter by name substring
  mr series list --name volume

  # JSON output piped into jq
  mr series list --json | jq -r '.[].Name'

  # mr-doctest: create a uniquely named series, list filtered by name, assert count >= 1
  NAME="list-test-$$-$RANDOM"
  mr series create --name "$NAME" --json > /dev/null
  mr series list --name "$NAME" --json | jq -e 'length >= 1'
