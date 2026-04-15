---
exitCodes: 0 on success; 1 on any error
relatedCmds: note-block get, note-blocks list, note delete
---

# Long

Delete a note block by ID. Destructive: removes the database row.
Deleting a nonexistent ID returns exit code 1 with an HTTP 404 error.
Deleting a block does not affect its parent Note or sibling blocks;
to remove every block on a note, delete the note itself.

# Example

  # Delete a note block by ID
  mr note-block delete 42

  # Delete and pipe the response to jq to confirm
  mr note-block delete 42 --json | jq .

  # mr-doctest: create, delete, verify subsequent get fails
  NID=$(mr note create --name "doctest-nb-del-$$-$RANDOM" --json | jq -r '.ID')
  BID=$(mr note-block create --note-id=$NID --type=text --content '{"text":"bye"}' --json | jq -r '.id')
  mr note-block delete $BID
  ! mr note-block get $BID 2>/dev/null
