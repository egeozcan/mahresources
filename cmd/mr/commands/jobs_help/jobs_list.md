---
outputShape: Canonical page with visible Jobs and an optional nextCursor
exitCodes: 0 on success; 1 on any error
relatedCmds: jobs get, jobs timeline, jobs summary, job submit
---

# Long

List the durable Jobs visible to the current account. The server orders results
newest first and returns an opaque `nextCursor` when another page is available.
Pass that value to `--cursor` to continue. Use the repeatable state, kind, and
origin filters, or narrow by owner, actor, accepted time, relationship, text,
advertised command, or your pin and dismissal preferences.

The canonical list endpoint is controlled by the server's Job Center release
gate. While that endpoint is unavailable, an unfiltered `jobs list` request
falls back to the legacy download queue. Use `jobs queue` when a script needs
the legacy response explicitly.

# Example

  # Find failed remote downloads
  mr jobs list --state failed --kind remote-download --limit 50

  # Continue from an opaque cursor on the next page
  mr jobs list --accepted-after 2026-01-01T00:00:00Z --cursor 'opaque-value'

  # mr-doctest: list returns a jobs array on the canonical route or compatibility fallback
  mr jobs list --json | jq -e 'has("jobs") and (.jobs | type == "array")'
