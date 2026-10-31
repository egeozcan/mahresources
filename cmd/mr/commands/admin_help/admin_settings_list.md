---
outputShape: Array of setting objects with key, label, description, group, type, current, bootDefault, overridden, updatedAt, reason; secret settings also include secret, configured and bootConfigured, with null current and bootDefault
exitCodes: 0 on success; 1 on any error
relatedCmds: admin settings get, admin settings set, admin settings reset
---

# Long

List all runtime-editable settings with their current value, boot default, override status, and last-updated timestamp. Overridden settings show the effective value alongside the original boot default so you can see what changed.

Pass `--json` to emit the raw JSON array for scripting or to inspect fields like `minNumeric`, `maxNumeric`, and `allowZero`.

Secret settings show `configured` or `not configured` in the table. JSON carries `secret`, `configured` and `bootConfigured` status; `current` and `bootDefault` are null, so saved API keys are never printed.

# Example

  # Show all settings in a table
  mr admin settings list

  # Emit raw JSON for scripting
  mr admin settings list --json

  # mr-doctest: list settings and verify the response is a non-empty array
  mr admin settings list --json | jq -e 'length > 0'
