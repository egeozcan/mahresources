---
sidebar_position: 5
title: Meta Schemas
---

# Meta Schemas

Categories and Resource Categories can define a JSON Schema in their `metaSchema` field. The schema validates the `meta` field of entities in that category and drives structured form generation in the UI.

## How It Works

1. An administrator creates a Category (or Resource Category) with a `metaSchema` field containing a JSON Schema document
2. When creating or editing a Group (or Resource) in that Category, the UI renders form fields matching the schema instead of free-form key-value inputs
3. The schema validates metadata on save

## Which Entity Types Support It

| Entity Type | MetaSchema Field | Validates |
|-------------|-----------------|-----------|
| Category | `metaSchema` | Group `meta` fields |
| Resource Category | `metaSchema` | Resource `meta` fields |
| Note Type | (none) | Not supported |

Note Types do not have a `metaSchema` field. They support custom HTML templates but not schema-driven metadata validation.

## Schema Format

The `metaSchema` field stores a standard JSON Schema document as a string:

```json
{
  "type": "object",
  "properties": {
    "email": {
      "type": "string",
      "format": "email",
      "description": "Primary email address"
    },
    "phone": {
      "type": "string",
      "pattern": "^\\+?[0-9-]+$"
    },
    "birthday": {
      "type": "string",
      "format": "date"
    }
  },
  "required": ["email"]
}
```

## Common Schema Patterns

### Contact Information (Person Category)

```json
{
  "type": "object",
  "properties": {
    "email": {"type": "string", "format": "email"},
    "phone": {"type": "string"},
    "birthday": {"type": "string", "format": "date"},
    "social": {
      "type": "object",
      "properties": {
        "twitter": {"type": "string"},
        "linkedin": {"type": "string"}
      }
    }
  },
  "required": ["email"]
}
```

### Project Tracking (Project Category)

```json
{
  "type": "object",
  "properties": {
    "status": {
      "type": "string",
      "enum": ["planning", "active", "on-hold", "completed"]
    },
    "deadline": {"type": "string", "format": "date"},
    "budget": {"type": "number"},
    "lead": {"type": "string"}
  }
}
```

### Receipt Classification (Receipt Resource Category)

```json
{
  "type": "object",
  "properties": {
    "vendor": {"type": "string"},
    "amount": {"type": "number"},
    "currency": {
      "type": "string",
      "enum": ["USD", "EUR", "GBP"]
    },
    "date": {"type": "string", "format": "date"},
    "category": {
      "type": "string",
      "enum": ["office", "travel", "software", "other"]
    }
  },
  "required": ["vendor", "amount"]
}
```

## Setting a Schema

### Via the UI

1. Navigate to **Categories** (or **Resource Categories**)
2. Create or edit a Category
3. Enter the JSON Schema in the **Meta Schema** field
4. Save

### Via the API

```bash
curl -X POST http://localhost:8181/v1/category \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "Name": "Person",
    "Description": "Individual contacts",
    "MetaSchema": "{\"type\":\"object\",\"properties\":{\"email\":{\"type\":\"string\",\"format\":\"email\"}},\"required\":[\"email\"]}"
  }'
```

## Form Generation

When a Category has a schema defined, the Group create/edit form replaces free-form metadata inputs with structured fields:

- `string` properties render as text inputs
- `string` with `format: "email"` renders as an email input
- `string` with `format: "date"` renders as a date picker
- `string` with `enum` renders as a dropdown select
- `number` properties render as numeric inputs
- `object` properties render as nested fieldsets
- `required` fields are marked as mandatory

Groups without a Category (or with a Category that has no schema) continue to use free-form key-value metadata inputs.
