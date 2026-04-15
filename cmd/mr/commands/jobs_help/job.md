---
exitCodes: 0 on success; 1 on any error
relatedCmds: jobs list, resource from-url, admin
---

# Long

A download job fetches a remote URL and stores the result as a new
Resource. Each submission creates one job per URL; the server downloads
in the background while the queue tracks progress, pause/resume, and
retry state. Jobs are ephemeral — they live in server memory and do not
persist across restarts.

Use the `job` subcommands to operate on a single job by ID: `submit`
new URLs, `cancel` an active job, `pause` / `resume` an in-flight
transfer, or `retry` a failed one. Use `jobs list` to discover IDs and
check current statuses.
