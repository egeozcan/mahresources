---
outputShape: Created Category object with ID (uint), Name (string), Description (string), MetaSchema, sectionConfig, CustomHeader/DetailFooter/Sidebar/Summary/Avatar/HoverCard/OwnEntities/ListHeader/ListFooter/MRQLResult/CSS, CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: category get, category edit-name, categories list
---

# Long

Create a new Category. `--name` is required; `--description` is optional
free-form text. A `--custom-*` flag exists for every category template
slot -- the detail page header, sidebar and footer, the list card summary,
avatar and hover card, the list page header and footer, the Own Entities
section body, MRQL result cards, and the CSS that styles them. Each takes
an HTML or template string applied to Groups in this category, except
`--custom-css`, which is injected as a `<style>` block on detail and list
pages. Run `mr category create --help` for the full list with a one-line
description of where each renders. `--meta-schema` and
`--section-config` take JSON strings
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
