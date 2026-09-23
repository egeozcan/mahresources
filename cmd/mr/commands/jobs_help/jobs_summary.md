---
outputShape: Aggregate counts and duration statistics for visible Jobs
exitCodes: 0 on success; 1 on any error
relatedCmds: jobs list, jobs summary export
---

# Long

Aggregate visible Jobs over the last 90 days or less. Apply the same state,
kind, origin, owner, actor, relationship, search, and viewer-preference filters
as `jobs list`. Use `--window` to choose a shorter interval. For an explicit
range longer than 90 days, use `jobs summary export` to create a durable Job
with a CSV or JSON artifact.

# Example

  # Count a month of remote downloads
  mr jobs summary --window 30d --kind remote-download --json

  # Queue a longer explicit range as a CSV export
  mr jobs summary export --from 2025-01-01T00:00:00Z --to 2026-01-01T00:00:00Z --format csv
