---
exitCodes: 0 on success; 1 on any error
relatedCmds: tags list, tags merge, tags delete
---

# Long

Tags are lightweight labels attached to Resources, Notes, and Groups.
A Tag has a name and optional description; the name is the user-visible
handle. Tags are the primary way to categorize content across entity
types and are commonly used as filter selectors in list and timeline
commands.

Use the `tag` subcommands to operate on a single tag by ID: fetch it,
create a new one, rename or redescribe it, or delete it. Use
`tags list` to discover tags and `tags merge` to fold a tag's
relationships into another.
