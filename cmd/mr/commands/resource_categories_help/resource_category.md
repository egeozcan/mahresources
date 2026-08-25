---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource-categories list, resource get, resources list
---

# Long

A ResourceCategory is a taxonomy label attached to Resources. It has a
name, optional description, and thirteen optional presentation slots,
including the preview, lightbox and table-cell slots no other carrier has
(see `mr resource-category create --help`), plus a MetaSchema and
SectionConfig used to shape resource detail pages for resources in this
category. Resource categories are distinct from Categories, which
label Groups.

Use the `resource-category` subcommands to operate on a single category
by ID: fetch it, create a new one, rename or redescribe it, or delete
it. Use `resource-categories list` to discover categories matching
filters.
