---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource edit, resource edit-name, resource edit-meta
---

# Long

Update only the description of an existing resource. Passing an empty
string clears the description. Shorthand for `mr resource edit <id> --description <value>`.

# Example

  # Set the description on resource 42
  mr resource edit-description 42 "scanned contract, Q1 2026"

  # Clear the description by passing an empty string
  mr resource edit-description 42 ""

  # mr-doctest: upload, set description, verify
  ID=$(mr resource upload ./testdata/sample.jpg --name "desc-test" --json | jq -r .id)
  mr resource edit-description $ID "hello world"
  mr resource get $ID --json | jq -e '.description == "hello world"'
