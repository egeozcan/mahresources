---
outputShape: Tag object with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: tag create, tag edit-name, tags list
---

# Long

Get a tag by ID and print its fields. The server has no single-tag GET
endpoint, so the CLI fetches the first page of the tag list, the 50 most
recently created, and filters in-process; a tag outside that window is
reported as not found, and the global `--page` flag does not move the
window. Output is a key/value table by default; pass the global `--json`
flag to emit the raw record for scripting.

# Example

  # Get a tag by ID (table output)
  mr tag get 42

  # Get as JSON and extract the name with jq
  mr tag get 42 --json | jq -r .Name

  # mr-doctest: create a tag and verify it is retrievable
  ID=$(mr tag create --name "doctest-get-$$-$RANDOM" --json | jq -r '.ID')
  mr tag get $ID --json | jq -e '.ID > 0 and (.Name | length) > 0'
