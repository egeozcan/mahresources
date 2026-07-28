# Dashboard Landing Page Design

## Summary

Replace the current `/` -> `/notes` redirect with a modern dashboard page showing recently added entities and an activity timeline.

## Requirements

- Dashboard at `/dashboard`, with `/` redirecting to it
- Show 6 most recent items for: Resources, Notes, Groups, Tags
- Card grid layout with thumbnails (Pinterest/Notion-style)
- Activity timeline showing creates + updates across all entity types (20 items)
- Full-width layout (no sidebar)
- Responsive: 3 columns desktop, 2 tablet, 1 mobile
- Accessible (WCAG compliant, keyboard navigable, proper semantics)
- Performant even with millions of resources

## Architecture

### Approach: Server-side rendered

Fully server-rendered like all other pages. New template + context provider, no new JS framework.

### Routing

- `GET /` -> 301 redirect to `/dashboard` (was `/notes`)
- `GET /dashboard` -> renders `dashboard.tpl` via new context provider

### Context Provider

New file: `server/template_handlers/template_context_providers/dashboard_template_context.go`

Provides template context:
- `recentResources` — `[]models.Resource` (6 items, sorted by created_at DESC)
- `recentNotes` — `[]models.Note` (6 items, sorted by created_at DESC)
- `recentGroups` — `[]models.Group` (6 items, sorted by created_at DESC)
- `recentTags` — `[]models.Tag` (6 items, sorted by created_at DESC)
- `activityFeed` — `[]ActivityEntry` (20 items)

Each entity query is a simple `ORDER BY created_at DESC LIMIT 6` using existing application_context methods.

### Activity Timeline

No new DB table. Computed at query time via UNION query:

```go
type ActivityEntry struct {
    EntityType string    // "resource", "note", "group", "tag"
    EntityID   uint      // ID for linking
    Name       string    // Display name
    Action     string    // "created" or "updated"
    Timestamp  time.Time // The relevant timestamp
}
```

SQL strategy: UNION ALL across entity tables for both created_at and updated_at events (excluding entries where updated_at == created_at to avoid duplicates), ordered by timestamp DESC, limited to 20.

### Template Layout

```
Dashboard Header (full-width, no sidebar)
├── Recent Resources [View All ->]
│   └── 6 cards: thumbnail, filename, file size, date
├── Recent Notes [View All ->]
│   └── 6 cards: note type badge, title, text preview, date
├── Recent Groups [View All ->]
│   └── 6 cards: name, description preview, member count
├── Recent Tags [View All ->]
│   └── 6 tag pills: name, usage count
└── Recent Activity
    └── 20 entries: entity icon, name (linked), action, relative time
```

Card grid: CSS Grid with `auto-fill, minmax(200px, 1fr)` for responsive columns.

### Visual Style

- Uses existing `.card` CSS classes plus dashboard-specific styles
- Teal/cyan accent colors (matching existing theme)
- Resource cards show thumbnails from existing thumbnail system
- Note cards show text preview (first ~100 chars)
- Group cards show member count badges
- Tags rendered as larger pills with usage count
- Activity timeline: vertical list with entity-type color coding

## Error Handling

- If any entity query fails, that section shows "No recent items" — doesn't break the page
- Empty database: friendly empty states like "No resources yet — upload your first file"

## Performance

- 5 total queries: 4 entity queries (LIMIT 6 each) + 1 activity UNION (LIMIT 20)
- All queries use existing indexes on created_at/updated_at
- Resource thumbnails preloaded (no N+1)
- Bounded and predictable query cost regardless of database size

## Testing

E2E tests (`e2e/tests/dashboard.spec.ts`):
1. Dashboard loads at `/dashboard` with all 4 entity sections
2. `/` redirects to `/dashboard`
3. Created items appear in recently added sections
4. Activity timeline shows creation events
5. "View All" links navigate correctly
6. Accessibility (axe-core) compliance

## Accessibility

- Proper heading hierarchy (h2 for section titles)
- Cards are keyboard-navigable links
- `<time>` elements with datetime attributes in timeline
- Section landmark roles with aria-label
