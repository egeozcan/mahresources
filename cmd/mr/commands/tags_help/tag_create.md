---
outputShape: Created Tag object with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: tag get, tag edit-name, tags list
---

# Long

Create a new tag. `--name` is required and must be unique; `--description`
is optional free-form text. On success prints a confirmation line with
the new ID; pass the global `--json` flag to emit the full record for
scripting (e.g., piping the new ID into follow-up commands).

# Example

  # Create a tag with just a name
  mr tag create --name "urgent"

  # Create with a description and capture the ID via jq
  ID=$(mr tag create --name "archived" --description "archived items" --json | jq -r .ID)

  # mr-doctest: create a tag, assert the returned ID is positive
  mr tag create --name "doctest-create-$$-$RANDOM" --json | jq -e '.ID > 0'
