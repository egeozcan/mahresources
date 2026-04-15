---
outputShape: Category object with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: category create, category edit-name, categories list
---

# Long

Get a category by ID and print its fields. The server has no single-category
GET endpoint, so the CLI fetches the full category list and filters
in-process; on large instances this is slower than a direct lookup would be.
Output is a key/value table by default; pass the global `--json` flag to
emit the raw record for scripting.

# Example

  # Get a category by ID (table output)
  mr category get 42

  # Get as JSON and extract the name with jq
  mr category get 42 --json | jq -r .Name

  # mr-doctest: create a category and verify it is retrievable
  ID=$(mr category create --name "doctest-get-$$-$RANDOM" --json | jq -r '.ID')
  mr category get $ID --json | jq -e '.ID > 0 and (.Name | length) > 0'
