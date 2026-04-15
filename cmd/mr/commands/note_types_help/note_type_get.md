---
outputShape: Full server NoteType JSON (--json); table with ID, Name, Description, Created, Updated (default)
exitCodes: 0 on success; 1 on any error
relatedCmds: note-type create, note-type edit, note-types list
---

# Long

Get a note type by ID and print its fields. The server has no
single-NoteType GET endpoint, so the CLI fetches the full list and
filters in-process; this is slower than a direct lookup on large
instances. The table output shows five core fields (ID, Name, Description,
Created, Updated). The `--json` flag emits the full server response,
including MetaSchema, SectionConfig, CustomHeader, and other Custom* fields.

# Example

  # Get a note type by ID (table output)
  mr note-type get 1

  # Get as JSON and extract the name with jq
  mr note-type get 1 --json | jq -r .Name

  # mr-doctest: create a note type with a meta schema, fetch via --json, assert the widened fields survive
  NAME="doctest-nt-$$-$RANDOM"
  NTID=$(mr note-type create --name "$NAME" --meta-schema '{"type":"object"}' --json | jq -r '.ID // .id')
  mr note-type get $NTID --json | jq -e 'has("MetaSchema") or has("metaSchema") or has("SectionConfig") or has("sectionConfig") or has("CustomHeader") or has("customHeader")'
