---
outputShape: Resource object with id (uint), name (string), tags ([]Tag), groups ([]Group), meta (object)
exitCodes: 0 on success; 1 on any error
relatedCmds: resource edit, resource versions, resource download
---

# Long

Get a resource by ID and print its metadata. Fetches the full record
including tags, groups, resource category, owner, dimensions, hash,
and any custom meta JSON. Output is a key/value table by default; pass
the global `--json` flag to get the full record for scripting.

# Example

  # Get a resource by ID (table output)
  mr resource get 42

  # Get as JSON and extract a single field with jq
  mr resource get 42 --json | jq -r .name

  # mr-doctest: upload a fixture and round-trip the name
  ID=$(mr resource upload ./testdata/sample.jpg --name "doctest-get" --json | jq -r .id)
  mr resource get $ID --json | jq -e '.name == "doctest-get"'
