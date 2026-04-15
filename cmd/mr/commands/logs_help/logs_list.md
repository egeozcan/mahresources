---
outputShape: Paginated wrapper with logs (array of entries), totalCount, page, perPage; each entry has id, level, action, entityType, entityId, entityName, message, requestPath, createdAt (lowercase keys)
exitCodes: 0 on success; 1 on any error
relatedCmds: log get, log entity, admin
---

# Long

List log entries across the whole system, optionally filtered. Filter
flags combine with AND. `--level` accepts `info`, `warning`, or
`error`; `--action` accepts `create`, `update`, `delete`, or `system`.
`--entity-type` and `--entity-id` scope results to a single entity
kind or row, while `--message` does a substring match. Date filters
(`--created-before`, `--created-after`) expect RFC3339 strings such
as `2026-04-15T00:00:00Z`.

Pagination uses the global `--page` flag with a fixed page size of 50.
The response wraps the `logs` array with `totalCount`, `page`, and
`perPage` so scripts can walk the full result set. JSON output uses
lowercase keys throughout — match them exactly when building jq
filters.

# Example

  # List recent log entries (first page, table output)
  mr logs list

  # Filter to deletions only, JSON + jq
  mr logs list --action delete --json | jq -r '.logs[] | "\(.entityType) \(.entityId) \(.message)"'

  # Filter by entity type and a date window
  mr logs list --entity-type group --created-after 2026-01-01T00:00:00Z --json

  # mr-doctest: create a group, confirm logs list returns at least one row with the expected shape
  mr group create --name "doctest-logs-$$-$RANDOM" --json >/dev/null
  mr logs list --json | jq -e 'has("logs") and (.logs | type == "array") and (.logs | length >= 1) and (.logs[0] | has("id") and has("action") and has("entityType") and has("createdAt"))'

  # mr-doctest: filter by entity type, assert every returned row matches the filter
  mr group create --name "doctest-logs-filter-$$-$RANDOM" --json >/dev/null
  mr logs list --entity-type group --json | jq -e '(.logs | type == "array") and (.logs | all(.entityType == "group"))'
