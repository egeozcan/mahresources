---
outputShape: Array of plugins; each entry has name, version, description, enabled, legacy (bool), capabilities ([]string), capability_labels (object), allow_private_hosts (bool), settings (nullable array of setting descriptors), and the optional api_version (int), network ([]string), dependencies ([]string), min_app_version (string) and values (object of stored configuration values)
exitCodes: 0 on success; 1 on any error
relatedCmds: plugin enable, plugin disable, plugin settings
---

# Long

Return every plugin installed on the server, regardless of whether it
is currently enabled. The response is a single array ordered by the
plugin's directory name, which is usually but not necessarily its
declared `name`. Each entry includes the plugin's `name`, `version`,
`description`, an `enabled` boolean, and a `settings` descriptor
array (or `null` when the plugin declares no settings). When a plugin
has stored configuration values, a `values` object is also present
keyed by setting name.

Each entry also reports what the plugin is granted, which is the same
information the management page shows before you enable it. `legacy`
is `true` for a plugin that declares no manifest: it says nothing about
what it needs, so nothing is withheld from it, and `capabilities` is
correctly the full list. `api_version` is the plugin API generation a
manifest declares, and is absent for a legacy plugin.

`capabilities` is the effective set, not the literal declaration:
`db:write` implies `db:read`, so a plugin that declared only the former
lists both. `capability_labels` maps each capability to the human
sentence describing that power; prefer it over the slug when showing
capabilities to a person.

`network` is the declared egress allowlist. When it is absent or empty
the plugin may reach any public host. That is the broadest policy, not
the absence of network access. Private network addresses are blocked
regardless, unless `allow_private_hosts` is `true`, which a manifest may
only declare alongside a `network` list that names its hosts exactly.

`dependencies` lists plugins that must be enabled first, or enabling
this one is refused. `min_app_version` is informational only: it is
recorded and displayed, and never enforced.

Plugin management info has a variable shape depending on what each
plugin reports, so `plugins list` always emits JSON; piping through
`jq` is the expected usage pattern.

# Example

  # Show every installed plugin as JSON
  mr plugins list

  # Print just the names of enabled plugins
  mr plugins list | jq -r '.[] | select(.enabled == true) | .name'

  # Print the human sentence for each capability a plugin is granted
  mr plugins list | jq -r '.[] | select(.name == "example-plugin") | .capability_labels[]'

  # Find plugins that may reach any public host (no declared allowlist)
  mr plugins list | jq -r '.[] | select((.network // []) == []) | .name'

  # Find plugins running without a manifest, which hold every capability
  mr plugins list | jq -r '.[] | select(.legacy == true) | .name'

  # mr-doctest: assert the response is an array (empty or populated)
  mr plugins list --json | jq -e 'type == "array"'

  # mr-doctest: assert every entry exposes the documented core keys
  mr plugins list --json | jq -e 'all(.[]; has("name") and has("version") and has("enabled"))'

  # mr-doctest: assert every entry reports its access, capabilities always as an array
  mr plugins list --json | jq -e 'all(.[]; has("legacy") and has("allow_private_hosts") and (.capabilities | type == "array") and has("capability_labels"))'

  # mr-doctest: assert a legacy entry declares no api_version, and db:write implies db:read
  mr plugins list --json | jq -e 'all(.[]; ((.legacy | not) or (has("api_version") | not)) and ((.capabilities | index("db:write")) == null or (.capabilities | index("db:read")) != null))'

  # mr-doctest: assert every reported capability carries a human label
  mr plugins list --json | jq -e 'all(.[]; . as $p | all($p.capabilities[]; $p.capability_labels[.] != null))'
