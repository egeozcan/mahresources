---
exitCodes: 0 on success; 1 on any error
relatedCmds: plugins list, plugin enable, plugin disable
---

# Long

Plugins are server-side extensions that register shortcodes, hook into
entity lifecycle events, or inject custom UI into the mahresources web
interface. Each plugin is identified by a unique name and reports its
version, human description, an `enabled` flag, and an optional settings
schema (a list of `{name, type, label, default}` descriptors the plugin
will read at runtime).

Use the `plugin` subcommands to operate on one plugin at a time by
name: `enable` / `disable` toggle activation, `settings` writes the
plugin's configuration values, and `purge-data` wipes the plugin's
persisted state. Use `plugins list` to discover the names of installed
plugins and inspect their current enablement and stored settings.
