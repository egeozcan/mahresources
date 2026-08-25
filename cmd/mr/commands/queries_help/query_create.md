---
outputShape: Created query object with ID, Name, Text, Template, Description, CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: query get, query run, query delete, queries list
---

# Long

Create a new saved query. Requires `--name` (unique label) and
`--text` (the SQL body). `--template` is optional and stores raw HTML
that the query's page renders under the results table once the query
has run. Result rows reach it in the browser as the Alpine variable
`results` and as `window.results`, not as template variables. Query
Text runs on the connection `-db-readonly-dsn` names; writes are
rejected only when that DSN is itself read-only (see `query run`).

# Example

  # Create a minimal query
  mr query create --name "count-resources" --text "select count(*) as n from resources"

  # Create with a template for custom display
  mr query create --name "recent-notes" --text "select id, name from notes order by created_at desc limit 10" --template '<p>Total results: <span x-text="results.length"></span></p>'

  # mr-doctest: create a query, verify the response carries a positive ID
  NAME="doctest-create-$$-$RANDOM"
  mr query create --name "$NAME" --text "select 1 as x" --json | jq -e '.ID > 0 and (.Name | length) > 0'
