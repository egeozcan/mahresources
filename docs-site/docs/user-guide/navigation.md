---
sidebar_position: 1
---

# Navigation

This guide covers how to navigate the Mahresources interface, including the main menu, global search, list views, and detail pages.

## Top Navigation Bar

The navigation bar appears at the top of every page and provides access to all main sections of the application.

### Desktop View

On larger screens, the navigation displays as a horizontal row of links:

- **Resources** - File uploads and management
- **Notes** - Text notes and documentation
- **Groups** - Hierarchical organization
- **Tags** - Labels for categorization
- **Categories** - Group type definitions
- **Queries** - Saved database queries
- **Relations** - Group-to-group relationships
- **Relation Types** - Relationship type definitions
- **Note Types** - Note type definitions

The currently active section is highlighted in bold.

### Mobile View

On smaller screens, the navigation collapses into a hamburger menu. Tap the menu icon to reveal a dropdown list of all sections.

### Settings

The gear icon in the top-right corner opens a settings dropdown with display preferences:

- **Show Descriptions** - Toggle whether entity descriptions appear in list views

## Global Search

Global search provides a fast way to find any item across all entity types.

### Opening Search

You can open the search dialog in two ways:

- Click the **Search** button in the header
- Press **Cmd+K** (macOS) or **Ctrl+K** (Windows/Linux)

### Using Search

1. Type at least 2 characters to begin searching
2. Results appear grouped by type (Resources, Notes, Groups, Tags, etc.)
3. Each result shows:
   - An icon indicating the entity type
   - The item name with matching text highlighted
   - A description (if available)
   - Additional metadata

### Keyboard Navigation

Navigate search results without using the mouse:

| Key | Action |
|-----|--------|
| Arrow Up/Down | Move through results |
| Enter | Open selected result |
| Escape | Close search dialog |

### Search Behavior

- Search requires a minimum of 2 characters
- Results are cached for 30 seconds for faster repeated searches
- Search matches against names and descriptions
- Results are limited to 15 items per search

## List Views

List views display collections of entities with filtering, sorting, and pagination.

### Table Layout

Most list views use a table format showing:

- Checkbox for selection (bulk operations)
- Entity ID
- Name (clickable link to detail page)
- Preview thumbnail (for resources)
- Timestamps (created, updated)
- Additional columns specific to the entity type

### Display Options

Some list views offer alternative display modes. Look for a display mode selector at the top of the list to switch between:

- **Details** - Full table with all columns
- **Simple** - Compact list with essential information
- **Gallery** - Grid of thumbnails (for resources)

### Sidebar Filters

The sidebar on list pages contains filter controls:

- **Popular Tags** - Quick filter buttons for frequently-used tags
- **Sort Options** - Multiple sort fields with direction (ascending/descending)
- **Text Filters** - Search within specific fields (name, description, etc.)
- **Autocomplete Filters** - Filter by related entities (tags, groups, owner)
- **Date Filters** - Filter by creation or modification date
- **Dimension Filters** - Filter images by width/height (resources only)

Click **Search** to apply filters. The URL updates to reflect your filter settings, making filtered views bookmarkable and shareable.

### Pagination

When results exceed a single page, pagination controls appear at the bottom:

- **Previous/Next** arrows for sequential navigation
- **Page numbers** for direct access to specific pages
- Current page is highlighted

## Detail Views

Detail pages show all information about a single entity.

### Page Header

Every detail page includes:

- **Breadcrumb** - Navigate back to the list view
- **Entity Name** - Inline editable by clicking (changes save automatically)
- **Action Buttons** - Edit, Delete, or entity-specific actions
- **Timestamps** - Created and Updated dates in the sidebar

### Main Content Area

The main content area displays:

- **Description** - Full text description with expandable sections for long content
- **Related Entities** - Lists of connected items (notes attached to a resource, resources in a group, etc.)
- **Entity-Specific Data** - Metadata, file information, or type-specific content

### Sidebar

The sidebar provides quick access to:

- **Owner** - The group that owns this entity
- **Tags** - Assigned tags with inline editing
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

The interface adapts to different screen sizes:

- **Desktop (1024px+)** - Full layout with sidebar filters and detailed tables
- **Tablet (768-1024px)** - Condensed navigation, scrollable tables
- **Mobile (<768px)** - Hamburger menu, stacked layouts, simplified views

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| Cmd/Ctrl + K | Open global search |
| Escape | Close dialogs and dropdowns |
| Space | Toggle selected checkbox (in lists) |
| Shift + Click | Select range of items |
| Right-click | Select range of items (alternative) |

## Lightbox

When viewing images or media files, clicking a preview opens the lightbox viewer:

- **Navigation** - Arrow keys or click to move between images
- **Close** - Press Escape or click outside the image
- **Download** - Access the original file
- **Gallery Mode** - Browse all images in the current view
