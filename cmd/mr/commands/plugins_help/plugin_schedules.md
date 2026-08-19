---
outputShape: Array of schedule objects with scheduleId, everySeconds, overlap, nextDueAt, runs, lastStatus, owned and registered
exitCodes: 0 on success; 1 on any error
relatedCmds: plugin enable, plugin disable, plugins list
---

# Long

List the recurring work one installed plugin has registered. A schedule
is declared by the plugin with `mah.schedule` and recorded when an
operator enables it, so this shows what the deployment has actually
stored rather than what the plugin file currently says.

Two fields carry the state that is easy to misread. `registered` is
false when the row exists but the plugin no longer declares that id,
which is what a disabled plugin, a renamed schedule and a deleted
`mah.schedule` call all look like; none of them run, and the row is kept
so that re-enabling or restoring the call resumes it with its history.
`owned` is false when the operator who enabled the plugin has since been
deleted, at which point the schedule has stopped rather than merely lost
its attribution, because every run executes as that operator and there
is no safe identity to fall back to. Naming a plugin the server does not
have returns an empty list rather than an error.

# Example

  # What a plugin has scheduled
  mr plugin schedules my-plugin

  # Just the ids and when each is next due
  mr plugin schedules my-plugin --json | jq -r '.[] | "\(.scheduleId) \(.nextDueAt)"'

  # mr-doctest: a plugin with no schedules returns an empty array rather than an error
  mr plugin schedules test-actions --json | jq -e 'type == "array"'

  # mr-doctest: a plugin the server does not have returns an empty array
  mr plugin schedules no-such-plugin --json | jq -e 'length == 0'
