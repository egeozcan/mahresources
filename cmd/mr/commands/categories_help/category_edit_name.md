---
exitCodes: 0 on success; 1 on any error
relatedCmds: category edit-description, category get, categories list
---

# Long

Update the name of an existing Category. Takes two positional arguments:
the category ID and the new name. The name must remain unique across
categories; the server rejects duplicates. To rename and verify in one
step, chain with `mr category get <id> --json`.

# Example

  # Rename category 42
  mr category edit-name 42 "Projects"

  # Rename and confirm with a follow-up get
  mr category edit-name 42 "renamed" && mr category get 42 --json | jq -r .Name

  # mr-doctest: create, rename, verify
  ID=$(mr category create --name "before-$$-$RANDOM" --json | jq -r '.ID')
  NEWNAME="after-$$-$RANDOM"
  mr category edit-name $ID "$NEWNAME"
  mr category get $ID --json | jq -e --arg n "$NEWNAME" '.Name == $n'
