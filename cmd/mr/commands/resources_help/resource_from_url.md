---
outputShape: Resource object with id
exitCodes: 0 on success; 1 on any error
relatedCmds: resource upload, resource from-local
---

# Long

Create a Resource by having the server fetch a remote URL. Useful when
you have a public asset that shouldn't be proxied through your local
machine. The `--url` flag is required; the server downloads, stores, and
indexes the file. Optional `--tags` / `--groups` attach relationships at
creation.

Content is deduplicated by hash, so re-fetching bytes the server already holds
never produces a second resource. With no `--owner-id` given, the request is
refused with HTTP 400 naming the existing one. The doctest below fetches the
same asset on every run, so its cleanup has to run on *every* exit path — a run
that dies after the create leaves the row behind, and every later run of the
block then fails on that duplicate however unique its `--name` is. Hence the
`trap` rather than a trailing `delete`, which `bash -e` would skip.

# Example

  # Create from a URL
  mr resource from-url --url https://example.com/photo.jpg

  # With metadata and groups
  mr resource from-url --url https://example.com/doc.pdf --name "Paper" --meta '{"source":"arxiv"}' --groups 5

  # mr-doctest: the server fetches an asset it serves itself, cleanup on every exit path
  ID=$(mr resource from-url --url "$MAHRESOURCES_URL/public/favicon/favicon-32x32.png" --name "from-url-test-$$-$RANDOM" --json | jq -r '.ID')
  trap '[ -n "$ID" ] && mr resource delete "$ID" > /dev/null 2>&1 || true' EXIT
  mr resource get $ID --json | jq -e '.ID > 0 and .ContentType == "image/png"' > /dev/null
