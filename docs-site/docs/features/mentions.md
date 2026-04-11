---
sidebar_position: 18
sidebar_label: "@-Mentions"
---

# @-Mentions

Type `@` in description fields to search and link entities inline. Mentions create relations automatically and render as cards or links when viewing.

## Syntax

Format: `@[type:id:name]` where `type` is the entity type (`group`, `note`, `resource`, `tag`), `id` is the entity ID, and `name` is the display name.

Examples:

```
@[group:42:Acme Corp]
@[tag:7:urgent]
@[resource:100:photo.jpg]
@[note:5:Meeting Notes]
```

## Autocomplete

Typing `@` followed by 2+ characters in a description textarea opens an autocomplete dropdown. Results are grouped by entity type with icons and description previews.

| Key | Action |
|-----|--------|
| Arrow Up / Arrow Down | Navigate results |
| Enter | Insert selected mention |
| Escape | Close dropdown |

Available entity types vary by context:

| Entity | Mentionable Types |
|--------|-------------------|
| Note | resources, groups, tags |
| Group | resources, notes, groups, tags |
| Resource | notes, groups, tags |

## Relation Syncing

When you save an entity, mentions in the description are parsed and synced to relations.

| Entity | Behavior | Details |
|--------|----------|---------|
| Note | Additive | Mentions add relations. Removing a mention does not remove the relation. Parses both description and text block content. |
| Group | Additive | Mentions add relations. Removing a mention does not remove the relation. |
| Resource | Additive | Mentions add relations. Removing a mention does not remove the relation. |

All three entity types use additive syncing: mentions create relations on save, but removing a mention does not remove the relation.

## Rendering

Resource mentions render differently based on position. Other types always render the same way.

- **Resource mentions (standalone)**: alone on a line, rendered as cards with thumbnails.
- **Resource mentions (inline)**: within other text, rendered as compact links with small inline thumbnails.
- **Other types** (groups, notes, tags): always render as badge-style links regardless of position.
