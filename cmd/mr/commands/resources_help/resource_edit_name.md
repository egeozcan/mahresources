---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource edit, resource edit-description, resource edit-meta
---

# Long

Update only the name of an existing resource. Shorthand for
`mr resource edit <id> --name <value>` when name is the only change.

# Example

  # Rename resource 42
  mr resource edit-name 42 "my new name"

  # Rename and confirm with a follow-up get
  mr resource edit-name 42 "renamed" && mr resource get 42 --json | jq -r .name

  # mr-doctest: upload, rename, verify
  ID=$(mr resource upload ./testdata/sample.jpg --name "before" --json | jq -r .id)
  mr resource edit-name $ID "after"
  mr resource get $ID --json | jq -e '.name == "after"'
