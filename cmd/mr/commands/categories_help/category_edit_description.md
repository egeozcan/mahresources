---
exitCodes: 0 on success; 1 on any error
relatedCmds: category edit-name, category get, categories list
---

# Long

Update the description of an existing Category. Takes two positional
arguments: the category ID and the new description. Passing an empty
string clears the description. Useful for annotating categories with
guidance about what Groups belong under them without renaming.

# Example

  # Set a description on category 42
  mr category edit-description 42 "places and venues"

  # Clear the description by passing an empty string
  mr category edit-description 42 ""

  # mr-doctest: create, set description, verify
  ID=$(mr category create --name "desc-test-$$-$RANDOM" --json | jq -r '.ID')
  mr category edit-description $ID "hello world"
  mr category get $ID --json | jq -e '.Description == "hello world"'
