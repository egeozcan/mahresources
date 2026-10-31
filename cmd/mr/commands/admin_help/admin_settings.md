---
exitCodes: 0 on success; 1 on any error
relatedCmds: admin stats, admin settings list, admin settings set
---

# Long

View and manage runtime configuration overrides. Overrides persist to the database and take effect immediately without restarting the server.

Use `list` to see all settings, `get` to inspect one, `set` to apply an override, and `reset` to revert a key to its boot-time default.

AI provider keys are write-only settings: reads show whether a key is configured, and never return its value. An empty override clears a key and suppresses its environment fallback; `reset` restores that fallback. Use `/admin/settings` to save a generation profile's provider, model and limits together.

# Example

  # Show all settings in a table
  mr admin settings list

  # Get a single setting
  mr admin settings get max_upload_size
