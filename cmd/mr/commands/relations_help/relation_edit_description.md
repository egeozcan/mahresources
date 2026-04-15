---
outputShape: Status object with id (uint) and ok (bool)
exitCodes: 0 on success; 1 on any error
relatedCmds: relation create, relation edit-name, group get
---

# Long

Replace a Relation's `Description` field. Takes the relation ID and
the new description as positional arguments; pass an empty string to
clear. Sends `POST /v1/relation/editDescription` and returns
`{id, ok}` on success. There is no `relation get`: to verify, re-fetch
a participating group with `mr group get <id> --json` and read the
description from its `Relationships` array.

# Example

  # Set the description on relation 7
  mr relation edit-description 7 "confirmed by archival records"

  # Clear the description by passing an empty string
  mr relation edit-description 7 ""

  # mr-doctest: create a relation, set description, verify via group get
  C1=$(mr category create --name "doctest-rel-desc-c1-$$-$RANDOM" --json | jq -r '.ID')
  C2=$(mr category create --name "doctest-rel-desc-c2-$$-$RANDOM" --json | jq -r '.ID')
  RT=$(mr relation-type create --name "doctest-rel-desc-rt-$$-$RANDOM" --from-category=$C1 --to-category=$C2 --json | jq -r '.ID')
  A=$(mr group create --name "doctest-rel-desc-a-$$-$RANDOM" --category-id=$C1 --json | jq -r '.ID')
  B=$(mr group create --name "doctest-rel-desc-b-$$-$RANDOM" --category-id=$C2 --json | jq -r '.ID')
  REL=$(mr relation create --from-group-id=$A --to-group-id=$B --relation-type-id=$RT --name "desc-test-$$" --json | jq -r '.ID')
  DESC="described-$$-$RANDOM"
  mr relation edit-description $REL "$DESC"
  mr group get $A --json | jq -e --argjson r "$REL" --arg d "$DESC" '.Relationships[] | select(.ID == $r) | .Description == $d'
