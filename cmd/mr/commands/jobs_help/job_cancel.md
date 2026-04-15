---
outputShape: Object with status set to "cancelled"
exitCodes: 0 on success; 1 on any error
relatedCmds: job submit, job pause, jobs list
---

# Long

Stop an active download job. Cancel only works while the job is still
in progress (pending, downloading, or processing); the server rejects
cancellation of jobs that have already finished, been cancelled, or are
paused. On success the server marks the job `cancelled` and leaves it
in the queue for inspection.

Use `jobs list` to see which jobs are eligible — any job with a status
other than pending, downloading, or processing cannot be cancelled.

# Example

  # Cancel a specific job
  mr job cancel a1b2c3d4

  # Pipe through jq to cancel every active job
  mr jobs list --json | jq -r '.jobs[] | select(.status == "downloading" or .status == "pending") | .id' | xargs -I {} mr job cancel {}

  # mr-doctest: submit a long-running job against the live server, cancel it, assert status flips
  JID=$(mr job submit --urls "$MAHRESOURCES_URL/v1/jobs/events" --json | jq -r '.jobs[0].id')
  sleep 0.3
  mr job cancel $JID --json | jq -e '.status == "cancelled"'
