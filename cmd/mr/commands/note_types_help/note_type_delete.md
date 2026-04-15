---
exitCodes: 0 on success; 1 on any error
relatedCmds: note-type get, note-type create, note-types list
---

# Long

Delete a note type by ID. Destructive: removes the note type row. Notes
that referenced it keep their rows but lose the typed schema link, so
use with care on instances where Notes depend on the type's MetaSchema
for rendering. Deleting a nonexistent ID is a no-op on the server but
still returns success.

# Example

  # Delete a note type by ID
  mr note-type delete 42

  # Delete and pipe the result to jq to confirm the response shape
  mr note-type delete 42 --json | jq .

  # mr-doctest: create, delete, assert follow-up get fails
  ID=$(mr note-type create --name "doctest-nt-del-$$-$RANDOM" --json | jq -r '.ID')
  mr note-type delete $ID
  ! mr note-type get $ID 2>/dev/null
