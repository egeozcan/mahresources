---
exitCodes: 0 on success; 1 on any error
relatedCmds: group create, groups delete, groups merge
---

# Long

Delete a Group by ID. Destructive: removes the Group row and its
direct join-table entries (tag links, m2m relations). Owned children,
resources, and notes are orphaned (their `OwnerId` becomes null) rather
than cascaded. Use `groups delete --ids=...` for bulk deletion, or
`groups merge` to consolidate rather than destroy.

# Example

  # Delete a single group
  mr group delete 42

  # mr-doctest: create a group, delete it, confirm follow-up get fails
  ID=$(mr group create --name "doctest-del-$$-$RANDOM" --json | jq -r '.ID')
  mr group delete $ID
  ! mr group get $ID 2>/dev/null
