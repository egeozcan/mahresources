---
outputShape: Status object with id
exitCodes: 0 on success; 1 on any error
relatedCmds: relation create, relation edit-name, group get
---

# Long

Delete a Relation by ID. Destructive: removes the link row entirely.
The two groups and the relation-type are unaffected. Deleting a
nonexistent ID returns exit code 1. To confirm the removal, re-fetch
either participating group with `mr group get <id> --json` and check
that the relation no longer appears in its `Relationships` array.

# Example

  # Delete relation 7
  mr relation delete 7

  # Delete and pipe the result to jq
  mr relation delete 7 --json | jq .

  # mr-doctest: create a relation, delete it, verify it's gone from the source group
  C1=$(mr category create --name "doctest-rel-del-c1-$$-$RANDOM" --json | jq -r '.ID')
  C2=$(mr category create --name "doctest-rel-del-c2-$$-$RANDOM" --json | jq -r '.ID')
  RT=$(mr relation-type create --name "doctest-rel-del-rt-$$-$RANDOM" --from-category=$C1 --to-category=$C2 --json | jq -r '.ID')
  A=$(mr group create --name "doctest-rel-del-a-$$-$RANDOM" --category-id=$C1 --json | jq -r '.ID')
  B=$(mr group create --name "doctest-rel-del-b-$$-$RANDOM" --category-id=$C2 --json | jq -r '.ID')
  REL=$(mr relation create --from-group-id=$A --to-group-id=$B --relation-type-id=$RT --name "rel-del-$$" --json | jq -r '.ID')
  mr relation delete $REL
  mr group get $A --json | jq -e '.Relationships == null or (.Relationships | length) == 0'
