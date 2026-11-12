---
outputShape: Object mapping table name (string) to an array of column names (string[])
exitCodes: 0 on success; 1 on any error
relatedCmds: query create, query run, mrql
---

# Long

List the tables saved SQL may read and their columns, for use as a
reference when authoring query Text. The response is a single JSON
object whose keys are table names and whose values are arrays of
column name strings. The content tables (e.g. `resources`, `notes`,
`groups`), their join tables and, on SQLite, their FTS tables
appear; `users`, `sessions`, `api_tokens` and every other table
`query run` cannot read are left out. A group-limited account,
which cannot run saved SQL, gets `403`; on PostgreSQL with `-auth`,
a read-only role that fails the server's check gets `503`.

Handy as a quick discovery tool before writing a new saved query.

# Example

  # Dump the full schema as JSON
  mr query schema

  # List only the column names of the `resources` table
  mr query schema --json | jq -r '.resources[]'

  # mr-doctest: assert the schema has the resources table with an id column
  mr query schema --json | jq -e '.resources | index("id") != null'
