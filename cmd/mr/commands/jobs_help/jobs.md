---
exitCodes: 0 on success; 1 on any error
relatedCmds: job submit, job cancel, resource from-url
---

# Long

The download queue is the server's in-memory list of URL download jobs. Each
job tracks a source URL, a status (pending, downloading, paused, completed,
failed, cancelled), progress counters, and the resulting Resource ID once
finished.

The plural `jobs` command group exposes read-only views of the queue. Use
`jobs list` for a full snapshot of every job the server is tracking. For
lifecycle controls (submit, pause, resume, retry, cancel) on a single job,
use the singular `job` subcommands.
