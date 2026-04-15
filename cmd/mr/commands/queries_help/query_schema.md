---
outputShape: Object mapping table name (string) to an array of column names (string[])
exitCodes: 0 on success; 1 on any error
relatedCmds: query create, query run, mrql
---

# Long

List every database table and its columns, for use as a reference
when authoring query Text. The response is a single JSON object whose
keys are table names and whose values are arrays of column name
strings. Both user-facing tables (e.g. `resources`, `notes`,
`groups`) and internal FTS/virtual tables appear in the output.

Handy as a quick discovery tool before writing a new saved query or
MRQL expression.

# Example

  # Dump the full schema as JSON
  mr query schema

  # List only the column names of the `resources` table
  mr query schema --json | jq -r '.resources[]'

  # mr-doctest: assert the schema has the resources table with an id column
  mr query schema --json | jq -e '.resources | index("id") != null'
