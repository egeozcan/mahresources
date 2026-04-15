---
outputShape: RelationType object with ID, Name, Description, FromCategoryId, ToCategoryId, BackRelationId, CreatedAt/UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: relation-type edit, relation-types list, relation create, category create
---

# Long

Create a new RelationType defining a typed link between two Categories.
`--name` is required. `--from-category` and `--to-category` take
Category IDs (not names); when set, the server enforces that relations
of this type link groups of those categories. `--description` is
free-form text shown in UIs. `--reverse-name` stores a readable label
for traversing the link in the opposite direction. Sends `POST
/v1/relationType` and returns the persisted record.

# Example

  # Create a basic relation type between two category IDs
  mr relation-type create --name "references" --from-category 1 --to-category 2

  # Create with a description and reverse-name, capture ID via jq
  ID=$(mr relation-type create --name "depends-on" --description "A depends on B" \
      --reverse-name "depended-on-by" --from-category 1 --to-category 2 --json | jq -r '.ID')

  # mr-doctest: create two categories, make a relation-type, verify via list
  C1=$(mr category create --name "doctest-rt-create-c1-$$-$RANDOM" --json | jq -r '.ID')
  C2=$(mr category create --name "doctest-rt-create-c2-$$-$RANDOM" --json | jq -r '.ID')
  RT=$(mr relation-type create --name "doctest-rt-create-$$-$RANDOM" --from-category=$C1 --to-category=$C2 --json | jq -r '.ID')
  mr relation-types list --json | jq -e --argjson r "$RT" 'map(select(.ID == $r)) | length == 1'
