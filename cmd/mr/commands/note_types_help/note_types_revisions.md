---
exitCodes: 0 on success; 1 on any error
relatedCmds: note-types revisions list, note-type edit, note-type get
---

# Long

Inspect and restore the Template Revisions of a Note Type. Every save that
changes one of its templates, its MetaSchema or its SectionConfig records a
revision, so each subcommand takes the note type ID with `--id` and works
on that note type's history.

The subcommands are `list`, `show`, `diff`, `restore` and `label`. All of
them require an editor or admin account, because history is visible only to those
who may edit the note type.
