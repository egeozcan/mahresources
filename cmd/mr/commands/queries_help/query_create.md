---
outputShape: Created query object with ID, Name, Text, Template, Description, CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: query get, query run, query delete, queries list
---

# Long

Create a new saved query. Requires `--name` (unique label) and
`--text` (the SQL body). `--template` is optional and lets you embed
a Pongo2 template that receives the query's result rows for custom
rendering in the web UI. Query Text runs against a read-only handle
when executed; writes to the database via `query run` are rejected.

# Example

  # Create a minimal query
  mr query create --name "count-resources" --text "select count(*) as n from resources"

  # Create with a template for custom display
  mr query create --name "recent-notes" --text "select id, name from notes order by created_at desc limit 10" --template "{{ rows|length }} rows"

  # mr-doctest: create a query, verify the response carries a positive ID
  NAME="doctest-create-$$-$RANDOM"
  mr query create --name "$NAME" --text "select 1 as x" --json | jq -e '.ID > 0 and (.Name | length) > 0'
