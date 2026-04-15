---
exitCodes: 0 on success; 1 on any error
relatedCmds: notes list, note-types list, groups list
---

# Long

Notes are free-form text records in mahresources. A Note has a name,
description, optional meta JSON, an optional owner group, an optional
note type (template), optional start/end dates, and many-to-many links
to Tags, Resources, and Groups. A Note may also carry a share token
that exposes it at `/s/<token>` for read-only public access.

Use the `note` subcommands to operate on a single note by ID: fetch the
full record, create a new one, edit the name/description/meta fields,
toggle sharing, or delete it. Use `notes list` to discover notes
matching filters, or the bulk subcommands under `notes` to mutate many
at once.
