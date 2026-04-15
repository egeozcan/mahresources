---
title: mr mrql
description: Execute and manage MRQL queries
sidebar_label: mrql
---

# mr mrql

Execute MRQL (Mahresources Query Language) queries and manage saved queries.

Examples:
  mr mrql 'type = resource AND tags = "photo"'
  mr mrql -f query.mrql
  echo 'tags = "photo"' | mr mrql -
  mr mrql --limit 10 --page 2 'type = note'

GROUP BY (aggregated — returns computed rows):
  mr mrql 'type = resource GROUP BY contentType COUNT()'
  mr mrql 'type = resource GROUP BY owner.name COUNT() SUM(fileSize)'

GROUP BY (bucketed — returns grouped entities):
  mr mrql 'type = resource GROUP BY contentType LIMIT 5'
  mr mrql --buckets 10 --page 2 'type = resource GROUP BY contentType LIMIT 5'

Scope (filter to group subtree):
  mr mrql 'type = resource SCOPE 42'
  mr mrql 'type = note SCOPE "My Project" ORDER BY created'
  mr mrql 'type = resource SCOPE 7 GROUP BY contentType COUNT()'

Rendering:
  mr mrql --render 'type = resource AND tags = "photo"'
  The --render flag requests server-side template rendering using CustomMRQLResult
  templates. Results include a renderedHTML field when a template is configured.

## Usage

    mr mrql [query]

Positional arguments:

- `<query>` (optional)


## Examples


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--file` | string | `` | Read query from file |
| `--limit` | int | `0` | Items per bucket for GROUP BY, or total items for regular queries |
| `--buckets` | int | `0` | Groups per page for bucketed GROUP BY queries |
| `--offset` | int | `0` | Bucket offset for cursor-based GROUP BY pagination |
| `--render` | bool | `false` | Request server-side template rendering via CustomMRQLResult |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

