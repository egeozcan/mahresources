---
exitCodes: 0 on success; 1 on any error
relatedCmds: tags delete, tags list, resources merge
---

# Long

Merge one or more "loser" tags into a single "winner". The winner's ID
and name are preserved; Resources, Notes, and Groups previously tagged
with any loser are re-tagged with the winner; the loser tag rows are
then deleted. Use to consolidate duplicate or redundant tags (e.g.,
`photo` and `photos`) without losing associations.

# Example

  # Merge tags 2 and 3 into winner 1
  mr tags merge --winner 1 --losers 2,3

  # Merge the result of a filter
  mr tags merge --winner 1 --losers $(mr tags list --name "dup-" --json | jq -r 'map(.ID) | join(",")')

  # mr-doctest: create winner and loser, merge, assert loser is gone
  W=$(mr tag create --name "merge-w-$$-$RANDOM" --json | jq -r '.ID')
  L=$(mr tag create --name "merge-l-$$-$RANDOM" --json | jq -r '.ID')
  mr tags merge --winner $W --losers $L
  ! mr tag get $L 2>/dev/null
