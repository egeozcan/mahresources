---
outputShape: Object with status set to "retrying" and canonicalJobId naming the new Retry Job
exitCodes: 0 on success; 1 on any error
relatedCmds: job submit, jobs list, job cancel
---

# Long

Queue another attempt of a failed or cancelled download. Retry creates
a new Job linked to the one it retries and leaves the original Job's
outcome as it was; the answer's `canonicalJobId` names the new Job.
`<id>` is the Job id `jobs list` prints, or the legacy handle `job
submit` returns as `id`. A legacy handle moves to the new Job, so the
same handle can be retried again later; a Job id keeps naming the Job
it was, and a Job that already has a Retry cannot be retried again
through its own id.

Retry only works against a Job that failed or was cancelled; the
server rejects it for a Job that is still active, paused, or
succeeded. A retry is also refused with HTTP 409 while any queued or
running job is already fetching the same URL, so one URL is never
transferred twice.

Useful when a transient network error blew up the first attempt.
Persistent failures need an updated URL, which means calling
`job submit` fresh rather than `job retry`.

# Example

  # Retry a specific failed Job
  mr job retry 018f4db1-9b40-7f54-8f16-37a449bcf01d

  # Retry every visible Job that currently offers Retry
  mr jobs list --command retry --json | jq -r '.jobs[].id' | xargs -I {} mr job retry {}

  # mr-doctest: submit to an unreachable URL, wait for it to fail, retry it by the id jobs list prints, assert the response
  JID=$(mr job submit --urls "http://127.0.0.1:9/nope.bin" --json | jq -r '.jobs[0].canonicalJobId')
  sleep 0.3
  mr jobs list --json | jq -e --arg j "$JID" '.jobs[] | select(.id == $j) | .state == "failed"'
  mr job retry $JID --json | jq -e --arg j "$JID" '.status == "retrying" and .canonicalJobId != $j'
