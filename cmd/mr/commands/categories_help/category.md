---
exitCodes: 0 on success; 1 on any error
relatedCmds: categories list, group, groups list
---

# Long

Categories are labels that classify Groups (distinct from ResourceCategory
which labels Resources). A Category has a name, optional description, and
eleven optional presentation fields (CustomHeader, CustomDetailFooter,
CustomSidebar, CustomSummary, CustomAvatar, CustomHoverCard,
CustomOwnEntities, CustomListHeader, CustomListFooter, CustomMRQLResult,
CustomCSS). It also carries a MetaSchema JSON that Groups assigned to this
category inherit for structured metadata, and a SectionConfig JSON that
controls which sections render on their detail pages.

Use the `category` subcommands to operate on a single Category by ID:
fetch it, create a new one, rename or redescribe it, or delete it. Use
`categories list` to discover categories and `categories timeline` to
view creation activity over time.
