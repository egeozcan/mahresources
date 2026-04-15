---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource-category edit-name, resource-category get, resource-categories list
---

# Long

Update the description of an existing resource category. Takes two
positional arguments: the resource category ID and the new description.
Passing an empty string clears the description. Useful for annotating
categories used across many resources without renaming them.

# Example

  # Set a description on resource category 42
  mr resource-category edit-description 42 "high-resolution scans"

  # Clear the description by passing an empty string
  mr resource-category edit-description 42 ""

  # mr-doctest: create, set description, verify
  ID=$(mr resource-category create --name "desc-rc-$$-$RANDOM" --json | jq -r '.ID')
  mr resource-category edit-description $ID "hello world"
  mr resource-category get $ID --json | jq -e '.Description == "hello world"'
