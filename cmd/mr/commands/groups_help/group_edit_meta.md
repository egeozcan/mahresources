---
outputShape: Status object with id (uint), ok (bool), and meta (object reflecting the merged Meta)
exitCodes: 0 on success; 1 on any error
relatedCmds: group get, groups add-meta, groups meta-keys
---

# Long

Edit a single metadata field by JSON path. Takes three positional
arguments: the Group ID, a dot-separated path (e.g. `address.city`),
and a JSON-literal value (e.g. `'"Berlin"'`, `42`, `'[1,2,3]'`,
`'{"nested":true}'`). The server deep-merges the value at the given
path onto the existing Meta object and returns the full merged Meta
in the response.

Values must be valid JSON literals — string values need to be quoted
twice (bash single quotes around a JSON-quoted string), as in the
examples below.

# Example

  # Set a top-level string value
  mr group edit-meta 5 status '"active"'

  # Set a nested field
  mr group edit-meta 5 address.city '"Berlin"'

  # Replace a field with an array
  mr group edit-meta 5 scores '[1,2,3]'

  # mr-doctest: set a meta field, verify via group get
  ID=$(mr group create --name "doctest-meta-$$-$RANDOM" --json | jq -r '.ID')
  mr group edit-meta $ID status '"active"'
  mr group get $ID --json | jq -e '.Meta.status == "active"'
