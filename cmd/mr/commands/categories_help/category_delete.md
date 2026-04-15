---
exitCodes: 0 on success; 1 on any error
relatedCmds: category get, category create, categories list
---

# Long

Delete a Category by ID. Destructive: removes the category row. Groups
previously assigned to this category become uncategorized (the group
records themselves are preserved). Deleting a nonexistent ID is a no-op
on the server but still returns success.

# Example

  # Delete a category by ID
  mr category delete 42

  # Delete and pipe the result to jq to confirm the response shape
  mr category delete 42 --json | jq .

  # mr-doctest: create, delete, assert follow-up get fails
  ID=$(mr category create --name "doctest-del-$$-$RANDOM" --json | jq -r '.ID')
  mr category delete $ID
  ! mr category get $ID 2>/dev/null
