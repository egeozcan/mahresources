---
outputShape: Array of revision summaries, newest first, with id, number, carrierType, carrierId, createdAt, createdByUserId, label, source, applied, changedSlots and restoredFrom
exitCodes: 0 on success; 1 on any error
relatedCmds: note-types revisions show, note-types revisions diff, note-type edit
---

# Long

List the revisions of a note type, newest first. `--id` is required.
The table shows the revision NUMBER (#1, #2, ...), the SOURCE that
recorded it (`create`, `form`, `api`, `cli`, `plugin`, `import`,
`restore` or `baseline`), the slots it CHANGED compared with the
previous revision, its LABEL, the USER ID and when it was CREATED. Pass
`--json` for the full array, including the `applied` list.

# Example

  # List the revisions of a note type
  mr note-types revisions list --id 3

  # Print only the revision numbers
  mr note-types revisions list --id 3 --json | jq -r '.[].number'

  # mr-doctest: a new note type has one create revision, and a CLI edit adds one
  ID=$(mr note-type create --name "rev-$$-$RANDOM" --json | jq -r .ID)
  mr note-types revisions list --id "$ID" --json | jq -e 'length == 1 and .[0].number == 1 and .[0].source == "create"'
  mr note-type edit --id "$ID" --custom-header '<h1>Hi</h1>'
  mr note-types revisions list --id "$ID" --json | jq -e 'length == 2 and .[0].number == 2 and .[0].source == "cli" and .[0].changedSlots == ["CustomHeader"]'
  mr note-type delete "$ID"
