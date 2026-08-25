---
outputShape: Search response {query (string), total (int), totalCapped (bool, present when total is a floor), results (array of {id, type, name, score, description, url, extra})}
exitCodes: 0 on success; 1 on any error
relatedCmds: mrql run, resources list, notes list, groups list
---

# Long

Search across resources, notes, groups, tags, categories, saved queries, saved MRQL queries, relation types, note types and resource categories. Results are ranked by the database's full-text score (bm25 on SQLite, `ts_rank` on PostgreSQL), and the server falls back to a LIKE scan when full-text search is disabled. `total` counts the matches the server collected, which stops at 50, and `totalCapped` is set when it did, so treat the number as a floor. There is no paging; narrow the query instead.

Use `--types` to restrict the search to a comma-separated list of entity types. The names are singular: `resource`, `note`, `group`, `tag`, `category`, `query`, `relationType`, `noteType`, `resourceCategory`, `mrqlQuery`. A name the server does not recognise is dropped, and if none of the names given survives, every type is searched. Use `--limit` to cap the number of rows returned (default 20); the server clamps it to 50.

The query string is not FTS5 syntax. Everything but letters, digits, spaces, underscore and dot is stripped before the search runs, so boolean operators match as ordinary words. A trailing `*` forces a prefix match, a leading `~` (or `~2`, `~3`) runs a fuzzy match at that edit distance, and wrapping the whole query in double quotes or prefixing it with `=` forces an exact match.

# Example

  # Simple keyword search across all entities
  mr search "invoice"

  # Restrict to resources only, JSON output
  mr search "invoice" --types resource --json

  # Cap results and pipe into jq to read the total
  mr search "report" --limit 5 --json | jq '.total'

  # mr-doctest: create a uniquely-named group and confirm search finds it by ID
  NAME="doctestsearch$$r$RANDOM"
  GID=$(mr group create --name "$NAME" --json | jq -r '.ID')
  mr search "$NAME" --json | jq -e --argjson g "$GID" '.total >= 1 and ([.results[].id] | any(. == $g))'
