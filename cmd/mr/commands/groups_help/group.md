---
exitCodes: 0 on success; 1 on any error
relatedCmds: groups list, resources list, tags list
---

# Long

Groups are hierarchical collections in mahresources. A Group has a name,
description, optional meta JSON, an optional owner (the parent group),
an optional category, and many-to-many links to Resources, Notes, Tags,
and other Groups. The owner relationship forms a tree, so a Group can
also have child groups (descendants whose `OwnerId` points at this one).

Use the `group` subcommands to operate on a single group by ID: fetch
metadata, edit its name/description/meta, walk its ancestor chain or
direct children, clone it, or export/import a self-contained subtree as
a portable tar archive. Use `groups list` to discover groups matching
filters, or the bulk subcommands under `groups` to mutate many at once.
