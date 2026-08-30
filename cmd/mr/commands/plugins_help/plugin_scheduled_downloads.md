---
outputShape: Array of scheduled download objects with id, pluginName, url, dueAt, status, jobId, lastError, attempts, owned, createdAt, updatedAt and claimedAt while a scheduler tick holds the submit claim, in JSON mode; a table in human mode whose STATE column reports stopped (no owner) for rows that can no longer fire
exitCodes: 0 on success; 1 on any error
relatedCmds: plugin schedules, plugin enable, plugin disable
---

# Long

List the one-shot downloads a plugin deferred with `mah.download.submit`.
A deferred download is stored durably until the plugin scheduler submits it
to the in-memory download queue. Pending rows have not started yet;
submitted rows carry the queue `jobId`; failed and cancelled rows are
terminal.

Every row fires under the plugin name that submitted it, so a restart does
not turn it into an unrestricted host download. It also fires as the user
who submitted it and is re-validated at fire time. If that user is deleted,
the row becomes `owned: false` and stops rather than falling back to an
administrator.

Naming a plugin the server does not have returns an empty list rather than
an error.

# Example

  # What a plugin has deferred
  mr plugin scheduled-downloads my-plugin

  # Just pending URLs and their due times
  mr plugin scheduled-downloads my-plugin --json | jq -r '.[] | select(.status=="pending") | "\(.dueAt) \(.url)"'

  # mr-doctest: a plugin with no deferred downloads returns an empty array rather than an error
  mr plugin scheduled-downloads test-actions --json | jq -e 'type == "array"'

  # mr-doctest: a plugin the server does not have returns an empty array
  mr plugin scheduled-downloads no-such-plugin --json | jq -e 'length == 0'
