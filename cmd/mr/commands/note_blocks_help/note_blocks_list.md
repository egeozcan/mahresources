---
outputShape: Array of NoteBlock objects with id, noteId, type, position, content, state, createdAt, updatedAt (ordered by position ascending)
exitCodes: 0 on success; 1 on any error
relatedCmds: note-block get, note-blocks reorder, note-blocks rebalance
---

# Long

List every block attached to a Note in position order. `--note-id` is
required; the server returns the full set (no pagination), ordered by
the fractional `position` string. Use this to inspect the current
layout before reordering, to dump a note's structured content to JSON
for processing, or to feed block IDs into downstream commands.

# Example

  # List every block on note 42 (table output)
  mr note-blocks list --note-id 42

  # Get blocks as JSON and extract id + position pairs
  mr note-blocks list --note-id 42 --json | jq -r '.[] | [.id, .position] | @tsv'

  # mr-doctest: create a note with two blocks, list, assert count >= 2
  NID=$(mr note create --name "doctest-nbs-list-$$-$RANDOM" --json | jq -r '.ID')
  mr note-block create --note-id=$NID --type=text --content '{"text":"a"}' --json > /dev/null
  mr note-block create --note-id=$NID --type=text --content '{"text":"b"}' --json > /dev/null
  mr note-blocks list --note-id=$NID --json | jq -e 'type == "array" and length >= 2'
