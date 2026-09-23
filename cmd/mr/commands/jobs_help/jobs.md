---
exitCodes: 0 on success; 1 on any error
relatedCmds: job submit, job command, jobs get, jobs summary
---

# Long

Browse durable background Jobs with `list`, `get`, and `timeline`, and
aggregate them with `summary`. A Job keeps its identity, owner, state, events,
and outputs after execution finishes, subject to the server's retention
settings. The server decides which Jobs and controls the current account can
see.

`job submit` remains the compatibility command for remote downloads. The
singular `job cancel`, `pause`, `resume`, and `retry` commands also remain
available for older clients and scripts. Use `job command` or
`job bulk-command` for controls currently advertised by canonical Job detail.
