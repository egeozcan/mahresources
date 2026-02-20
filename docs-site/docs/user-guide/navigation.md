---
sidebar_position: 1
---

# Navigation

## Top Navigation Bar

The navigation bar appears at the top of every page.

### Desktop View

On larger screens, the navigation displays as a two-tier horizontal menu:

**Main menu links:**
- **Notes** - Text notes and documentation
- **Resources** - File uploads and management
- **Tags** - Labels for categorization
- **Groups** - Hierarchical organization
- **Queries** - Saved database queries

**Admin dropdown** (click to expand):
- **Categories** - Group type definitions
- **Resource Categories** - Resource classification types
- **Relations** - Group-to-group relationships
- **Relation Types** - Relationship type definitions
- **Note Types** - Note type definitions
- **Logs** - System activity log viewer

The currently active section is highlighted in bold. If the current page belongs to the Admin dropdown, the dropdown button is also highlighted.

### Mobile View

On smaller screens, the navigation collapses into a hamburger menu. Tap the menu icon to reveal a dropdown list of all sections.

### Settings

The gear icon in the top-right corner opens a settings dropdown with display preferences:

- **Show Descriptions** - Toggle whether entity descriptions appear in list views

## Global Search

Open the search dialog by clicking **Search** in the header or pressing **Cmd+K** (macOS) / **Ctrl+K** (Windows/Linux).

Type at least 2 characters. Results appear grouped by type (Resources, Notes, Groups, Tags, etc.), each showing a type icon, the item name with matches highlighted, and a description preview.

### Keyboard Navigation

| Key | Action |
|-----|--------|
| Arrow Up/Down | Move through results |
| Enter | Open selected result |
| Escape | Close search dialog |

Results cache for 30 seconds and are limited to 15 items per search.

## List Views

### Table Layout

Most list views use a table showing:

- Checkbox for bulk selection
- Entity ID
- Name (links to detail page)
- Preview thumbnail (for resources)
- Timestamps (created, updated)
- Entity-specific columns

### Display Options

List views offer alternative display modes. Use the selector at the top of the list to switch between them.

**Resources:**
- **Thumbnails** (default) - Card grid with thumbnail previews
- **Details** - Full table with all columns
- **Simple** - Compact list with essential information

**Groups:**
- **List** (default) - Standard list view
- **Text** - Text-focused view

### Sidebar Filters

The sidebar on list pages contains filter controls:

- **Popular Tags** - Quick-filter buttons for frequently-used tags
- **Sort Options** - Sort by multiple fields with ascending/descending direction
- **Text Filters** - Search within name, description, etc.
- **Autocomplete Filters** - Filter by tags, groups, or owner
- **Date Filters** - Filter by creation or modification date
- **Dimension Filters** - Filter images by width/height (resources only)

Click **Search** to apply. The URL updates to reflect your filters, so you can bookmark or share filtered views.

### Pagination

When results exceed a single page, pagination controls appear at the bottom:

- **Previous/Next** arrows for sequential navigation
- **Page numbers** for direct access to specific pages
- Current page is highlighted

## Detail Views

### Page Header

Every detail page includes:

- **Breadcrumb** - Navigate back to the list view
- **Entity Name** - Inline editable by clicking (changes save automatically)
- **Action Buttons** - Edit, Delete, or entity-specific actions
- **Timestamps** - Created and Updated dates in the sidebar

### Main Content Area

- **Description** - Full text, with expandable sections for long content
- **Related Entities** - Connected items (notes on a resource, resources in a group, etc.)
- **Entity-Specific Data** - Metadata, file info, or type-specific content

### Sidebar

- **Owner** - The group that owns this entity
- **Tags** - Assigned tags (inline editable)
- **Metadata** - Custom key-value pairs
- **Quick Actions** - Entity-specific operations

### Related Entity Sections

Related entities appear in expandable sections:

- **Own Entities** (for groups) - Items directly owned by this group
- **Related Entities** (for groups) - Items associated but not owned
- **Relations** - Custom typed relationships to other groups

Each section includes:
- A count of related items
- Thumbnails or previews where applicable
- A link to view all related items
- A quick-add button to create new related items

## Responsive Design

- **Desktop (1024px+)** - Full layout with sidebar filters and detailed tables
- **Tablet (768-1024px)** - Condensed navigation, scrollable tables
- **Mobile (under 768px)** - Hamburger menu, stacked layouts, simplified views

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| Cmd/Ctrl + K | Open global search |
| Cmd/Ctrl + Shift + D | Toggle Download Cockpit |
| Escape | Close dialogs and dropdowns |
| Space | Toggle selected checkbox (in lists) |
| Shift + Click | Select range of items |
| Right-click | Select range of items (alternative) |

## Lightbox

Click any image or media preview to open the lightbox viewer.

- **Navigate** - Arrow keys, swipe, or drag between images. The lightbox loads additional pages of results automatically as you browse.
- **Zoom** - Scroll wheel or pinch to zoom. Double-click to toggle native resolution.
- **Zoom Presets** - Click the zoom percentage indicator for presets (Fit, Stretch, 25%-500%) calculated from the image's native resolution.
- **Pan** - Drag or swipe to move around a zoomed image.
- **Edit Panel** - Edit the resource name, description, and tags in a side panel without leaving the lightbox.
- **Video** - Play video files directly in the viewer.
- **Fullscreen** - Enter fullscreen mode to hide all other UI.
- **Touch** - Pinch-to-zoom, swipe to navigate, two-finger pan.
- **Close** - Press Escape or click outside the image.
