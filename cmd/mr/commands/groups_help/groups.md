---
exitCodes: 0 on success; 1 on any error
relatedCmds: group get, group create, resources list, mrql
---

# Long

Discover and bulk-mutate Groups. The `groups` subcommands operate on
multiple Groups at once: `list` for filtered queries (with pagination
via the global `--page` flag), `add-tags` / `remove-tags` for bulk
tag ops, `add-meta` for bulk metadata merges, `delete` / `merge` for
destructive consolidation, `meta-keys` to enumerate the observed meta
vocabulary, and `timeline` for an ASCII activity chart.

Most bulk-mutation commands select targets via `--ids=<csv>`; `merge`
uses `--winner` / `--losers` instead. The current CLI does not accept
MRQL selectors on bulk commands — pipe from `groups list --json | jq`
to extract IDs when you need query-based selection.
