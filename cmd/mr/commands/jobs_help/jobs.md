---
exitCodes: 0 on success; 1 on any error
relatedCmds: job submit, job cancel, resource from-url
---

# Long

The download queue is the server's in-memory job list. Most entries are URL
downloads; group exports and imports run on the same queue and appear in
`jobs list` with a `source` of `group-export`, `group-import-parse` or
`group-import-apply` and no URL. Each job tracks a source URL, a status
(pending, downloading, processing, paused, completed, failed, cancelled),
progress counters, and the resulting Resource ID once finished.

The plural `jobs` command group exposes read-only views of the queue. Use
`jobs list` for a snapshot of the jobs visible to you: administrators see
every job the server is tracking, and every other account sees only the
jobs it submitted. For
lifecycle controls (submit, pause, resume, retry, cancel) on a single job,
use the singular `job` subcommands.
