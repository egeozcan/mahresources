---
exitCodes: 0 on success; 1 on any error
relatedCmds: query get, query run, mrql
---

# Long

Discover and summarize saved Queries. The `queries` subcommands
operate on the collection: `list` returns queries (paged via the
global `--page` flag, optionally filtered by `--name`), and `timeline`
aggregates query creation and update activity into an ASCII bar chart.

To execute a query, use `query run <id>` or `query run-by-name --name
<name>` from the singular `query` subtree.
