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
same asset on every run, so its cleanup has to run on *every* exit path -- a run
that dies after the create leaves the row behind, and every later run of the
block then fails on that duplicate however unique its `--name` is. Hence the
`trap` rather than a trailing `delete`, which `bash -e` would skip.

The trap is armed *before* the create and resolves the resource by its unique
name at exit time, never from an id the create handed back. It looks the name up
through `--mrql` rather than `--name`: the latter is a LIKE filter, so it matches
any name *containing* the generated one and returns at most a page of them, and
deleting `.[0]` of that could remove a different row while leaking its own. MRQL
`name = "..."` is an equality, which is what "resolves by its unique name"
requires to be true. Arming it after
an `ID=$(... | jq ...)` capture would leave the one window that matters: the
create has committed on the server, the pipeline that reads its id fails, and
`bash -e` exits with no trap installed. The name is generated before the create
and is all the trap needs, so nothing about the create's output can decide
whether the row is cleaned up.

# Example

  # Create from a URL
  mr resource from-url --url https://example.com/photo.jpg

  # With metadata and groups
  mr resource from-url --url https://example.com/doc.pdf --name "Paper" --meta '{"source":"arxiv"}' --groups 5

  # mr-doctest: the server fetches an asset it serves itself, cleanup on every exit path
  N="from-url-test-$$-$RANDOM"
  cleanup() {
    LEFTOVER=$(mr resources list --mrql "name = \"$N\"" --json | jq -r '.[0].ID // empty') || LEFTOVER=""
    [ -n "$LEFTOVER" ] && mr resource delete "$LEFTOVER" > /dev/null 2>&1 || true
    return 0
  }
  trap cleanup EXIT
  ID=$(mr resource from-url --url "$MAHRESOURCES_URL/public/favicon/favicon-32x32.png" --name "$N" --json | jq -r '.ID')
  mr resource get $ID --json | jq -e '.ID > 0 and .ContentType == "image/png"' > /dev/null
