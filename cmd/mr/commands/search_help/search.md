---
outputShape: Search response {query (string), total (int), results (array of {id, type, name, score, description, url, extra})}
exitCodes: 0 on success; 1 on any error
relatedCmds: mrql run, resources list, notes list, groups list
---

# Long

Search across resources, notes, and groups using the server's full-text index. Results are ranked by FTS5 score; the response reports the total number of matches so callers can decide whether to broaden the query or page.

Use `--types` to restrict to a comma-separated subset of entity types (e.g. `--types resources,notes`). Use `--limit` to cap the number of rows returned (default 20). The query string supports FTS5 syntax — phrase queries with double-quoted tokens, boolean operators, and prefix matching with `*`.

# Example

  # Simple keyword search across all entities
  mr search "invoice"

  # Restrict to resources only, JSON output
  mr search "invoice" --types resources --json

  # Cap results and pipe into jq to read the total
  mr search "report" --limit 5 --json | jq '.total'

  # mr-doctest: create a uniquely-named group and confirm search finds it by ID
  NAME="doctestsearch$$r$RANDOM"
  GID=$(mr group create --name "$NAME" --json | jq -r '.ID')
  mr search "$NAME" --json | jq -e --argjson g "$GID" '.total >= 1 and ([.results[].id] | any(. == $g))'
