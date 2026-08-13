# MRQL Recipes

Task-shaped examples. Each is a complete shell command against a running mahresources
server. Set `MAHRESOURCES_URL` (or pass `--server`) first, and `MR_TOKEN` if the server
runs with `-auth`.

## Finding things

**Recently added files of a kind**

```bash
mr mrql 'type = resource AND contentType ~ "image" AND created > -7d ORDER BY created DESC LIMIT 50'
```

**Big files, largest first**

```bash
mr mrql 'type = resource AND fileSize > 100mb ORDER BY fileSize DESC LIMIT 20'
```

**Files in a project group and everything beneath it**

```bash
# mr-doctest: skip, needs a group named "Project Alpha"
mr mrql 'type = resource SCOPE "Project Alpha" ORDER BY created DESC LIMIT 100'
```

`SCOPE` takes a group ID or a name. A name that matches several groups errors and lists
the matches, so resolve it once and use the ID in scripts. An unknown name or ID is an
error too (HTTP 404) rather than an empty result, so a typo fails loudly; `SCOPE 0` is the
one number that means "no scope". Under `-auth`, a group-limited principal's own subtree
always wins and the `SCOPE` written in the query is ignored.

**Anything mentioning a phrase, across all three entity types**

```bash
mr mrql 'TEXT ~ "quarterly earnings" LIMIT 30'
```

**Most relevant notes for a phrase**

```bash
mr mrql 'type = note AND TEXT ~ "kubernetes migration" ORDER BY RANK LIMIT 10'
```

Requires full-text search enabled on the server.

**A random sample to spot-check**

```bash
mr mrql 'type = resource AND tags IS EMPTY ORDER BY RANDOM() LIMIT 20'
```

## Housekeeping and audits

**Untagged resources**

```bash
mr mrql 'type = resource AND tags.count = 0 ORDER BY created DESC LIMIT 100'
```

**Resources with no description**

```bash
mr mrql 'type = resource AND description IS EMPTY LIMIT 100'
```

**Duplicate content, by hash**

```bash
mr mrql 'type = resource GROUP BY hash COUNT() HAVING COUNT() > 1 ORDER BY count DESC'
```

**Near-duplicate images of a known resource**

```bash
mr mrql 'type = resource AND SIMILAR TO resource(1234) WITHIN 2 ORDER BY distance ASC LIMIT 20'
```

**Groups that have grown large**

```bash
mr mrql 'type = group AND resources.count >= 100 ORDER BY resources.count DESC LIMIT 25'
```

**Orphans: resources with no owning group**

```bash
mr mrql 'type = resource AND owner IS NULL LIMIT 100'
```

**Stale tags: tags whose newest resource is over a year old**

```bash
mr mrql 'type = resource GROUP BY tags COUNT() HAVING MAX(created) < -1y ORDER BY count DESC'
```

## Counting and reporting

**Storage by content type**

```bash
mr mrql 'type = resource GROUP BY contentType COUNT() SUM(fileSize) ORDER BY sum_fileSize DESC'
```

**Notes created per month**

```bash
mr mrql 'type = note GROUP BY created.month COUNT() ORDER BY created.month ASC'
```

**Uploads per week for one group's subtree**

```bash
# mr-doctest: skip, needs group 42 to exist
mr mrql 'type = resource SCOPE 42 GROUP BY created.week COUNT() ORDER BY created.week ASC'
```

**A sample of items per content type** (bucketed mode, where `LIMIT` is per bucket)

```bash
mr mrql --buckets 10 'type = resource GROUP BY contentType LIMIT 5'
```

Page further buckets with `--offset`, reading `nextOffset` from the previous response.

## Metadata

**Numeric metadata comparison**

```bash
mr mrql 'type = resource AND meta.rating >= 4 ORDER BY meta.rating DESC LIMIT 50'
```

**Records missing a metadata key**

```bash
mr mrql 'type = resource AND meta.license IS NULL LIMIT 100'
```

**Distribution of a metadata value**

```bash
mr mrql 'type = resource GROUP BY meta.camera_model COUNT() ORDER BY count DESC'
```

## Hierarchy

**Everything anywhere under an "Archive" group, including items directly in it**

```bash
mr mrql 'type = resource AND (owner.name = "Archive" OR ancestors.name = "Archive") LIMIT 100'
```

`ancestors.` is strict, excluding the base group, which is why the `owner` branch is
there.

**Groups whose parent is a specific group**

```bash
mr mrql 'type = group AND parent.name = "Clients" ORDER BY name ASC'
```

**Resources under any group tagged for a region**

```bash
mr mrql 'type = resource AND ancestors.meta.region = "eu" LIMIT 100'
```

**Groups containing work in progress somewhere below them**

```bash
mr mrql 'type = group AND descendants.tags = "wip" ORDER BY name ASC'
```

## Parameterized and saved queries

**Bind values instead of interpolating them**

```bash
mr mrql 'type = resource AND tags = $tag AND created > $since' \
  --param tag=photo --param since=-7d
```

**Save it, then run it by name**

```bash
# mr-doctest: a rerun against the same server hits the unique-name error, tolerate=/already exists/
mr mrql save recent-tagged \
  'type = resource AND tags = $tag AND created > $since' \
  --description "Recently added resources with a given tag"

mr mrql run recent-tagged --param tag=photo --param since=-30d --json
```

Saved-query names are unique: saving over an existing name returns HTTP 400 rather than
replacing it.

**Find a saved query's ID, then delete it** (`delete` takes only a numeric ID)

```bash
mr mrql save scratch-query 'type = resource LIMIT 1' >/dev/null
ID=$(mr mrql list --json | jq -r '.[] | select(.name == "scratch-query") | .id')
mr mrql delete "$ID"
```

## Inspecting before executing

**See the SQL a query would run**

```bash
mr mrql explain 'type = resource AND fileSize > 1mb'
mr mrql explain 'type = note AND name ~ $needle' --param needle=meeting
mr mrql explain --saved recent-tagged --param tag=photo --param since=-7d --json
```

`explain` never executes the query. It resolves `SCOPE`, applies the default `LIMIT` (and
says so on stderr), and includes any RBAC forced scoping. One statement for a flat or
aggregated query; three for cross-entity; a bucketed `GROUP BY` shows the key-discovery
query plus a fan-out note.

**Read the parameterized SQL and its bound variables**

```bash
mr mrql explain 'type = resource' --json | jq '.statements[] | {sql, vars}'
```

## Getting data out

**CSV to a file**

```bash
mr mrql export 'type = resource AND created > -30d' --format csv -o recent.csv
```

CSV requires a single entity type. Flat queries emit a fixed scalar column set per entity
(with `meta` as a JSON string); aggregated queries emit group keys followed by aggregate
aliases; bucketed queries prepend the bucket-key columns.

**JSON for cross-entity results**

```bash
mr mrql export 'name ~ "budget*"' --format json -o budget.json
```

**Export a saved query with parameters**

```bash
mr mrql export --saved recent-tagged --format json --param tag=photo --param since=-7d
```

## Composing with the shell

**IDs into another command**

```bash
mr mrql 'type = resource AND tags = "to-review"' --quiet | while read -r id; do
  mr resource get "$id" --json
done
```

**Count without printing rows**

```bash
mr mrql 'type = resource AND created > -1d GROUP BY contentType COUNT()' --json \
  | jq '[.rows[]?.count] | add // 0'
```

**Query text held in a file** (useful for long queries, and for keeping them in version
control)

```bash
cat > /tmp/audit.mrql <<'EOF'
type = resource
  AND tags.count = 0
  AND fileSize > 10mb
ORDER BY fileSize DESC
LIMIT 200
EOF

mr mrql -f /tmp/audit.mrql --json
```

**Query built by another process, via stdin**

```bash
echo 'type = note AND updated > -1d' | mr mrql - --json
```

## Filtering list commands

When the goal is "the normal list page, narrowed", `--mrql` on a list command is shorter
than a full query and composes with the command's other flags and paging.

```bash
mr resources list --mrql 'tags = "vacation" AND created > -30d'
mr notes list --mrql 'tags = "todo" AND updated > -7d'
mr groups list --mrql 'resources.count = 0'
```

That grammar is the filter expression only: no `ORDER BY`, `LIMIT`, `OFFSET`,
`GROUP BY`, `SCOPE`, `$name` parameters, or explicit `type`. Use the full `mr mrql`
command when any of those are needed.
