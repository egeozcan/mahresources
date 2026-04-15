---
outputShape: Created NoteBlock object with id (uint), noteId (uint), type (string), position (string), content (object), state (object)
exitCodes: 0 on success; 1 on any error
relatedCmds: note-block types, note-block update, note-blocks list
---

# Long

Create a new block attached to a Note. `--note-id` and `--type` are
required. Use `--content` to supply the block's content JSON (the exact
shape depends on the chosen type — see `note-block types` for the
default content schema of each built-in type). `--position` is optional;
when omitted the server assigns a position after the current last block.
The created record is returned; capture `.id` from JSON output for use
in follow-up commands.

# Example

  # Create a text block on note 42
  mr note-block create --note-id 42 --type text --content '{"text":"hello"}'

  # Create a heading block with an explicit position
  mr note-block create --note-id 42 --type heading --content '{"text":"Intro","level":2}' --position a

  # mr-doctest: create a block and verify id and type
  NID=$(mr note create --name "doctest-nb-create-$$-$RANDOM" --json | jq -r '.ID')
  BID=$(mr note-block create --note-id=$NID --type=text --content '{"text":"hi"}' --json | jq -r '.id')
  mr note-block get $BID --json | jq -e '.id > 0 and .type == "text"'
