---
sidebar_position: 1
---

# API Overview

All features are accessible via a REST API. The API supports both JSON and form-encoded requests, with responses available in JSON format.

:::danger No Authentication

Mahresources does not include authentication or authorization. The API is designed for use on private, trusted networks only. **Never expose Mahresources directly to the public internet.**

:::

## Base Path

All API endpoints are prefixed with `/v1`:

```
http://localhost:8181/v1/resources
http://localhost:8181/v1/notes
http://localhost:8181/v1/groups
```

## Dual Response Format

Mahresources supports a dual response format system. The same endpoints can return either HTML (for browser access) or JSON (for API access).

### Getting JSON Responses

**1. Add `.json` suffix to the URL:**

```bash
# HTML response (default)
curl http://localhost:8181/v1/resources

# JSON response
curl http://localhost:8181/v1/resources.json
```

**2. Use the `Accept` header:**

```bash
curl -H "Accept: application/json" http://localhost:8181/v1/resources
```

**3. Add `.body` suffix to get just the HTML body (no layout wrapper):**

```bash
curl http://localhost:8181/v1/resources.body
```

This is useful for embedding partial HTML content or HTMX-style updates.

## Request Content Types

The API accepts requests in two formats:

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
curl http://localhost:8181/v1/resources.json

# Get page 2
curl http://localhost:8181/v1/resources.json?page=2

# Get page 3
curl http://localhost:8181/v1/resources.json?page=3
```

The default page size is 30 results per page. This is not configurable via the API.

## Common Query Parameters

Most list endpoints support these common filtering parameters:

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | integer | Page number for pagination (default: 1) |
| `Name` | string | Filter by name (partial match) |
| `Description` | string | Filter by description (partial match) |
| `CreatedBefore` | string | Filter items created before this date (ISO 8601) |
| `CreatedAfter` | string | Filter items created after this date (ISO 8601) |
| `SortBy` | string[] | Sort order (e.g., `SortBy=Name`, `SortBy=-CreatedAt`) |

### Sorting

Use the `SortBy` parameter to control result ordering:

```bash
# Sort by name ascending
curl "http://localhost:8181/v1/resources.json?SortBy=Name"

# Sort by creation date descending (prefix with -)
curl "http://localhost:8181/v1/resources.json?SortBy=-CreatedAt"

# Multiple sort fields
curl "http://localhost:8181/v1/resources.json?SortBy=Name&SortBy=-CreatedAt"
```

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
| 400 | Bad Request - Invalid parameters |
| 404 | Not Found - Resource doesn't exist |
| 500 | Internal Server Error |

## ID Parameters

For endpoints that operate on a single entity, pass the ID as a query parameter:

```bash
# Get a specific resource
curl http://localhost:8181/v1/resource.json?id=123

# Delete a specific tag
curl -X POST http://localhost:8181/v1/tag/delete?Id=456
```

:::caution Inconsistent ID casing

Some endpoints use `id` (lowercase) and others use `Id` (capitalized). This is a known inconsistency in the API. Check the specific endpoint documentation for the correct casing.

:::

## OpenAPI Specification

Mahresources can generate an OpenAPI 3.0 specification from its route definitions:

```bash
# Generate YAML spec (default)
go run ./cmd/openapi-gen

# Generate with custom output path
go run ./cmd/openapi-gen -output api-spec.yaml

# Generate JSON format
go run ./cmd/openapi-gen -output api-spec.json -format json
```

This generates a complete OpenAPI specification that can be used with tools like Swagger UI, Postman, or code generators.

## API Endpoint Categories

The API is organized into these categories:

- **[Resources](./resources)** - File management (upload, download, metadata)
- **[Notes](./notes)** - Text content and note types
- **[Groups](./groups)** - Hierarchical organization and relations
- **[Tags, Categories, Queries & More](./other-endpoints)** - Tags, Categories, Queries, Search, Logs, Download Queue
