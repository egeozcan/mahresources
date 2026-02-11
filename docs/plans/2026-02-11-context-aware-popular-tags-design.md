# Context-Aware Popular Tags on List Pages

## Problem

The popular tags shown at the top of the Resources list page are globally popular — they reflect tag frequency across all resources regardless of active filters. This makes them less useful when the user has narrowed results by content type, date range, owner, or other criteria.

Notes and Groups list pages don't show popular tags at all.

## Design

### Behavior

- Popular tag chips reflect tag frequency among the **currently filtered result set**, not all entities.
- All active filters apply, including already-selected tags. This enables progressive narrowing: "given these results, what other tags are common?"
- Each tag chip shows a count badge, e.g. `photography (42)`.
- Top 20 tags returned, ordered by frequency descending.
- Applies to Resources, Notes, and Groups list pages.

### Backend: Scoped Popular Tag Queries

Each entity gets a popular tags function that accepts the current search query:

```go
func (ctx *MahresourcesContext) GetPopularResourceTags(query *query_models.ResourceSearchQuery) ([]PopularTag, error)
func (ctx *MahresourcesContext) GetPopularNoteTags(query *query_models.NoteSearchQuery) ([]PopularTag, error)
func (ctx *MahresourcesContext) GetPopularGroupTags(query *query_models.GroupSearchQuery) ([]PopularTag, error)
```

Shared return type:

```go
type PopularTag struct {
    Name  string
    Id    uint
    Count int
}
```

Implementation uses a join-based approach (no subqueries) for performance on multi-million row databases. The existing query scopes (`ResourceQuery`, `NoteQuery`, `GroupQuery`) are applied directly to a query that joins through to the tags table:

```go
db := ctx.db.Table("resources")
db = database_scopes.ResourceQuery(query, db, ctx.db)
db = db.
    Joins("INNER JOIN resource_tags rt ON rt.resource_id = resources.id").
    Joins("INNER JOIN tags t ON t.id = rt.tag_id").
    Select("t.id AS id, t.name AS name, COUNT(*) AS count").
    Group("t.id, t.name").
    Order("count DESC").
    Limit(20)
```

This lets the database optimizer apply all WHERE clauses and the tag counting in a single query plan, using existing indexes on the join tables.

### Template Context Providers

- `ResourceListContextProvider`: Pass the decoded `ResourceSearchQuery` to `GetPopularResourceTags(&query)` instead of calling with no arguments.
- `NoteListContextProvider`: Add call to `GetPopularNoteTags(&query)`, pass result as `popularTags` in template context.
- `GroupListContextProvider`: Add call to `GetPopularGroupTags(&query)`, pass result as `popularTags` in template context.

### Templates

- `searchFormResource.tpl`: Update existing popular tags loop to show count badge.
- `tag.tpl` partial: If `count` variable is provided, render it after the tag name.
- `listNotes.tpl`: Add popular tags chip section above the filter form.
- `listGroups.tpl`: Add popular tags chip section above the filter form.

## Files to Change

1. `application_context/resource_bulk_context.go` — Modify `GetPopularResourceTags` signature and implementation
2. `application_context/note_context.go` or similar — Add `GetPopularNoteTags`
3. `application_context/group_context.go` or similar — Add `GetPopularGroupTags`
4. `server/template_handlers/template_context_providers/resource_template_context.go` — Pass query to popular tags call
5. `server/template_handlers/template_context_providers/note_template_context.go` — Add popular tags to context
6. `server/template_handlers/template_context_providers/group_template_context.go` — Add popular tags to context
7. `server/interfaces/` — Update reader interfaces if needed
8. `templates/partials/form/searchFormResource.tpl` — Add count badge
9. `templates/partials/tag.tpl` — Conditional count rendering
10. `templates/listNotes.tpl` — Add popular tags section
11. `templates/listGroups.tpl` — Add popular tags section
