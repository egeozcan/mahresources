---
exitCodes: 0 on success; 1 on any error
relatedCmds: group get, groups delete, groups list
---

# Long

Merge one or more "loser" Groups into a single "winner". The winner's
ID and fields are preserved; tags, owned resources, owned notes, and
m2m relations from the losers are moved onto the winner; the loser
records are then deleted. Use to consolidate duplicates after manual
review or deduplication.

Both flags are required: `--winner <id>` is a single ID, and
`--losers` is a comma-separated list of IDs to merge in.

# Example

  # Merge groups 2 and 3 into winner 1
  mr groups merge --winner 1 --losers 2,3

  # mr-doctest: create winner + 2 losers, merge, assert losers are gone
  W=$(mr group create --name "merge-win-$$-$RANDOM" --json | jq -r '.ID')
  L1=$(mr group create --name "merge-loser1-$$-$RANDOM" --json | jq -r '.ID')
  L2=$(mr group create --name "merge-loser2-$$-$RANDOM" --json | jq -r '.ID')
  mr groups merge --winner=$W --losers=$L1,$L2
  ! mr group get $L1 2>/dev/null && ! mr group get $L2 2>/dev/null
