---
exitCodes: 0 on success; 1 on any error
relatedCmds: note edit-meta, notes meta-keys, notes add-tags
---

# Long

Add metadata keys to every Note listed in `--ids` by passing a JSON
string via `--meta`. The server-side endpoint at
`POST /v1/notes/addMeta` determines whether this merges on top of
existing meta or replaces it — see the admin interface docs for exact
semantics. For single-note single-key edits, use `note edit-meta`
(dot-path syntax).

# Example

  # Set a single key on multiple notes
  mr notes add-meta --ids 1,2,3 --meta '{"status":"reviewed"}'

  # Set multiple keys at once (JSON object)
  mr notes add-meta --ids 1,2 --meta '{"priority":5,"owner":"alice"}'

  # mr-doctest: create note, add-meta, verify via get
  ID=$(mr note create --name "doctest-addmeta-$$-$RANDOM" --json | jq -r '.ID')
  mr notes add-meta --ids $ID --meta '{"probe":"hello"}'
  mr note get $ID --json | jq -e '.Meta.probe == "hello"'
