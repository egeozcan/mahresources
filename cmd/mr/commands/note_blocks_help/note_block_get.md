---
outputShape: NoteBlock object with id (uint), noteId (uint), type (string), position (string), content (object), state (object), createdAt (RFC3339), updatedAt (RFC3339)
exitCodes: 0 on success; 1 on any error
relatedCmds: note-block update, note-block update-state, note-blocks list
---

# Long

Get a single note block by ID and print its fields. Fetches the full
record including the parent note ID, block type, fractional position,
content JSON, state JSON, and timestamps. Output is a key/value table
by default; pass the global `--json` flag to get the full record for
scripting.

# Example

  # Get a note block by ID (table output)
  mr note-block get 42

  # Get as JSON and extract the block type
  mr note-block get 42 --json | jq -r .type

  # mr-doctest: create a block and verify it is retrievable
  NID=$(mr note create --name "doctest-nb-get-$$-$RANDOM" --json | jq -r '.ID')
  BID=$(mr note-block create --note-id=$NID --type=text --content '{"text":"hello"}' --json | jq -r '.id')
  mr note-block get $BID --json | jq -e '.id > 0 and (.type | length) > 0'
