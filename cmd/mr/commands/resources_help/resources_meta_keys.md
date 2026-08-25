---
outputShape: Array of objects, each with a single `key` field, one per distinct meta key across the entire resource corpus
exitCodes: 0 on success; 1 on any error
relatedCmds: resource edit-meta, resources add-meta
---

# Long

List every distinct `meta` key observed across the entire Resource
corpus. Useful for discovering the vocabulary of an evolving meta
schema. The command has no filter flags in the current CLI; pair it
with client-side `jq` filtering if you only want a subset of keys.
Output is JSON in both modes, an array of `{"key": ...}` objects.

# Example

  # List all meta keys
  mr resources meta-keys

  # Filter client-side with jq
  mr resources meta-keys --json | jq -r '.[].key | select(startswith("image_"))'

  # mr-doctest: upload with a known meta key, verify it appears in meta-keys
  GRP=$(mr group create --name "doctest-metakeys-$$-$RANDOM" --json | jq -r '.ID')
  ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "metakeys-$$" --json | jq -r '.[0].ID')
  mr resources add-meta --ids $ID --meta '{"probe_xyz":1}'
  mr resources meta-keys --json | jq -e '[.[].key] | any(startswith("probe_xyz"))'
