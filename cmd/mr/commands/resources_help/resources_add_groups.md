---
exitCodes: 0 on success; 1 on any error
relatedCmds: resources add-tags, resources add-meta, groups list
---

# Long

Add group IDs to every Resource listed in `--ids`. Idempotent. Both
`--ids` and `--groups` accept comma-separated unsigned integer lists
and are required.

# Example

  # Add groups 2 and 3 to resources 1, 2
  mr resources add-groups --ids 1,2 --groups 2,3

  # Bulk from a list query
  mr resources list --content-type image/jpeg --json | jq -r 'map(.id) | join(",")' | xargs -I {} mr resources add-groups --ids {} --groups 7

  # mr-doctest: create group, upload, add-groups, assert membership
  GRP=$(mr group create --name "doctest-addgroups-$$-$RANDOM" --json | jq -r '.ID')
  OWNER=$(mr group create --name "doctest-addgroups-owner-$$-$RANDOM" --json | jq -r '.ID')
  ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$OWNER --name "addgroup-$$" --json | jq -r '.[0].ID')
  mr resources add-groups --ids $ID --groups $GRP
  mr resource get $ID --json | jq -e '(.Groups | length) >= 1'
