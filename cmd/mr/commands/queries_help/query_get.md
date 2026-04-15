---
outputShape: Query object with ID (uint), Name (string), Text (string), Template (string), Description (string), CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: query run, query edit-name, query edit-description, queries list
---

# Long

Get a saved query by ID and print its metadata. Fetches the full
record including Name, Text (the SQL), Template, Description, and
created/updated timestamps. Output is a key/value table by default;
pass the global `--json` flag to get the full record for scripting.

# Example

  # Get a query by ID (table output)
  mr query get 42

  # Get as JSON and extract the SQL text
  mr query get 42 --json | jq -r .Text

  # mr-doctest: create a query and verify get returns the same name
  NAME="doctest-get-$$-$RANDOM"
  ID=$(mr query create --name "$NAME" --text "select 1 as x" --json | jq -r '.ID')
  mr query get $ID --json | jq -e --arg n "$NAME" '.ID > 0 and .Name == $n'
