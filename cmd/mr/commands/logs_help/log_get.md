---
outputShape: Log entry object with id (uint), level, action, entityType, entityId, entityName, message, requestPath, createdAt (all lowercase keys)
exitCodes: 0 on success; 1 on any error
relatedCmds: logs list, log entity
---

# Long

Get a single log entry by its numeric ID and print its fields. Output
is a key/value table by default; pass the global `--json` flag to emit
the raw record for scripting. Note that log entries use lowercase JSON
keys (`id`, `level`, `action`, `entityType`, `entityId`, `message`,
`createdAt`) rather than the PascalCase names most other mahresources
entities use.

Log IDs are discovered via `logs list` or `log entity`; they are not
stable across fresh databases, so doctests create an entity first and
then look up the triggered row.

# Example

  # Get a log entry by ID (table output)
  mr log get 42

  # Get as JSON and extract the action field with jq
  mr log get 42 --json | jq -r .action

  # mr-doctest: create a group to generate a log row, find its ID, fetch it
  GID=$(mr group create --name "doctest-logget-$$-$RANDOM" --json | jq -r '.ID')
  LID=$(mr log entity --entity-type=group --entity-id=$GID --json | jq -r '.logs[0].id')
  mr log get $LID --json | jq -e --argjson l "$LID" '.id == $l and .entityType == "group" and (.action | length) > 0'
