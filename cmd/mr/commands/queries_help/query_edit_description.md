---
exitCodes: 0 on success; 1 on any error
relatedCmds: query get, query edit-name, queries list
---

# Long

Update the description of an existing saved query. Passing an empty
string clears the description. Description is metadata only and does
not affect execution.

# Example

  # Set the description on query 42
  mr query edit-description 42 "Counts resources grouped by content type"

  # Clear the description by passing an empty string
  mr query edit-description 42 ""

  # mr-doctest: create a query, set description, verify via get
  NAME="doctest-desc-$$-$RANDOM"
  ID=$(mr query create --name "$NAME" --text "select 1 as x" --json | jq -r '.ID')
  mr query edit-description $ID "hello world"
  mr query get $ID --json | jq -e '.Description == "hello world"'
