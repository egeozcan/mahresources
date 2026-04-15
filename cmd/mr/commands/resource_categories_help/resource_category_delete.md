---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource-category get, resource-category create, resource-categories list
---

# Long

Delete a resource category by ID. Destructive: removes the resource
category row. Resources that reference this category remain but lose
their category association. Deleting a nonexistent ID may still return
success at the server level.

# Example

  # Delete a resource category by ID
  mr resource-category delete 42

  # Delete and pipe the result to jq to inspect the response
  mr resource-category delete 42 --json | jq .

  # mr-doctest: create, delete, assert follow-up get fails
  ID=$(mr resource-category create --name "doctest-rc-del-$$-$RANDOM" --json | jq -r '.ID')
  mr resource-category delete $ID
  ! mr resource-category get $ID 2>/dev/null
