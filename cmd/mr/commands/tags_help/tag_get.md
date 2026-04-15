---
outputShape: Tag object with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: tag create, tag edit-name, tags list
---

# Long

Get a tag by ID and print its fields. The server has no single-tag GET
endpoint, so the CLI fetches the full tag list and filters in-process;
on large instances this is slower than a direct lookup would be. Output
is a key/value table by default; pass the global `--json` flag to emit
the raw record for scripting.

# Example

  # Get a tag by ID (table output)
  mr tag get 42

  # Get as JSON and extract the name with jq
  mr tag get 42 --json | jq -r .Name

  # mr-doctest: create a tag and verify it is retrievable
  ID=$(mr tag create --name "doctest-get-$$-$RANDOM" --json | jq -r '.ID')
  mr tag get $ID --json | jq -e '.ID > 0 and (.Name | length) > 0'
