---
outputShape: Status object with id (uint) and ok (bool)
exitCodes: 0 on success; 1 on any error
relatedCmds: group get, group edit-name, group edit-meta
---

# Long

Replace a Group's `Description` field. Takes the Group ID and the new
description as positional arguments. Sends `POST /v1/group/editDescription`
and returns `{id, ok}` on success. Descriptions are free-form text used
for human-readable context; for structured metadata use `edit-meta`.

# Example

  # Update the description on group 42
  mr group edit-description 42 "Our summer 2026 travel photos"

  # mr-doctest: create, set description, verify by reading back
  ID=$(mr group create --name "doctest-desc-$$-$RANDOM" --json | jq -r '.ID')
  DESC="a test description $RANDOM"
  mr group edit-description $ID "$DESC"
  mr group get $ID --json | jq --arg d "$DESC" -e '.Description == $d'
