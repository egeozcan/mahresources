---
outputShape: Object with status set to "cancelled"
exitCodes: 0 on success; 1 on any error
relatedCmds: job submit, job pause, jobs list
---

# Long

Stop a job that has not finished. Cancel works while the job is pending,
downloading, processing, or **paused**; the server rejects cancellation of
jobs that have already completed, failed, or been cancelled, answering
HTTP 409 Conflict. On success the server marks the job `cancelled` and
leaves it in the queue for inspection.

Use `jobs list` to see which jobs are eligible — any job with a status
other than pending, downloading, processing, or paused cannot be
cancelled.

# Example

  # Cancel a specific job
  mr job cancel a1b2c3d4

  # Pipe through jq to cancel every active job
  mr jobs list --json | jq -r '.jobs[] | select(.status == "downloading" or .status == "pending") | .id' | xargs -I {} mr job cancel {}

  # mr-doctest: submit a long-running job against the live server, cancel it, assert status flips
  JID=$(mr job submit --urls "$MAHRESOURCES_URL/v1/jobs/events" --json | jq -r '.jobs[0].id')
  sleep 0.3
  mr job cancel $JID --json | jq -e '.status == "cancelled"'
