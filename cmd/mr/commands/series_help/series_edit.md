---
exitCodes: 0 on success; 1 on any error
relatedCmds: series edit-name, series get, series list
---

# Long

Edit a series. `--name` is required on every call; `--meta` is optional
and takes a JSON string merged into the series meta. The slug is derived
from the original name at creation time and is not updated by this
command, so changing the name here leaves the slug untouched.

# Example

  # Rename a series and set meta in one call
  mr series edit 42 --name "volume-1-final" --meta '{"season":"fall"}'

  # Rename only (meta unchanged)
  mr series edit 42 --name "renamed"

  # mr-doctest: create, edit, verify name via get
  ID=$(mr series create --name "before-$$-$RANDOM" --json | jq -r '.ID')
  NEWNAME="after-$$-$RANDOM"
  mr series edit $ID --name "$NEWNAME" --meta '{"ok":true}'
  mr series get $ID --json | jq -e --arg n "$NEWNAME" '.Name == $n'
