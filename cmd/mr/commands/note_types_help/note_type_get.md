---
outputShape: NoteType projection with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: note-type create, note-type edit, note-types list
---

# Long

Get a note type by ID and print its core fields. The server has no
single-NoteType GET endpoint, so the CLI fetches the full list and
filters in-process; this is slower than a direct lookup on large
instances. The JSON output is a 5-key projection (ID, Name, Description,
CreatedAt, UpdatedAt); use `note-types list --json` when you need the
full record including MetaSchema, SectionConfig, or the Custom* fields.

# Example

  # Get a note type by ID (table output)
  mr note-type get 1

  # Get as JSON and extract the name with jq
  mr note-type get 1 --json | jq -r .Name

  # mr-doctest: create a note type and verify it is retrievable
  ID=$(mr note-type create --name "doctest-nt-get-$$-$RANDOM" --json | jq -r '.ID')
  mr note-type get $ID --json | jq -e '.ID > 0 and (.Name | length) > 0'
