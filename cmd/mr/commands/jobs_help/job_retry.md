---
outputShape: Object with status set to "retrying"
exitCodes: 0 on success; 1 on any error
relatedCmds: job submit, jobs list, job cancel
---

# Long

Re-queue a failed or cancelled download job for another attempt.
Retry only works against jobs in the `failed` or `cancelled` state;
the server rejects retry on jobs that are still active, paused, or
already completed. A retry is also refused with HTTP 409 while any queued
or running job is already fetching the same URL, so one URL is never
transferred twice, and for a failure a retry would repeat: a duplicate
of content the library holds, a stream the server will not assemble, or
a remote 4xx other than 403, 408, 423, 425 and 429.

The ID you pass keeps working. For a download the Job Center records,
the retry is a new Job linked to the failed one, which keeps its
outcome, and the ID moves to the new Job. A download from before the
Job Center is retried in place: its progress, error message and
completion times are cleared, then the worker fetches the URL again.

Useful when a transient network error blew up the first attempt.
Persistent failures need an updated URL, which means calling
`job submit` fresh rather than `job retry`.

# Example

  # Retry a specific failed job
  mr job retry a1b2c3d4

  # Retry every failed job in the queue
  mr jobs list --json | jq -r '.jobs[] | select(.status == "failed") | .id' | xargs -I {} mr job retry {}

  # mr-doctest: submit to an unreachable URL, wait for it to fail, retry it, assert the response
  SUBMISSION=$(mr job submit --urls "http://127.0.0.1:9/nope.bin" --json)
  JID=$(printf '%s' "$SUBMISSION" | jq -r '.jobs[0].id')
  CID=$(printf '%s' "$SUBMISSION" | jq -r '.jobs[0].canonicalJobId')
  sleep 0.3
  mr jobs list --json | jq -e --arg j "$CID" '.jobs[] | select(.id == $j) | .state == "failed"'
  mr job retry $JID --json | jq -e '.status == "retrying"'
