---
outputShape: Array of objects with shape [{"key": string}] — one entry per distinct Meta key across all Groups
exitCodes: 0 on success; 1 on any error
relatedCmds: group edit-meta, groups add-meta
---

# Long

List every distinct `Meta` key observed across the entire Group corpus.
Useful for discovering the vocabulary of an evolving meta schema and
for building UI dropdowns of known keys. The command has no filter
flags in the current CLI; pair it with client-side `jq` filtering if
you only want a subset of keys.

The JSON shape is an array of objects with a `key` field
(`[{"key":"status"}, {"key":"owner"}]`), not a flat string array.

# Example

  # List all meta keys
  mr groups meta-keys

  # Filter client-side with jq
  mr groups meta-keys --json | jq -r '.[].key | select(startswith("probe_"))'

  # mr-doctest: stamp a distinctive meta key, verify it appears in meta-keys
  GID=$(mr group create --name "doctest-mkey-$$-$RANDOM" --json | jq -r '.ID')
  KEY="probe_mkey_$RANDOM"
  mr groups add-meta --ids=$GID --meta="{\"$KEY\":1}"
  mr groups meta-keys --json | jq --arg k "$KEY" -e '[.[].key] | contains([$k])'
