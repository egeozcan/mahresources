---
outputShape: Resource object with id, name, tags, groups, meta
exitCodes: 0 on success; 1 on any error
relatedCmds: resource edit, resource versions, resource download
---

# Example

  # Get a resource by ID (table output)
  mr resource get 42

  # mr-doctest: upload, fetch, assert name
  ID=$(mr resource upload ./testdata/sample.jpg --name "sample" --json | jq -r .id)
  mr resource get $ID --json | jq -e '.name == "sample"'
