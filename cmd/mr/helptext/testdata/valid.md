---
outputShape: Resource object with id, name, tags, groups, meta
exitCodes: 0 on success; 1 on any error
relatedCmds: resource edit, resource versions, resource download
---

# Long

Get a resource by ID and print its metadata.

Fetches a single resource with its tags, groups, categories, and custom meta
fields. The resource ID is required as a positional argument.

# Example

  # Get a resource by ID (table output)
  mr resource get 42

  # mr-doctest: upload, fetch, assert name
  ID=$(mr resource upload ./testdata/sample.jpg --name "sample" --json | jq -r .id)
  mr resource get $ID --json | jq -e '.name == "sample"'
