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

  # Update a plugin's banner text
  mr plugin settings my-plugin --data '{"banner_text":"Hello from CLI"}'

  # Write multiple settings in one call
  mr plugin settings my-plugin --data '{"banner_text":"Hi","show_banner":true}'

  # mr-doctest: write settings and assert the response shape (test-banner requires api_key)
  mr plugin settings test-banner --data '{"banner_text":"doctest-value","api_key":"doctest-key"}' --json | jq -e '.ok == true and .name == "test-banner"'

  # mr-doctest: write settings and confirm the value round-trips through plugins list
  mr plugin settings test-banner --data '{"banner_text":"round-trip","api_key":"doctest-key"}' --json >/dev/null
  mr plugins list --json | jq -e --arg n "test-banner" 'map(select(.name == $n))[0].values.banner_text == "round-trip"'
  mr plugin purge-data test-banner --json >/dev/null
