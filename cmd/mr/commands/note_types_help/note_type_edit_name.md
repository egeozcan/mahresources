---
exitCodes: 0 on success; 1 on any error
relatedCmds: note-type edit-description, note-type edit, note-type get
---

# Long

Update only the name of an existing note type. Takes two positional
arguments: the note type ID and the new name. Shorthand for
`mr note-type edit --id <id> --name <value>` when name is the only
change. Returns `{"id":N,"ok":true}` on success; chain with
`mr note-type get <id>` to inspect the renamed record.

# Example

  # Rename note type 1
  mr note-type edit-name 1 "Team Meeting"

  # Rename and confirm with a follow-up get
  mr note-type edit-name 1 "renamed" && mr note-type get 1 --json | jq -r .Name

  # mr-doctest: create, rename, verify
  ID=$(mr note-type create --name "before-nt-$$-$RANDOM" --json | jq -r '.ID')
  NEWNAME="after-nt-$$-$RANDOM"
  mr note-type edit-name $ID "$NEWNAME"
  mr note-type get $ID --json | jq -e --arg n "$NEWNAME" '.Name == $n'
