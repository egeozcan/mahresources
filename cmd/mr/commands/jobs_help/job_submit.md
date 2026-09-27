---
outputShape: Object with queued=true, a jobs array containing each created job's id, canonicalJobId, url, and initial status, and a refused array naming each URL the server refused
exitCodes: 0 when every URL was queued; 1 on any error, including a batch in which the server refused a URL
relatedCmds: jobs list, job cancel, job retry
---

# Long

Submit one or more URLs to the download queue. The server creates one
job per URL and immediately begins fetching in the background; this
command returns as soon as the jobs are queued, not when downloads
finish. Attach tags, groups, an owner, or a custom name with the
remaining flags.

Name the URLs with `--url` or `--urls`; both can be repeated and
combined. A `--url` value is exactly one URL, taken as written. A
`--urls` value is a list: it is split at newlines and at a comma that
starts another `http://` or `https://` URL. A comma anywhere else is
part of the URL, because commas are legal in URL paths and queries. Use
`--url` for a URL that itself contains a comma followed by `http`.

The server answers each URL separately: it queues the ones it accepts
and names the ones it refuses, such as a URL that is not an absolute
`http` or `https` URL. When any URL is refused the command prints the
answer and exits 1 with every refusal in the error.

Downloaded content becomes a new Resource once the fetch succeeds.
Each job's `canonicalJobId` is the Job id `jobs list` prints. Watch
progress with `jobs list` or the `/v1/jobs/events?version=2` stream.

# Example

  # Queue a single download
  mr job submit --url https://example.com/photo.jpg

  # Queue multiple URLs with tags and an owner group
  mr job submit --urls https://a.example.com/a.jpg,https://b.example.com/b.jpg --tags 5,7 --owner-id 3

  # A comma inside a URL stays in that URL
  mr job submit --url "https://img.example.com/w_96,h_64/photo.jpg"

  # mr-doctest: submit a job against the live ephemeral server and verify the response shape
  mr job submit --urls "$MAHRESOURCES_URL/v1/jobs/events" --json | jq -e '.queued == true and (.jobs | length == 1) and (.jobs[0].id | length > 0)'

  # mr-doctest: a comma inside a URL submits one job, not two, skip-on=auth
  JID=$(mr job submit --urls "$MAHRESOURCES_URL/v1/jobs/events?version=2&a=1,b.txt" --json | jq -er 'select(.jobs | length == 1) | .jobs[0].canonicalJobId')
  mr job cancel $JID --json >/dev/null

  # mr-doctest: submit, capture the ID, confirm the job appears in the queue listing
  JID=$(mr job submit --urls "$MAHRESOURCES_URL/v1/jobs/events" --json | jq -r '.jobs[0].canonicalJobId')
  mr jobs list --json | jq -e --arg j "$JID" '.jobs | map(.id) | index($j) != null'
  mr job cancel $JID --json >/dev/null
