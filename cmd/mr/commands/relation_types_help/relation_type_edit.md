---
outputShape: RelationType object with ID, Name, Description, FromCategoryId, ToCategoryId, BackRelationId, CreatedAt/UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: relation-type edit-name, relation-type edit-description, relation-types list
---

# Long

Edit fields on an existing RelationType. `--id` is required; any other
flag left unset keeps the existing value (partial update). `--name`
and `--description` replace those fields; `--reverse-name` is accepted
but has no effect here, since a reverse type is established at creation
time (see `relation-type create`). `--from-category` and
`--to-category` rewire the allowed category pairing. Every existing
relation of this type whose groups no longer match the new categories
is deleted, database-wide, along with the matching back-relation edges,
and that is not reversible. Sends `POST /v1/relationType/edit` and
returns the full updated record.

# Example

  # Rename a relation type and update its description
  mr relation-type edit --id 5 --name "referenced-by" --description "backward link"

  # Rewire the target category (relation-type 5 now points to category 7)
  mr relation-type edit --id 5 --to-category 7

  # mr-doctest: create a relation-type, edit name + description, verify via list
  C1=$(mr category create --name "doctest-rt-edit-c1-$$-$RANDOM" --json | jq -r '.ID')
  C2=$(mr category create --name "doctest-rt-edit-c2-$$-$RANDOM" --json | jq -r '.ID')
  RT=$(mr relation-type create --name "doctest-rt-edit-init-$$-$RANDOM" --from-category=$C1 --to-category=$C2 --json | jq -r '.ID')
  NEW="doctest-rt-edit-new-$$-$RANDOM"
  mr relation-type edit --id=$RT --name "$NEW" --description "updated"
  mr relation-types list --name "doctest-rt-edit-new" --json | jq -e --argjson r "$RT" --arg n "$NEW" 'map(select(.ID == $r)) | .[0].Name == $n'
