---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource-category edit-description, resource-category get, resource-categories list
---

# Long

Update the name of an existing resource category. Takes two positional
arguments: the resource category ID and the new name. The name should
remain unique across resource categories. To rename and verify in one
step, chain with `mr resource-category get <id> --json`.

# Example

  # Rename resource category 42
  mr resource-category edit-name 42 "Photos"

  # Rename and confirm with a follow-up get
  mr resource-category edit-name 42 "renamed" && mr resource-category get 42 --json | jq -r .Name

  # mr-doctest: create, rename, verify
  ID=$(mr resource-category create --name "before-rc-$$-$RANDOM" --json | jq -r '.ID')
  NEWNAME="after-rc-$$-$RANDOM"
  mr resource-category edit-name $ID "$NEWNAME"
  mr resource-category get $ID --json | jq -e --arg n "$NEWNAME" '.Name == $n'
