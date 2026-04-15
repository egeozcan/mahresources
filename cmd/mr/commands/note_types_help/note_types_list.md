---
outputShape: Array of NoteType objects with ID, Name, Description, MetaSchema, SectionConfig, CustomHeader/Sidebar/Summary/Avatar/MRQLResult, CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: note-type get, note-type create, notes list
---

# Long

List Note Types, optionally filtered by name or description. The
`--name` and `--description` flags do substring matching on the server.
Results are paginated via the global `--page` flag (default page size
50). Default output is a table with ID, NAME, DESCRIPTION, and CREATED
columns; pass `--json` for the full array including MetaSchema,
SectionConfig, and the Custom* rendering fields.

# Example

  # List all note types (first page)
  mr note-types list

  # Filter by name substring
  mr note-types list --name meeting

  # JSON output piped into jq to extract just names
  mr note-types list --json | jq -r '.[].Name'

  # mr-doctest: create a uniquely named note type, list filtered by name, assert count >= 1
  NAME="nt-list-$$-$RANDOM"
  mr note-type create --name "$NAME" --json > /dev/null
  mr note-types list --name "$NAME" --json | jq -e 'length >= 1'
