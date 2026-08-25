---
exitCodes: 0 on success; 1 on any error
relatedCmds: tags delete, tag get, tag create
---

# Long

Delete a tag by ID. Destructive: removes the tag row and detaches it
from any Resources, Notes, or Groups it was attached to (the related
entities themselves are preserved). Deleting an ID that does not exist
fails with `HTTP 404: record not found` and exit code 1.

# Example

  # Delete a tag by ID
  mr tag delete 42

  # Delete and pipe the result to jq to confirm the response shape
  mr tag delete 42 --json | jq .

  # mr-doctest: create, delete, assert follow-up get fails
  ID=$(mr tag create --name "doctest-del-$$-$RANDOM" --json | jq -r '.ID')
  mr tag delete $ID
  ! mr tag get $ID 2>/dev/null
