---
outputShape: Object with name and ok=true on success
exitCodes: 0 on success; 1 on any error
relatedCmds: plugin enable, plugin purge-data, plugins list
---

# Long

Write configuration values for an installed plugin. Pass the values as
a JSON object via the required `--data` flag; keys must match the
`name` fields declared in the plugin's settings descriptor (see the
`settings` array on `plugins list`). The server stores the decoded
object as the plugin's persisted values and returns `ok=true` on
success.

This command replaces the stored values wholesale — keys omitted from
the `--data` payload are not preserved. Run `plugins list --json` to
inspect the current `values` object before writing a new one.

# Example

  # Update a plugin's greeting setting
  mr plugin settings example-plugin --data '{"greeting":"Hello from CLI"}'

  # Write multiple settings in one call
  mr plugin settings example-plugin --data '{"greeting":"Hi","show_footer":true}'

  # mr-doctest: write settings and assert the response shape
  mr plugin settings example-plugin --data '{"greeting":"doctest-value"}' --json | jq -e '.ok == true and .name == "example-plugin"'

  # mr-doctest: write settings and confirm the value round-trips through plugins list
  mr plugin settings example-plugin --data '{"greeting":"round-trip"}' --json >/dev/null
  mr plugins list --json | jq -e --arg n "example-plugin" 'map(select(.name == $n))[0].values.greeting == "round-trip"'
  mr plugin purge-data example-plugin --json >/dev/null
