---
exitCodes: 0 on success; 1 on any error
relatedCmds: notes add-tags, notes add-meta, groups list
---

# Long

Add group IDs to every Note listed in `--ids`. Idempotent. Both
`--ids` and `--groups` accept comma-separated unsigned integer lists
and are required. The linked Groups appear in the Note's `Groups`
array on subsequent `get` responses.

# Example

  # Add groups 2 and 3 to notes 1, 2
  mr notes add-groups --ids 1,2 --groups 2,3

  # Bulk from a list query
  mr notes list --tags 5 --json | jq -r '[.[].ID] | join(",")' | xargs -I {} mr notes add-groups --ids {} --groups 7

  # mr-doctest: create group + note, add-groups, assert membership
  GRP=$(mr group create --name "doctest-addgroups-$$-$RANDOM" --json | jq -r '.ID')
  ID=$(mr note create --name "doctest-addgroup-note-$$-$RANDOM" --json | jq -r '.ID')
  mr notes add-groups --ids $ID --groups $GRP
  mr note get $ID --json | jq -e '(.Groups | length) >= 1'
