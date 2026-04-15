---
outputShape: Group object with ID (uint), Name, Description, Meta (object), OwnerId, CategoryId, CreatedAt/UpdatedAt, plus related collections (Tags, OwnResources, OwnNotes, OwnGroups)
exitCodes: 0 on success; 1 on any error
relatedCmds: group create, group edit-name, group parents, group children
---

# Long

Get a group by ID and print its metadata. Fetches the full record
including the owner chain, category, tags, and any custom Meta JSON
object. Output is a key/value table by default; pass the global `--json`
flag to get the full record for scripting (related collections such as
`Tags`, `OwnResources`, `OwnNotes`, and `OwnGroups` are included).

# Example

  # Get a group by ID (table output)
  mr group get 42

  # Get as JSON and extract a single field with jq
  mr group get 42 --json | jq -r .Name

  # mr-doctest: create a group and verify it is retrievable
  ID=$(mr group create --name "doctest-get-$$-$RANDOM" --json | jq -r '.ID')
  mr group get $ID --json | jq -e '.ID > 0 and (.Name | length) > 0'
