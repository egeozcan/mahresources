---
outputShape: Object with status set to "cancelled" and canonicalJobId naming the cancelled Job
exitCodes: 0 on success; 1 on any error
relatedCmds: job submit, job pause, jobs list
---

# Long

Stop a job that has not finished. `<id>` is the Job id `jobs list`
prints, or the legacy handle `job submit` returns as `id`. Cancel works
while the Job is queued, running, or paused; the server rejects
cancellation of a Job that has already succeeded, failed, or been
cancelled, answering HTTP 409 Conflict. On success the Job is recorded
as `cancelled` and stays readable through `jobs get` for inspection.

Use `jobs list --command cancel` to see which Jobs currently offer
cancellation.

# Example

  # Cancel a specific Job
  mr job cancel 018f4db1-9b40-7f54-8f16-37a449bcf01d

  # Cancel every visible Job that currently offers cancellation
  mr jobs list --command cancel --json | jq -r '.jobs[].id' | xargs -I {} mr job cancel {}

  # mr-doctest: submit a long-running job, cancel it by the id jobs list prints, assert status flips, skip-on=auth
  JID=$(mr job submit --urls "$MAHRESOURCES_URL/v1/jobs/events" --json | jq -r '.jobs[0].canonicalJobId')
  sleep 0.3
  mr jobs list --json | jq -e --arg j "$JID" '.jobs | map(.id) | index($j) != null'
  mr job cancel $JID --json | jq -e --arg j "$JID" '.status == "cancelled" and .canonicalJobId == $j'
