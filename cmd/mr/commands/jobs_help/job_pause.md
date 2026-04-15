---
outputShape: Object with status set to "paused"
exitCodes: 0 on success; 1 on any error
relatedCmds: job resume, job cancel, jobs list
---

# Long

Suspend an in-flight download without cancelling it. Pause only works
while the job is pending or downloading; the server rejects pause
requests against finished, cancelled, or already-paused jobs. The
background goroutine stops after the current chunk and the job stays
in the queue with status `paused` until you call `job resume`.

Generic jobs (group exports, imports) cannot be paused — their runners
are not re-entrant. Pause is intended for long URL fetches.

# Example

  # Pause a specific job
  mr job pause a1b2c3d4

  # Pause every job currently downloading
  mr jobs list --json | jq -r '.jobs[] | select(.status == "downloading") | .id' | xargs -I {} mr job pause {}

  # mr-doctest: submit, pause, verify the reported status
  JID=$(mr job submit --urls "$MAHRESOURCES_URL/v1/jobs/events" --json | jq -r '.jobs[0].id')
  sleep 0.3
  mr job pause $JID --json | jq -e '.status == "paused"'
  mr job cancel $JID --json >/dev/null || true
