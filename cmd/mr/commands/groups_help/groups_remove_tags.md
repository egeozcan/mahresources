---
outputShape: Status object with ok (bool)
exitCodes: 0 on success; 1 on any error
relatedCmds: groups add-tags, group get, tags list
---

# Long

Detach one or more Tags from a set of Groups in a single request. Both
arguments are comma-separated ID lists: `--ids` selects the target
Groups and `--tags` selects the Tags to remove. Other tag links on the
targeted Groups are left untouched, and removing a tag that was never
linked is a no-op (not an error).

# Example

  # Remove tag 5 from three groups
  mr groups remove-tags --ids 10,11,12 --tags 5

  # Remove multiple tags from one group
  mr groups remove-tags --ids 10 --tags 5,6,7

  # mr-doctest: add a tag then remove it, assert Tags array no longer contains the tag id
  GID=$(mr group create --name "doctest-rmtag-$$-$RANDOM" --json | jq -r '.ID')
  TID=$(mr tag create --name "doctest-rmtag-tag-$$-$RANDOM" --json | jq -r '.ID')
  mr groups add-tags --ids=$GID --tags=$TID
  mr groups remove-tags --ids=$GID --tags=$TID
  mr group get $GID --json | jq --argjson t "$TID" -e '[.Tags[].ID // empty] | contains([$t]) | not'
