---
outputShape: Object with name, enabled=false, and ok=true on success
exitCodes: 0 on success; 1 on any error
relatedCmds: plugin enable, plugin purge-data, plugins list
---

# Long

Disable an installed plugin by name. Once disabled, the plugin stops
contributing shortcodes, hooks, and UI injections, but its stored
settings values and persisted KV data are preserved (use `plugin
purge-data` to remove the KV data). Disabling a plugin that is
already disabled is idempotent and returns `ok`.

# Example

  # Disable a plugin by name
  mr plugin disable my-plugin

  # Disable and confirm via the JSON response
  mr plugin disable my-plugin --json | jq -e '.enabled == false'

  # mr-doctest: disable a bundled plugin and assert the response shape
  mr plugin disable test-actions --json | jq -e '.ok == true and .enabled == false and .name == "test-actions"'

  # mr-doctest: enable then disable and verify the list view flips enabled to false
  mr plugin enable test-actions --json >/dev/null
  mr plugin disable test-actions --json >/dev/null
  mr plugins list --json | jq -e --arg n "test-actions" 'map(select(.name == $n))[0].enabled == false'
