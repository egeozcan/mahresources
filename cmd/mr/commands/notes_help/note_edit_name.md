---
exitCodes: 0 on success; 1 on any error
relatedCmds: note edit-description, note edit-meta, note get
---

# Long

Update only the name of an existing note. Takes two positional
arguments: the note ID and the new name. Use this when renaming is the
only change; for multi-field edits, prefer a single request via the
server API.

# Example

  # Rename note 42
  mr note edit-name 42 "renamed title"

  # Rename and confirm with a follow-up get
  mr note edit-name 42 "final draft" && mr note get 42 --json | jq -r .Name

  # mr-doctest: create, rename, verify
  ID=$(mr note create --name "before-$$-$RANDOM" --json | jq -r '.ID')
  NEWNAME="after-$$-$RANDOM"
  mr note edit-name $ID "$NEWNAME"
  mr note get $ID --json | jq -e --arg n "$NEWNAME" '.Name == $n'
