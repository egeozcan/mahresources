---
outputShape: Setting object after reset with key, label, group, type, current (equals bootDefault), bootDefault, overridden (false), updatedAt, reason
exitCodes: 0 on success; 1 on unknown key or error
relatedCmds: admin settings set, admin settings get, admin settings list
---

# Long

Remove a runtime override and revert the setting to its boot-time default. The command prints the post-reset view so you can confirm the current value is back to the default.

Use `--reason` to record why the override was removed; the reason is stored in the database alongside the reset timestamp.

# Example

  # Reset max_upload_size to its boot default
  mr admin settings reset max_upload_size

  # Reset with a reason for the audit log
  mr admin settings reset mrql_query_timeout --reason "back to default after testing"

  # mr-doctest: set then reset max_upload_size and verify overridden is false
  mr admin settings set max_upload_size 1048576 --reason "cli-smoke-reset"
  mr admin settings reset max_upload_size --reason "cli-smoke-reset-cleanup"
  mr admin settings get max_upload_size --json | jq -e '.overridden == false'
