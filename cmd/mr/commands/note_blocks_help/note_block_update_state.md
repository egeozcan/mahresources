---
outputShape: Updated NoteBlock object with id (uint), noteId (uint), type (string), position (string), content (object), state (object)
exitCodes: 0 on success; 1 on any error
relatedCmds: note-block update, note-block get, note-block types
---

# Long

Replace a block's `state` payload. Takes the block ID as a positional
argument and the new state as `--state` JSON. `state` is separate from
`content`: it holds runtime/UI state like which todo items are checked,
which gallery layout is selected, or a calendar's current view. The
shape depends on the block's type (see `note-block types` for default
state schemas). Sending `null` or an empty body returns an error: the
state column has a NOT NULL constraint.

# Example

  # Mark a text block as "done" via a custom state field
  mr note-block update-state 42 --state '{"done":true}'

  # Check off a todo item (todos blocks use `{"checked":[itemId,...]}`)
  mr note-block update-state 42 --state '{"checked":["task-1"]}'

  # mr-doctest: create, update state, verify
  NID=$(mr note create --name "doctest-nb-state-$$-$RANDOM" --json | jq -r '.ID')
  BID=$(mr note-block create --note-id=$NID --type=text --content '{"text":"hi"}' --json | jq -r '.id')
  mr note-block update-state $BID --state '{"done":true}' --json > /dev/null
  mr note-block get $BID --json | jq -e '.state.done == true'
