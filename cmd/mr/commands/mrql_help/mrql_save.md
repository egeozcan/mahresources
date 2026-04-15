---
outputShape: Created saved MRQL query object with id (uint), name (string), query (string), description (string), createdAt, updatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: mrql list, mrql run, mrql delete
---

# Long

Save a named MRQL query for later reuse. Takes two positional arguments:
`<name>` (a unique label) and `<query>` (the MRQL text). The optional
`--description` flag attaches a human-readable note. The query text is
validated at save time — malformed MRQL returns HTTP 400 with a parse
error pointing at the offending token, and the record is not persisted.

The created record is returned; capture `.id` from JSON output to run
or delete the query in follow-up commands. Saved queries can be executed
by ID or by name via `mrql run`.

# Example

  # Save a simple named query
  mr mrql save "recent-photos" 'type = resource AND tags = "photo"'

  # Save with a description
  mr mrql save "resources-by-type" 'type = resource GROUP BY contentType COUNT()' --description "Resource count per content type"

  # mr-doctest: save a query and verify the response carries a positive id and the supplied name
  NAME="doctest-mrql-save-$$-$RANDOM"
  mr mrql save "$NAME" 'type = resource' --json | jq -e --arg n "$NAME" '.id > 0 and .name == $n'
