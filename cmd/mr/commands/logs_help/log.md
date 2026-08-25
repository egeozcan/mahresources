---
exitCodes: 0 on success; 1 on any error
relatedCmds: logs list, log entity, log get
---

# Long

The activity log is an append-only record of entity changes and system
events, written as resources, notes, groups, tags, and other entities
change, and as settings, plugins and background work report. Each entry
captures a level, action, entity type and ID, a human message, the
request path, and a timestamp. The raw JSON uses lowercase keys (`id`,
`level`, `action`, `entityType`, etc.), not the PascalCase shape used
elsewhere in the API.

Use the `log` subcommands to inspect single rows. `log get <id>` fetches
one entry by its numeric ID. `log entity --entity-type=X --entity-id=Y`
returns the newest 50 entries for one specific entity. For a broad
query across the whole system, use `logs list`.
