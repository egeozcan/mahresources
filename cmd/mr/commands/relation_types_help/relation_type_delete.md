---
outputShape: Status object with id
exitCodes: 0 on success; 1 on any error
relatedCmds: relation-type create, relation-types list, relation delete
---

# Long

Delete a RelationType by ID. Destructive: removes the type row
entirely. Existing Relations that reference this type may be orphaned
or cascade-deleted depending on the server's foreign-key configuration;
inspect affected groups with `mr group get <id> --json` after a
delete. Deleting a nonexistent ID returns exit code 1.

# Example

  # Delete relation-type 5
  mr relation-type delete 5

  # Delete and pipe the result to jq
  mr relation-type delete 5 --json | jq .

  # mr-doctest: create a relation-type, delete it, verify it's gone from the list
  C1=$(mr category create --name "doctest-rt-del-c1-$$-$RANDOM" --json | jq -r '.ID')
  C2=$(mr category create --name "doctest-rt-del-c2-$$-$RANDOM" --json | jq -r '.ID')
  RT=$(mr relation-type create --name "doctest-rt-del-$$-$RANDOM" --from-category=$C1 --to-category=$C2 --json | jq -r '.ID')
  mr relation-type delete $RT
  mr relation-types list --json | jq -e --argjson r "$RT" 'map(select(.ID == $r)) | length == 0'
