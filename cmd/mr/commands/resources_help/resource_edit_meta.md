---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource edit, resources add-meta, resources meta-keys
---

# Long

Edit a single metadata field at a dot-separated JSON path. Takes three
positional arguments: the resource ID, the path (e.g., `address.city`),
and a JSON literal value (e.g., `'"Berlin"'`, `42`, `'{"nested":"obj"}'`,
`'[1,2,3]'`). Creates intermediate path segments as needed and leaves
sibling keys at each level untouched.

# Example

  # Set a top-level string field (note: shell-quoted JSON string)
  mr resource edit-meta 5 status '"active"'

  # Set a nested numeric field (creates address.postalCode if missing)
  mr resource edit-meta 5 address.postalCode 10115

  # mr-doctest: upload, set meta, verify
  GRP=$(mr group create --name "doctest-editmeta-$$-$RANDOM" --json | jq -r '.ID')
  ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "meta-test-$$" --json | jq -r '.[0].ID')
  mr resource edit-meta $ID priority 5
  mr resource get $ID --json | jq -e '.Meta.priority == 5'
