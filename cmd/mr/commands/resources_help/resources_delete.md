---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource delete, resources merge, resources list
---

# Long

Bulk-delete Resources. Destructive: removes both the database rows and
the stored file bytes. Target Resources are selected via `--ids` (CSV
of unsigned ints). The current CLI has no dry-run; pipe
`resources list --json` first if you need to preview targets.

# Example

  # Delete specific resources
  mr resources delete --ids 42,43,44

  # Delete the output of a filter query
  mr resources list --tags 7 --json | jq -r 'map(.id) | join(",")' | xargs -I {} mr resources delete --ids {}

  # mr-doctest: upload, delete, assert follow-up get fails, tolerate=/not found|404|does not exist/i
  ID=$(mr resource upload ./testdata/sample.jpg --name "bulkdel-$$" --json | jq -r .id)
  mr resources delete --ids $ID
  # mr-doctest: verify deletion, expect-exit=1, tolerate=/not found|404|does not exist/i
  mr resource get $ID
