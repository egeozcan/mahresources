---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource get, resources delete, search
---

# Long

Merge one or more "loser" Resources into a single "winner". The
winner's bytes and ID are preserved. Tags, notes and related groups move
onto the winner, as does each loser's owner group; the losers' versions
are reassigned to the winner and renumbered, and their `meta` is merged
into the winner's. A JSON copy of each loser row is kept under the
winner's `meta.backups`, and the loser files follow the same `/deleted/`
backup path as `resources delete`. Use to consolidate duplicates after
perceptual-hash detection or manual review.

# Example

  # Merge resources 2 and 3 into winner 1
  mr resources merge --winner 1 --losers 2,3

  # Pipe duplicate IDs from a search
  mr resources merge --winner 1 --losers $(mr resources list --hash abcd1234 --json | jq -r 'map(.ID) | join(",")')

  # mr-doctest: create winner + 2 losers with distinct tags, merge, assert winner has all tags
  T1=$(mr tag create --name "merge-t1-$$-$RANDOM" --json | jq -r '.ID')
  T2=$(mr tag create --name "merge-t2-$$-$RANDOM" --json | jq -r '.ID')
  GRP=$(mr group create --name "doctest-merge-$$-$RANDOM" --json | jq -r '.ID')
  W=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "winner-$$" --json | jq -r '.[0].ID')
  L1=$(mr resource upload ./testdata/sample.png --owner-id=$GRP --name "loser1-$$" --json | jq -r '.[0].ID')
  L2=$(mr resource upload ./testdata/sample.txt --owner-id=$GRP --name "loser2-$$" --json | jq -r '.[0].ID')
  mr resources add-tags --ids $L1 --tags $T1
  mr resources add-tags --ids $L2 --tags $T2
  mr resources merge --winner $W --losers $L1,$L2
  mr resource get $W --json | jq -e '(.Tags | length) >= 2'
