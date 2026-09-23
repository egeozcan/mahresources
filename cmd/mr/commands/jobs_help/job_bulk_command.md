---
outputShape: Per-Job command outcomes, including partial success
exitCodes: 0 on success; 1 on any error
relatedCmds: job command, jobs get
---

# Long

Run one bulk-capable command key on the listed Jobs. Before submitting, the
CLI reads each Job and verifies that the command is advertised as bulk-capable
and that all advertisements have a current version. The server returns a
per-Job result, so some Jobs can succeed while others are refused. Commands
marked destructive or requiring confirmation need `--confirm`.

Provide `--idempotency-key` to safely retry the same request after a network
failure. Otherwise a key is generated and printed with the result.

# Example

  # Cancel two Jobs after confirming the advertised command
  mr job bulk-command cancel 018f4db1-9b40-7f54-8f16-37a449bcf01d 018f4db2-01e5-74b3-9960-653f90e09fa1 --confirm

  # Supply a stable key for a request that may need replay
  mr job bulk-command retry 018f4db1-9b40-7f54-8f16-37a449bcf01d 018f4db2-01e5-74b3-9960-653f90e09fa1 --idempotency-key retry-batch-17

  # mr-doctest: bulk command help documents the stable key flag
  mr job bulk-command --help | grep -- '--idempotency-key' >/dev/null
