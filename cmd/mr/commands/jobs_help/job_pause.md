---
outputShape: Object with status set to "paused" and canonicalJobId naming the paused Job
exitCodes: 0 on success; 1 on any error
relatedCmds: job resume, job cancel, jobs list
---

# Long

Suspend an in-flight download without cancelling it. `<id>` is the Job
id `jobs list` prints, or the legacy handle `job submit` returns as
`id`. Pause only works while the download is queued or running in the
server process that holds its transfer; the server rejects pause
requests against finished, cancelled, or already-paused jobs. The
transfer is cancelled, discarding the bytes received so far, and the
Job waits until you call `job resume`.

Generic jobs (group exports, imports) cannot be paused -- their runners
are not re-entrant. Pause is intended for long URL fetches.

# Example

  # Pause a specific download
  mr job pause 018f4db1-9b40-7f54-8f16-37a449bcf01d

  # Pause every download currently running
  mr jobs list --kind remote-download --state running --json | jq -r '.jobs[].id' | xargs -I {} mr job pause {}

  # mr-doctest: submit, pause by the id jobs list prints, verify the reported status, skip-on=auth
  JID=$(mr job submit --urls "$MAHRESOURCES_URL/v1/jobs/events" --json | jq -r '.jobs[0].canonicalJobId')
  sleep 0.3
  mr job pause $JID --json | jq -e '.status == "paused"'
  mr job cancel $JID --json >/dev/null
