---
outputShape: ResourceCategory object with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: resource-category create, resource-category edit-name, resource-categories list
---

# Long

Get a resource category by ID and print its fields. The server has no
single-resource-category GET endpoint, so the CLI fetches the full list
and filters in-process; on large instances this is slower than a direct
lookup would be. Output is a key/value table by default; pass the
global `--json` flag to emit the raw record for scripting.

# Example

  # Get a resource category by ID (table output)
  mr resource-category get 42

  # Get as JSON and extract the name with jq
  mr resource-category get 42 --json | jq -r .Name

  # mr-doctest: create a resource category and verify it is retrievable
  ID=$(mr resource-category create --name "doctest-rc-get-$$-$RANDOM" --json | jq -r '.ID')
  mr resource-category get $ID --json | jq -e '.ID > 0 and (.Name | length) > 0'
