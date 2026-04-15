---
outputShape: Object with queued=true and a jobs array containing each created job's id, url, and initial status
exitCodes: 0 on success; 1 on any error
relatedCmds: jobs list, job cancel, job retry
---

# Long

Submit one or more URLs to the download queue. The server creates one
job per URL and immediately begins fetching in the background; this
command returns as soon as the jobs are queued, not when downloads
finish. Use `--urls` with a comma-separated list; attach tags, groups,
an owner, or a custom name with the remaining flags.

Downloaded content becomes a new Resource once the fetch succeeds. Watch
progress with `jobs list` or the `/v1/download/events` SSE stream.

# Example

  # Queue a single download
  mr job submit --urls https://example.com/photo.jpg

  # Queue multiple URLs with tags and an owner group
  mr job submit --urls https://a.example.com/a.jpg,https://b.example.com/b.jpg --tags 5,7 --owner-id 3

  # mr-doctest: submit a job against the live ephemeral server and verify the response shape
  mr job submit --urls "$MAHRESOURCES_URL/v1/jobs/events" --json | jq -e '.queued == true and (.jobs | length == 1) and (.jobs[0].id | length > 0)'

  # mr-doctest: submit, capture the ID, confirm the job appears in the queue listing
  JID=$(mr job submit --urls "$MAHRESOURCES_URL/v1/jobs/events" --json | jq -r '.jobs[0].id')
  mr jobs list --json | jq -e --arg j "$JID" '.jobs | map(.id) | index($j) != null'
  mr job cancel $JID --json >/dev/null || true
