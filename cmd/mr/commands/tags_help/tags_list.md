---
outputShape: Array of Tag objects with ID, Name, Description, CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: tag get, tags timeline, resources list
---

# Long

List Tags, optionally filtered by name or description. The `--name` and
`--description` flags do substring matching on the server. Results are
paginated via the global `--page` flag (default page size 50). Default
output is a table with ID, NAME, DESCRIPTION, and CREATED columns; pass
`--json` for the full array.

# Example

  # List all tags (first page)
  mr tags list

  # Filter by name substring
  mr tags list --name urgent

  # JSON output piped into jq
  mr tags list --json | jq -r '.[].Name'

  # mr-doctest: create a uniquely named tag, list filtered by name, assert count >= 1
  NAME="list-test-$$-$RANDOM"
  mr tag create --name "$NAME" --json > /dev/null
  mr tags list --name "$NAME" --json | jq -e 'length >= 1'
