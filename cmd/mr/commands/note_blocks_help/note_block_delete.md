---
exitCodes: 0 on success; 1 on any error
relatedCmds: note-block get, note-blocks list, note delete
---

# Long

Delete a note block by ID. Destructive: removes the database row.
Deleting a nonexistent ID returns exit code 1 with an HTTP 404 error.
Sibling blocks are untouched, but deleting a `text` block re-syncs the
parent Note's description to whatever text block now sorts first. To
remove every block on a note, delete the note itself.

# Example

  # Delete a note block by ID
  mr note-block delete 42

  # Delete, then confirm the block is gone from its note
  mr note-block delete 42 && mr note-blocks list --note-id 7

  # mr-doctest: create, delete, verify subsequent get fails
  NID=$(mr note create --name "doctest-nb-del-$$-$RANDOM" --json | jq -r '.ID')
  BID=$(mr note-block create --note-id=$NID --type=text --content '{"text":"bye"}' --json | jq -r '.id')
  mr note-block delete $BID
  ! mr note-block get $BID 2>/dev/null
