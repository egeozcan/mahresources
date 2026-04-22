---
outputShape: Updated setting object with key, label, group, type, current, bootDefault, overridden, updatedAt, reason
exitCodes: 0 on success; 1 on unknown key, invalid value, or error
relatedCmds: admin settings reset, admin settings get, admin settings list
---

# Long

Override a runtime setting. The override persists to the database and takes effect on the next use of the setting — no restart required. The command prints the updated setting view so you can confirm the new value.

Size values accept suffix notation (e.g., `1G`, `500M`, `2048K`). Duration values use Go's time.ParseDuration format (`30s`, `5m`, `2h`). Use `--reason` to record why the change was made; the reason is stored in the database and shown by `mr admin settings get`.

# Example

  # Set max_upload_size to 2 GB
  mr admin settings set max_upload_size 2147483648 --reason "increase for video workflow"

  # Set mrql query timeout
  mr admin settings set mrql_query_timeout 30s

  # mr-doctest: set max_upload_size then verify it is overridden
  mr admin settings set max_upload_size 1048576 --reason "cli-smoke-set"
  mr admin settings get max_upload_size --json | jq -e '.overridden == true'
