---
outputShape: Relation object with ID, Name, Description, FromGroupId, ToGroupId, RelationTypeId, CreatedAt/UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: relation delete, relation edit-name, relation-type create, group get
---

# Long

Create a new Relation linking two Groups with a typed relationship.
`--from-group-id`, `--to-group-id`, and `--relation-type-id` are all
required. The referenced relation-type's `FromCategory` and
`ToCategory` must match the categories of the two groups; otherwise
the server rejects the request. `--name` and `--description` are
optional labels stored on the relation itself. Sends `POST /v1/relation`
and returns the persisted record.

# Example

  # Create a relation linking group 3 to group 4 with relation-type 2
  mr relation create --from-group-id 3 --to-group-id 4 --relation-type-id 2

  # Create a named relation with a description
  mr relation create --from-group-id 3 --to-group-id 4 --relation-type-id 2 \
      --name "directed-by" --description "Kubrick directed 2001"

  # mr-doctest: set up categories, rel-type, two groups, then create a relation and verify
  C1=$(mr category create --name "doctest-rel-create-c1-$$-$RANDOM" --json | jq -r '.ID')
  C2=$(mr category create --name "doctest-rel-create-c2-$$-$RANDOM" --json | jq -r '.ID')
  RT=$(mr relation-type create --name "doctest-rel-create-rt-$$-$RANDOM" --from-category=$C1 --to-category=$C2 --json | jq -r '.ID')
  A=$(mr group create --name "doctest-rel-create-a-$$-$RANDOM" --category-id=$C1 --json | jq -r '.ID')
  B=$(mr group create --name "doctest-rel-create-b-$$-$RANDOM" --category-id=$C2 --json | jq -r '.ID')
  REL=$(mr relation create --from-group-id=$A --to-group-id=$B --relation-type-id=$RT --name "rel-$$" --json | jq -r '.ID')
  mr group get $A --json | jq -e --argjson r "$REL" '.Relationships[0].ID == $r'
