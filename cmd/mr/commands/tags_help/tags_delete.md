---
exitCodes: 0 on success; 1 on any error
relatedCmds: tag delete, tags merge, tags list
---

# Long

Bulk-delete Tags. Destructive: removes the tag rows and detaches them
from any Resources, Notes, or Groups they were attached to (the related
entities themselves are preserved). Target tags are selected via
`--ids` (CSV of unsigned ints). The current CLI has no dry-run; pipe
`tags list --json` first if you need to preview targets.

# Example

  # Delete specific tags
  mr tags delete --ids 42,43,44

  # Delete all tags matching a name filter
  mr tags delete --ids $(mr tags list --name "obsolete-" --json | jq -r 'map(.ID) | join(",")')

  # mr-doctest: create two, bulk-delete, assert both are gone
  A=$(mr tag create --name "bulkdel-a-$$-$RANDOM" --json | jq -r '.ID')
  B=$(mr tag create --name "bulkdel-b-$$-$RANDOM" --json | jq -r '.ID')
  mr tags delete --ids $A,$B
  ! mr tag get $A 2>/dev/null
