---
exitCodes: 0 on success; 1 on any error
relatedCmds: series list, series get, series create
---

# Long

Delete a series by ID. Destructive: removes the series row. Resources
previously attached to the series keep their bytes but have their
`SeriesId` cleared (the foreign key uses `ON DELETE SET NULL`). Deleting
a nonexistent ID returns exit code 1.

# Example

  # Delete a series by ID
  mr series delete 42

  # Delete and pipe the result to jq to confirm the response shape
  mr series delete 42 --json | jq .

  # mr-doctest: create, delete, assert a follow-up get fails
  ID=$(mr series create --name "doctest-del-$$-$RANDOM" --json | jq -r '.ID')
  mr series delete $ID
  ! mr series get $ID 2>/dev/null
