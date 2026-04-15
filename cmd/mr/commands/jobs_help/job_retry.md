---
outputShape: Object with status set to "retrying"
exitCodes: 0 on success; 1 on any error
relatedCmds: job submit, jobs list, job cancel
---

# Long

Re-queue a failed or cancelled download job for another attempt.
Retry only works against jobs in the `failed` or `cancelled` state;
the server rejects retry on jobs that are still active, paused, or
already completed. The existing job's ID is reused — progress, error
message, and completion times are cleared, then the worker re-runs the
original URL fetch.

Useful when a transient network error blew up the first attempt.
Persistent failures need an updated URL, which means calling
`job submit` fresh rather than `job retry`.

# Example

  # Retry a specific failed job
  mr job retry a1b2c3d4

  # Retry every failed job in the queue
  mr jobs list --json | jq -r '.jobs[] | select(.status == "failed") | .id' | xargs -I {} mr job retry {}

  # mr-doctest: submit to an unreachable URL, wait for it to fail, retry it, assert the response
  JID=$(mr job submit --urls "http://127.0.0.1:9/nope.bin" --json | jq -r '.jobs[0].id')
  sleep 0.3
  mr jobs list --json | jq -e --arg j "$JID" '.jobs[] | select(.id == $j) | .status == "failed"'
  mr job retry $JID --json | jq -e '.status == "retrying"'
