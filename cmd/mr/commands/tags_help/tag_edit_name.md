---
exitCodes: 0 on success; 1 on any error
relatedCmds: tag edit-description, tag get, tags list
---

# Long

Update the name of an existing tag. Takes two positional arguments: the
tag ID and the new name. The name must remain unique across tags; the
server rejects duplicates. To rename and verify in one step, chain with
`mr tag get <id> --json`.

# Example

  # Rename tag 42
  mr tag edit-name 42 "important"

  # Rename and confirm with a follow-up get
  mr tag edit-name 42 "renamed" && mr tag get 42 --json | jq -r .Name

  # mr-doctest: create, rename, verify
  ID=$(mr tag create --name "before-$$-$RANDOM" --json | jq -r '.ID')
  NEWNAME="after-$$-$RANDOM"
  mr tag edit-name $ID "$NEWNAME"
  mr tag get $ID --json | jq -e --arg n "$NEWNAME" '.Name == $n'
