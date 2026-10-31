---
exitCodes: 0 on success; 1 on any error
relatedCmds: mr categories revisions list, category edit, category get
---

# Long

Inspect and restore the Template Revisions of a Category. Every save that
changes one of its templates, its MetaSchema or its SectionConfig records a
revision, so each subcommand takes the category ID with `--id` and works
on that category's history.

The subcommands are `list`, `show`, `diff`, `restore` and `label`. All of
them require an admin account, because history is visible only to those
who may edit the category.
