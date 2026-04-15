---
outputShape: Updated NoteType with ID, Name, Description, MetaSchema, SectionConfig, CustomHeader/Sidebar/Summary/Avatar/MRQLResult, CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: note-type edit-name, note-type edit-description, note-type get
---

# Long

Edit a note type. `--id` is required; every other flag is optional and
only fields explicitly passed are modified (server-side PATCH
semantics). Use this command when you need to change the `MetaSchema`,
`SectionConfig`, or any of the Custom* rendering fields; the dedicated
`edit-name` / `edit-description` commands only touch those two scoped
fields.

# Example

  # Swap the JSON Schema on note type 1
  mr note-type edit --id 1 \
    --meta-schema '{"type":"object","properties":{"priority":{"type":"string"}}}'

  # Update the custom summary template and confirm via list
  mr note-type edit --id 1 --custom-summary "<div>{{ Note.Name }}</div>"
  mr note-types list --json | jq '.[] | select(.ID == 1).CustomSummary'

  # mr-doctest: create, edit meta-schema, verify via list (get omits MetaSchema)
  ID=$(mr note-type create --name "doctest-nt-edit-$$-$RANDOM" --json | jq -r '.ID')
  mr note-type edit --id $ID --meta-schema '{"type":"object"}' --json | jq -e '.MetaSchema == "{\"type\":\"object\"}"'
