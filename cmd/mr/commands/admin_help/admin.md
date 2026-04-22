---
exitCodes: 0 on success; 1 on any error
relatedCmds: admin stats, admin settings list
---

# Long

Server administration commands. The default subcommand is `stats`, which prints a full health and data overview. The `settings` subgroup lets you view and change runtime configuration overrides without restarting the server.

Run `mr admin stats --help` for the full stats flags, or `mr admin settings --help` for the settings subcommands.

# Example

  # Show server stats (same as `mr admin stats`)
  mr admin

  # Show help for all subcommands
  mr admin --help
