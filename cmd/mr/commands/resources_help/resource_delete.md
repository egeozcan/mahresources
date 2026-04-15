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

  # mr-doctest: upload, delete, expect not-found on follow-up get
  ID=$(mr resource upload ./testdata/sample.jpg --name "to-delete" --json | jq -r .id)
  mr resource delete $ID

  # mr-doctest: follow-up get should fail, tolerate=/not found|404|does not exist/i
  mr resource get $ID
