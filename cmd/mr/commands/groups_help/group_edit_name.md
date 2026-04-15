---
outputShape: Status object with id (uint) and ok (bool)
exitCodes: 0 on success; 1 on any error
relatedCmds: group get, group edit-description, group edit-meta
---

# Long

Replace a Group's `Name` field. Takes the Group ID and the new name
as positional arguments. Sends `POST /v1/group/editName` and returns
`{id, ok}` on success. Use `group get` afterward to view the updated
record.

# Example

  # Rename group 42
  mr group edit-name 42 "Trips to Berlin"

  # mr-doctest: create, rename, verify by reading the group back
  ID=$(mr group create --name "doctest-rename-$$-$RANDOM" --json | jq -r '.ID')
  NEW="renamed-$$-$RANDOM"
  mr group edit-name $ID "$NEW"
  mr group get $ID --json | jq --arg n "$NEW" -e '.Name == $n'
