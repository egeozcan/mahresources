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

## Notes in Context

Notes appear in several contexts:

### On Resource Pages

Resources show their linked notes in the **Notes** section, providing context and documentation for files.

### In Groups

Groups can own notes (in **Own Entities**) or be related to notes. Owned notes typically document the group itself, while related notes provide cross-references.

### Search Results

Notes appear in global search results, searchable by title and text content.
