---
outputShape: Single setting object with key, label, group, type, current, bootDefault, overridden, updatedAt, reason
exitCodes: 0 on success; 1 on unknown key or error
relatedCmds: admin settings list, admin settings set, admin settings reset
---

# Long

Show a single runtime setting by key. The output includes the effective current value, the boot-time default, whether an override is active, and when it was last changed.

Pass `--json` to emit the raw JSON object for scripting.

# Example

  # Show max_upload_size in table form
  mr admin settings get max_upload_size

  # Get as JSON and extract the current value
  mr admin settings get max_upload_size --json | jq -r .current

  # mr-doctest: get max_upload_size and verify the key is correct
  mr admin settings get max_upload_size --json | jq -e '.key == "max_upload_size"'
