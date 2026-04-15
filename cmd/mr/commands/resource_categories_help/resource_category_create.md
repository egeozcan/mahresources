---
outputShape: Created ResourceCategory object with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: resource-category get, resource-category edit-name, resource-categories list
---

# Long

Create a new resource category. `--name` is required; all other flags
are optional, including a plain `--description`, presentation
fields (`--custom-header`, `--custom-sidebar`, `--custom-summary`,
`--custom-avatar`, `--custom-mrql-result`) and structural fields
(`--meta-schema`, `--section-config`). On success prints a confirmation
line with the new ID; pass the global `--json` flag to emit the full
record for scripting.

# Example

  # Create a resource category with just a name
  mr resource-category create --name "Photos"

  # Create with a description and capture the ID via jq
  ID=$(mr resource-category create --name "Scans" --description "scanned documents" --json | jq -r .ID)

  # mr-doctest: create a resource category, assert the returned ID is positive
  mr resource-category create --name "doctest-rc-create-$$-$RANDOM" --json | jq -e '.ID > 0'
