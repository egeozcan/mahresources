---
outputShape: Accepted Job command result
exitCodes: 0 on success; 1 on any error
relatedCmds: jobs get, job bulk-command
---

# Long

Run a command key that the Job currently advertises in `jobs get`. The CLI
re-reads detail immediately before submission, uses the advertised endpoint
and version, and sends an idempotency key. Supply `--idempotency-key` to replay
the same logical request after a network failure; otherwise the CLI generates
and prints a key for this request. Commands marked destructive or requiring
confirmation need `--confirm`.

# Example

  # Run a command currently advertised by the server
  mr job command 018f4db1-9b40-7f54-8f16-37a449bcf01d retry

  # Confirm a destructive command and provide a reusable key
  mr job command 018f4db1-9b40-7f54-8f16-37a449bcf01d cancel --confirm --idempotency-key ops-2026-09-23-1

  # mr-doctest: the command help documents the key and confirmation flags
  mr job command --help | grep -- '--idempotency-key' >/dev/null
