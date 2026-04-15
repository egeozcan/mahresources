---
outputShape: Object with status set to "resumed"
exitCodes: 0 on success; 1 on any error
relatedCmds: job pause, job cancel, jobs list
---

# Long

Restart a previously paused download job. Resume only works against
jobs currently in the `paused` state — jobs that are pending, running,
finished, or cancelled return an error. The server opens a fresh HTTP
request, resets the progress counters, and marks the job `pending`;
the background worker picks it up on the next scheduler tick.

Because the server does not keep partial bytes across pauses, resume
effectively restarts the download from the beginning.

# Example

  # Resume a specific paused job
  mr job resume a1b2c3d4

  # Resume every paused job in one pass
  mr jobs list --json | jq -r '.jobs[] | select(.status == "paused") | .id' | xargs -I {} mr job resume {}

  # mr-doctest: submit, pause, resume, verify each transition succeeds
  JID=$(mr job submit --urls "$MAHRESOURCES_URL/v1/jobs/events" --json | jq -r '.jobs[0].id')
  sleep 0.3
  mr job pause $JID --json | jq -e '.status == "paused"'
  mr job resume $JID --json | jq -e '.status == "resumed"'
  mr job cancel $JID --json >/dev/null || true
