---
outputShape: Canonical Job detail with currently advertised commands and outputs
exitCodes: 0 on success; 1 on any error
relatedCmds: jobs list, jobs timeline, job command
---

# Long

Read one visible Job by UUID. The response includes its current state, owner,
version, timeline summary, outputs, and the commands the server currently
allows this account to run. Use a command key from this response with
`mr job command`; the CLI checks the advertised version and endpoint before
submitting it.

# Example

  # Inspect a visible Job and its current commands
  mr jobs get 018f4db1-9b40-7f54-8f16-37a449bcf01d --json

  # Extract command keys that the server currently advertises
  mr jobs get 018f4db1-9b40-7f54-8f16-37a449bcf01d --json | jq -r '.commands[].key'

  # mr-doctest: the canonical detail command documents its Job ID argument
  mr jobs get --help | grep 'job-id' >/dev/null
