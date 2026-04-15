---
outputShape: Object with name and ok=true on success
exitCodes: 0 on success; 1 on any error
relatedCmds: plugin disable, plugin settings, plugins list
---

# Long

Purge all key/value data a plugin has written through the plugin KV
API. Destructive: wipes every row the plugin has persisted in its
private KV tables. The plugin itself stays installed and its stored
settings values (written via `plugin settings`) are preserved. The
plugin must be disabled first; calling `purge-data` on an enabled
plugin returns a non-zero exit code.

This is the reset button for plugin KV state; use it when a plugin's
stored data is corrupt, stale, or no longer needed. There is no
confirmation prompt and no undo.

# Example

  # Purge all KV data for a plugin by name
  mr plugin purge-data my-plugin

  # Purge and confirm the JSON response
  mr plugin purge-data my-plugin --json | jq -e '.ok == true'

  # mr-doctest: ensure the plugin is disabled, then purge and assert the response shape
  mr plugin disable test-actions --json >/dev/null
  mr plugin purge-data test-actions --json | jq -e '.ok == true and .name == "test-actions"'

  # mr-doctest: purge is idempotent on a disabled plugin with no KV data
  mr plugin disable test-actions --json >/dev/null
  mr plugin purge-data test-actions --json >/dev/null
  mr plugin purge-data test-actions --json | jq -e '.ok == true'
