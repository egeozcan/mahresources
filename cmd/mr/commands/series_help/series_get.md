---
outputShape: Series object with ID (uint), Name (string), Slug (string), Meta (object), Resources ([]Resource), CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: series list, series edit, series delete
---

# Long

Get a series by ID and print its fields. Fetches the full record
including the slug, meta JSON, and the first 50 resources attached to
the series. The key/value table shows ID, name, slug, meta and
timestamps; pass the global `--json` flag for the raw record, which is
where the resource list appears.

# Example

  # Get a series by ID (table output)
  mr series get 42

  # Get as JSON and extract the name with jq
  mr series get 42 --json | jq -r .Name

  # mr-doctest: create a series and verify it is retrievable
  NAME="doctest-get-$$-$RANDOM"
  ID=$(mr series create --name "$NAME" --json | jq -r '.ID')
  mr series get $ID --json | jq -e --arg n "$NAME" '.ID > 0 and .Name == $n'
