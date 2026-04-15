---
outputShape: Created Category object with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: category get, category edit-name, categories list
---

# Long

Create a new Category. `--name` is required; `--description` is optional
free-form text. The optional `--custom-header`, `--custom-sidebar`,
`--custom-summary`, `--custom-avatar`, and `--custom-mrql-result` flags
accept template or HTML strings applied to Groups assigned to this
category. `--meta-schema` and `--section-config` take JSON strings
controlling structured metadata and which sections render on group
detail pages. On success prints a confirmation line with the new ID;
pass the global `--json` flag to emit the full record for scripting.

# Example

  # Create a category with just a name
  mr category create --name "Project"

  # Create with a description and capture the ID via jq
  ID=$(mr category create --name "Location" --description "Places you know about" --json | jq -r .ID)

  # mr-doctest: create a category, assert the returned ID is positive
  mr category create --name "doctest-create-$$-$RANDOM" --json | jq -e '.ID > 0'
