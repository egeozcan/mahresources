---
outputShape: Status object with id (uint) and ok (bool)
exitCodes: 0 on success; 1 on any error
relatedCmds: relation create, relation edit-description, group get
---

# Long

Replace a Relation's `Name` field. Takes the relation ID and the new
name as positional arguments. Sends `POST /v1/relation/editName` and
returns `{id, ok}` on success. There is no `relation get`: to verify,
re-fetch a participating group with `mr group get <id> --json` and
read the name from its `Relationships` array.

# Example

  # Rename relation 7
  mr relation edit-name 7 "directed-by"

  # Rename and confirm via the source group
  mr relation edit-name 7 "produced-by" && \
      mr group get 3 --json | jq -r '.Relationships[] | select(.ID == 7) | .Name'

  # mr-doctest: create a relation, rename it, verify via group get
  C1=$(mr category create --name "doctest-rel-rename-c1-$$-$RANDOM" --json | jq -r '.ID')
  C2=$(mr category create --name "doctest-rel-rename-c2-$$-$RANDOM" --json | jq -r '.ID')
  RT=$(mr relation-type create --name "doctest-rel-rename-rt-$$-$RANDOM" --from-category=$C1 --to-category=$C2 --json | jq -r '.ID')
  A=$(mr group create --name "doctest-rel-rename-a-$$-$RANDOM" --category-id=$C1 --json | jq -r '.ID')
  B=$(mr group create --name "doctest-rel-rename-b-$$-$RANDOM" --category-id=$C2 --json | jq -r '.ID')
  REL=$(mr relation create --from-group-id=$A --to-group-id=$B --relation-type-id=$RT --name "before-$$" --json | jq -r '.ID')
  NEW="after-$$-$RANDOM"
  mr relation edit-name $REL "$NEW"
  mr group get $A --json | jq -e --argjson r "$REL" --arg n "$NEW" '.Relationships[] | select(.ID == $r) | .Name == $n'
