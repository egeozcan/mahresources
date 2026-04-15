---
exitCodes: 0 on success; 1 on any error
relatedCmds: query get, query create, queries list
---

# Long

Delete a saved query by ID. Destructive: removes the database row
for the query. Any downstream references (saved dashboards, bookmarks)
should be updated separately. Deleting a nonexistent ID returns exit
code 1.

# Example

  # Delete a query by ID
  mr query delete 42

  # Delete and pipe the result to jq to confirm
  mr query delete 42 --json | jq .

  # mr-doctest: create a query, delete it, verify get fails
  NAME="doctest-del-$$-$RANDOM"
  ID=$(mr query create --name "$NAME" --text "select 1 as x" --json | jq -r '.ID')
  mr query delete $ID
  ! mr query get $ID 2>/dev/null
