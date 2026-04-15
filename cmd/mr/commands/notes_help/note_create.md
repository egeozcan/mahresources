---
outputShape: Created Note object with ID (uint), Name (string), Description (string), Meta (object), Tags ([]Tag), Groups ([]Group), Resources ([]Resource)
exitCodes: 0 on success; 1 on any error
relatedCmds: note get, note edit-name, note edit-meta, notes list
---

# Long

Create a new Note. Only `--name` is required; every other field is
optional. Use `--tags`, `--groups`, and `--resources` (comma-separated
unsigned integer IDs) to link the new Note to existing entities at
creation time. Use `--meta` to attach free-form JSON metadata, and
`--owner-id` / `--note-type-id` to set the owner group and note type
respectively. The created record is returned; capture `.ID` from JSON
output for use in follow-up commands.

# Example

  # Create a minimal note
  mr note create --name "shopping list"

  # Create with description, tags, and owner group
  mr note create --name "meeting-notes" --description "Q2 planning" --tags 5,6 --owner-id 42

  # mr-doctest: create a note and verify ID and name
  NAME="doctest-create-$$-$RANDOM"
  ID=$(mr note create --name "$NAME" --json | jq -r '.ID')
  mr note get $ID --json | jq -e --arg n "$NAME" '.ID > 0 and .Name == $n'
