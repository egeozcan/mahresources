---
outputShape: Status object with ok (bool)
exitCodes: 0 on success; 1 on any error
relatedCmds: groups remove-tags, group get, tags list
---

# Long

Attach one or more Tags to a set of Groups in a single request. Both
arguments are comma-separated ID lists: `--ids` selects the target
Groups and `--tags` selects the Tags to add. The server merges the
requested tag links with whatever each Group already has; existing
links are unaffected, and no tag links are removed.

Verify the result by reading a target Group back with
`mr group get <id> --json | jq '.Tags'`.

# Example

  # Tag three groups with tag 5
  mr groups add-tags --ids 10,11,12 --tags 5

  # Add multiple tags to one group
  mr groups add-tags --ids 10 --tags 5,6,7

  # mr-doctest: create a tag + group, add the tag, assert the group's Tags include it
  GID=$(mr group create --name "doctest-addtag-$$-$RANDOM" --json | jq -r '.ID')
  TID=$(mr tag create --name "doctest-addtag-tag-$$-$RANDOM" --json | jq -r '.ID')
  mr groups add-tags --ids=$GID --tags=$TID
  mr group get $GID --json | jq --argjson t "$TID" -e '[.Tags[].ID] | contains([$t])'
