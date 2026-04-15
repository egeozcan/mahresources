---
exitCodes: 0 on success; 1 on any error
relatedCmds: tag get, resources list, notes list
---

# Long

Discover and bulk-manage Tags. The `tags` subcommands operate across
multiple tags: `list` for filtered queries (with pagination via global
`--page`), `merge` for folding one or more tags into a single winner,
`delete` for bulk removal, and `timeline` for an activity histogram.

Selection for destructive commands is by ID: `merge` uses
`--winner` / `--losers`, `delete` uses `--ids`. Pipe `tags list --json`
through `jq` when you need to derive IDs from a filter.
