---
exitCodes: 0 on success; 1 on any error
relatedCmds: group delete, groups merge, groups list
---

# Long

Bulk-delete Groups. Destructive: removes each selected Group row and
its direct join-table entries (tag links, m2m relations). Owned
children, resources, and notes are orphaned (their `OwnerId` becomes
null). Targets are selected via `--ids` (CSV of unsigned ints).

The current CLI has no dry-run; pipe `groups list --json | jq` first
if you need to preview targets, or use `groups merge` to consolidate
rather than destroy.

# Example

  # Delete specific groups
  mr groups delete --ids 42,43,44

  # Delete the output of a filter query
  mr groups list --tags 7 --json | jq -r 'map(.ID) | join(",")' | xargs -I {} mr groups delete --ids {}

  # mr-doctest: create a group, bulk-delete it, assert follow-up get fails
  ID=$(mr group create --name "doctest-bulkdel-$$-$RANDOM" --json | jq -r '.ID')
  mr groups delete --ids=$ID
  ! mr group get $ID 2>/dev/null
