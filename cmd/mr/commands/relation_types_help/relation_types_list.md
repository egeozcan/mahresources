---
outputShape: Array of relation types with ID, Name, Description, FromCategoryId, ToCategoryId, CreatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: relation-type create, relation-type edit, relation create, categories list
---

# Long

List RelationTypes, optionally filtered. `--name` and `--description`
do substring matches on those fields. Pagination via the global
`--page` flag (default page size 50). Use the JSON output to feed
scripted workflows: look up a type ID by name and pass it to
`mr relation create --relation-type-id <id>`.

# Example

  # List all relation types (paged)
  mr relation-types list

  # Filter by name substring
  mr relation-types list --name references

  # JSON output + jq to extract the ID for a known name
  mr relation-types list --name "depends-on" --json | jq -r '.[0].ID'

  # mr-doctest: create a uniquely-named relation-type and assert list finds it
  C1=$(mr category create --name "doctest-rts-list-c1-$$-$RANDOM" --json | jq -r '.ID')
  C2=$(mr category create --name "doctest-rts-list-c2-$$-$RANDOM" --json | jq -r '.ID')
  UNIQ="doctest-rts-list-$$-$RANDOM"
  mr relation-type create --name "$UNIQ" --from-category=$C1 --to-category=$C2
  mr relation-types list --name "$UNIQ" --json | jq -e 'length >= 1'
