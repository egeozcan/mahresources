---
outputShape: Paginated wrapper with logs (array of entries), totalCount, page, perPage; each entry has id, level, action, entityType, entityId, entityName, message, requestPath, createdAt (lowercase keys)
exitCodes: 0 on success; 1 on any error
relatedCmds: logs list, log get
---

# Long

Fetch every log entry recorded for one specific entity. Both
`--entity-type` (e.g. `group`, `resource`, `note`, `tag`) and
`--entity-id` are required. The response is the same paginated wrapper
`logs list` returns, so the `logs` array contains the actual rows and
pagination is controlled by the global `--page` flag.

This is the reliable way to discover a log row's ID from code: create
or touch an entity, then query its history to get the `id` value used
by `log get`. The action field (`create`, `update`, `delete`, `system`)
lets scripts filter to just the events they care about.

# Example

  # List every log entry for group 42
  mr log entity --entity-type=group --entity-id=42

  # Pull only the actions for one resource, via jq
  mr log entity --entity-type=resource --entity-id=7 --json | jq -r '.logs[].action'

  # mr-doctest: create a group, list its logs, assert exactly one "create" entry exists
  GID=$(mr group create --name "doctest-logent-$$-$RANDOM" --json | jq -r '.ID')
  mr log entity --entity-type=group --entity-id=$GID --json | jq -e '(.logs | type == "array") and (.logs | map(select(.action == "create")) | length) >= 1'
