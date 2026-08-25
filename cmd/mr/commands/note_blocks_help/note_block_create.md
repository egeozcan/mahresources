---
outputShape: Created NoteBlock object with id (uint), noteId (uint), type (string), position (string), content (object), state (object), createdAt (RFC3339), updatedAt (RFC3339)
exitCodes: 0 on success; 1 on any error
relatedCmds: note-block types, note-block update, note-blocks list
---

# Long

Create a new block attached to a Note. `--note-id` and `--type` are
required, and `--content` matters more than its default suggests: the
CLI sends `{}` when the flag is omitted, so the server validates and
stores that rather than substituting the type's own default content.
`text` and `heading` reject an empty object (`text block content must
have a 'text' field`, `heading level must be 1-6`); the other built-in
types accept it and are then created empty. The exact content shape
depends on the chosen type; see `note-block types` for the default
content schema of each built-in type.

`--position` is optional; when omitted the server assigns a position
after the current last block. A `text` block that sorts first on the
note is kept in sync with the note's description: creating one rewrites
the description, and creating an empty one adopts the description as its
text. The created record is returned; capture `.id` from JSON output for
use in follow-up commands.

# Example

  # Create a text block on note 42
  mr note-block create --note-id 42 --type text --content '{"text":"hello"}'

  # Create a heading block with an explicit position
  mr note-block create --note-id 42 --type heading --content '{"text":"Intro","level":2}' --position a

  # mr-doctest: create a block and verify id and type
  NID=$(mr note create --name "doctest-nb-create-$$-$RANDOM" --json | jq -r '.ID')
  BID=$(mr note-block create --note-id=$NID --type=text --content '{"text":"hi"}' --json | jq -r '.id')
  mr note-block get $BID --json | jq -e '.id > 0 and .type == "text"'
