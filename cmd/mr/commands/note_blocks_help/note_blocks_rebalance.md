---
exitCodes: 0 on success; 1 on any error
relatedCmds: note-blocks reorder, note-blocks list
---

# Long

Rewrite every block's `position` string on a note to evenly spaced,
compact values while preserving the current display order. Use this
as a cleanup step after heavy reordering, when fractional positions
have grown long (e.g. `"aaamzzz"`), or when you want a predictable
position layout before a batch of reorders. The block IDs, types,
content, and state are untouched.

# Example

  # Rebalance all block positions on note 42
  mr note-blocks rebalance --note-id 42

  # Rebalance, then dump the new positions for inspection
  mr note-blocks rebalance --note-id 42
  mr note-blocks list --note-id 42 --json | jq -r '.[] | [.id, .position] | @tsv'

  # mr-doctest: create two blocks, rebalance, verify every block has a non-empty position
  NID=$(mr note create --name "doctest-nbs-rebal-$$-$RANDOM" --json | jq -r '.ID')
  mr note-block create --note-id=$NID --type=text --content '{"text":"a"}' --json > /dev/null
  mr note-block create --note-id=$NID --type=text --content '{"text":"b"}' --json > /dev/null
  mr note-blocks rebalance --note-id=$NID
  mr note-blocks list --note-id=$NID --json | jq -e 'type == "array" and length >= 2 and all(.[]; (.position | length) > 0)'
