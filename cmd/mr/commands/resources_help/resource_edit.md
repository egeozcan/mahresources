---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource get, resource upload, resource versions
---

# Long

Edit fields on an existing resource. Any flag left unset keeps the
existing value (partial update). Collection flags (`--tags`, `--groups`,
`--notes`) take comma-separated ID lists and replace the current set;
`--meta` takes a JSON string merged onto existing meta.

# Example

  # Rename and update the description
  mr resource edit 42 --name "renamed" --description "new description"

  # Attach tags 5 and 7, replacing the current tag set
  mr resource edit 42 --tags 5,7

  # mr-doctest: upload, rename, verify
  ID=$(mr resource upload ./testdata/sample.jpg --name "orig" --json | jq -r .id)
  mr resource edit $ID --name "edited"
  mr resource get $ID --json | jq -e '.name == "edited"'
