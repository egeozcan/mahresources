# Dashboard Landing Page Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the `/notes` redirect with a modern dashboard showing recently added Resources, Notes, Groups, Tags and an activity timeline.

**Architecture:** Server-side rendered dashboard using a new Pongo2 template + context provider. Reuses existing `application_context` query methods for entity data. Activity feed uses a raw UNION SQL query across entity tables. No new JS framework or API endpoints needed.

**Tech Stack:** Go (Gorilla Mux, GORM, Pongo2), Tailwind CSS, existing card components

---

### Task 1: Add `GetRecentActivity` method to application_context

**Files:**
- Create: `application_context/dashboard_context.go`

**Step 1: Create the activity entry struct and query method**

Create `application_context/dashboard_context.go`:

```go
package application_context

import (
	"time"
)

// ActivityEntry represents a single item in the dashboard activity feed.
type ActivityEntry struct {
	EntityType string
	EntityID   uint
	Name       string
	Action     string
	Timestamp  time.Time
}

// GetRecentActivity returns a mixed timeline of recently created and updated entities.
func (ctx *MahresourcesContext) GetRecentActivity(limit int) ([]ActivityEntry, error) {
	var entries []ActivityEntry

	query := `
		SELECT 'resource' AS entity_type, id AS entity_id, name, 'created' AS action, created_at AS timestamp FROM resources
		UNION ALL
		SELECT 'resource', id, name, 'updated', updated_at FROM resources WHERE updated_at != created_at
		UNION ALL
		SELECT 'note' AS entity_type, id, name, 'created', created_at FROM notes
		UNION ALL
		SELECT 'note', id, name, 'updated', updated_at FROM notes WHERE updated_at != created_at
		UNION ALL
		SELECT 'group' AS entity_type, id, name, 'created', created_at FROM "groups"
		UNION ALL
		SELECT 'group', id, name, 'updated', updated_at FROM "groups" WHERE updated_at != created_at
		UNION ALL
		SELECT 'tag' AS entity_type, id, name, 'created', created_at FROM tags
		UNION ALL
		SELECT 'tag', id, name, 'updated', updated_at FROM tags WHERE updated_at != created_at
		ORDER BY timestamp DESC
		LIMIT ?
	`

	err := ctx.db.Raw(query, limit).Scan(&entries).Error
	return entries, err
}
```

**Step 2: Run Go tests to verify compilation**

Run: `go build --tags 'json1 fts5' ./...`
Expected: compiles without errors

**Step 3: Commit**

```bash
git add application_context/dashboard_context.go
git commit -m "feat(dashboard): add GetRecentActivity query method"
```

---

### Task 2: Add `timeago` template filter

We need a "time ago" filter to display relative timestamps (e.g., "2 minutes ago") in the activity timeline.

**Files:**
- Create: `server/template_handlers/template_filters/timeago_filter.go`
- Modify: `server/template_handlers/template_filters/template_filters.go`

**Step 1: Create the timeago filter**

Create `server/template_handlers/template_filters/timeago_filter.go`:

```go
package template_filters

import (
	"fmt"
	"time"

	"github.com/flosch/pongo2/v4"
)

func timeagoFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	t, ok := in.Interface().(time.Time)
	if !ok {
		return pongo2.AsValue(""), nil
	}

	duration := time.Since(t)

	switch {
	case duration < time.Minute:
		return pongo2.AsValue("just now"), nil
	case duration < time.Hour:
		mins := int(duration.Minutes())
		if mins == 1 {
			return pongo2.AsValue("1 minute ago"), nil
		}
		return pongo2.AsValue(fmt.Sprintf("%d minutes ago", mins)), nil
	case duration < 24*time.Hour:
		hours := int(duration.Hours())
		if hours == 1 {
			return pongo2.AsValue("1 hour ago"), nil
		}
		return pongo2.AsValue(fmt.Sprintf("%d hours ago", hours)), nil
	case duration < 30*24*time.Hour:
		days := int(duration.Hours() / 24)
		if days == 1 {
			return pongo2.AsValue("1 day ago"), nil
		}
		return pongo2.AsValue(fmt.Sprintf("%d days ago", days)), nil
	default:
		return pongo2.AsValue(t.Format("2006-01-02")), nil
	}
}
```

**Step 2: Register the filter**

Modify `server/template_handlers/template_filters/template_filters.go`, add after the `lookupErr` block (before the closing `}`):

```go
	timeagoErr := pongo2.RegisterFilter("timeago", timeagoFilter)

	if timeagoErr != nil {
		fmt.Println("error when registering timeago filter", timeagoErr)
	}
```

**Step 3: Verify compilation**

Run: `go build --tags 'json1 fts5' ./...`
Expected: compiles without errors

**Step 4: Commit**

```bash
git add server/template_handlers/template_filters/timeago_filter.go server/template_handlers/template_filters/template_filters.go
git commit -m "feat(dashboard): add timeago template filter for relative timestamps"
```

---

### Task 3: Create dashboard context provider

**Files:**
- Create: `server/template_handlers/template_context_providers/dashboard_template_context.go`

**Step 1: Create the dashboard context provider**

Create `server/template_handlers/template_context_providers/dashboard_template_context.go`:

```go
package template_context_providers

import (
	"net/http"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/models/query_models"
)

const dashboardItemsPerSection = 6
const dashboardActivityLimit = 20

func DashboardContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := staticTemplateCtx(request)

		recentResourcesQuery := &query_models.ResourceSearchQuery{
			SortBy: []string{"created_at desc"},
		}
		recentResources, _ := context.GetResources(0, dashboardItemsPerSection, recentResourcesQuery)

		recentNotesQuery := &query_models.NoteQuery{
			SortBy: []string{"created_at desc"},
		}
		recentNotes, _ := context.GetNotes(0, dashboardItemsPerSection, recentNotesQuery)

		recentGroupsQuery := &query_models.GroupQuery{
			SortBy: []string{"created_at desc"},
		}
		recentGroups, _ := context.GetGroups(0, dashboardItemsPerSection, recentGroupsQuery)

		recentTagsQuery := &query_models.TagQuery{
			SortBy: []string{"created_at desc"},
		}
		recentTags, _ := context.GetTags(0, dashboardItemsPerSection, recentTagsQuery)

		activityFeed, _ := context.GetRecentActivity(dashboardActivityLimit)

		return pongo2.Context{
			"pageTitle":       "Dashboard",
			"recentResources": recentResources,
			"recentNotes":     recentNotes,
			"recentGroups":    recentGroups,
			"recentTags":      recentTags,
			"activityFeed":    activityFeed,
		}.Update(baseContext)
	}
}
```

**Step 2: Verify compilation**

Run: `go build --tags 'json1 fts5' ./...`
Expected: compiles without errors

**Step 3: Commit**

```bash
git add server/template_handlers/template_context_providers/dashboard_template_context.go
git commit -m "feat(dashboard): add dashboard context provider"
```

---

### Task 4: Create the dashboard template

**Files:**
- Create: `templates/dashboard.tpl`

**Step 1: Create the dashboard template**

Create `templates/dashboard.tpl`:

```django
{% extends "/layouts/base.tpl" %}

{% block body %}
<div class="dashboard">
    {# Recent Resources #}
    <section class="dashboard-section" aria-label="Recent resources">
        <header class="dashboard-section-header">
            <h2 class="dashboard-section-title">Recent Resources</h2>
            <a href="/resources" class="dashboard-view-all">View All &rarr;</a>
        </header>
        {% if recentResources %}
        <div class="dashboard-grid">
            {% for entity in recentResources %}
                {% include "partials/resource.tpl" %}
            {% endfor %}
        </div>
        {% else %}
        <p class="dashboard-empty">No resources yet &mdash; <a href="/resource/new">upload your first file</a>.</p>
        {% endif %}
    </section>

    {# Recent Notes #}
    <section class="dashboard-section" aria-label="Recent notes">
        <header class="dashboard-section-header">
            <h2 class="dashboard-section-title">Recent Notes</h2>
            <a href="/notes" class="dashboard-view-all">View All &rarr;</a>
        </header>
        {% if recentNotes %}
        <div class="dashboard-grid">
            {% for entity in recentNotes %}
                {% include "partials/note.tpl" %}
            {% endfor %}
        </div>
        {% else %}
        <p class="dashboard-empty">No notes yet &mdash; <a href="/note/new">create your first note</a>.</p>
        {% endif %}
    </section>

    {# Recent Groups #}
    <section class="dashboard-section" aria-label="Recent groups">
        <header class="dashboard-section-header">
            <h2 class="dashboard-section-title">Recent Groups</h2>
            <a href="/groups" class="dashboard-view-all">View All &rarr;</a>
        </header>
        {% if recentGroups %}
        <div class="dashboard-grid">
            {% for entity in recentGroups %}
                {% include "partials/group.tpl" %}
            {% endfor %}
        </div>
        {% else %}
        <p class="dashboard-empty">No groups yet &mdash; <a href="/group/new">create your first group</a>.</p>
        {% endif %}
    </section>

    {# Recent Tags #}
    <section class="dashboard-section" aria-label="Recent tags">
        <header class="dashboard-section-header">
            <h2 class="dashboard-section-title">Recent Tags</h2>
            <a href="/tags" class="dashboard-view-all">View All &rarr;</a>
        </header>
        {% if recentTags %}
        <div class="dashboard-tags">
            {% for tag in recentTags %}
                <a href="/tag?id={{ tag.ID }}" class="dashboard-tag-pill">
                    {{ tag.Name }}
                </a>
            {% endfor %}
        </div>
        {% else %}
        <p class="dashboard-empty">No tags yet &mdash; <a href="/tag/new">create your first tag</a>.</p>
        {% endif %}
    </section>

    {# Activity Timeline #}
    <section class="dashboard-section" aria-label="Recent activity">
        <header class="dashboard-section-header">
            <h2 class="dashboard-section-title">Recent Activity</h2>
        </header>
        {% if activityFeed %}
        <div class="dashboard-activity">
            {% for entry in activityFeed %}
            <div class="dashboard-activity-item">
                <span class="dashboard-activity-dot dashboard-activity-dot--{{ entry.EntityType }}"></span>
                <span class="dashboard-activity-type">{{ entry.EntityType }}</span>
                {% if entry.EntityType == "resource" %}
                <a href="/resource?id={{ entry.EntityID }}" class="dashboard-activity-name">{{ entry.Name }}</a>
                {% elif entry.EntityType == "note" %}
                <a href="/note?id={{ entry.EntityID }}" class="dashboard-activity-name">{{ entry.Name }}</a>
                {% elif entry.EntityType == "group" %}
                <a href="/group?id={{ entry.EntityID }}" class="dashboard-activity-name">{{ entry.Name }}</a>
                {% elif entry.EntityType == "tag" %}
                <a href="/tag?id={{ entry.EntityID }}" class="dashboard-activity-name">{{ entry.Name }}</a>
                {% endif %}
                <span class="dashboard-activity-action">{{ entry.Action }}</span>
                <time class="dashboard-activity-time" datetime="{{ entry.Timestamp|date:"2006-01-02T15:04:05Z" }}">
                    {{ entry.Timestamp|timeago }}
                </time>
            </div>
            {% endfor %}
        </div>
        {% else %}
        <p class="dashboard-empty">No activity yet. Start by creating some content!</p>
        {% endif %}
    </section>
</div>
{% endblock %}
```

**Step 2: Commit**

```bash
git add templates/dashboard.tpl
git commit -m "feat(dashboard): add dashboard template with entity sections and activity timeline"
```

---

### Task 5: Add dashboard CSS styles

**Files:**
- Modify: `public/index.css`

**Step 1: Add dashboard styles**

Append the following CSS to the end of `public/index.css` (before any closing comments if present):

```css
/* ── Dashboard ────────────────────────────────────────────────── */

.dashboard {
    max-width: 1400px;
    margin: 0 auto;
    display: flex;
    flex-direction: column;
    gap: 2.5rem;
}

.dashboard-section {
    display: flex;
    flex-direction: column;
    gap: 1rem;
}

.dashboard-section-header {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    border-bottom: 1px solid var(--color-border, rgba(0, 0, 0, 0.08));
    padding-bottom: 0.5rem;
}

.dashboard-section-title {
    font-size: 1.25rem;
    font-weight: 600;
    color: var(--color-text, #2d3748);
    margin: 0;
}

.dashboard-view-all {
    font-size: 0.875rem;
    font-weight: 500;
    color: #0d9488;
    text-decoration: none;
    transition: color var(--transition-fast, 120ms) ease;
}

.dashboard-view-all:hover {
    color: #0f766e;
}

.dashboard-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
    gap: 1rem;
}

.dashboard-tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
}

.dashboard-tag-pill {
    display: inline-flex;
    align-items: center;
    padding: 0.5rem 1rem;
    background: var(--color-accent-bg, #e8fafa);
    color: #0d9488;
    border: 1px solid rgba(13, 148, 136, 0.2);
    border-radius: 999px;
    font-size: 0.875rem;
    font-weight: 500;
    text-decoration: none;
    transition: background var(--transition-fast, 120ms) ease,
                border-color var(--transition-fast, 120ms) ease;
}

.dashboard-tag-pill:hover {
    background: #c5f0f0;
    border-color: rgba(13, 148, 136, 0.4);
}

.dashboard-empty {
    color: var(--color-text-muted, #64748b);
    font-size: 0.9375rem;
    padding: 2rem;
    text-align: center;
    background: var(--color-surface, #f8fafc);
    border-radius: var(--radius-md, 8px);
    border: 1px dashed var(--color-border, rgba(0, 0, 0, 0.08));
}

.dashboard-empty a {
    color: #0d9488;
    text-decoration: underline;
}

/* Activity Timeline */

.dashboard-activity {
    display: flex;
    flex-direction: column;
    gap: 0;
    border: 1px solid var(--color-border, rgba(0, 0, 0, 0.08));
    border-radius: var(--radius-md, 8px);
    overflow: hidden;
}

.dashboard-activity-item {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid var(--color-border, rgba(0, 0, 0, 0.06));
    font-size: 0.875rem;
    transition: background var(--transition-fast, 120ms) ease;
}

.dashboard-activity-item:last-child {
    border-bottom: none;
}

.dashboard-activity-item:hover {
    background: var(--color-surface, #f8fafc);
}

.dashboard-activity-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex-shrink: 0;
}

.dashboard-activity-dot--resource { background: #3b82f6; }
.dashboard-activity-dot--note     { background: #8b5cf6; }
.dashboard-activity-dot--group    { background: #f59e0b; }
.dashboard-activity-dot--tag      { background: #10b981; }

.dashboard-activity-type {
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--color-text-muted, #64748b);
    min-width: 5rem;
}

.dashboard-activity-name {
    color: var(--color-text, #2d3748);
    text-decoration: none;
    font-weight: 500;
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.dashboard-activity-name:hover {
    color: #0d9488;
}

.dashboard-activity-action {
    color: var(--color-text-muted, #64748b);
    font-size: 0.8125rem;
}

.dashboard-activity-time {
    color: var(--color-text-muted, #64748b);
    font-size: 0.8125rem;
    flex-shrink: 0;
    min-width: 6rem;
    text-align: right;
}

/* Dashboard overrides: full-width content (no sidebar) */
.dashboard-page .content {
    grid-template-columns: minmax(0, 1fr);
}

.dashboard-page .sidebar {
    display: none;
}

@media (max-width: 640px) {
    .dashboard-grid {
        grid-template-columns: 1fr;
    }

    .dashboard-activity-type {
        display: none;
    }

    .dashboard-activity-time {
        min-width: auto;
    }
}
```

**Step 2: Commit**

```bash
git add public/index.css
git commit -m "feat(dashboard): add dashboard CSS styles"
```

---

### Task 6: Wire up routes and navigation

**Files:**
- Modify: `server/routes.go` (add route to templates map, change redirect)
- Modify: `server/template_handlers/template_context_providers/static_template_context.go` (add Dashboard to menu)

**Step 1: Add dashboard to the templates map**

In `server/routes.go`, add to the `templates` map (around line 20, with the other routes):

```go
"/dashboard": {template_context_providers.DashboardContextProvider, "dashboard.tpl", http.MethodGet},
```

**Step 2: Change the `/` redirect from `/notes` to `/dashboard`**

In `server/routes.go` line 119, change:

```go
http.Redirect(writer, request, "/notes", http.StatusMovedPermanently)
```

to:

```go
http.Redirect(writer, request, "/dashboard", http.StatusMovedPermanently)
```

**Step 3: Add Dashboard to the navigation menu**

In `server/template_handlers/template_context_providers/static_template_context.go`, add Dashboard as the first menu entry (before Notes, around line 39):

```go
"menu": []template_entities.Entry{
    {
        Name: "Dashboard",
        Url:  "/dashboard",
    },
    {
        Name: "Notes",
        Url:  "/notes",
    },
    // ... rest unchanged
```

**Step 4: Update the dashboard template to use a body class for full-width layout**

The dashboard needs full-width content (no sidebar). Looking at how `base.tpl` works, the simplest approach is to pass a context variable and use it in the template. Actually, since the dashboard template extends `base.tpl` and doesn't define a `{% block sidebar %}`, the sidebar will be empty. But the CSS grid still reserves 400px for it.

We need to handle this. The cleanest approach: add a `dashboardPage` context variable and apply the `dashboard-page` body class conditionally.

In `templates/dashboard.tpl`, change the extends block to add a head block:

```django
{% block head %}
<style>.content { grid-template-columns: minmax(0, 1fr) !important; } .sidebar { display: none !important; }</style>
{% endblock %}
```

Actually, cleaner: just add a `noDashboardSidebar` variable from the context provider. But even simpler — we can add the override style inline in the head block of the dashboard template. This avoids modifying the base template.

**Step 5: Verify build**

Run: `go build --tags 'json1 fts5' ./...`
Expected: compiles without errors

**Step 6: Run the app and manually verify**

Run: `./mahresources -ephemeral -bind-address=:8181`
Visit: `http://localhost:8181/`
Expected: redirects to `/dashboard`, shows empty-state dashboard with all 5 sections

**Step 7: Commit**

```bash
git add server/routes.go server/template_handlers/template_context_providers/static_template_context.go templates/dashboard.tpl
git commit -m "feat(dashboard): wire up routes, navigation, and full-width layout"
```

---

### Task 7: Write E2E tests

**Files:**
- Create: `e2e/tests/dashboard.spec.ts`

**Step 1: Create the dashboard E2E test file**

Create `e2e/tests/dashboard.spec.ts`:

```typescript
import { test, expect } from '../fixtures/base.fixture';

test.describe('Dashboard', () => {
  test('should redirect root to dashboard', async ({ page, baseURL }) => {
    await page.goto(baseURL!);
    await page.waitForURL(/\/dashboard/);
    expect(page.url()).toContain('/dashboard');
  });

  test('should load dashboard page', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/dashboard`);
    await expect(page.locator('h2:has-text("Recent Resources")')).toBeVisible();
    await expect(page.locator('h2:has-text("Recent Notes")')).toBeVisible();
    await expect(page.locator('h2:has-text("Recent Groups")')).toBeVisible();
    await expect(page.locator('h2:has-text("Recent Tags")')).toBeVisible();
    await expect(page.locator('h2:has-text("Recent Activity")')).toBeVisible();
  });

  test('should show empty states when no data exists', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/dashboard`);
    await expect(page.locator('text=No resources yet')).toBeVisible();
    await expect(page.locator('text=No notes yet')).toBeVisible();
    await expect(page.locator('text=No groups yet')).toBeVisible();
    await expect(page.locator('text=No tags yet')).toBeVisible();
    await expect(page.locator('text=No activity yet')).toBeVisible();
  });

  test('should show View All links that navigate correctly', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/dashboard`);

    const viewAllLinks = page.locator('.dashboard-view-all');
    await expect(viewAllLinks).toHaveCount(4);

    // Check href attributes
    await expect(viewAllLinks.nth(0)).toHaveAttribute('href', '/resources');
    await expect(viewAllLinks.nth(1)).toHaveAttribute('href', '/notes');
    await expect(viewAllLinks.nth(2)).toHaveAttribute('href', '/groups');
    await expect(viewAllLinks.nth(3)).toHaveAttribute('href', '/tags');
  });
});

test.describe('Dashboard with data', () => {
  test('should display recently created tag', async ({ page, baseURL, apiClient }) => {
    const tag = await apiClient.createTag('Dashboard Test Tag', 'Test description');

    try {
      await page.goto(`${baseURL}/dashboard`);
      await expect(page.locator('.dashboard-tag-pill:has-text("Dashboard Test Tag")')).toBeVisible();
      // Activity feed should show the created tag
      await expect(page.locator('.dashboard-activity-name:has-text("Dashboard Test Tag")')).toBeVisible();
    } finally {
      await apiClient.deleteTag(tag.ID);
    }
  });

  test('should display recently created note', async ({ page, baseURL, apiClient }) => {
    const note = await apiClient.createNote({ name: 'Dashboard Test Note', description: 'Test note body' });

    try {
      await page.goto(`${baseURL}/dashboard`);
      await expect(page.locator('.card-title:has-text("Dashboard Test Note")')).toBeVisible();
    } finally {
      await apiClient.deleteNote(note.ID);
    }
  });

  test('should display recently created group', async ({ page, baseURL, apiClient }) => {
    const category = await apiClient.createCategory('Dashboard Cat');

    try {
      const group = await apiClient.createGroup({ name: 'Dashboard Test Group', categoryId: category.ID });

      try {
        await page.goto(`${baseURL}/dashboard`);
        await expect(page.locator('.card-title:has-text("Dashboard Test Group")')).toBeVisible();
      } finally {
        await apiClient.deleteGroup(group.ID);
      }
    } finally {
      await apiClient.deleteCategory(category.ID);
    }
  });

  test('should have accessible section landmarks', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/dashboard`);
    // Check that all dashboard sections have aria-label
    const sections = page.locator('.dashboard-section[aria-label]');
    await expect(sections).toHaveCount(5);
  });

  test('should navigate to dashboard from menu', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/notes`);
    await page.locator('.navbar-link:has-text("Dashboard")').click();
    await page.waitForURL(/\/dashboard/);
    expect(page.url()).toContain('/dashboard');
  });
});
```

**Step 2: Run the E2E tests**

Run: `cd e2e && npm run test:with-server -- --grep "Dashboard"`
Expected: All tests pass

**Step 3: Commit**

```bash
git add e2e/tests/dashboard.spec.ts
git commit -m "test(dashboard): add E2E tests for dashboard page"
```

---

### Task 8: Add accessibility test for dashboard

**Files:**
- Check existing: `e2e/tests/accessibility/` for the pattern
- Create or modify: accessibility test to include `/dashboard`

**Step 1: Check existing accessibility test pattern**

Read `e2e/tests/accessibility/` files to understand how accessibility tests are structured (they likely use axe-core via the a11y fixture).

**Step 2: Add dashboard to the accessibility test suite**

If there's a test that iterates over pages, add `/dashboard` to the list. If tests are per-page, create a new test. Follow the existing pattern exactly.

**Step 3: Run accessibility tests**

Run: `cd e2e && npm run test:with-server:a11y`
Expected: Dashboard passes axe-core checks

**Step 4: Commit**

```bash
git add e2e/tests/accessibility/
git commit -m "test(dashboard): add accessibility test for dashboard page"
```

---

### Task 9: Build and run full test suite

**Step 1: Build CSS**

Run: `npm run build-css`

**Step 2: Build the full application**

Run: `npm run build`

**Step 3: Run Go unit tests**

Run: `go test ./...`
Expected: All pass

**Step 4: Run full E2E suite**

Run: `cd e2e && npm run test:with-server`
Expected: All pass (including new dashboard tests)

**Step 5: Fix any failures**

If tests fail, investigate and fix. Pay attention to:
- Existing tests that relied on `/` redirecting to `/notes` — update them
- Template rendering errors from the UNION query on empty DB
- CSS conflicts with existing card styles

**Step 6: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "fix(dashboard): address test feedback"
```
