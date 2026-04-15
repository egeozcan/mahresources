---
outputShape: Note object with ID (uint), Name (string), Description (string), Meta (object), Tags ([]Tag), Groups ([]Group), Resources ([]Resource), OwnerId (*uint), NoteTypeId (*uint), shareToken (*string, omitempty)
exitCodes: 0 on success; 1 on any error
relatedCmds: note create, note edit-name, notes list
---

# Long

Get a note by ID and print its metadata. Fetches the full record
including name, description, meta JSON, attached tags/groups/resources,
owner group, note type, optional start/end dates, and the share token
(when the note is currently shared). Output is a key/value table by
default; pass the global `--json` flag to get the full record for
scripting.

# Example

  # Get a note by ID (table output)
  mr note get 42

  # Get as JSON and extract the name with jq
  mr note get 42 --json | jq -r .Name

  # mr-doctest: create a note and verify it is retrievable
  ID=$(mr note create --name "doctest-get-$$-$RANDOM" --json | jq -r '.ID')
  mr note get $ID --json | jq -e '.ID > 0 and (.Name | length) > 0'
