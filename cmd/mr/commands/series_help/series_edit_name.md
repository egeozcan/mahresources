---
exitCodes: 0 on success; 1 on any error
relatedCmds: series edit, series get, series list
---

# Long

Update only the name of an existing series. Shorthand for `mr series
edit <id> --name <value>` when the name is the only change. Takes two
positional arguments: the series ID and the new name. The slug is
derived from the original name at creation time and is not changed by
this command.

# Example

  # Rename series 42
  mr series edit-name 42 "volume-1-final"

  # Rename and confirm with a follow-up get
  mr series edit-name 42 "renamed" && mr series get 42 --json | jq -r .Name

  # mr-doctest: create, rename, verify
  ID=$(mr series create --name "before-$$-$RANDOM" --json | jq -r '.ID')
  NEWNAME="after-$$-$RANDOM"
  mr series edit-name $ID "$NEWNAME"
  mr series get $ID --json | jq -e --arg n "$NEWNAME" '.Name == $n'
