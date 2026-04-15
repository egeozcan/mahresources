---
exitCodes: 0 on success; 1 on any error
relatedCmds: query get, query edit-description, queries list
---

# Long

Update the name of an existing saved query. Query names are used by
`query run-by-name`, so renaming a query breaks callers that reference
it by the old name. Shorthand for a direct field update; does not
modify the query Text or Template.

# Example

  # Rename query 42
  mr query edit-name 42 "count-resources-v2"

  # Rename and confirm with a follow-up get
  mr query edit-name 42 "renamed" && mr query get 42 --json | jq -r .Name

  # mr-doctest: create a query, rename it, verify via get
  OLD="doctest-editname-$$-$RANDOM"
  NEW="$OLD-new"
  ID=$(mr query create --name "$OLD" --text "select 1 as x" --json | jq -r '.ID')
  mr query edit-name $ID "$NEW"
  mr query get $ID --json | jq -e --arg n "$NEW" '.Name == $n'
