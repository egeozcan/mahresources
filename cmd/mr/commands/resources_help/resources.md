---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource get, groups list, search, mrql
---

# Long

Discover and bulk-mutate Resources. The `resources` subcommands operate
on multiple Resources at once: `list` for filtered queries (with
pagination via global `--page`), `add-tags` / `remove-tags` /
`replace-tags` for bulk tag ops, `add-groups` / `add-meta` for bulk
annotation, and `delete` / `merge` for destructive operations.

Most bulk-mutation commands select targets via `--ids=<csv>`; `merge`
uses `--winner` / `--losers` instead. The current CLI does not support
MRQL selectors on bulk commands — pipe from `resources list --json | jq`
to extract IDs when you need query-based selection.
