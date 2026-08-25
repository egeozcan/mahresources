---
outputShape: Full server NoteType JSON (--json); table with ID, Name, Description, Created, Updated (default)
exitCodes: 0 on success; 1 on any error
relatedCmds: note-type create, note-type edit, note-types list
---

# Long

Get a note type by ID and print its fields. The server has no
single-NoteType GET endpoint, so the CLI fetches the note type listing
and filters in-process. Only the first page (50 rows) is fetched, so a
note type beyond it reports `note type <id> not found`; use
`mr note-types list --name <substring>` to locate it instead.

The table output shows five core fields (ID, Name, Description,
Created, Updated). The `--json` flag emits the full server response,
including MetaSchema, SectionConfig, CustomHeader, CustomCSS, and other
Custom* fields.

# Example

  # Get a note type by ID (table output)
  mr note-type get 1

  # Get as JSON and extract the name with jq
  mr note-type get 1 --json | jq -r .Name

  # mr-doctest: create a note type with a meta schema, fetch via --json, assert the widened fields survive
  NAME="doctest-nt-$$-$RANDOM"
  NTID=$(mr note-type create --name "$NAME" --meta-schema '{"type":"object"}' --json | jq -r '.ID // .id')
  mr note-type get $NTID --json | jq -e 'has("MetaSchema") or has("metaSchema") or has("SectionConfig") or has("sectionConfig") or has("CustomHeader") or has("customHeader") or has("CustomCSS") or has("customCSS")'
