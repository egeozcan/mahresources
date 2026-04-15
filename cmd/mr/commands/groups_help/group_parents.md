---
outputShape: Array of Group objects representing the ancestor chain (up to 20 levels deep), including the queried group itself
exitCodes: 0 on success; 1 on any error
relatedCmds: group get, group children, groups list
---

# Long

Walk up the owner chain from a Group to its top-level ancestor. Returns
an array of Group objects ordered from outermost ancestor down to the
queried Group itself (so the last element is always the group you asked
about, and root groups return a single-element array containing just
themselves). The walk is bounded to 20 levels to defend against cycles
in corrupted data.

Use this to render breadcrumbs or to detect whether a group lives under
a particular root.

# Example

  # Show the ancestor chain for group 42
  mr group parents 42

  # Extract ancestor IDs as CSV
  mr group parents 42 --json | jq -r 'map(.ID) | join(",")'

  # mr-doctest: create parent + child, assert child's parents list contains both
  P=$(mr group create --name "doctest-par-$$-$RANDOM" --json | jq -r '.ID')
  C=$(mr group create --name "doctest-par-child-$$-$RANDOM" --owner-id=$P --json | jq -r '.ID')
  mr group parents $C --json | jq --argjson p "$P" --argjson c "$C" -e '[.[].ID] | (contains([$p]) and contains([$c]))'
