---
exitCodes: 0 on success; 1 on any error
relatedCmds: jobs list, jobs get, resource from-url, admin
---

# Long

A download job fetches a remote URL and stores the result as a new
Resource. Each submission creates one job per URL; the server downloads
in the background while the queue tracks progress, pause/resume, and
retry state. Queue entries are ephemeral: they live in server memory and
do not persist across restarts. Every download is also a durable Job,
readable in the Job Center at `/jobs` and through `jobs list` until Job
retention removes it; exports and imports are durable Jobs as well.

Use the `job` subcommands to operate on a single job by ID: `submit`
new URLs, `cancel` an active job, `pause` / `resume` an in-flight
transfer, or `retry` a failed one. `cancel`, `pause`, `resume` and
`retry` take either the Job id that `jobs list` prints or the legacy
handle `job submit` returns as `id`. A legacy handle follows the
current Retry of its Job; a Job id always names the one Job. Use
`jobs list` to discover Job ids and their current `state`, and
`job command` for any other command a Job advertises.
