---
sidebar_position: 3
---

# Managing Notes

Notes in Mahresources are text content containers that can be linked to resources, groups, and tags. They support rich metadata, date ranges, and custom note types for specialized formatting.

## Creating Notes

### Basic Note Creation

1. Navigate to **Notes** in the top menu
2. Click the **New Note** button
3. Fill in the note fields:
   - **Title** - The note's display name (required)
   - **Text** - The main content of the note
   - **Start Date** - Optional start date/time
   - **End Date** - Optional end date/time
4. Click **Save**

### Adding Relationships

When creating or editing a note, you can link it to other entities:

- **Tags** - Labels for categorization and filtering
- **Groups** - Associate with one or more groups
- **Owner** - Set the group that owns this note
- **Note Type** - Apply a note type for custom formatting

### From a Group

Create notes directly owned by a group:

1. Navigate to a group's detail page
2. In the **Own Entities** section, find **Notes**
3. Click **Add New**
4. The owner is pre-filled with the current group

## Viewing Notes

### Notes List

The notes list displays:

| Column | Description |
|--------|-------------|
| Checkbox | For bulk selection |
| ID | Unique identifier |
| Title | Note name (click to view details) |
| Description | Text preview (truncated) |
| Tags | Assigned tags |
| Created | Creation timestamp |
| Updated | Last modification timestamp |

### Note Detail Page

The detail page shows:

**Header**
- Note title (inline editable)
- Edit and Delete buttons
- Wide display link

**Main Content**
- Note type custom header (if configured)
- Wide display link for full-screen reading
- Full text content with formatting preserved
- Related groups
- Attached resources

**Sidebar**
- Note type custom sidebar (if configured)
- Start/End dates (if set)
- Owner group
- Note type link
- Tags
- Custom metadata

## Editing Notes

1. Click **Edit** on any note detail page
2. Modify fields as needed
3. Click **Save**

### Inline Title Editing

Click the note title in the header to edit it directly. Changes save automatically.

## Note Types

Note types let you define custom templates and styling for different kinds of notes.

### What Note Types Provide

- **Name** - Identifier for the type
- **Custom Header** - HTML template displayed above note content
- **Custom Sidebar** - HTML template displayed in the sidebar

### Using Note Types

1. Create a note type in **Note Types** > **New Note Type**
2. Define the name and optional custom HTML
3. When creating/editing notes, select the type from the **Note Type** field

### Custom Templates

Note type templates have access to the note data through JavaScript:

```html
<div x-data>
  <p x-text="entity.Name"></p>
  <p x-text="entity.Description"></p>
</div>
```

The `entity` object contains all note fields including custom metadata.

### Creating a Note Type

1. Navigate to **Note Types** in the menu
2. Click **New Note Type**
3. Enter a name
4. Optionally add:
   - **Custom Header** - HTML displayed above note content
   - **Custom Sidebar** - HTML displayed in the sidebar
5. Click **Save**

## Wide Display Mode

For notes with longer content, use wide display mode for distraction-free reading:

1. Navigate to a note detail page
2. Click **Wide display** link below the title
3. The note content displays in a full-width layout

Wide display shows only the note text without the sidebar, optimized for reading.

## Attaching Resources

Link resources (files) to notes for reference:

### From Note Detail

1. Navigate to the note detail page
2. In the **Resources** section, click **Add New**
3. Upload or import a new resource
4. The resource is automatically linked to the note

### From Resource Creation

1. When creating a new resource
2. In the **Notes** field, search for and select notes
3. The resource is linked to selected notes

### Viewing Attached Resources

Attached resources appear in the **Resources** section of the note detail page:
- Thumbnail previews (for images)
- Click to view resource details
- Click thumbnail to open in lightbox

## Date Ranges

Notes support optional date ranges useful for:
- Event documentation
- Time-bounded information
- Historical records

### Setting Dates

1. In the note create/edit form
2. Find the **Start Date** and **End Date** fields
3. Use the datetime picker to select dates
4. Both fields are optional and independent

### Date Display

When set, dates appear in the sidebar:
- **Started:** [date]
- **Ended:** [date]

## Note Metadata

### Custom Metadata

Add key-value pairs using the **Meta Data** section:

1. Enter a key name
2. Enter a value
3. Click **+** for additional fields
4. Save the note

### Metadata Keys

Existing keys from other notes appear as autocomplete suggestions, promoting consistent naming.

## Deleting Notes

### Single Note

1. Navigate to the note detail page
2. Click **Delete** in the header
3. Confirm deletion

### Bulk Deletion

From the notes list:
1. Select multiple notes using checkboxes
2. Use the bulk delete operation

:::warning

Note deletion is permanent. Related resources are not deleted but the link to them is removed.

:::

## Using the Block Editor

The block editor provides a structured way to organize note content using different block types. It appears on the note detail page below the main content area.

### Entering and Exiting Edit Mode

- Click the **Edit Blocks** button in the top-right corner of the block editor to enter edit mode
- The button changes to **Done** when in edit mode
- Click **Done** to exit edit mode and return to view mode
- Changes to block content are saved automatically when you click away from a field

### Block Types

The block editor supports seven block types:

| Block Type | Description |
|------------|-------------|
| **Text** | Rich text content with Markdown support |
| **Heading** | Section headers with H1, H2, or H3 levels |
| **Divider** | Horizontal line to separate content sections |
| **Gallery** | Grid display of resource thumbnails by ID |
| **References** | Links to groups by ID |
| **Todos** | Checklist with interactive checkboxes |
| **Table** | Data table with sortable columns |

### Adding Blocks

1. Enter edit mode by clicking **Edit Blocks**
2. Click the **+ Add Block** button at the bottom of the block list
3. Select a block type from the dropdown menu
4. The new block appears at the end of the list

### Editing Block Content

Each block type has its own editing interface:

**Text blocks**: A textarea appears for entering Markdown-formatted text.

**Heading blocks**: A dropdown to select the heading level (H1, H2, H3) and a text input for the heading text.

**Divider blocks**: No content to edit - displays as a horizontal line.

**Gallery blocks**: Enter comma-separated resource IDs (e.g., "1, 2, 3") to display those resources as thumbnails.

**References blocks**: Enter comma-separated group IDs (e.g., "1, 2, 3") to create links to those groups.

**Todos blocks**:
- Edit item labels in text inputs
- Click **+ Add item** to add new todo items
- Click the **x** button to remove items

**Table blocks**:
- Add/remove columns with labels
- Add/remove rows with values for each column
- Click **+ Add column** or **+ Add row** to expand the table

### Reordering Blocks

In edit mode, each block displays control buttons in its header:

- Click the **up arrow** to move the block up one position
- Click the **down arrow** to move the block down one position
- The up arrow is disabled for the first block
- The down arrow is disabled for the last block

### Deleting Blocks

1. Enter edit mode
2. Click the **trash icon** on the block you want to delete
3. Confirm the deletion in the dialog that appears

:::warning

Block deletion is immediate and cannot be undone.

:::

### Interactive Features (View Mode)

Some blocks have interactive features available in view mode:

**Todos**: Click checkboxes to mark items as complete or incomplete. Completed items show strikethrough text. Checkbox state is saved automatically.

**Tables**: Click column headers to sort the table by that column. Click again to toggle between ascending and descending order. Sort state is preserved during your session.

**Gallery**: Click thumbnails to open the resource detail page.

**References**: Click group links to navigate to the group detail page.

### Description Field Synchronization

The note's **Description** field (shown in the main content area and note lists) automatically stays synchronized with the first text block in the block editor:

- When you edit the first text block, the Description field updates to match
- When you create a new text block and it becomes the first one, its content becomes the Description
- If you delete the first text block, the next text block's content becomes the Description
- This ensures backward compatibility with notes that don't use the block editor

## Notes in Context

Notes appear in several contexts:

### On Resource Pages

Resources show their linked notes in the **Notes** section, providing context and documentation for files.

### In Groups

Groups can own notes (in **Own Entities**) or be related to notes. Owned notes typically document the group itself, while related notes provide cross-references.

### Search Results

Notes appear in global search results, searchable by title and text content.
