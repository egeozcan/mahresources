---
exitCodes: 0 on success; 1 on any error
relatedCmds: plugins list, plugin enable, plugin settings
---

# Long

Discover and inspect the plugins installed on the mahresources server.
The plural `plugins` command group is read-only: use `plugins list` for
a full snapshot of every plugin the server knows about, including its
current `enabled` state and any stored setting values. For lifecycle
controls (enable, disable, configure, purge) on a single plugin, use
the singular `plugin` subcommands.
