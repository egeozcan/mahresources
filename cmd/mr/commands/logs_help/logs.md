---
exitCodes: 0 on success; 1 on any error
relatedCmds: log get, log entity, admin
---

# Long

The plural `logs` command group reads the server's activity log across
the whole system rather than a single entry. It exposes filtered,
paginated listings so scripts can audit changes, inspect recent
deletes, or build dashboards. Only read operations are provided — the
log is append-only and the server writes it automatically as entities
change.

Use `logs list` with the filter flags (`--level`, `--action`,
`--entity-type`, `--entity-id`, `--message`, `--created-before`,
`--created-after`) to narrow the result set. For single-row lookups
use the singular `log` subcommands (`log get`, `log entity`).
