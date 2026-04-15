---
exitCodes: 0 on success; 1 on any error
relatedCmds: note get, notes delete, notes list
---

# Long

Delete a note by ID. Destructive: removes the database row and all of
its tag/group/resource associations. Deleting a nonexistent ID returns
exit code 1 with an HTTP 404 error message.

# Example

  # Delete a note by ID
  mr note delete 42

  # Delete and pipe the response to jq to confirm
  mr note delete 42 --json | jq .

  # mr-doctest: create, delete, verify subsequent get fails
  ID=$(mr note create --name "doctest-del-$$-$RANDOM" --json | jq -r '.ID')
  mr note delete $ID
  ! mr note get $ID 2>/dev/null
