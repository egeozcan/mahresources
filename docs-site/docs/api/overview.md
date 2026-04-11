---
sidebar_position: 1
---

# API Overview

All features are accessible via a REST API. Most write endpoints accept either JSON or form-encoded requests, and most `/v1` endpoints return JSON.

:::danger No Authentication

There is no authentication or authorization. The API is designed for use on private, trusted networks only. **Never expose the server directly to the public internet.**

:::

## Base Path

All API endpoints are prefixed with `/v1`:

```
http://localhost:8181/v1/resources
http://localhost:8181/v1/notes
http://localhost:8181/v1/groups
```

## Response Formats

Most API endpoints under `/v1/` return JSON, with a few important exceptions:

- `GET /v1/resource/view` returns a `302` redirect to the stored file
- `GET /v1/download/events` and `GET /v1/jobs/events` return Server-Sent Events streams
- `GET /v1/plugins/{pluginName}/block/render` returns an HTML fragment

Template routes (without the `/v1/` prefix) return HTML by default and support two suffixes:

| Suffix | Effect |
|--------|--------|
| `.json` | Returns JSON instead of HTML |
| `.body` | Returns the HTML body without the layout wrapper |

```bash
# API route: always JSON
curl http://localhost:8181/v1/resources

# Template route: HTML by default
curl http://localhost:8181/resources

# Template route: JSON via suffix
curl http://localhost:8181/resources.json

# Template route: body only (useful for HTMX-style updates)
curl http://localhost:8181/resources.body
```

:::note
The `.json` and `.body` suffixes do **not** work on `/v1/` API routes. Use `/v1/` paths directly for JSON responses.
:::

## Request Content Types

Many write endpoints accept requests in two formats:

- **JSON**: `Content-Type: application/json`
- **Form data**: `Content-Type: application/x-www-form-urlencoded` or `multipart/form-data`

```bash
# JSON request
curl -X POST http://localhost:8181/v1/tag \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{"Name": "example-tag"}'

# Form request
curl -X POST http://localhost:8181/v1/tag \
  -H "Accept: application/json" \
  -d "Name=example-tag"
```

## Pagination

List endpoints support pagination using the `page` query parameter:

```bash
# Get page 1 (default)
curl http://localhost:8181/v1/resources

# Get page 2
curl http://localhost:8181/v1/resources?page=2

# Get page 3
curl http://localhost:8181/v1/resources?page=3
```

The default page size depends on the endpoint. The `MaxResults` parameter on Resource queries can override the page size.

## Common Query Parameters

Most list endpoints support these common filtering parameters:

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | integer | Page number for pagination (default: 1) |
| `Name` | string | Filter by name (partial match) |
| `Description` | string | Filter by description (partial match) |
| `CreatedBefore` | string | Filter items created before this date (ISO 8601) |
| `CreatedAfter` | string | Filter items created after this date (ISO 8601) |
| `SortBy` | string[] | Sort order (e.g., `SortBy=name`, `SortBy=created_at desc`) |

### Sorting

Use the `SortBy` parameter to control result ordering. Append `asc` or `desc` (space-separated) for direction. Default is ascending.

```bash
# Sort by name ascending
curl "http://localhost:8181/v1/resources?SortBy=name"

# Sort by creation date descending
curl -G http://localhost:8181/v1/resources \
  --data-urlencode "SortBy=created_at desc"

# Multiple sort fields
curl -G http://localhost:8181/v1/resources \
  --data-urlencode "SortBy=name" \
  --data-urlencode "SortBy=created_at desc"

# Sort by metadata field
curl -G http://localhost:8181/v1/resources \
  --data-urlencode "SortBy=meta->>'priority' desc"
```

Sort columns are validated against: `^(meta->>?'[a-z_]+'|[a-z_]+)(\s(desc|asc))?$`

## Error Responses

When an error occurs, the API returns an appropriate HTTP status code with an error message:

```json
{
  "error": "resource not found"
}
```

Common HTTP status codes:

| Status | Description |
|--------|-------------|
| 200 | Success |
| 201 | Created (block creation) |
| 202 | Accepted (async plugin actions, download submissions) |
| 204 | No Content (block deletion, reorder, rebalance) |
| 400 | Bad Request - Invalid parameters |
| 404 | Not Found - Entity does not exist |
| 409 | Conflict - Duplicate resource upload (returns `existingResourceId`) |
| 500 | Internal Server Error |

## ID Parameters

For endpoints that operate on a single entity, pass the ID as a query parameter:

```bash
# Get a specific resource
curl http://localhost:8181/v1/resource?id=123

# Delete a specific tag
curl -X POST http://localhost:8181/v1/tag/delete?Id=456
```

:::caution Inconsistent ID casing

Some endpoints use `id` (lowercase) and others use `Id` (capitalized). This is a known inconsistency in the API. Check the specific endpoint documentation for the correct casing.

:::

## OpenAPI Specification

An OpenAPI 3.0 specification can be generated from the routes registered with the OpenAPI generator:

```bash
# Generate YAML spec (default)
go run ./cmd/openapi-gen

# Generate with custom output path
go run ./cmd/openapi-gen -output api-spec.yaml

# Generate JSON format
go run ./cmd/openapi-gen -output api-spec.json -format json
```

### Validate a Spec

Validate a generated OpenAPI spec against the OpenAPI 3.0 schema:

```bash
go run ./cmd/openapi-gen/validate.go openapi.yaml
```

If validation succeeds, the command prints "Valid OpenAPI 3.0 spec" and exits with code 0. On failure, it prints the validation error and exits with code 1.

The generated spec works with Swagger UI, Postman, or code generators.

:::note
The generated spec currently focuses on the core documented API surface. Some newer aliases or convenience endpoints may exist in the server before they are added to the generated spec.
:::

## API Endpoint Categories

The API is organized into these categories:

- **[Resources](./resources)** - File management (upload, download, metadata)
- **[Notes](./notes)** - Text content and note types
- **[Groups](./groups)** - Hierarchical organization and relations
- **[Plugins](./plugins)** - Plugin management, actions, and job monitoring
- **[Tags, Categories, Queries & More](./other-endpoints)** - Tags, Categories, Queries, MRQL, Series, Search, Logs, Download Queue, Admin Stats, Timeline
