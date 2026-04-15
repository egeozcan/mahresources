---
outputShape: Object with name, enabled=true, and ok=true on success
exitCodes: 0 on success; 1 on any error
relatedCmds: plugin disable, plugin settings, plugins list
---

# Long

Enable an installed plugin by name. Once enabled, the plugin's
registered shortcodes, event hooks, and UI injections become active on
the server until a matching `plugin disable` call runs. Enabling a
plugin that declares required settings will fail until those settings
have been written via `plugin settings`. Enabling an already-enabled
or unknown plugin name returns a non-zero exit code and an error
message from the server.

# Example

  # Enable a plugin by name
  mr plugin enable example-plugin

  # Enable and confirm via the JSON response
  mr plugin enable example-plugin --json | jq -e '.enabled == true'

  # mr-doctest: disable first to guarantee a clean slate, then enable and assert the response shape
  mr plugin disable example-plugin --json >/dev/null
  mr plugin enable example-plugin --json | jq -e '.ok == true and .enabled == true and .name == "example-plugin"'
  mr plugin disable example-plugin --json >/dev/null

  # mr-doctest: enable and confirm the list view reports enabled=true
  mr plugin disable example-plugin --json >/dev/null
  mr plugin enable example-plugin --json >/dev/null
  mr plugins list --json | jq -e --arg n "example-plugin" 'map(select(.name == $n))[0].enabled == true'
  mr plugin disable example-plugin --json >/dev/null
