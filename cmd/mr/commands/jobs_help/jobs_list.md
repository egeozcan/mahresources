---
outputShape: Object with a jobs array; each entry has id, url, status, progress, totalSize, progressPercent, createdAt, and optional error, startedAt, completedAt, resourceId, source
exitCodes: 0 on success; 1 on any error
relatedCmds: job submit, job cancel, job retry
---

# Long

Return a snapshot of every job the server is currently tracking,
including pending, running, paused, finished, and failed ones. The
response is a single object whose `jobs` key is an array ordered by
submission time. Each entry exposes enough detail to drive CLI
dashboards, pause/resume decisions, or cleanup scripts.

The queue lives in server memory; a restart empties it. Pagination is
not supported — the full list is returned in one response.

# Example

  # Show every job (human-readable)
  mr jobs list

  # Filter to still-running jobs and pull just their URLs
  mr jobs list --json | jq -r '.jobs[] | select(.status == "downloading" or .status == "pending") | .url'

  # mr-doctest: verify the empty-queue shape on a fresh ephemeral server
  mr jobs list --json | jq -e 'has("jobs") and (.jobs | type == "array")'

  # mr-doctest: submit a job, list the queue, assert it shows up with the expected keys
  JID=$(mr job submit --urls "http://127.0.0.1:9/nope.bin" --json | jq -r '.jobs[0].id')
  mr jobs list --json | jq -e --arg j "$JID" '(.jobs | map(select(.id == $j)) | length) == 1 and (.jobs | map(select(.id == $j))[0] | has("status") and has("url") and has("createdAt"))'
