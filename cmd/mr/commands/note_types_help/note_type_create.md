---
outputShape: Created NoteType with ID, Name, Description, MetaSchema, SectionConfig, CustomHeader/DetailFooter/Sidebar/Summary/Avatar/HoverCard/ListHeader/ListFooter/MRQLResult/CSS, ApplyTemplatesToShares, CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: note-type get, note-type edit, note-types list
---

# Long

Create a new note type. `--name` is required; all other fields are
optional. Pass a JSON Schema string to `--meta-schema` to constrain the
metadata shape of Notes of this type, and a JSON object to
`--section-config` to control which sections render on note detail
pages. There is a `--custom-*` flag for every note type template slot --
the detail page header, sidebar and footer, the list card summary, avatar
and hover card, the list page header and footer, and MRQL result cards.
Each accepts raw HTML or a template string that the server injects into
note pages and MRQL result cards; `--custom-css` is injected as a
`<style>` block on detail and list pages. Run
`mr note-type create --help` for the full list with a one-line
description of where each renders.

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
