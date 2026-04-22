---
outputShape: Combined stats object {serverStats, dataStats, expensiveStats} in JSON mode; three sectioned tables in human mode
exitCodes: 0 on success; 1 on any error
relatedCmds: resources versions-cleanup, jobs list, logs list
---

# Long

Show administrative statistics about the running server and its data. By default the command fetches three sections — server health (uptime, memory, DB connections), data counts (entity totals), and expensive stats that require full-table scans (hash collisions, dangling references). Together they give a one-page picture of instance size and health.

Use `--server-only` to fetch just the server health block, or `--data-only` to fetch just the data counts — useful for lightweight monitoring that skips the expensive scans. Neither flag is required; when both are unset the command fetches all three sections.

# Example

  # Full admin stats (human-readable, three sections)
  mr admin

  # Server health only, JSON output
  mr admin --server-only --json

  # Data counts only
  mr admin --data-only

  # mr-doctest: fetch combined stats and assert the response shape
  mr admin --json | jq -e '.serverStats and .dataStats and .expensiveStats'
