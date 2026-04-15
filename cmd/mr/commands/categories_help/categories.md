---
exitCodes: 0 on success; 1 on any error
relatedCmds: category get, groups list, resource-category
---

# Long

Discover and inspect Categories. The `categories` subcommands operate
across multiple categories: `list` for filtered queries (with pagination
via the global `--page` flag) and `timeline` for an ASCII histogram of
category creation activity.

The CLI has no bulk-mutate variants for categories; use the singular
`category` commands (`create`, `delete`, `edit-name`, `edit-description`)
and pipe `categories list --json` through `jq` when you need to derive
IDs from a filter.
