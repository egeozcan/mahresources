---
outputShape: Created NoteType with ID, Name, Description, MetaSchema, SectionConfig, CustomHeader/Sidebar/Summary/Avatar/MRQLResult, CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: note-type get, note-type edit, note-types list
---

# Long

Create a new note type. `--name` is required; all other fields are
optional. Pass a JSON Schema string to `--meta-schema` to constrain the
metadata shape of Notes of this type, and a JSON object to
`--section-config` to control which sections render on note detail
pages. The Custom* flags accept raw HTML or Pongo2 template strings
that the server injects into note pages and MRQL result cards.

On success prints a confirmation line with the new ID; pass the global
`--json` flag to emit the full created record for scripting.

# Example

  # Create a minimal note type (name only)
  mr note-type create --name "Meeting Minutes"

  # Create with a JSON Schema constraining metadata
  mr note-type create --name "Bug Report" \
    --meta-schema '{"type":"object","properties":{"severity":{"type":"string"}}}'

  # Capture the new ID via jq for follow-up commands
  NT=$(mr note-type create --name "Code Review" --json | jq -r .ID)

  # mr-doctest: create a note type, assert the returned ID is positive
  mr note-type create --name "doctest-nt-create-$$-$RANDOM" --json | jq -e '.ID > 0'
