---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource edit-meta, resources meta-keys, resources add-tags
---

# Long

Add metadata keys to every Resource listed in `--ids` by passing a JSON
string via `--meta`. The server-side endpoint at
`POST /v1/resources/addMeta` determines whether this merges on top of
existing meta or replaces it — see the admin interface docs for exact
semantics. For single-resource single-key edits, use
`resource edit-meta` (dot-path syntax).

# Example

  # Set a single key on multiple resources
  mr resources add-meta --ids 1,2,3 --meta '{"status":"reviewed"}'

  # Set multiple keys at once (JSON object)
  mr resources add-meta --ids 1,2 --meta '{"priority":5,"owner":"alice"}'

  # mr-doctest: upload, add-meta, verify via get
  GRP=$(mr group create --name "doctest-addmeta-$$-$RANDOM" --json | jq -r '.ID')
  ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "addmeta-$$" --json | jq -r '.[0].ID')
  mr resources add-meta --ids $ID --meta '{"probe":"hello"}'
  mr resource get $ID --json | jq -e '.Meta.probe == "hello"'
