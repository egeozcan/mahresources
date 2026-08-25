---
outputShape: Group object with ID (uint), Name, Description, Meta (object), OwnerId, CategoryId, CreatedAt/UpdatedAt, plus related collections (Tags, OwnResources, OwnNotes, OwnGroups)
exitCodes: 0 on success; 1 on any error
relatedCmds: group create, group edit-name, group parents, group children
---

# Long

Get a group by ID and print its metadata. Fetches the record
including its immediate owner, category, tags, and any custom Meta JSON
object. Output is a key/value table by default; pass the global `--json`
flag to get the record for scripting (related collections such as
`Tags`, `OwnResources`, `OwnNotes`, and `OwnGroups` are included).

The preloaded collections are truncated, `OwnResources` and
`RelatedResources` to 5 entries and `OwnGroups`, `OwnNotes`,
`RelatedNotes` and `RelatedGroups` to 50, so use
`resources list --owner-id <id>` or `group children <id>` when you need
the whole set.

# Example

  # Get a group by ID (table output)
  mr group get 42

  # Get as JSON and extract a single field with jq
  mr group get 42 --json | jq -r .Name

  # mr-doctest: create a group and verify it is retrievable
  ID=$(mr group create --name "doctest-get-$$-$RANDOM" --json | jq -r '.ID')
  mr group get $ID --json | jq -e '.ID > 0 and (.Name | length) > 0'
