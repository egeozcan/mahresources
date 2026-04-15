---
exitCodes: 0 on success; 1 on any error
relatedCmds: note get, groups list, search, mrql
---

# Long

Discover and bulk-mutate Notes. The `notes` subcommands operate on
multiple Notes at once: `list` for filtered queries (with pagination
via global `--page`), `add-tags` / `remove-tags` for bulk tag ops,
`add-groups` / `add-meta` for bulk annotation, `delete` for destructive
bulk removal, `meta-keys` for discovering the meta-schema vocabulary,
and `timeline` for ASCII activity charts.

Bulk-mutation commands select targets via `--ids=<csv>`. The current
CLI does not support MRQL selectors on bulk commands — pipe from
`notes list --json | jq` to extract IDs when you need query-based
selection.
