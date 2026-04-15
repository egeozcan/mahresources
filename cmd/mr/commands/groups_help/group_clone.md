---
outputShape: Group object for the newly-created clone (new ID, new guid; copied Name, Description, Meta, owner/category references)
exitCodes: 0 on success; 1 on any error
relatedCmds: group get, group create, group export
---

# Long

Create a copy of an existing Group. The clone receives a new ID and
GUID but inherits the source Group's `Name`, `Description`, `Meta`,
`OwnerId`, `CategoryId`, and tag associations. Related resources,
notes, and sub-groups are NOT cloned — use `group export` + `group
import` for a deep subtree copy.

# Example

  # Clone group 42
  mr group clone 42

  # Clone and capture the new ID with jq
  NEW=$(mr group clone 42 --json | jq -r '.ID')

  # mr-doctest: create a group, clone it, assert new ID differs from source
  SRC=$(mr group create --name "doctest-clone-$$-$RANDOM" --json | jq -r '.ID')
  NEW=$(mr group clone $SRC --json | jq -r '.ID')
  test "$NEW" != "$SRC" && test "$NEW" -gt 0
