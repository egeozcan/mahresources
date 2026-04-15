---
exitCodes: 0 on success; 1 on any error
relatedCmds: note-types list, note create, notes list
---

# Long

Note Types are typed schemas for Notes. A NoteType defines the shape of
a Note's metadata via a JSON Schema (`MetaSchema`) and may carry custom
rendering bits: `CustomHeader`, `CustomSidebar`, `CustomSummary`,
`CustomAvatar`, `CustomMRQLResult`, and a `SectionConfig` JSON toggle
for which sections appear on note detail pages. Typical examples are
"Meeting Minutes", "Code Review", or "Bug Report".

Use the `note-type` subcommands to operate on a single note type by ID:
fetch it, create a new one, edit it (whole record or scoped name /
description), or delete it. Use `note-types list` to discover the
available note types and feed their IDs into `note create --note-type-id`.
