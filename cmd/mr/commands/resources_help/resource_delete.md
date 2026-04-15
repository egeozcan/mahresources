---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource get, resources list, resources delete
---

# Long

Delete a resource by ID. Destructive: removes both the database row and
the stored file bytes. Deleting a nonexistent ID returns exit code 1.

# Example

  # Delete a resource by ID
  mr resource delete 42

  # Delete and pipe the result to jq to confirm the response
  mr resource delete 42 --json | jq .

  # mr-doctest: upload, delete, verify via tolerant get
  GRP=$(mr group create --name "doctest-del-$$-$RANDOM" --json | jq -r '.ID')
  ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "to-delete-$$" --json | jq -r '.[0].ID')
  mr resource delete $ID
  ! mr resource get $ID 2>/dev/null
