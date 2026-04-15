---
exitCodes: 0 on success; 1 on any error
relatedCmds: resources add-tags, resources replace-tags, tag list
---

# Long

Remove tag IDs from every Resource listed in `--ids`. Idempotent:
removing a tag that isn't attached is a no-op. Both `--ids` and `--tags`
accept comma-separated unsigned integer lists and are required.

# Example

  # Remove tag 5 from resources 1,2
  mr resources remove-tags --ids 1,2 --tags 5

  # Remove multiple tags at once
  mr resources remove-tags --ids 1,2,3 --tags 5,6

  # mr-doctest: add then remove, assert count drops to 0
  TAG=$(mr tag create --name "remove-tags-test-$$" --json | jq -r .id)
  ID=$(mr resource upload ./testdata/sample.jpg --name "remtag-$$" --json | jq -r .id)
  mr resources add-tags --ids $ID --tags $TAG
  mr resources remove-tags --ids $ID --tags $TAG
  mr resources list --tags $TAG --json | jq -e 'length == 0'
