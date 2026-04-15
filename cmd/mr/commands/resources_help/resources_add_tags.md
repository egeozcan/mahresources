---
exitCodes: 0 on success; 1 on any error
relatedCmds: resources remove-tags, resources replace-tags, tags list
---

# Long

Add tag IDs to every Resource listed in `--ids`. Idempotent: adding a
tag that's already attached is a no-op. Both `--ids` and `--tags`
accept comma-separated unsigned integer lists and are required.

# Example

  # Add tag 5 to resources 1,2,3
  mr resources add-tags --ids 1,2,3 --tags 5

  # Add multiple tags at once
  mr resources add-tags --ids 1,2,3 --tags 5,6,7

  # mr-doctest: create tag, upload two fixtures, add-tags, list by tag, assert count >= 2
  TAG=$(mr tag create --name "add-tags-test-$$-$RANDOM" --json | jq -r '.ID')
  GRP=$(mr group create --name "doctest-addtags-$$-$RANDOM" --json | jq -r '.ID')
  ID1=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "addtag-a-$$" --json | jq -r '.[0].ID')
  ID2=$(mr resource upload ./testdata/sample.png --owner-id=$GRP --name "addtag-b-$$" --json | jq -r '.[0].ID')
  mr resources add-tags --ids $ID1,$ID2 --tags $TAG
  mr resources list --tags $TAG --json | jq -e 'length >= 2'
