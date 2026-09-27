---
outputShape: Object with status set to "resumed" and canonicalJobId naming the resumed Job
exitCodes: 0 on success; 1 on any error
relatedCmds: job pause, job cancel, jobs list
---

# Long

Restart a previously paused download job. `<id>` is the Job id `jobs
list` prints, or the legacy handle `job submit` returns as `id`. Resume
only works against a paused download; a Job that is queued, running,
finished, or cancelled returns an error. The server opens a fresh HTTP
request and queues the Job again; the transfer starts when the
deployment's job budget has room.

Because the server does not keep partial bytes across pauses, resume
effectively restarts the download from the beginning.

# Example

  # Resume a specific paused download
  mr job resume 018f4db1-9b40-7f54-8f16-37a449bcf01d

  # Resume every visible Job that currently offers resume
  mr jobs list --command resume --json | jq -r '.jobs[].id' | xargs -I {} mr job resume {}

  # mr-doctest: submit, pause, resume by the id jobs list prints, verify each transition succeeds, skip-on=auth
  JID=$(mr job submit --urls "$MAHRESOURCES_URL/v1/jobs/events" --json | jq -r '.jobs[0].canonicalJobId')
  sleep 0.3
  mr job pause $JID --json | jq -e '.status == "paused"'
  mr job resume $JID --json | jq -e '.status == "resumed"'
  mr job cancel $JID --json >/dev/null
