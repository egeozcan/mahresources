---
exitCodes: 0 on success; 1 on any error
relatedCmds: note-type edit-name, note-type edit, note-type get
---

# Long

Update only the description of an existing note type. Takes two
positional arguments: the note type ID and the new description.
Passing an empty string clears the description. Useful for annotating
a note type's intended use without touching its MetaSchema or rendering
fields. Returns `{"id":N,"ok":true}` on success.

# Example

  # Set a description on note type 1
  mr note-type edit-description 1 "for weekly engineering standups"

  # Clear the description by passing an empty string
  mr note-type edit-description 1 ""

  # mr-doctest: create, set description, verify
  ID=$(mr note-type create --name "nt-desc-$$-$RANDOM" --json | jq -r '.ID')
  mr note-type edit-description $ID "hello world"
  mr note-type get $ID --json | jq -e '.Description == "hello world"'
