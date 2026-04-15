---
outputShape: Status object with id (uint) and ok (bool)
exitCodes: 0 on success; 1 on any error
relatedCmds: relation-type edit, relation-type edit-description, relation-types list
---

# Long

Replace a RelationType's `Name` field. Takes the relation-type ID and
the new name as positional arguments. Shorthand for `mr relation-type
edit --id <id> --name <value>` when name is the only change. Sends
`POST /v1/relationType/editName` and returns `{id, ok}` on success.
There is no `relation-type get`: to verify, re-read with
`mr relation-types list --name <substring>` and match the ID in jq.

# Example

  # Rename relation-type 5
  mr relation-type edit-name 5 "references"

  # Rename and confirm via a filtered list
  mr relation-type edit-name 5 "contains" && \
      mr relation-types list --name "contains" --json | jq -r '.[] | select(.ID == 5) | .Name'

  # mr-doctest: create a relation-type, rename it, verify the new name via list
  C1=$(mr category create --name "doctest-rt-en-c1-$$-$RANDOM" --json | jq -r '.ID')
  C2=$(mr category create --name "doctest-rt-en-c2-$$-$RANDOM" --json | jq -r '.ID')
  RT=$(mr relation-type create --name "doctest-rt-en-init-$$-$RANDOM" --from-category=$C1 --to-category=$C2 --json | jq -r '.ID')
  NEW="doctest-rt-en-renamed-$$-$RANDOM"
  mr relation-type edit-name $RT "$NEW"
  mr relation-types list --name "doctest-rt-en-renamed" --json | jq -e --argjson r "$RT" --arg n "$NEW" 'map(select(.ID == $r)) | .[0].Name == $n'
