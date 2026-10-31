---
outputShape: Array of revision summaries, newest first, with id, number, carrierType, carrierId, createdAt, createdByUserId, label, source, applied, changedSlots and restoredFrom
exitCodes: 0 on success; 1 on any error
relatedCmds: categories revisions show, categories revisions diff, category edit
---

# Long

List the revisions of a category, newest first. `--id` is required.
The table shows the revision NUMBER (#1, #2, ...), the SOURCE that
recorded it (`create`, `form`, `api`, `cli`, `plugin`, `import`,
`restore` or `baseline`), the slots it CHANGED compared with the
previous revision, its LABEL, the USER ID and when it was CREATED. Pass
`--json` for the full array, including the `applied` list.

# Example

  # List the revisions of a category
  mr categories revisions list --id 3

  # Print only the revision numbers
  mr categories revisions list --id 3 --json | jq -r '.[].number'

  # mr-doctest: a new category has one create revision, and a CLI edit adds one
  ID=$(mr category create --name "rev-$$-$RANDOM" --json | jq -r .ID)
  mr categories revisions list --id "$ID" --json | jq -e 'length == 1 and .[0].number == 1 and .[0].source == "create"'
  mr category edit --id "$ID" --custom-header '<h1>Hi</h1>'
  mr categories revisions list --id "$ID" --json | jq -e 'length == 2 and .[0].number == 2 and .[0].source == "cli" and .[0].changedSlots == ["CustomHeader"]'
  mr category delete "$ID"
