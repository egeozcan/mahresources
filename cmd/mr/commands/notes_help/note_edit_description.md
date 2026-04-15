---
exitCodes: 0 on success; 1 on any error
relatedCmds: note edit-name, note edit-meta, note get
---

# Long

Update only the description of an existing note. Takes two positional
arguments: the note ID and the new description. Passing an empty
string clears the description.

# Example

  # Set the description on note 42
  mr note edit-description 42 "raw brainstorm, needs polish"

  # Clear the description by passing an empty string
  mr note edit-description 42 ""

  # mr-doctest: create, set description, verify
  ID=$(mr note create --name "doctest-desc-$$-$RANDOM" --json | jq -r '.ID')
  mr note edit-description $ID "hello world"
  mr note get $ID --json | jq -e '.Description == "hello world"'
