---
exitCodes: 0 on success; 1 on any error
relatedCmds: resources add-tags, resources remove-tags, tag list
---

# Long

Set the exact tag set on every Resource listed in `--ids` to the tags
in `--tags`. Any tag not in the list is removed; any tag in the list is
added. Use when you want exact-state semantics rather than delta
semantics. Pass `--tags ""` to clear all tags.

# Example

  # Replace tags with exactly [5, 7]
  mr resources replace-tags --ids 1 --tags 5,7

  # Clear all tags from a resource
  mr resources replace-tags --ids 1 --tags ""

  # mr-doctest: replace with two tags, then replace with one, assert the final set size
  T1=$(mr tag create --name "replace-t1-$$" --json | jq -r .id)
  T2=$(mr tag create --name "replace-t2-$$" --json | jq -r .id)
  ID=$(mr resource upload ./testdata/sample.jpg --name "replacetag-$$" --json | jq -r .id)
  mr resources replace-tags --ids $ID --tags $T1,$T2
  mr resources replace-tags --ids $ID --tags $T1
  mr resource get $ID --json | jq -e '([.tags[]? // .Tags[]?] | length) == 1'
