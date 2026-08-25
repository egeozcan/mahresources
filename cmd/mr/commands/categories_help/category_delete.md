---
exitCodes: 0 on success; 1 on any error
relatedCmds: category get, category create, categories list
---

# Long

Delete a Category by ID. Destructive: removes the category row. Groups
previously assigned to this category become uncategorized (the group
records themselves are preserved). Every group relation type naming this
category on either side is deleted with it, and so is every relation edge
of those types, across the whole database. Deleting an ID that does not
exist fails with `HTTP 404: record not found` and exit code 1.

# Example

  # Delete a category by ID
  mr category delete 42

  # Delete and pipe the result to jq to confirm the response shape
  mr category delete 42 --json | jq .

  # mr-doctest: create, delete, assert follow-up get fails
  ID=$(mr category create --name "doctest-del-$$-$RANDOM" --json | jq -r '.ID')
  mr category delete $ID
  ! mr category get $ID 2>/dev/null
