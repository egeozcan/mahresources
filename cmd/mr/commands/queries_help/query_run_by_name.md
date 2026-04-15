---
outputShape: Array of row objects; each object's keys are the query's selected column names
exitCodes: 0 on success; 1 on any error
relatedCmds: query run, query get, queries list
---

# Long

Execute a saved query by its unique `Name` instead of its numeric
ID. Same semantics as `query run`: read-only handle, 400 on SQL
errors, 404 when the name does not resolve. Useful in scripts where
the ID is not known ahead of time but the name is a stable contract.

Renaming a query via `query edit-name` invalidates callers that
pointed at the old name, so prefer `query run <id>` for
long-running integrations.

# Example

  # Run by name
  mr query run-by-name --name "count-resources"

  # Run by name and extract the count column
  mr query run-by-name --name "count-resources" --json | jq '.[0].n'

  # mr-doctest: create a named query, run it by name, verify the expected row
  NAME="doctest-runbyname-$$-$RANDOM"
  mr query create --name "$NAME" --text "select 42 as answer" --json >/dev/null
  mr query run-by-name --name "$NAME" --json | jq -e '.[0].answer == 42'
