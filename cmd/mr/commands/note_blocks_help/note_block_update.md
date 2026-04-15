---
outputShape: Updated NoteBlock object with id (uint), noteId (uint), type (string), position (string), content (object), state (object)
exitCodes: 0 on success; 1 on any error
relatedCmds: note-block update-state, note-block get, note-block create
---

# Long

Replace a block's `content` payload. Takes the block ID as a positional
argument and the new content as `--content` JSON. The content shape
must match the block's type (see `note-block types` for the default
content schema of each built-in type). This command does not touch the
block's `state`, `position`, or `type` — use `note-block update-state`
for state changes and `note-blocks reorder` for position changes.

# Example

  # Update a text block's content
  mr note-block update 42 --content '{"text":"new body"}'

  # Update and print the updated record as JSON
  mr note-block update 42 --content '{"text":"new body"}' --json | jq .

  # mr-doctest: create, update, verify content
  NID=$(mr note create --name "doctest-nb-upd-$$-$RANDOM" --json | jq -r '.ID')
  BID=$(mr note-block create --note-id=$NID --type=text --content '{"text":"before"}' --json | jq -r '.id')
  mr note-block update $BID --content '{"text":"after"}' --json > /dev/null
  mr note-block get $BID --json | jq -e '.content.text == "after"'
