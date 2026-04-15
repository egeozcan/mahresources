---
outputShape: Array of resources with id, name, content type, size, dimensions, owner id, created
exitCodes: 0 on success; 1 on any error
relatedCmds: resource get, groups list, mrql
---

# Long

List Resources, optionally filtered. Filter flags combine with AND.
Comma-separated ID lists on `--tags`, `--groups`, `--notes` use the
`?Add` query parameter to match any of the given IDs. Date flags
(`--created-before`, `--created-after`) expect `YYYY-MM-DD`. Sort with
`--sort-by=field1,-field2` (prefix with `-` for descending). Pagination
via the global `--page` flag (default page size 50).

# Example

  # List all resources (paged)
  mr resources list

  # Filter by content type
  mr resources list --content-type image/jpeg

  # Filter by tag + date, JSON + jq
  mr resources list --tags 5 --created-after 2026-01-01 --json | jq -r '.[].Name'

  # mr-doctest: upload two fixtures with a known tag, list by tag, assert count >= 2
  TAG=$(mr tag create --name "list-test-$$-$RANDOM" --json | jq -r '.ID')
  GRP=$(mr group create --name "doctest-list-$$-$RANDOM" --json | jq -r '.ID')
  ID1=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "list-a-$$" --json | jq -r '.[0].ID')
  ID2=$(mr resource upload ./testdata/sample.png --owner-id=$GRP --name "list-b-$$" --json | jq -r '.[0].ID')
  mr resources add-tags --ids $ID1,$ID2 --tags $TAG
  mr resources list --tags $TAG --json | jq -e 'length >= 2'
