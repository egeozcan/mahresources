---
outputShape: Status object with id (uint) and ok (bool)
exitCodes: 0 on success; 1 on any error
relatedCmds: relation-type edit, relation-type edit-name, relation-types list
---

# Long

Replace a RelationType's `Description` field. Takes the relation-type
ID and the new description as positional arguments; pass an empty
string to clear. Shorthand for `mr relation-type edit --id <id>
--description <value>`. Sends `POST /v1/relationType/editDescription`
and returns `{id, ok}`. There is no `relation-type get`: to verify,
re-read with `mr relation-types list --name <substring>` and inspect
the `.Description` field in jq.

# Example

  # Set the description on relation-type 5
  mr relation-type edit-description 5 "references another record"

  # Clear the description by passing an empty string
  mr relation-type edit-description 5 ""

  # mr-doctest: create a relation-type, set description, verify via list
  C1=$(mr category create --name "doctest-rt-ed-c1-$$-$RANDOM" --json | jq -r '.ID')
  C2=$(mr category create --name "doctest-rt-ed-c2-$$-$RANDOM" --json | jq -r '.ID')
  RT=$(mr relation-type create --name "doctest-rt-ed-init-$$-$RANDOM" --from-category=$C1 --to-category=$C2 --json | jq -r '.ID')
  DESC="doctest-rt-ed-desc-$$-$RANDOM"
  mr relation-type edit-description $RT "$DESC"
  mr relation-types list --name "doctest-rt-ed-init" --json | jq -e --argjson r "$RT" --arg d "$DESC" 'map(select(.ID == $r)) | .[0].Description == $d'
