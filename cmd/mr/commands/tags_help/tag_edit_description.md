---
exitCodes: 0 on success; 1 on any error
relatedCmds: tag edit-name, tag get, tags list
---

# Long

Update the description of an existing tag. Takes two positional
arguments: the tag ID and the new description. Passing an empty string
clears the description. Useful for annotating tags used across many
resources without renaming them.

# Example

  # Set a description on tag 42
  mr tag edit-description 42 "used for Q1 2026 scans"

  # Clear the description by passing an empty string
  mr tag edit-description 42 ""

  # mr-doctest: create, set description, verify
  ID=$(mr tag create --name "desc-test-$$-$RANDOM" --json | jq -r '.ID')
  mr tag edit-description $ID "hello world"
  mr tag get $ID --json | jq -e '.Description == "hello world"'
