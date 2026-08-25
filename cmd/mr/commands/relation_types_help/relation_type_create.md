---
outputShape: RelationType object with ID, Name, Description, FromCategoryId, ToCategoryId, BackRelationId, CreatedAt/UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: relation-type edit, relation-types list, relation create, category create
---

# Long

Create a new RelationType defining a typed link between two Categories.
`--name`, `--from-category` and `--to-category` are all required in
practice: the two category flags take Category IDs (not names), and the
server rejects a missing category outright, and on SQLite rejects one
that does not resolve to an existing Category; nothing is persisted
either way. It then enforces that relations of this type link groups of
those categories. `--description` is free-form text shown in UIs.

`--reverse-name` is not a label. It creates a second RelationType of that
name with the categories swapped and links the pair, so each is the
other's back relation; an existing unlinked reverse type of that name and
category pair is adopted rather than duplicated. Passing the same value
as `--name` self-links the type, which requires `--from-category` and
`--to-category` to be equal. Sends `POST /v1/relationType` and returns
the persisted record.

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
