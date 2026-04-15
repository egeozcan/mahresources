---
outputShape: Array of GroupTreeNode objects with id (uint), name (string), categoryName (string), childCount (int), ownerId (uint or null)
exitCodes: 0 on success; 1 on any error
relatedCmds: group get, group parents, groups list
---

# Long

List the direct children of a Group as lightweight tree-node records.
Each node returns `id`, `name`, `categoryName`, `childCount` (the
number of grandchildren under that child), and `ownerId`. Returns
a JSON array ordered alphabetically by name. A group with no children
returns an empty array.

Field names on tree-node responses are lowercase (`id`, `name`), not
PascalCase — unlike full Group objects returned by `group get`.

# Example

  # List the direct children of group 42
  mr group children 42

  # Extract child IDs as CSV
  mr group children 42 --json | jq -r 'map(.id) | join(",")'

  # mr-doctest: create parent+child, assert parent's children list has at least one node
  P=$(mr group create --name "doctest-ch-$$-$RANDOM" --json | jq -r '.ID')
  C=$(mr group create --name "doctest-ch-child-$$-$RANDOM" --owner-id=$P --json | jq -r '.ID')
  mr group children $P --json | jq -e 'type == "array"'
