---
outputShape: ResourceCategory object with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: resource-category create, resource-category edit-name, resource-categories list
---

# Long

Get a resource category by ID and print its fields. The server has no
single-resource-category GET endpoint, so the CLI fetches the first page
of the list and filters in-process. Only the first 50 resource
categories are searched: beyond that, a category that exists is reported
as `resource category <id> not found`, so use
`resource-categories list --name <substring>` to locate it instead.

Output is a key/value table by default; pass the global `--json` flag to
emit those fields as JSON for scripting. The template slots, MetaSchema
and SectionConfig are not included; read them from
`resource-categories list --json`.

# Example

  # Get a resource category by ID (table output)
  mr resource-category get 42

  # Get as JSON and extract the name with jq
  mr resource-category get 42 --json | jq -r .Name

  # mr-doctest: create a resource category and verify it is retrievable
  ID=$(mr resource-category create --name "doctest-rc-get-$$-$RANDOM" --json | jq -r '.ID')
  mr resource-category get $ID --json | jq -e '.ID > 0 and (.Name | length) > 0'
