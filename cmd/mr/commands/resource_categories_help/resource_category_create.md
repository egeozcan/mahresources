---
outputShape: Created ResourceCategory object with ID, Name, Description, MetaSchema, AutoDetectRules, sectionConfig, CustomHeader/DetailFooter/Sidebar/Preview/Lightbox/Summary/Avatar/HoverCard/Cell/ListHeader/ListFooter/MRQLResult/CSS, CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: resource-category get, resource-category edit-name, resource-categories list
---

# Long

Create a new resource category. `--name` is required; all other flags
are optional, including a plain `--description`, a `--custom-*` flag for
every template slot, and structural fields (`--meta-schema`,
`--section-config`). Resource categories carry three slots the other
carriers do not: `--custom-preview` (above the built-in preview image),
`--custom-lightbox` (the lightbox details panel), and `--custom-cell`
(an extra column in the resources details table). Run
`mr resource-category create --help` for the full list with a one-line
description of where each renders. `--custom-css`
is injected as a `<style>` block on detail and list pages. On success
prints a confirmation line with the new ID; pass the global `--json`
flag to emit the full record for scripting.

# Example

  # Create a resource category with just a name
  mr resource-category create --name "Photos"

  # Create with a description and capture the ID via jq
  ID=$(mr resource-category create --name "Scans" --description "scanned documents" --json | jq -r .ID)

  # mr-doctest: create a resource category, assert the returned ID is positive
  mr resource-category create --name "doctest-rc-create-$$-$RANDOM" --json | jq -e '.ID > 0'
