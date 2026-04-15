---
outputShape: Array of plugins; each entry has name, version, description, enabled, settings (nullable array of setting descriptors), and an optional values object holding stored configuration values
exitCodes: 0 on success; 1 on any error
relatedCmds: plugin enable, plugin disable, plugin settings
---

# Long

Return every plugin installed on the server, regardless of whether it
is currently enabled. The response is a single array ordered by plugin
name. Each entry includes the plugin's `name`, `version`,
`description`, an `enabled` boolean, and a `settings` descriptor
array (or `null` when the plugin declares no settings). When a plugin
has stored configuration values, a `values` object is also present
keyed by setting name.

Plugin management info has a variable shape depending on what each
plugin reports, so `plugins list` always emits JSON; piping through
`jq` is the expected usage pattern.

# Example

  # Show every installed plugin as JSON
  mr plugins list

  # Print just the names of enabled plugins
  mr plugins list | jq -r '.[] | select(.enabled == true) | .name'

  # mr-doctest: assert the response is an array (empty or populated)
  mr plugins list --json | jq -e 'type == "array"'

  # mr-doctest: assert every entry exposes the documented core keys
  mr plugins list --json | jq -e 'all(.[]; has("name") and has("version") and has("enabled"))'
