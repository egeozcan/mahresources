---
exitCodes: 0 if every non-skipped doctest passes its declared expectation; 1 otherwise
relatedCmds: docs lint, docs dump
---

# Long

Walks the command tree, extracts every example tagged `# mr-doctest:`, and
evaluates each block against the connected server. Per-example metadata on the
label line controls behavior: `expect-exit=N`, `tolerate=/regex/`,
`skip-on=ephemeral`, `timeout=Ns`, and `stdin=<fixture>`.

The runner pipes each block through `bash -e -o pipefail -c`, with cwd set to
`cmd/mr/` so examples can reference `./testdata/*` fixtures. Requires
`MAHRESOURCES_URL`, `bash`, and `jq` on PATH.

# Example

  # Run against a local ephemeral server
  mr docs check-examples --server http://localhost:8181 --environment=ephemeral

  # Inherit server URL from the environment
  MAHRESOURCES_URL=http://localhost:8181 mr docs check-examples --environment=ephemeral
