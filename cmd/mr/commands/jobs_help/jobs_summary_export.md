---
outputShape: Accepted summary-export Job snapshot
exitCodes: 0 on success; 1 on any error
relatedCmds: jobs summary, jobs get, jobs timeline
---

# Long

Queue a filtered summary export for an explicit RFC3339 range longer than 90
days. The server applies the same visibility rules and filters as interactive
summary, then publishes a typed artifact on the accepted Job. The artifact
expires according to the server's export-retention setting. Read the Job with
`jobs get` to inspect its output and commands.

The accepted Job is owned by the submitting account. Filtering by another
owner or actor does not grant access to that person's Jobs.

An export cannot filter by `--state partial`, `--inbound-relationship` or
`--no-inbound-relationship`; the server refuses those with a 400. The export's
filter is stored and run later, possibly by a worker from an older release.
Such a worker fails an export filtered by the partial state, but it silently
ignores the inbound relationship filters and exports a wider summary. Use
`jobs summary` for those filters over a window of up to 90 days.

# Example

  # Queue a CSV export for a year of remote downloads
  mr jobs summary export --from 2025-01-01T00:00:00Z --to 2026-01-01T00:00:00Z --kind remote-download --format csv

  # Queue a JSON export with an owner filter
  mr jobs summary export --from 2024-01-01T00:00:00Z --to 2026-01-01T00:00:00Z --format json --owner-id 7

  # mr-doctest: export help exposes the explicit range and format flags
  mr jobs summary export --help | grep -- '--from' >/dev/null
