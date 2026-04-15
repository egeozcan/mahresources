---
exitCodes: 0 on success; 1 on any error
relatedCmds: queries list, mrql, search
---

# Long

A Query is a saved, named search definition. Queries store SQL text
(with optional template interpolation) that can be re-executed on
demand against the mahresources database. Each Query has an ID, name,
description, the SQL Text itself, and an optional Template. Queries
are read-only: `run` executes against a read-only database handle and
returns rows as JSON objects.

Use the `query` subcommands to operate on a single query by ID:
`create` to register new SQL, `get` to fetch metadata, `edit-name` /
`edit-description` to update fields, `run` / `run-by-name` to execute,
and `schema` to inspect the available tables and columns when
authoring query text. Use `queries list` to discover existing queries.
