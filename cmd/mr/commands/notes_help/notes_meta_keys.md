---
outputShape: Array of objects with key (string), one per distinct meta key observed across the entire Note corpus
exitCodes: 0 on success; 1 on any error
relatedCmds: note edit-meta, notes add-meta
---

# Long

List every distinct `meta` key observed across the entire Note corpus.
Useful for discovering the vocabulary of an evolving meta schema. The
response is a JSON array of objects each shaped `{"key": "..."}`. The
command has no filter flags in the current CLI; pair it with
client-side `jq` filtering if you only want a subset of keys.

# Example

  # List all meta keys
  mr notes meta-keys

  # Filter client-side with jq
  mr notes meta-keys --json | jq '.[] | select(.key | startswith("project_"))'

  # mr-doctest: create a note with a known meta key, verify it appears in meta-keys
  ID=$(mr note create --name "doctest-metakeys-$$-$RANDOM" --json | jq -r '.ID')
  mr notes add-meta --ids $ID --meta '{"probe_xyz":1}'
  mr notes meta-keys --json | jq -e '[.[].key] | any(startswith("probe_xyz"))'
