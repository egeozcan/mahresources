---
outputShape: Object with name, allow_scoped_principals, and ok=true on success
exitCodes: 0 on success; 1 on any error
relatedCmds: plugin enable, plugin disable, plugins list
---

# Long

Decide whether group-limited users and guests may reach one installed
plugin. This is a separate decision from enabling: enabling is the
operator's consent to what the plugin may do, while this is consent to
who may ask it to. It is off by default, so a confined account is
refused the plugin's pages, API endpoints, shortcodes, slots and
actions, and the actions it may not run are not offered to it either.
Guests are read-only, and running an action is a write, so a guest is
refused every action run however this is set: opening a plugin gives a
guest its pages, shortcodes and slots, and it is offered no actions
either way.

Pass the decision explicitly via the required `--allowed` flag; a bare
call would otherwise read as a revocation. Opening a plugin says
nothing about what it may then do on a confined caller's behalf,
because that caller's database access stays bound to its own group
subtree and role. Accounts with no group limit (admins, editors and
unscoped users) are unaffected either way. Naming a plugin the server
does not have returns a non-zero exit code and the server's message.

# Example

  # Let group-limited users and guests reach a plugin
  mr plugin scoped-access my-plugin --allowed=true

  # Close it again and confirm via the JSON response
  mr plugin scoped-access my-plugin --allowed=false --json | jq -e '.allow_scoped_principals == false'

  # mr-doctest: open a plugin to group-limited accounts and assert the response shape before closing it again
  mr plugin scoped-access test-actions --allowed=true --json | jq -e '.ok == true and .name == "test-actions" and .allow_scoped_principals == true'
  mr plugin scoped-access test-actions --allowed=false --json | jq -e '.allow_scoped_principals == false'

  # mr-doctest: an unknown plugin name is refused, expect-exit=1
  mr plugin scoped-access no-such-plugin --allowed=true
