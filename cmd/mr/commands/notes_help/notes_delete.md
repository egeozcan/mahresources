---
exitCodes: 0 on success; 1 on any error
relatedCmds: note delete, notes list
---

# Long

Bulk-delete Notes. Destructive: removes database rows for every Note
listed in `--ids` along with their tag/group/resource associations.
The current CLI has no dry-run; pipe `notes list --json` first if you
need to preview targets before deleting.

# Example

  # Delete specific notes
  mr notes delete --ids 42,43,44

  # Delete the output of a filter query
  mr notes list --tags 7 --json | jq -r '[.[].ID] | join(",")' | xargs -I {} mr notes delete --ids {}

  # mr-doctest: create, delete, assert follow-up get fails
  ID=$(mr note create --name "doctest-bulkdel-$$-$RANDOM" --json | jq -r '.ID')
  mr notes delete --ids $ID
  ! mr note get $ID 2>/dev/null
